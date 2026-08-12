# Catalog testdata

Hostile and synthetic catalog fixtures for unit tests that load from the
repository tree. **Not embedded** into release binaries (Section 24.2).

Product catalog roots live in sibling directories (`core/`, `archetypes/`,
`profiles/`, `schemas/`, `versions.toml`) and are embedded via `go:embed`.
