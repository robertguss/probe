package resolve

import (
	"fmt"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/spec"
)

// Resolve selects the archetype and profiles, enforces direct predicates,
// collects contributions, and returns an immutable ResolvedProject
// (Section 27; REQ-098/099).
//
// Pure: no FS/env/subprocess/clock/logging/output. Safe for concurrent use
// with immutable inputs.
//
// Phase 4: ImplementedProfileIDs includes "distribution". Unknown IDs fail
// with resolve.unknown_profile and the sorted available set.
//
// Catalog shape and resolved-project invariants consumed by plan.Construct are
// enforced by TestCatalogToResolveManifestShapeContract and
// TestResolveToPlanResolvedProjectContract in contract_test.go.
func Resolve(vs *spec.ValidatedSpecification, cat *catalog.Catalog) (*ResolvedProject, error) {
	return resolveFlat(vs, cat, ImplementedProfileIDs())
}

// resolveFlat is the testable core. implemented is the generation-selectable
// profile set (sorted copy not required; available set is sorted for errors).
func resolveFlat(vs *spec.ValidatedSpecification, cat *catalog.Catalog, implemented []string) (*ResolvedProject, error) {
	if vs == nil {
		return nil, diagnostic.New(
			diagnostic.IDSpecInvalidField,
			"validated specification is nil",
			diagnostic.Location{},
		)
	}
	if cat == nil {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"catalog is nil",
			diagnostic.Location{},
		)
	}

	implemented = sortedCopy(implemented)
	availProfiles := availableForErrors(implemented)

	// --- 1. Archetype by exact ID (Section 27 step 1) ---
	archetypeID := vs.Archetype()
	availableArchetypes := cat.ArchetypeIDs()
	if archetypeID == "" || !containsString(availableArchetypes, archetypeID) {
		return nil, unknownArchetypeError(archetypeID, availableArchetypes)
	}
	archManifest, ok := cat.Manifest(archetypeID)
	if !ok || archManifest == nil || archManifest.Kind != catalog.KindArchetype {
		return nil, missingArchetypeUnitError(archetypeID, availableArchetypes)
	}

	// --- 2. Profiles: duplicates, then implemented membership (steps 2) ---
	requested := vs.Profiles() // defensive copy from accessor
	if err := checkDuplicateProfiles(requested, availProfiles); err != nil {
		return nil, err
	}
	// Unknown / recipe-only / unimplemented (including distribution in MVP).
	// First unknown wins; order of requested preserved for which ID is reported.
	// available set in the error is always sorted.
	if err := catalog.RejectUnknownProfiles(requested, availProfiles, diagnostic.Location{}); err != nil {
		return nil, err
	}
	// Recipe-only IDs are already rejected by RejectUnknownProfiles even if
	// they were somehow listed as implemented; keep a hard fence.
	for _, id := range requested {
		if catalog.IsRecipeOnlyProfile(id) {
			return nil, rejectProfileSelection(id, availProfiles)
		}
	}

	// Sorted selected profile IDs (selection order irrelevant — Section 23.2).
	selected := sortedCopy(requested)

	// --- 3. Direct predicates (step 3) ---
	profileManifests, err := profileManifestsForSelection(cat, selected)
	if err != nil {
		return nil, err
	}
	// Enforce predicates in sorted profile-ID order for determinism.
	for _, m := range profileManifests {
		if err := CheckProfilePredicates(m, archetypeID, vs.Visibility(), vs.Module()); err != nil {
			return nil, err
		}
	}

	// --- 4. Collect contributions (step 4) ---
	core, ok := cat.Manifest("core")
	if !ok || core == nil || core.Kind != catalog.KindCore {
		return nil, missingCoreError()
	}
	units := unitOrder(core, archManifest, profileManifests)
	files, deps := collectContributions(units)

	// --- 5. Collisions (step 5) ---
	if err := detectFileCollisions(files); err != nil {
		return nil, err
	}
	if err := detectDependencyConflicts(deps); err != nil {
		return nil, err
	}

	// --- 6. Sort (step 6) ---
	sortFiles(files)
	sortDeps(deps)

	// Profiles slice is never nil after success.
	if selected == nil {
		selected = []string{}
	}

	return &ResolvedProject{
		name:             vs.Name(),
		module:           vs.Module(),
		description:      vs.Description(),
		archetype:        archetypeID,
		destination:      vs.Destination(),
		binary:           vs.Binary(),
		visibility:       vs.Visibility(),
		gitInit:          vs.GitInit(),
		gitInitialBranch: vs.GitInitialBranch(),
		profiles:         selected,
		catalogDigest:    cat.Digest(),
		files:            files,
		dependencies:     deps,
	}, nil
}

// checkDuplicateProfiles returns spec.duplicate_profile when the same ID
// appears twice (Section 23.2). Indexes are source order from the validated
// specification.
func checkDuplicateProfiles(requested, available []string) error {
	seen := make(map[string]int, len(requested))
	for i, id := range requested {
		if prev, ok := seen[id]; ok {
			return duplicateProfileError(id, prev, i, available)
		}
		seen[id] = i
	}
	return nil
}

// Summary returns a log-friendly one-line description of a successful resolve
// for agent-oriented step logs (selected archetype, profiles, counts).
// Not used by Resolve itself (purity: no logging inside Resolve).
func Summary(r *ResolvedProject) string {
	if r == nil {
		return "resolve: <nil>"
	}
	return fmt.Sprintf(
		"archetype=%s profiles=%s files=%d deps=%d digest=%s",
		r.archetype, formatIDSet(r.profiles), len(r.files), len(r.dependencies), r.catalogDigest,
	)
}
