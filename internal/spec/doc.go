// Package spec owns Project Specification parse and validation
// (SPEC-FOUNDRY-002 Sections 14–15, REQ-038–046, REQ-182).
//
// Pipeline:
//
//	bytes → Decode → RawSpecification (no defaults, positions retained)
//	       → Validate → ValidatedSpecification (defaults applied after full structural validation)
//
// Validation order (Section 14.5): TOML syntax → schema version → unknown
// fields/duplicate keys → per-field rules → cross-field rules. Independent
// per-field errors within a stage are aggregated in source order.
//
// Profile ID existence is checked in resolve (not here). Duplicate profile
// IDs fail with spec.duplicate_profile. Schema physical position has no
// semantic effect (FND-013); compare ValidatedSpecification.NormalizedBytes.
//
// This package is pure: no filesystem writes, no network, no subprocesses.
// Decode accepts in-memory bytes (and optional source name for locations).
package spec
