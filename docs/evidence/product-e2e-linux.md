# Product E2E — Linux execution

- **Date:** 2026-08-01
- **Bead:** `go-foundry-cli-79a.9`
- **Host:** Linux (agent workspace)
- **Command:** `make product-e2e`
- **Result:** **PASS** (exit 0)
- **Wall time:** ~11.5 minutes (warm caches)

## Suite results

| Step | Package / target | Result | Approx duration |
| ---- | ---------------- | ------ | --------------- |
| Write-free e2e | `./cmd/foundry -run TestWriteFree` | ok | ~1.8s |
| CLI surface/flags | `./internal/cli` filtered runs | ok | ~0.1s |
| Generate e2e | `./integration/generate/` | ok | ~268s |
| Dogfood + product matrix | `./integration/dogfood/` | ok | ~322s |
| TUI lifecycle | `./integration/tui/` | ok | ~2s |
| TUI PTY | `-tags=tui_pty ./integration/tui/ -run TestTUI_PTY` | ok | ~7s |
| Fixtures | `./integration/fixtures/` | ok | ~82s |

## Notes

- Product matrix cells (examples + distribution TUI) included in dogfood package.
- No CI job; local/`make` only per epic non-goals.
- macOS: operator runs the same `make product-e2e` on MBP (see
  [`docs/dev/product-e2e-plan.md`](../dev/product-e2e-plan.md) §2).
