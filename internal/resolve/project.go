package resolve

import (
	"fmt"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
)

// ResolvedProject is the immutable resolve → plan contract (Section 43).
//
// Collections are collision-free and deterministically ordered (REQ-100).
// Construct only via Resolve / resolveFlat.
type ResolvedProject struct {
	// Project identity copied from the validated specification.
	name        string
	module      string
	description string
	archetype   string
	destination string
	binary      string
	visibility  string

	// git fields (pass-through for plan; resolve does not act on them).
	gitInit          bool
	gitInitialBranch string

	// Selected profiles, sorted by ID (empty non-nil for MVP).
	profiles []string

	// Catalog digest bound into the plan later (REQ-090).
	catalogDigest catalog.DigestHex

	// Sorted file contributions (by Path).
	files []FileContribution

	// Sorted dependency contributions (by Module, then Owner).
	dependencies []DependencyContribution
}

// FileContribution is one owned ordinary output file (Section 25.1).
// go.mod is never listed here; it is produced by the typed gomod generator.
type FileContribution struct {
	// Path is the destination-relative output path (may contain template
	// tokens such as {{binary}} until render expands them).
	Path string
	// Owner is a stable owner label: "core", "archetype:<id>", or "profile:<id>".
	Owner string
	// OwnerKind / OwnerID decompose Owner for consumers that need structured IDs.
	OwnerKind catalog.Kind
	OwnerID   string
	// Render is static|template (gomod is not a file render mode).
	Render catalog.RenderMode
	// Source is the catalog-relative source path (unit dir + entry source).
	Source string
	// Mode is the planned file mode string (v1.0: "0644").
	Mode string
}

// DependencyContribution is one typed go.mod contribution (Section 26.4).
type DependencyContribution struct {
	Module  string
	Version string
	Scope   catalog.DependencyScope
	// Owner is "core", "archetype:<id>", or "profile:<id>".
	Owner string
}

// Name returns the project name.
func (r *ResolvedProject) Name() string {
	if r == nil {
		return ""
	}
	return r.name
}

// Module returns the module path.
func (r *ResolvedProject) Module() string {
	if r == nil {
		return ""
	}
	return r.module
}

// Description returns the project description.
func (r *ResolvedProject) Description() string {
	if r == nil {
		return ""
	}
	return r.description
}

// Archetype returns the selected archetype ID ("cli" or "tui").
func (r *ResolvedProject) Archetype() string {
	if r == nil {
		return ""
	}
	return r.archetype
}

// Destination returns the specification destination path string.
func (r *ResolvedProject) Destination() string {
	if r == nil {
		return ""
	}
	return r.destination
}

// Binary returns the binary name.
func (r *ResolvedProject) Binary() string {
	if r == nil {
		return ""
	}
	return r.binary
}

// Visibility returns "private" or "public".
func (r *ResolvedProject) Visibility() string {
	if r == nil {
		return ""
	}
	return r.visibility
}

// GitInit reports whether isolated git init is requested.
func (r *ResolvedProject) GitInit() bool {
	if r == nil {
		return false
	}
	return r.gitInit
}

// GitInitialBranch returns the initial branch name.
func (r *ResolvedProject) GitInitialBranch() string {
	if r == nil {
		return ""
	}
	return r.gitInitialBranch
}

// Profiles returns a copy of the sorted selected profile IDs (never nil after
// a successful Resolve — empty slice for MVP).
func (r *ResolvedProject) Profiles() []string {
	if r == nil {
		return nil
	}
	out := make([]string, len(r.profiles))
	copy(out, r.profiles)
	return out
}

// CatalogDigest returns the SHA-256 hex digest of the catalog used for resolve.
func (r *ResolvedProject) CatalogDigest() catalog.DigestHex {
	if r == nil {
		return ""
	}
	return r.catalogDigest
}

// Files returns a copy of the sorted file contributions.
func (r *ResolvedProject) Files() []FileContribution {
	if r == nil {
		return nil
	}
	out := make([]FileContribution, len(r.files))
	copy(out, r.files)
	return out
}

// Dependencies returns a copy of the sorted dependency contributions.
func (r *ResolvedProject) Dependencies() []DependencyContribution {
	if r == nil {
		return nil
	}
	out := make([]DependencyContribution, len(r.dependencies))
	copy(out, r.dependencies)
	return out
}

// FileCount returns the number of ordinary file contributions.
func (r *ResolvedProject) FileCount() int {
	if r == nil {
		return 0
	}
	return len(r.files)
}

// DependencyCount returns the number of dependency contributions.
func (r *ResolvedProject) DependencyCount() int {
	if r == nil {
		return 0
	}
	return len(r.dependencies)
}

// Equal reports whether two ResolvedProject values have identical fields
// (including catalog digest and ordered collections). Used by determinism tests.
func (r *ResolvedProject) Equal(o *ResolvedProject) bool {
	if r == nil || o == nil {
		return r == o
	}
	if r.name != o.name ||
		r.module != o.module ||
		r.description != o.description ||
		r.archetype != o.archetype ||
		r.destination != o.destination ||
		r.binary != o.binary ||
		r.visibility != o.visibility ||
		r.gitInit != o.gitInit ||
		r.gitInitialBranch != o.gitInitialBranch ||
		r.catalogDigest != o.catalogDigest {
		return false
	}
	if len(r.profiles) != len(o.profiles) {
		return false
	}
	for i := range r.profiles {
		if r.profiles[i] != o.profiles[i] {
			return false
		}
	}
	if len(r.files) != len(o.files) {
		return false
	}
	for i := range r.files {
		if r.files[i] != o.files[i] {
			return false
		}
	}
	if len(r.dependencies) != len(o.dependencies) {
		return false
	}
	for i := range r.dependencies {
		if r.dependencies[i] != o.dependencies[i] {
			return false
		}
	}
	return true
}

// String returns a short debug summary (not a serialization format).
func (r *ResolvedProject) String() string {
	if r == nil {
		return "ResolvedProject(nil)"
	}
	return fmt.Sprintf(
		"ResolvedProject{name=%q archetype=%q profiles=%s files=%d deps=%d}",
		r.name, r.archetype, formatIDSet(r.profiles), len(r.files), len(r.dependencies),
	)
}

// OwnerLabel builds the stable owner string for a unit kind+id.
func OwnerLabel(kind catalog.Kind, id string) string {
	switch kind {
	case catalog.KindCore:
		return "core"
	case catalog.KindArchetype:
		return "archetype:" + id
	case catalog.KindProfile:
		return "profile:" + id
	default:
		if id == "" {
			return string(kind)
		}
		return string(kind) + ":" + id
	}
}

// formatIDSet renders a sorted ID list for messages.
func formatIDSet(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}
	return "[" + strings.Join(ids, ", ") + "]"
}
