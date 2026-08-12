// Package resolve owns flat profile/archetype resolution (SPEC-FOUNDRY-002
// Sections 23 and 27; REQ-045, REQ-077, REQ-093, REQ-098–REQ-101, REQ-183).
//
// Resolve is a pure function of a ValidatedSpecification and a loaded Catalog.
// It performs no filesystem, environment, subprocess, clock, logging, or
// output side effects (REQ-098). Results are immutable with deterministic
// sorting (REQ-100).
//
// Flat model (FND-009 / Section 23.3):
//   - Exact implemented profile IDs only; no fuzzy / did-you-mean (REQ-101)
//   - Direct archetype, visibility, and module-host predicates only
//   - No graph traversal, cycles, capability mapping, or provenance
//
// Resolution order (Section 27):
//  1. Select archetype by exact ID; unknown → resolve.unknown_archetype
//  2. Validate each selected profile against the implemented set; unknown →
//     resolve.unknown_profile (sorted available set); duplicates →
//     spec.duplicate_profile (defensive; normally caught by package spec)
//  3. Enforce each profile's direct predicates → resolve.profile_constraint
//  4. Collect file and go.mod contributions from core, archetype, profiles
//  5. Detect output-path collisions and dependency-version conflicts →
//     plan.file_collision (REQ-093)
//  6. Sort everything deterministically
//
// MVP (Phase 1): ImplementedProfileIDs is empty. Only profiles = [] succeeds.
// The distribution unit may be present in the catalog but is not selectable
// until Phase 4. configuration / local-persistence are recipe-only
// (REQ-073/075) and fail with resolve.unknown_profile plus Section 21
// remediation via catalog.UnknownProfileError.
package resolve
