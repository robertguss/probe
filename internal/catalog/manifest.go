package catalog

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Kind is the unit kind declared in a catalog manifest (Section 24.3).
type Kind string

const (
	KindCore      Kind = "core"
	KindArchetype Kind = "archetype"
	KindProfile   Kind = "profile"
)

// RenderMode is a [[files]] render mechanism. Manifest file entries allow
// only static|template (Section 24.3 / REQ-092). Product-wide there are three
// mechanisms (static|template|gomod); gomod is typed go.mod generation and
// MUST NOT appear as a file render mode (go.mod is not an owned file).
type RenderMode string

const (
	RenderStatic   RenderMode = "static"
	RenderTemplate RenderMode = "template"
)

// DependencyScope is a [[dependencies]] scope (Appendix G).
type DependencyScope string

const (
	ScopeRuntime DependencyScope = "runtime"
	ScopeTest    DependencyScope = "test"
	ScopeTool    DependencyScope = "tool"
)

// Manifest is a validated flat catalog unit manifest (Section 24.3, REQ-092).
// Immutable after ParseManifest / ValidateUnitManifests returns.
//
// No profile DAG, capabilities graph, helper schema, or provenance fields
// exist (FND-009). Profile lists are flat IDs only.
type Manifest struct {
	// Path is the catalog-relative path of manifest.toml.
	Path string
	// UnitDir is the catalog-relative directory containing the unit.
	UnitDir string
	Schema  int
	ID      string
	Kind    Kind
	// Description is the non-empty human description.
	Description string
	// CompatibleArchetypes is set only for kind=profile (flat archetype IDs).
	CompatibleArchetypes []string
	// RequiresVisibility is an optional direct predicate (profiles only), e.g. "public".
	RequiresVisibility string
	Files              []FileEntry
	Dependencies       []Dependency
}

// FileEntry is one [[files]] contribution (Section 24.3).
type FileEntry struct {
	Path   string     // relative output path (may include [[ ]] / {{ }} tokens)
	Render RenderMode // static | template
	Source string     // path relative to the unit directory
	Mode   string     // "0644" in v1.0
}

// Dependency is one [[dependencies]] go.mod contribution (typed; Section 26.4).
type Dependency struct {
	Module  string
	Version string
	Scope   DependencyScope
}

// SupportedManifestSchema is the only admitted catalog manifest schema.
const SupportedManifestSchema = 1

// FileModeV1 is the only admitted output mode in v1.0 catalogs (Section 25.3).
const FileModeV1 = "0644"

// knownKinds is the closed set of unit kinds.
var knownKinds = map[Kind]struct{}{
	KindCore:      {},
	KindArchetype: {},
	KindProfile:   {},
}

// knownRenderModes is the closed set for [[files]].render.
var knownRenderModes = map[RenderMode]struct{}{
	RenderStatic:   {},
	RenderTemplate: {},
}

// knownScopes is the closed set for [[dependencies]].scope.
var knownScopes = map[DependencyScope]struct{}{
	ScopeRuntime: {},
	ScopeTest:    {},
	ScopeTool:    {},
}

// knownArchetypeIDs are archetype IDs present in the initial catalog.
// Profile compatible_archetypes entries must be drawn from this flat set
// (Section 23.1; combination matrix stays bounded — Section 45 / 19.4.7).
var knownArchetypeIDs = map[string]struct{}{
	"cli": {},
	"tui": {},
}

// forbiddenDAGKeys are Stage 4 / schema-1-deleted relational fields that must
// hard-reject if reintroduced (REQ-092, FND-009). Matched on top-level or as
// the first key segment of an undecoded path.
var forbiddenDAGKeys = map[string]string{
	"requires":      "requires is a deleted profile-DAG field (FND-009/REQ-092); profiles compose flatly",
	"conflicts":     "conflicts is a deleted profile-DAG field (FND-009/REQ-092); profiles compose flatly",
	"capabilities":  "capabilities is a deleted capability-graph field (FND-009/REQ-092)",
	"provides":      "provides is a deleted capability-provider field (FND-009/REQ-092)",
	"helper_binary": "helper_binary is deferred and has no reserved schema-1 field (Section 22)",
	// Extra defensive keys that would reintroduce graph/provenance machinery.
	"provenance":       "provenance chains do not exist in schema 1 (REQ-100)",
	"requires_profile": "profile-to-profile requires do not exist; use flat selection only",
	"depends_on":       "depends_on is not a catalog field; manifests are flat (REQ-092)",
}

// wireManifest is the strict TOML decode target. Pointers distinguish absence
// from zero values for required scalars.
type wireManifest struct {
	Schema               *int64           `toml:"schema"`
	ID                   *string          `toml:"id"`
	Kind                 *string          `toml:"kind"`
	Description          *string          `toml:"description"`
	CompatibleArchetypes []string         `toml:"compatible_archetypes"`
	RequiresVisibility   *string          `toml:"requires_visibility"`
	Files                []wireFileEntry  `toml:"files"`
	Dependencies         []wireDependency `toml:"dependencies"`
}

type wireFileEntry struct {
	Path   *string `toml:"path"`
	Render *string `toml:"render"`
	Source *string `toml:"source"`
	Mode   *string `toml:"mode"`
}

type wireDependency struct {
	Module  *string `toml:"module"`
	Version *string `toml:"version"`
	Scope   *string `toml:"scope"`
}

// ParseManifest strictly decodes and validates one unit manifest.
//
// path is the catalog-relative path (e.g. "core/manifest.toml") used for
// diagnostic locations and unit-dir derivation. files, when non-nil, is the
// full catalog path→content map used to require that declared sources exist.
//
// All failures return *diagnostic.FoundryError with id catalog.invalid.
func ParseManifest(manifestPath string, data []byte, files map[string][]byte) (*Manifest, error) {
	if manifestPath == "" {
		manifestPath = "manifest.toml"
	}
	unitDir := path.Dir(manifestPath)
	if unitDir == "." {
		unitDir = ""
	}

	var wire wireManifest
	md, err := toml.Decode(string(data), &wire)
	if err != nil {
		return nil, diagnostic.Wrap(
			diagnostic.IDCatalogInvalid,
			"manifest parse error",
			diagnostic.PathLocation(manifestPath),
			err,
		)
	}

	if undec := md.Undecoded(); len(undec) > 0 {
		return nil, rejectUndecoded(manifestPath, undec)
	}

	// Presence of profile-only keys (even empty) is tracked via MetaData so
	// empty compatible_archetypes = [] still counts as set.
	compatSet := md.IsDefined("compatible_archetypes")
	visSet := md.IsDefined("requires_visibility")

	m, err := validateWireManifest(manifestPath, unitDir, &wire, compatSet, visSet)
	if err != nil {
		return nil, err
	}

	if files != nil {
		if err := validateSourcesExist(m, files); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func rejectUndecoded(manifestPath string, undec []toml.Key) error {
	// Prefer specialized DAG-key messaging when any undecoded key is forbidden.
	var forbidden []string
	var unknown []string
	seen := map[string]struct{}{}
	for _, k := range undec {
		full := k.String()
		if full == "" {
			continue
		}
		if _, ok := seen[full]; ok {
			continue
		}
		seen[full] = struct{}{}
		top := full
		if i := strings.IndexByte(full, '.'); i >= 0 {
			top = full[:i]
		}
		// Array items decode as files.N.field — only top-level relational
		// keys are DAG; nested unknown fields under files/dependencies are
		// ordinary unknown fields.
		if reason, ok := forbiddenDAGKeys[top]; ok && !strings.Contains(full, ".") {
			forbidden = append(forbidden, fmt.Sprintf("%s (%s)", full, reason))
			continue
		}
		if reason, ok := forbiddenDAGKeys[top]; ok && (top == full || strings.HasPrefix(full, top+".")) {
			// Top-level table like [capabilities] or capabilities.foo
			// when capabilities is not on the wire struct.
			if top == "requires" || top == "conflicts" || top == "capabilities" ||
				top == "provides" || top == "helper_binary" || top == "provenance" ||
				top == "requires_profile" || top == "depends_on" {
				forbidden = append(forbidden, fmt.Sprintf("%s (%s)", full, reason))
				continue
			}
		}
		unknown = append(unknown, full)
	}
	sort.Strings(forbidden)
	sort.Strings(unknown)

	if len(forbidden) > 0 {
		msg := "manifest contains deleted/forbidden field(s): " + strings.Join(forbidden, "; ")
		if len(unknown) > 0 {
			msg += "; also unknown field(s): " + strings.Join(unknown, ", ")
		}
		return diagnostic.New(
			diagnostic.IDCatalogInvalid,
			msg,
			diagnostic.PathLocation(manifestPath),
		)
	}
	return diagnostic.Newf(
		diagnostic.IDCatalogInvalid,
		diagnostic.PathLocation(manifestPath),
		"manifest unknown field(s): %s",
		strings.Join(unknown, ", "),
	)
}

func validateWireManifest(manifestPath, unitDir string, wire *wireManifest, compatSet, visSet bool) (*Manifest, error) {
	loc := func(field string) diagnostic.Location {
		if field == "" {
			return diagnostic.PathLocation(manifestPath)
		}
		return diagnostic.PathLocation(manifestPath + "#" + field)
	}
	fail := func(field, format string, args ...any) error {
		return diagnostic.Newf(diagnostic.IDCatalogInvalid, loc(field), format, args...)
	}

	// schema
	if wire.Schema == nil {
		return nil, fail("schema", "manifest missing required field schema")
	}
	if *wire.Schema != SupportedManifestSchema {
		return nil, fail("schema", "manifest schema must be %d, got %d", SupportedManifestSchema, *wire.Schema)
	}

	// id
	if wire.ID == nil {
		return nil, fail("id", "manifest missing required field id")
	}
	id := strings.TrimSpace(*wire.ID)
	if id == "" {
		return nil, fail("id", "manifest id must be non-empty")
	}
	if id != *wire.ID {
		return nil, fail("id", "manifest id must not have leading/trailing whitespace")
	}
	if strings.ContainsAny(id, " \t\n\r") {
		return nil, fail("id", "manifest id must not contain whitespace")
	}
	// Recipe-only IDs must never enter the catalog as unit IDs (REQ-073/075).
	if IsRecipeOnlyProfile(id) {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			recipeOnlyManifestMessage(id),
			loc("id"),
		).WithRemediation(unknownProfileRemediation(id, nil))
	}

	// kind
	if wire.Kind == nil {
		return nil, fail("kind", "manifest missing required field kind")
	}
	kind := Kind(strings.TrimSpace(*wire.Kind))
	if _, ok := knownKinds[kind]; !ok {
		return nil, fail("kind", "manifest kind must be core|archetype|profile, got %q", *wire.Kind)
	}

	// description
	if wire.Description == nil {
		return nil, fail("description", "manifest missing required field description")
	}
	desc := strings.TrimSpace(*wire.Description)
	if desc == "" {
		return nil, fail("description", "manifest description must be non-empty")
	}

	// Path/kind consistency for standard layout locations.
	if err := checkPathKindID(manifestPath, unitDir, id, kind); err != nil {
		return nil, err
	}

	// Profile-only fields (flat predicates; no DAG).
	var compat []string
	var vis string
	switch kind {
	case KindProfile:
		if !compatSet {
			return nil, fail("compatible_archetypes", "profile manifest requires compatible_archetypes (flat archetype id list)")
		}
		if len(wire.CompatibleArchetypes) == 0 {
			return nil, fail("compatible_archetypes", "profile compatible_archetypes must be a non-empty flat list")
		}
		seen := map[string]struct{}{}
		for i, a := range wire.CompatibleArchetypes {
			a = strings.TrimSpace(a)
			if a == "" {
				return nil, fail(fmt.Sprintf("compatible_archetypes[%d]", i),
					"compatible_archetypes entry must be non-empty")
			}
			if _, ok := knownArchetypeIDs[a]; !ok {
				return nil, fail(fmt.Sprintf("compatible_archetypes[%d]", i),
					"compatible_archetypes entry %q is not a known archetype id (bounded set: cli, tui)", a)
			}
			if _, dup := seen[a]; dup {
				return nil, fail(fmt.Sprintf("compatible_archetypes[%d]", i),
					"compatible_archetypes duplicate id %q", a)
			}
			seen[a] = struct{}{}
			compat = append(compat, a)
		}
		// Stable order for determinism (REQ-100).
		sort.Strings(compat)

		if visSet {
			if wire.RequiresVisibility == nil || strings.TrimSpace(*wire.RequiresVisibility) == "" {
				return nil, fail("requires_visibility", "requires_visibility must be non-empty when set")
			}
			vis = strings.TrimSpace(*wire.RequiresVisibility)
			if vis != "public" {
				// v1.0 admits only "public" (Section 20 distribution predicate).
				return nil, fail("requires_visibility",
					"requires_visibility must be %q when set, got %q", "public", vis)
			}
		}
	default:
		if compatSet {
			return nil, fail("compatible_archetypes",
				"compatible_archetypes is only valid on kind=profile (got kind=%s)", kind)
		}
		if visSet {
			return nil, fail("requires_visibility",
				"requires_visibility is only valid on kind=profile (got kind=%s)", kind)
		}
	}

	files, err := validateFileEntries(manifestPath, wire.Files)
	if err != nil {
		return nil, err
	}
	deps, err := validateDependencies(manifestPath, wire.Dependencies)
	if err != nil {
		return nil, err
	}

	return &Manifest{
		Path:                 manifestPath,
		UnitDir:              unitDir,
		Schema:               SupportedManifestSchema,
		ID:                   id,
		Kind:                 kind,
		Description:          desc,
		CompatibleArchetypes: compat,
		RequiresVisibility:   vis,
		Files:                files,
		Dependencies:         deps,
	}, nil
}

func checkPathKindID(manifestPath, unitDir, id string, kind Kind) error {
	// Standard layout (Section 24.2):
	//   core/manifest.toml              → kind=core, id=core
	//   archetypes/<id>/manifest.toml   → kind=archetype
	//   profiles/<id>/manifest.toml     → kind=profile
	base := path.Base(unitDir)
	switch {
	case unitDir == "core" || strings.HasPrefix(manifestPath, "core/"):
		if kind != KindCore {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(manifestPath+"#kind"),
				"unit under core/ must have kind=core, got %q", kind,
			)
		}
		if id != "core" {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(manifestPath+"#id"),
				"core unit id must be %q, got %q", "core", id,
			)
		}
	case strings.HasPrefix(unitDir, "archetypes/") || strings.HasPrefix(manifestPath, "archetypes/"):
		if kind != KindArchetype {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(manifestPath+"#kind"),
				"unit under archetypes/ must have kind=archetype, got %q", kind,
			)
		}
		if base != "" && base != "archetypes" && id != base {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(manifestPath+"#id"),
				"archetype id must match directory name %q, got %q", base, id,
			)
		}
	case strings.HasPrefix(unitDir, "profiles/") || strings.HasPrefix(manifestPath, "profiles/"):
		if kind != KindProfile {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(manifestPath+"#kind"),
				"unit under profiles/ must have kind=profile, got %q", kind,
			)
		}
		if base != "" && base != "profiles" && id != base {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(manifestPath+"#id"),
				"profile id must match directory name %q, got %q", base, id,
			)
		}
	}
	return nil
}

func validateFileEntries(manifestPath string, wires []wireFileEntry) ([]FileEntry, error) {
	out := make([]FileEntry, 0, len(wires))
	seenPaths := map[string]int{}
	for i, w := range wires {
		field := func(name string) string {
			return fmt.Sprintf("files[%d].%s", i, name)
		}
		loc := func(name string) diagnostic.Location {
			return diagnostic.PathLocation(manifestPath + "#" + field(name))
		}
		fail := func(name, format string, args ...any) error {
			return diagnostic.Newf(diagnostic.IDCatalogInvalid, loc(name), format, args...)
		}

		if w.Path == nil || strings.TrimSpace(*w.Path) == "" {
			return nil, fail("path", "files[%d].path is required and must be non-empty", i)
		}
		p := strings.TrimSpace(*w.Path)
		if errMsg := outputPathOK(p); errMsg != "" {
			return nil, fail("path", "files[%d].path %s", i, errMsg)
		}
		if prev, dup := seenPaths[p]; dup {
			return nil, fail("path", "files[%d].path %q duplicates files[%d].path", i, p, prev)
		}
		seenPaths[p] = i

		if w.Render == nil || strings.TrimSpace(*w.Render) == "" {
			return nil, fail("render", "files[%d].render is required (static|template)", i)
		}
		render := RenderMode(strings.TrimSpace(*w.Render))
		if render == "gomod" {
			// Product mechanism exists but is not a file-entry render mode.
			return nil, fail("render",
				"files[%d].render %q is not allowed on file entries; go.mod is produced by the typed generator (not an owned file). Allowed: static|template",
				i, render)
		}
		if _, ok := knownRenderModes[render]; !ok {
			return nil, fail("render",
				"files[%d].render must be static|template (product mechanisms: static|template|gomod), got %q",
				i, *w.Render)
		}

		if w.Source == nil || strings.TrimSpace(*w.Source) == "" {
			return nil, fail("source", "files[%d].source is required and must be non-empty", i)
		}
		src := strings.TrimSpace(*w.Source)
		if errMsg := sourcePathOK(src); errMsg != "" {
			return nil, fail("source", "files[%d].source %s", i, errMsg)
		}

		mode := FileModeV1
		if w.Mode != nil {
			mode = strings.TrimSpace(*w.Mode)
		}
		if mode != FileModeV1 {
			return nil, fail("mode", "files[%d].mode must be %q in v1.0, got %q", i, FileModeV1, mode)
		}

		out = append(out, FileEntry{
			Path:   p,
			Render: render,
			Source: src,
			Mode:   mode,
		})
	}
	return out, nil
}

func validateDependencies(manifestPath string, wires []wireDependency) ([]Dependency, error) {
	out := make([]Dependency, 0, len(wires))
	for i, w := range wires {
		field := func(name string) string {
			return fmt.Sprintf("dependencies[%d].%s", i, name)
		}
		loc := func(name string) diagnostic.Location {
			return diagnostic.PathLocation(manifestPath + "#" + field(name))
		}
		fail := func(name, format string, args ...any) error {
			return diagnostic.Newf(diagnostic.IDCatalogInvalid, loc(name), format, args...)
		}

		if w.Module == nil || strings.TrimSpace(*w.Module) == "" {
			return nil, fail("module", "dependencies[%d].module is required and must be non-empty", i)
		}
		mod := strings.TrimSpace(*w.Module)
		if strings.ContainsAny(mod, " \t\n\r") {
			return nil, fail("module", "dependencies[%d].module must not contain whitespace", i)
		}

		if w.Version == nil || strings.TrimSpace(*w.Version) == "" {
			return nil, fail("version", "dependencies[%d].version is required and must be non-empty", i)
		}
		ver := strings.TrimSpace(*w.Version)
		if !strings.HasPrefix(ver, "v") {
			// Exact semver tags in the lock use a leading v (Section 12 / 33.4).
			return nil, fail("version", "dependencies[%d].version must be an exact semver tag with leading v, got %q", i, ver)
		}

		if w.Scope == nil || strings.TrimSpace(*w.Scope) == "" {
			return nil, fail("scope", "dependencies[%d].scope is required (runtime|test|tool)", i)
		}
		scope := DependencyScope(strings.TrimSpace(*w.Scope))
		if _, ok := knownScopes[scope]; !ok {
			return nil, fail("scope", "dependencies[%d].scope must be runtime|test|tool, got %q", i, *w.Scope)
		}

		out = append(out, Dependency{
			Module:  mod,
			Version: ver,
			Scope:   scope,
		})
	}
	return out, nil
}

// outputPathOK returns an empty string when p is a safe relative output path.
func outputPathOK(p string) string {
	if p == "" {
		return "must be non-empty"
	}
	if path.IsAbs(p) || strings.HasPrefix(p, "/") {
		return "must be relative (not absolute)"
	}
	if strings.Contains(p, "\\") {
		return "must use forward slashes only"
	}
	if strings.Contains(p, "\x00") {
		return "must not contain NUL"
	}
	// Clean then inspect components (allow template tokens like {{binary}}).
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "must not contain \"..\" segments"
		}
		if seg == "" {
			return "must not contain empty path segments"
		}
	}
	return ""
}

// sourcePathOK returns an empty string when s is a safe unit-relative source.
func sourcePathOK(s string) string {
	if s == "" {
		return "must be non-empty"
	}
	if path.IsAbs(s) || strings.HasPrefix(s, "/") {
		return "must be relative to the unit directory"
	}
	if strings.Contains(s, "\\") {
		return "must use forward slashes only"
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return "must not contain \"..\" segments"
		}
		if seg == "" {
			return "must not contain empty path segments"
		}
	}
	return ""
}

func validateSourcesExist(m *Manifest, files map[string][]byte) error {
	for i, f := range m.Files {
		srcPath := path.Join(m.UnitDir, f.Source)
		if _, ok := files[srcPath]; !ok {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(m.Path+"#files["+itoaField(i)+"].source"),
				"files[%d].source %q not found in catalog (expected %s)",
				i, f.Source, srcPath,
			)
		}
	}
	return nil
}

func itoaField(n int) string {
	return fmt.Sprintf("%d", n)
}

// UnitManifestPaths returns sorted catalog-relative paths of unit manifests
// (core, archetypes/*, profiles/*). schemas/ and other trees are ignored.
func UnitManifestPaths(files map[string][]byte) []string {
	var out []string
	for p := range files {
		if path.Base(p) != "manifest.toml" {
			continue
		}
		// Only unit trees.
		if p == "core/manifest.toml" ||
			strings.HasPrefix(p, "archetypes/") ||
			strings.HasPrefix(p, "profiles/") {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// ValidateUnitManifests parses and validates every unit manifest in files.
// Returns manifests sorted by unit ID. Duplicate IDs fail. When lock is
// non-nil, every declared dependency module+version must match a lock pin.
//
// Does not require full lock-entry consumption (foundry-only pins are consumed
// outside unit manifests); use ValidateConsumption for exactly-once checks.
func ValidateUnitManifests(files map[string][]byte, lock *Lock) ([]*Manifest, error) {
	paths := UnitManifestPaths(files)
	if len(paths) == 0 {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"catalog has no unit manifests",
			diagnostic.PathLocation("catalog"),
		)
	}

	out := make([]*Manifest, 0, len(paths))
	byID := map[string]string{} // id → path
	for _, p := range paths {
		data, ok := files[p]
		if !ok {
			return nil, diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(p),
				"manifest path listed but missing: %s", p,
			)
		}
		m, err := ParseManifest(p, data, files)
		if err != nil {
			return nil, err
		}
		if prev, dup := byID[m.ID]; dup {
			return nil, diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(p+"#id"),
				"duplicate manifest id %q (also at %s)", m.ID, prev,
			)
		}
		byID[m.ID] = p
		if lock != nil {
			if err := matchDepsToLock(m, lock); err != nil {
				return nil, err
			}
		}
		out = append(out, m)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// matchDepsToLock requires each dependency's module path+version to match a
// lock [[modules]] pin (Section 33.4 / REQ-090 coordinate).
func matchDepsToLock(m *Manifest, lock *Lock) error {
	if lock == nil {
		return nil
	}
	// path+version → module id
	byPV := map[string]string{}
	for _, mod := range lock.Modules {
		byPV[mod.Path+"@"+mod.Version] = mod.ID
	}
	for i, d := range m.Dependencies {
		key := d.Module + "@" + d.Version
		if _, ok := byPV[key]; !ok {
			return diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation(fmt.Sprintf("%s#dependencies[%d]", m.Path, i)),
				"dependency %s@%s is not pinned in versions.toml (manifest %s)",
				d.Module, d.Version, m.ID,
			)
		}
	}
	return nil
}

// LockConsumptionFromManifests returns module:<lock-id> → reference count for
// every unit dependency that matches a lock pin. Other lock entries
// (toolchain, tools, actions, foundry-only modules) are absent (count 0).
// Callers that need full exactly-once validation must merge additional
// consumers before ValidateConsumption.
func LockConsumptionFromManifests(lock *Lock, manifests []*Manifest) map[string]int {
	consumed := map[string]int{}
	if lock == nil {
		return consumed
	}
	byPV := map[string]string{}
	for _, mod := range lock.Modules {
		byPV[mod.Path+"@"+mod.Version] = mod.ID
	}
	for _, m := range manifests {
		if m == nil {
			continue
		}
		for _, d := range m.Dependencies {
			id, ok := byPV[d.Module+"@"+d.Version]
			if !ok {
				continue
			}
			consumed["module:"+id]++
		}
	}
	return consumed
}

// String returns a short summary for logging (id + kind + file count).
func (m *Manifest) String() string {
	if m == nil {
		return "manifest<nil>"
	}
	return fmt.Sprintf("manifest{id=%s kind=%s files=%d deps=%d}", m.ID, m.Kind, len(m.Files), len(m.Dependencies))
}
