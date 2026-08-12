# P4.2 — Foundry self-distribution (1wl)

- **Date:** 2026-07-31
- **Module path (OQ-300):** `github.com/robertguss/go-foundry-cli`
- **Install:** `go install github.com/robertguss/go-foundry-cli/cmd/foundry@latest` (or `@vX.Y.Z`)

## Architecture applied

| Item | Path |
| ---- | ---- |
| Release workflow | `.github/workflows/release.yml` (tag-only, SHA-pinned actions, license gate) |
| GoReleaser | `.goreleaser.yaml` — CGO=0, linux/darwin amd64/arm64, checksums, SBOM |
| Distribution profile | Used for **generated** public projects; Foundry repo uses the same release shape |
| Self-update | **None** (rejected feature) |

## Verified commands

```bash
GOTOOLCHAIN=go1.26.5 go build -o foundry ./cmd/foundry
./foundry version
# module path for consumers:
# go install github.com/robertguss/go-foundry-cli/cmd/foundry@<tag>
```

Local build embeds version/commit when using go build with module VCS metadata.

## Dogfood

Generated public CLI with `profiles = ["distribution"]` produces release.yml +
goreleaser + releasing.md (see P4-distribution-profile.md). Foundry itself
releases via the repository workflow on `v*` tags after LICENSE is present.
