// Package catalog holds the Git-visible Foundry source catalog tree
// (SPEC-FOUNDRY-002 Section 24) and exposes it as an embed.FS for
// internal/catalog to load, digest, and validate.
//
// testdata/ is intentionally not embedded (Section 24.2): it holds hostile
// and synthetic fixtures used only by tests that load from the repository
// tree. Release binaries embed only the production catalog roots.
//
// Loading, digest, layout validation, and lock consumption live in
// internal/catalog — this package is the embed surface only.
package catalog

import "embed"

// FS is the embedded production catalog (Section 24.2 roots + versions.toml).
//
// Embedded roots:
//   - versions.toml          — single lock manifest (Section 33.4 / REQ-090)
//   - core/                  — core unit
//   - archetypes/            — cli, tui
//   - profiles/              — distribution (post-MVP enabled; present in tree)
//   - schemas/               — human-readable manifest field contract
//
// Not embedded: testdata/, this package's .go sources.
//
//go:embed versions.toml
//go:embed core
//go:embed archetypes
//go:embed profiles
//go:embed schemas
var FS embed.FS
