package resolve

import (
	"fmt"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// unknownArchetypeError builds resolve.unknown_archetype with the sorted
// available archetype set (REQ-101 — exact IDs only).
func unknownArchetypeError(requested string, available []string) *diagnostic.FoundryError {
	avail := sortedCopy(available)
	msg := fmt.Sprintf(
		"unknown archetype %q; available archetypes: %s",
		requested, formatIDSet(avail),
	)
	fe := diagnostic.New(diagnostic.IDResolveUnknownArchetype, msg, diagnostic.Location{})
	return fe.WithRemediation(fmt.Sprintf(
		"Set archetype to exactly one of %s. "+
			"Foundry does not suggest nearby spellings (exact IDs only).",
		formatIDSet(avail),
	))
}

// duplicateProfileError builds spec.duplicate_profile naming both indexes
// (Section 23.2). Defensive: package spec normally rejects duplicates before
// resolve. Includes the sorted available implemented set for agent actionability.
func duplicateProfileError(id string, first, second int, available []string) *diagnostic.FoundryError {
	avail := sortedCopy(available)
	msg := fmt.Sprintf(
		"profiles lists duplicate ID %q at indexes %d and %d; available profiles: %s",
		id, first, second, formatIDSet(avail),
	)
	fe := diagnostic.New(diagnostic.IDSpecDuplicateProfile, msg, diagnostic.Location{})
	return fe.WithRemediation(fmt.Sprintf(
		"List each profile ID at most once in the specification profiles array. "+
			"Remove the duplicate %q entry (indexes %d and %d). "+
			"Available implemented profiles: %s. MVP: profiles = [].",
		id, first, second, formatIDSet(avail),
	))
}

// missingImplementedProfileInCatalog is catalog allowlist skew: an ID is in
// the implemented set but absent (or non-profile) in the loaded catalog.
func missingImplementedProfileInCatalog(id string, catalogProfiles []string) *diagnostic.FoundryError {
	avail := sortedCopy(catalogProfiles)
	msg := fmt.Sprintf(
		"implemented profile %q is missing from the catalog; catalog profile units: %s",
		id, formatIDSet(avail),
	)
	return diagnostic.New(diagnostic.IDCatalogInvalid, msg, diagnostic.PathLocation("profiles/"+id)).
		WithRemediation(fmt.Sprintf(
			"Catalog/allowlist skew: %q is listed as implemented for selection but has no profile unit in the catalog. "+
				"Add the unit under catalog/profiles/%s/ or remove it from the implemented set.",
			id, id,
		))
}

// missingCoreError / missingArchetypeUnitError for required units.
func missingCoreError() *diagnostic.FoundryError {
	return diagnostic.New(
		diagnostic.IDCatalogInvalid,
		"catalog is missing required core unit",
		diagnostic.PathLocation("core"),
	).WithRemediation("The embedded catalog must include core/manifest.toml. Reinstall or rebuild Foundry.")
}

func missingArchetypeUnitError(id string, available []string) *diagnostic.FoundryError {
	// Prefer resolve.unknown_archetype when the ID is simply not among units —
	// same agent-facing surface as an unknown selection.
	return unknownArchetypeError(id, available)
}

// fileCollisionError builds plan.file_collision naming every claimant (REQ-093).
func fileCollisionError(path string, owners []string) *diagnostic.FoundryError {
	sorted := sortedCopy(owners)
	msg := fmt.Sprintf(
		"output path %q has multiple owners: %s",
		path, strings.Join(sorted, ", "),
	)
	return diagnostic.New(diagnostic.IDPlanFileCollision, msg, diagnostic.PathLocation(path)).
		WithRemediation(fmt.Sprintf(
			"Resolve the file ownership collision on %q: each ordinary output path must have exactly one owner "+
				"(core, one archetype, or one profile). Claimants: %s. "+
				"Remove or rename one of the conflicting catalog contributions (including byte-identical duplicates).",
			path, strings.Join(sorted, ", "),
		))
}

// parentChildCollisionError builds plan.file_collision for parent-file vs
// child-path conflicts (Section 25.1).
func parentChildCollisionError(parent, child string, parentOwner, childOwner string) *diagnostic.FoundryError {
	msg := fmt.Sprintf(
		"output path %q (owner %s) conflicts with child path %q (owner %s)",
		parent, parentOwner, child, childOwner,
	)
	return diagnostic.New(diagnostic.IDPlanFileCollision, msg, diagnostic.PathLocation(parent)).
		WithRemediation(fmt.Sprintf(
			"Parent-file versus child-path collisions are fatal. "+
				"Path %q (owner %s) cannot coexist with %q (owner %s). "+
				"Remove or rename one contribution so paths do not nest under another owned file.",
			parent, parentOwner, child, childOwner,
		))
}

// dependencyConflictError builds plan.file_collision for same-module version
// conflicts across contributors (Section 23.2 cites plan.file_collision).
func dependencyConflictError(module, v1, owner1, v2, owner2 string) *diagnostic.FoundryError {
	msg := fmt.Sprintf(
		"dependency %q has conflicting versions %s (owner %s) and %s (owner %s)",
		module, v1, owner1, v2, owner2,
	)
	return diagnostic.New(diagnostic.IDPlanFileCollision, msg, diagnostic.PathLocation("go.mod#"+module)).
		WithRemediation(fmt.Sprintf(
			"Resolve the go.mod version conflict for %q: pin a single exact version across core/archetype/profile contributions. "+
				"Owners: %s (%s) vs %s (%s).",
			module, owner1, v1, owner2, v2,
		))
}

// rejectProfileSelection reuses catalog.UnknownProfileError so recipe-only IDs
// get Section 21 remediation and all unknowns include the sorted available set.
func rejectProfileSelection(requested string, available []string) error {
	return catalog.UnknownProfileError(requested, available, diagnostic.Location{})
}
