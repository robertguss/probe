package catalog

// RequiredFiles are paths that MUST exist in every valid embedded catalog
// (Section 24.2 tree + Section 33.4 lock). Paths use forward slashes.
var RequiredFiles = []string{
	"versions.toml",
	"core/manifest.toml",
	"archetypes/cli/manifest.toml",
	"archetypes/tui/manifest.toml",
	"profiles/distribution/manifest.toml",
	"schemas/catalog-manifest.md",
}

// RequiredRoots are the top-level directory names under the catalog root
// that must be present (as non-empty trees) in the embedded FS.
var RequiredRoots = []string{
	"core",
	"archetypes",
	"profiles",
	"schemas",
}

// EmbeddedTopLevel excludes testdata (Section 24.2) and non-catalog package files.
// Used when comparing the repository tree to the embed.
var EmbeddedTopLevel = []string{
	"versions.toml",
	"core",
	"archetypes",
	"profiles",
	"schemas",
}

// Forbidden layout paths (enforced by isForbiddenPath; REQ-073/075):
//   - profiles/configuration/**   (recipe-only; Section 21.1)
//   - profiles/local-persistence/** (recipe-only; Section 21.2)
//   - versions.lock (dual lock; only versions.toml is allowed)
//
// See also RecipeOnlyProfileIDs and RejectUnknownProfiles for selection-time
// rejection with resolve.unknown_profile + agent remediation.
