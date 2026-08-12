package render

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Catalog language / path constants for typed go.mod (REQ-002, Section 26.4).
const (
	// GomodPath is the sole structured-file output path.
	GomodPath = "go.mod"
	// GomodMode is the planned mode for go.mod (Section 25.3 files 0644).
	GomodMode = "0644"
	// GomodSourceTyped is the inventory source id for typed generation.
	GomodSourceTyped = "typed"
	// GomodOwnerDefault is the plan owner for the shared go.mod file.
	GomodOwnerDefault = "shared:gomod"
	// CatalogGoVersion is the `go` language directive for catalog v1.0 (REQ-002).
	// Exact toolchain patch is separate (toolchain go1.26.5).
	CatalogGoVersion = "1.26.0"
)

// GomodRequire is one exact-version require pin (runtime or test scope).
// Versions MUST be exact (v-semver); ranges and floating tags are rejected.
type GomodRequire struct {
	// Path is the module path (e.g. github.com/spf13/cobra).
	Path string
	// Version is the exact pin (e.g. v1.10.2).
	Version string
}

// GomodTool is one declared Go tool: a tool directive plus its module require.
//
// Path is the package path for the `tool` directive (e.g.
// honnef.co/go/tools/cmd/staticcheck). Module is the module path to require;
// when empty, Path is used as the module path.
type GomodTool struct {
	// Path is the package path for the tool directive.
	Path string
	// Module is the module path for the require line. Empty → use Path.
	Module string
	// Version is the exact pin for the tool module.
	Version string
}

// GomodInput is the pure typed input for go.mod generation (Section 26.4 / REQ-095).
//
// Built from aggregated resolve contributions + catalog lock toolchain pins.
// No replace, exclude, or retract directives are ever emitted.
type GomodInput struct {
	// Module is the project module path (must pass module.CheckPath).
	Module string
	// GoVersion is the `go` directive (catalog v1.0: "1.26.0").
	GoVersion string
	// Toolchain is the `toolchain` directive (catalog v1.0: "go1.26.5").
	Toolchain string
	// Requires are exact-version module requirements (runtime + test).
	Requires []GomodRequire
	// Tools are tool package paths with exact module pins.
	Tools []GomodTool
	// Owner is the inventory owner label; empty defaults to GomodOwnerDefault.
	Owner string
}

// RenderGomod builds a deterministic go.mod from typed contributions, records
// an inventory entry (mechanism=gomod, source=typed), and optionally writes
// through w (MemoryWriter in P1; RootedWriter in P2).
//
// Pipeline (P1.5.c / REQ-095):
//  1. Validate module path (module.CheckPath) — reject before emit
//  2. Validate go/toolchain pins and exact require/tool versions
//  3. Detect same-module version conflicts
//  4. Emit via golang.org/x/mod/modfile (never replace/exclude/retract)
//  5. Sort blocks for deterministic byte output
//  6. Digest content; record inventory
func RenderGomod(in GomodInput, w Writer) (*Inventory, error) {
	content, err := GenerateGoMod(in)
	if err != nil {
		return nil, err
	}
	digest := ContentDigest(content)
	owner := strings.TrimSpace(in.Owner)
	if owner == "" {
		owner = GomodOwnerDefault
	}
	entry := Entry{
		Path:          GomodPath,
		Mode:          GomodMode,
		Mechanism:     MechanismGomod,
		Source:        GomodSourceTyped,
		Owner:         owner,
		SourceDigest:  "", // no catalog source bytes for typed generation
		ContentDigest: digest,
	}
	if w != nil {
		if err := w.WriteFile(entry.Path, entry.Mode, content); err != nil {
			return nil, wrapWriteError(entry.Path, err)
		}
	}
	return newInventory([]Entry{entry}, map[string][]byte{entry.Path: content}), nil
}

// GenerateGoMod returns the deterministic go.mod bytes for in without writing
// or building an inventory. Used by plan digests and unit tests.
func GenerateGoMod(in GomodInput) ([]byte, error) {
	modPath := strings.TrimSpace(in.Module)
	if modPath == "" {
		return nil, gomodError("<module>", "module path is required")
	}
	if err := module.CheckPath(modPath); err != nil {
		return nil, gomodError(modPath, fmt.Sprintf("invalid module path %q: %v", modPath, err))
	}

	goVer := strings.TrimSpace(in.GoVersion)
	if goVer == "" {
		return nil, gomodError(modPath, "go version directive is required")
	}
	if !modfile.GoVersionRE.MatchString(goVer) {
		return nil, gomodError(modPath, fmt.Sprintf("invalid go version %q", goVer))
	}

	toolchain := strings.TrimSpace(in.Toolchain)
	if toolchain == "" {
		return nil, gomodError(modPath, "toolchain directive is required")
	}
	if !modfile.ToolchainRE.MatchString(toolchain) {
		return nil, gomodError(modPath, fmt.Sprintf("invalid toolchain %q", toolchain))
	}

	// Aggregate exact pins: requires + tool modules. First-write wins; conflict
	// on same path with different version is fatal (REQ-095).
	type pin struct {
		version string
		from    string // "require" or "tool"
	}
	pins := make(map[string]pin, len(in.Requires)+len(in.Tools))
	var requirePaths []string

	addPin := func(path, version, from string) error {
		path = strings.TrimSpace(path)
		version = strings.TrimSpace(version)
		if path == "" {
			return gomodError(modPath, from+" module path is required")
		}
		if err := module.CheckPath(path); err != nil {
			return gomodError(path, fmt.Sprintf("invalid %s module path %q: %v", from, path, err))
		}
		if version == "" {
			return gomodError(path, fmt.Sprintf("%s version is required for %q", from, path))
		}
		if !isExactVersion(version) {
			return gomodError(path, fmt.Sprintf(
				"%s version %q for %q is not an exact pin (no ranges, latest, or unprefixed versions)",
				from, version, path,
			))
		}
		if prev, ok := pins[path]; ok {
			if prev.version != version {
				return gomodError(path, fmt.Sprintf(
					"version conflict for %q: %s has %s, %s has %s",
					path, prev.from, prev.version, from, version,
				))
			}
			return nil // identical pin; keep one
		}
		pins[path] = pin{version: version, from: from}
		requirePaths = append(requirePaths, path)
		return nil
	}

	for _, r := range in.Requires {
		if err := addPin(r.Path, r.Version, "require"); err != nil {
			return nil, err
		}
	}

	// Tools: validate package paths, collect unique sorted tool paths, pin modules.
	type toolEntry struct {
		pkg    string
		module string
		vers   string
	}
	tools := make([]toolEntry, 0, len(in.Tools))
	seenToolPkg := make(map[string]struct{}, len(in.Tools))
	for i, t := range in.Tools {
		pkg := strings.TrimSpace(t.Path)
		if pkg == "" {
			return nil, gomodError(modPath, fmt.Sprintf("tools[%d].path is required", i))
		}
		if err := module.CheckImportPath(pkg); err != nil {
			return nil, gomodError(pkg, fmt.Sprintf("invalid tool package path %q: %v", pkg, err))
		}
		mod := strings.TrimSpace(t.Module)
		if mod == "" {
			// Default: tool package path is also the module path (unusual but
			// allowed for single-package modules). Prefer explicit Module.
			mod = pkg
		}
		if err := addPin(mod, t.Version, "tool"); err != nil {
			return nil, err
		}
		if _, dup := seenToolPkg[pkg]; dup {
			// Identical tool package twice is OK (idempotent); skip re-add.
			continue
		}
		seenToolPkg[pkg] = struct{}{}
		tools = append(tools, toolEntry{pkg: pkg, module: mod, vers: strings.TrimSpace(t.Version)})
	}

	// Deterministic require order: sort by module path.
	sort.Strings(requirePaths)
	// Deterministic tool order: sort by package path.
	sort.SliceStable(tools, func(i, j int) bool {
		return tools[i].pkg < tools[j].pkg
	})

	f := new(modfile.File)
	if err := f.AddModuleStmt(modPath); err != nil {
		return nil, gomodError(modPath, fmt.Sprintf("AddModuleStmt: %v", err))
	}
	if err := f.AddGoStmt(goVer); err != nil {
		return nil, gomodError(modPath, fmt.Sprintf("AddGoStmt: %v", err))
	}
	if err := f.AddToolchainStmt(toolchain); err != nil {
		return nil, gomodError(modPath, fmt.Sprintf("AddToolchainStmt: %v", err))
	}

	for _, p := range requirePaths {
		pin := pins[p]
		f.AddNewRequire(p, pin.version, false)
	}
	for _, t := range tools {
		if err := f.AddTool(t.pkg); err != nil {
			return nil, gomodError(t.pkg, fmt.Sprintf("AddTool: %v", err))
		}
	}

	// Defense-in-depth: never leave replace/exclude/retract even if a future
	// edit path accidentally added them.
	if len(f.Replace) > 0 || len(f.Exclude) > 0 || len(f.Retract) > 0 {
		return nil, gomodError(modPath, "internal: replace/exclude/retract must not be present")
	}

	f.Cleanup()
	f.SortBlocks()

	out, err := f.Format()
	if err != nil {
		return nil, gomodError(modPath, fmt.Sprintf("Format: %v", err))
	}

	// Re-parse to guarantee parseability and re-assert forbidden directives.
	parsed, err := modfile.Parse(GomodPath, out, nil)
	if err != nil {
		return nil, gomodError(modPath, fmt.Sprintf("generated go.mod is not parseable: %v", err))
	}
	if len(parsed.Replace) > 0 || len(parsed.Exclude) > 0 || len(parsed.Retract) > 0 {
		return nil, gomodError(modPath, "generated go.mod must not contain replace, exclude, or retract")
	}

	return out, nil
}

// isExactVersion reports whether v is an exact module version pin.
// Rejects empty, "latest", branch names, and ranges; requires valid semver
// (v-prefix) per golang.org/x/mod/semver.
func isExactVersion(v string) bool {
	if v == "" {
		return false
	}
	// Common floating / range shapes (fail closed even if semver accepts).
	switch strings.ToLower(v) {
	case "latest", "master", "main", "head", "tip":
		return false
	}
	if strings.ContainsAny(v, " \t<>=") {
		return false
	}
	if strings.HasPrefix(v, "~") || strings.HasPrefix(v, "^") || strings.HasPrefix(v, "*") {
		return false
	}
	// Exact pins are valid semantic versions (v1.2.3, v1.2.3+incompatible,
	// or well-formed pseudo-versions).
	return semver.IsValid(v)
}

func gomodError(path, msg string) *diagnostic.FoundryError {
	loc := diagnostic.PathLocation(GomodPath)
	if path != "" && path != GomodPath && path != "<module>" {
		loc = diagnostic.PathLocation(path)
	}
	return diagnostic.New(diagnostic.IDRenderFailed, msg, loc).WithRemediation(
		"Fix the typed go.mod generator inputs: module path (module.CheckPath), " +
			"exact go/toolchain pins from the catalog lock, and exact require/tool versions. " +
			"replace/exclude/retract directives are never emitted (REQ-095).",
	)
}

// RequireCount returns the number of require pins that would be emitted
// (unique module paths across Requires and Tools). Pure helper for step logs.
func (in GomodInput) RequireCount() int {
	seen := make(map[string]struct{}, len(in.Requires)+len(in.Tools))
	for _, r := range in.Requires {
		p := strings.TrimSpace(r.Path)
		if p != "" {
			seen[p] = struct{}{}
		}
	}
	for _, t := range in.Tools {
		p := strings.TrimSpace(t.Module)
		if p == "" {
			p = strings.TrimSpace(t.Path)
		}
		if p != "" {
			seen[p] = struct{}{}
		}
	}
	return len(seen)
}

// ToolCount returns the number of unique tool package paths.
func (in GomodInput) ToolCount() int {
	seen := make(map[string]struct{}, len(in.Tools))
	for _, t := range in.Tools {
		p := strings.TrimSpace(t.Path)
		if p != "" {
			seen[p] = struct{}{}
		}
	}
	return len(seen)
}
