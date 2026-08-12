package resolve

import (
	"sort"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
)

// implementedProfileIDs is the closed generation-selectable profile set.
//
// Phase 4: distribution is selectable when predicates hold (public +
// github.com module path). Do not derive this from Catalog.ProfileIDs() —
// catalog presence ≠ generation-ready implementation.
var implementedProfileIDs = []string{
	"distribution",
}

// ImplementedProfileIDs returns a sorted copy of profile IDs that Resolve
// accepts for generation selection.
func ImplementedProfileIDs() []string {
	out := make([]string, len(implementedProfileIDs))
	copy(out, implementedProfileIDs)
	sort.Strings(out)
	return out
}

// IsImplementedProfile reports whether id is in the implemented selection set.
// Exact match only — no fuzzy matching (REQ-101).
func IsImplementedProfile(id string) bool {
	for _, p := range implementedProfileIDs {
		if p == id {
			return true
		}
	}
	return false
}

// sortedCopy returns a defensive sorted unique copy of ids.
// sortedCopy returns a deduplicated, sorted copy of ids with empty strings
// removed. Intentionally duplicated in catalog.sortedAvailable to avoid
// cross-package coupling for a trivial helper.
func sortedCopy(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// implementedSetFrom builds the O(1) membership map from a sorted implemented list.
func implementedSetFrom(implemented []string) map[string]struct{} {
	m := make(map[string]struct{}, len(implemented))
	for _, id := range implemented {
		if id == "" {
			continue
		}
		m[id] = struct{}{}
	}
	return m
}

// availableForErrors returns the sorted available set shown in unknown-profile
// errors. Prefer the explicit implemented list; never include recipe-only IDs.
func availableForErrors(implemented []string) []string {
	return sortedCopy(implemented)
}

// profileManifestsForSelection looks up catalog manifests for selected IDs.
// Callers must already have verified membership in the implemented set.
// Missing catalog units for an implemented ID fail closed as catalog.invalid
// (broken catalog / allowlist skew) — not resolve.unknown_profile.
func profileManifestsForSelection(cat *catalog.Catalog, selected []string) ([]*catalog.Manifest, error) {
	out := make([]*catalog.Manifest, 0, len(selected))
	for _, id := range selected {
		m, ok := cat.Manifest(id)
		if !ok || m == nil {
			return nil, missingImplementedProfileInCatalog(id, cat.ProfileIDs())
		}
		if m.Kind != catalog.KindProfile {
			return nil, missingImplementedProfileInCatalog(id, cat.ProfileIDs())
		}
		out = append(out, m)
	}
	return out, nil
}
