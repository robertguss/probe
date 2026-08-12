// Package catalog owns embedded catalog loading, the deterministic SHA-256
// digest, layout validation, the versions.toml lock surface, and flat unit
// manifest schema validation (SPEC-FOUNDRY-002 Sections 24, 33.4; REQ-090–094,
// REQ-092).
//
// The Git-visible tree lives at module-root catalog/ and is embedded via
// go:embed in package github.com/robertguss/go-foundry-cli/catalog (Section
// 24.2). This package loads that FS, validates required roots, parses the
// single lock manifest, validates every unit manifest as a flat schema-1
// document (no profile DAG / capabilities / helper_binary), and exposes an
// immutable Catalog value.
//
// Manifest rules (Section 24.3 / Appendix G / catalog/schemas/catalog-manifest.md):
//   - Required: schema=1, id, kind (core|archetype|profile), description
//   - [[files]]: path, render (static|template), source, mode=0644
//   - [[dependencies]]: module, version (exact v-semver), scope (runtime|test|tool)
//   - Profiles only: compatible_archetypes (flat ids), optional requires_visibility
//   - Hard-reject: requires, conflicts, capabilities, provides, helper_binary
//
// Lock-entry exactly-once consumption remains a pure helper
// (ValidateConsumption); dependency pins are matched to the lock at load.
//
// Recipe-only exclusions (REQ-073/075, FND-008):
//   - configuration and local-persistence are NOT schema-1 profile IDs
//   - layout forbids profiles/configuration/** and profiles/local-persistence/**
//   - ParseManifest rejects those unit IDs with catalog.invalid
//   - RejectUnknownProfiles / UnknownProfileError emit resolve.unknown_profile
//     with sorted available set and Section 21 / docs/recipes remediation
//
// List/show data model (REQ-035; CLI: foundry catalog list|show in internal/cli):
//   - List / ListByKind / UnitIDs / ArchetypeIDs / ProfileIDs
//   - Show(id) → Manifest or catalog.invalid with available unit set
//
// Dev filesystem loading (REQ-091 / Section 24.4):
//   - LoadDir is compiled only under the foundrydev build tag
//   - Applies the same LoadFS validation; fails closed
//   - Release binaries (default build) do not contain LoadDir
//
// Invariants:
//   - No package-level mutable state after init (REQ-188).
//   - Pure load path: no filesystem writes, no network, no subprocesses.
//   - Release binaries embed only production roots (testdata/ excluded).
package catalog
