package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Recipe-only profile IDs (REQ-073, REQ-075, FND-008 / Section 21).
//
// These identifiers keep their original subjects but MUST NOT appear as
// schema-1 catalog profile units. Selecting them fails with
// resolve.unknown_profile and agent-actionable remediation pointing at the
// Section 21 recipes (docs authored under docs/recipes/ by bead 6hp).

// RecipeOnlyProfileConfiguration is the retired configuration profile ID.
const RecipeOnlyProfileConfiguration = "configuration"

// RecipeOnlyProfileLocalPersistence is the retired local-persistence profile ID.
const RecipeOnlyProfileLocalPersistence = "local-persistence"

// RecipeOnlyProfileIDs is the sorted closed set of recipe-only profile IDs.
// Never appear in a valid catalog; always rejected on selection.
var RecipeOnlyProfileIDs = []string{
	RecipeOnlyProfileConfiguration,
	RecipeOnlyProfileLocalPersistence,
}

// Stable documentation anchors used in remediation text (REQ-073–075).
// Section 21 is the normative anchor; docs/recipes/* land with bead 6hp.
const (
	// RecipeSectionRef is the stable normative section pointer.
	RecipeSectionRef = "Section 21"
	// RecipeDocsDir is the documentation directory for written recipes.
	RecipeDocsDir = "docs/recipes"
	// RecipeDocConfiguration is the configuration recipe path (6hp).
	RecipeDocConfiguration = "docs/recipes/configuration.md"
	// RecipeDocLocalPersistence is the local-persistence recipe path (6hp).
	RecipeDocLocalPersistence = "docs/recipes/local-persistence.md"
)

// recipeOnlySet is the O(1) membership set for RecipeOnlyProfileIDs.
var recipeOnlySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(RecipeOnlyProfileIDs))
	for _, id := range RecipeOnlyProfileIDs {
		m[id] = struct{}{}
	}
	return m
}()

// IsRecipeOnlyProfile reports whether id is a retired recipe-only profile ID
// (configuration or local-persistence). Exact match only — no fuzzy matching
// (REQ-101).
func IsRecipeOnlyProfile(id string) bool {
	_, ok := recipeOnlySet[id]
	return ok
}

// RecipeDocPath returns the docs/recipes path for a recipe-only profile ID,
// or "" when id is not recipe-only.
func RecipeDocPath(id string) string {
	switch id {
	case RecipeOnlyProfileConfiguration:
		return RecipeDocConfiguration
	case RecipeOnlyProfileLocalPersistence:
		return RecipeDocLocalPersistence
	default:
		return ""
	}
}

// ProfileIDs returns sorted profile-unit IDs present in the loaded catalog
// (kind=profile). Recipe-only IDs never appear here on a valid catalog.
//
// Note: presence in the catalog is not the same as generation-ready
// implementation (e.g. distribution is catalogued for post-MVP; the resolver
// decides the implemented set for selection).
func (c *Catalog) ProfileIDs() []string {
	if c == nil {
		return nil
	}
	var out []string
	for _, m := range c.manifests {
		if m != nil && m.Kind == KindProfile {
			out = append(out, m.ID)
		}
	}
	// manifests are already sorted by ID; filter preserves order.
	return out
}

// sortedAvailable returns a defensive sorted copy of available profile IDs.
// sortedAvailable returns a deduplicated, sorted copy with empty strings
// removed. Intentionally duplicated from resolve.sortedCopy to avoid
// cross-package coupling for a trivial helper.
func sortedAvailable(available []string) []string {
	out := make([]string, 0, len(available))
	seen := make(map[string]struct{}, len(available))
	for _, id := range available {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// formatAvailableSet renders the sorted available profile list for messages.
func formatAvailableSet(available []string) string {
	if len(available) == 0 {
		return "[]"
	}
	return "[" + strings.Join(available, ", ") + "]"
}

// UnknownProfileError builds resolve.unknown_profile for a requested profile
// ID that is not in the available (implemented) set.
//
// Message always names the rejected ID and the sorted available set.
// Recipe-only IDs (configuration, local-persistence) get remediation that
// points at Section 21 / docs/recipes/* (REQ-073/075). Other unknowns still
// fail closed with the available set (no did-you-mean — REQ-101).
//
// available may be empty (no optional profiles implemented). loc is typically
// a spec field location (profiles[i]); zero location is allowed.
func UnknownProfileError(requested string, available []string, loc diagnostic.Location) *diagnostic.FoundryError {
	avail := sortedAvailable(available)
	msg := fmt.Sprintf(
		"unknown profile %q; available profiles: %s",
		requested, formatAvailableSet(avail),
	)
	fe := diagnostic.New(diagnostic.IDResolveUnknownProfile, msg, loc)
	return fe.WithRemediation(unknownProfileRemediation(requested, avail))
}

// unknownProfileRemediation is agent-actionable fix text for an unknown or
// recipe-only profile selection.
func unknownProfileRemediation(requested string, available []string) string {
	availText := formatAvailableSet(available)
	if IsRecipeOnlyProfile(requested) {
		doc := RecipeDocPath(requested)
		section := RecipeSectionRef
		switch requested {
		case RecipeOnlyProfileConfiguration:
			section = "Section 21.1"
		case RecipeOnlyProfileLocalPersistence:
			section = "Section 21.2"
		}
		return fmt.Sprintf(
			"Profile ID %q is recipe-only (REQ-073/REQ-075, FND-008), not a schema-1 catalog profile. "+
				"Remove it from the specification profiles array (default: profiles = []; optional: distribution). "+
				"Do not reintroduce it as a generated profile. "+
				"Post-generation guidance: %s and %s. "+
				"Available implemented profiles: %s. "+
				"Run `foundry catalog list` to inspect catalog units.",
			requested, section, doc, availText,
		)
	}
	return fmt.Sprintf(
		"Replace profile %q with one from the available set %s, or remove it (profiles = [] is always valid). "+
			"Foundry does not suggest nearby spellings (exact IDs only). "+
			"configuration and local-persistence are not profiles — see %s recipes under %s/. "+
			"Run `foundry catalog list` to inspect catalog units.",
		requested, availText, RecipeSectionRef, RecipeDocsDir,
	)
}

// RejectUnknownProfiles checks each requested profile ID against the available
// (implemented) set. The first unknown or recipe-only ID fails with
// resolve.unknown_profile (sorted available set + remediation).
//
// Empty requested always succeeds. Duplicates are not checked here
// (spec.duplicate_profile is owned by package spec). Order of requested is
// preserved for which ID is reported first; available is sorted in the error.
//
// This is the catalog-side selection gate used by the resolver (and unit tests
// for REQ-073/075). Pure: no I/O.
func RejectUnknownProfiles(requested, available []string, loc diagnostic.Location) error {
	avail := sortedAvailable(available)
	availSet := make(map[string]struct{}, len(avail))
	for _, id := range avail {
		availSet[id] = struct{}{}
	}
	for _, id := range requested {
		if id == "" {
			// Empty entries should not reach here after spec validation; reject closed.
			return UnknownProfileError(id, avail, loc)
		}
		// Recipe-only IDs always fail even if somehow listed as available.
		if IsRecipeOnlyProfile(id) {
			return UnknownProfileError(id, avail, loc)
		}
		if _, ok := availSet[id]; !ok {
			return UnknownProfileError(id, avail, loc)
		}
	}
	return nil
}

// recipeOnlyManifestMessage is the catalog.invalid message when a unit
// manifest declares a recipe-only profile ID.
func recipeOnlyManifestMessage(id string) string {
	doc := RecipeDocPath(id)
	return fmt.Sprintf(
		"profile id %q is recipe-only and MUST NOT appear in the catalog (REQ-073/REQ-075); "+
			"see %s / %s",
		id, RecipeSectionRef, doc,
	)
}

// recipeOnlyLayoutRemediation is attached to catalog.invalid when forbidden
// recipe profile trees are present.
func recipeOnlyLayoutRemediation(paths []string) string {
	return fmt.Sprintf(
		"Remove forbidden catalog path(s) %s. "+
			"configuration and local-persistence are Section 21 recipes (docs under %s/), not schema-1 profiles. "+
			"Dual lock files are also rejected — keep a single versions.toml.",
		strings.Join(paths, ", "), RecipeDocsDir,
	)
}
