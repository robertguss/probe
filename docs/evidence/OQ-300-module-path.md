# OQ-300 — Public module path / release repository (P4.0 / 9tr)

- **Date:** 2026-07-31
- **Bead:** `go-foundry-cli-9tr`
- **Decision authority:** maintainer (recorded by implementer from live module)

## Decisions

| Topic | Decision |
| ----- | -------- |
| Public module path | `github.com/robertguss/go-foundry-cli` |
| Release repository | Same GitHub repository (`robertguss/go-foundry-cli`) |
| Install path | `go install github.com/robertguss/go-foundry-cli/cmd/foundry@latest` (or version tag) |
| Version tags | Semver tags `vX.Y.Z` on the release repo; GoReleaser consumes tags |
| License at generate | **Not** a generate precondition (FND-019). Owner adds LICENSE post-generate; `docs/releasing.md` blocks publication until license + settings reviewed |

## Residual open questions

None blocking P4.1. Future rename of the public path would be a catalog-bearing
release + import path migration, not a silent generate change.

## Non-goals (this decision)

- Does not publish a release.
- Does not implement the distribution profile (chf).
