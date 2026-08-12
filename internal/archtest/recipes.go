package archtest

// Section 21 recipe documents (REQ-073–075 / go-foundry-cli-6hp).
// These are user-facing post-generation guidance — never catalog profile IDs.

// Doc paths relative to the repository root (must match catalog.RecipeDoc* constants).
const (
	DocRecipesDir                 = "docs/recipes"
	DocRecipeConfigurationPath    = "docs/recipes/configuration.md"
	DocRecipeLocalPersistencePath = "docs/recipes/local-persistence.md"
)

// RecipeDocPaths is the closed set of Section 21 recipe markdown files.
var RecipeDocPaths = []string{
	DocRecipeConfigurationPath,
	DocRecipeLocalPersistencePath,
}

// RequiredRecipeConfigurationHeadings are ATX headings that MUST appear in
// docs/recipes/configuration.md (Section 21.1 / REQ-074).
var RequiredRecipeConfigurationHeadings = []string{
	"# Configuration recipe (Section 21.1)",
	"## Admission bar (when this may become a profile)",
	"## What Foundry will not do",
	"## Normative bounds (Section 21.1 / REQ-074)",
	"## Recommended package placement",
	"## Testing guidance",
}

// RequiredRecipeLocalPersistenceHeadings are ATX headings for
// docs/recipes/local-persistence.md (Section 21.2 / REQ-075).
var RequiredRecipeLocalPersistenceHeadings = []string{
	"# Local persistence recipe (Section 21.2)",
	"## Admission bar (when this may become a profile)",
	"## What Foundry will not do",
	"## Normative bounds (Section 21.2 / REQ-075)",
	"## Recommended package placement",
	"## Pattern A — ordinary files (default)",
	"## Pattern B — bbolt (transactional need only)",
}

// RequiredRecipeSharedNeedles appear in both recipe docs (admission + exclusion).
var RequiredRecipeSharedNeedles = []string{
	"Section 19.4",
	"Two real",
	"resolve.unknown_profile",
	"profiles = []",
	"REQ-247",
	"profile-admission.md",
}

// RequiredRecipeConfigurationNeedles bound the configuration recipe to §21.1.
var RequiredRecipeConfigurationNeedles = []string{
	"REQ-073",
	"REQ-074",
	"flags > environment",
	"caarlos0/env",
	"BurntSushi/toml",
	"0600",
	"Secrets",
	"No automatic",
}

// RequiredRecipeLocalPersistenceNeedles bound the persistence recipe to §21.2.
var RequiredRecipeLocalPersistenceNeedles = []string{
	"REQ-075",
	"Ordinary files first",
	"Atomic write-rename",
	"go.etcd.io/bbolt",
	"domain-named buckets",
	"schema_version",
}
