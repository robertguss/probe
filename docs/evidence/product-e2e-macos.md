# Product E2E — macOS (MBP) execution

- **Date:** 2026-08-01
- **Bead:** `go-foundry-cli-mpc` (H2; closes `874.9.2` / residual epic `q8b`)
- **Host:** Darwin arm64 (MacBook Pro, macOS 26.5.2)
- **Go:** go1.26.5 darwin/arm64 (`GOTOOLCHAIN=local`)
- **Commit:** `a27eda5` (main at run time; `git status` clean)
- **Command:** `make product-e2e`
- **Result:** **PASS** (exit 0; all package steps `ok`)
- **Wall time:** ~3.0 minutes (warm module/cache; START 19:44:28Z → END 19:47:30Z UTC)

## Suite results

| Step | Package / target | Result | Duration |
| ---- | ---------------- | ------ | -------- |
| Write-free e2e | `./cmd/foundry -run TestWriteFree` | ok | 0.770s |
| CLI surface/flags | `./internal/cli` filtered runs | ok | 0.251s |
| Generate e2e | `./integration/generate/` | ok | 72.843s |
| Dogfood + product matrix | `./integration/dogfood/` | ok | 75.553s |
| TUI lifecycle | `./integration/tui/` | ok | 1.780s |
| TUI PTY | `-tags=tui_pty ./integration/tui/ -run TestTUI_PTY` | ok | 6.654s |
| Fixtures | `./integration/fixtures/` | ok | 19.495s |

## Environment notes

- Artifact dirs set: `FOUNDRY_DOGFOOD_ARTIFACT_DIR=docs/evidence/product-e2e-macos`,
  `FOUNDRY_GENERATE_E2E_ARTIFACT_DIR=docs/evidence/product-e2e-macos-generate`
  (generate cancel/failure probes wrote under the generate dir; dogfood dir empty).
- No macOS-only failures observed (paths, PTY, git template, case sensitivity).
- Linux counterpart: [`product-e2e-linux.md`](product-e2e-linux.md) (~11.5m on
  Linux agent workspace; same entrypoint).

## Closeout

```text
bd close go-foundry-cli-mpc --reason="make product-e2e green on MBP..."
bd close go-foundry-cli-874.9.2  # H2
bd close go-foundry-cli-874.9    # §H Platforms
bd close go-foundry-cli-874      # inventory epic
bd close go-foundry-cli-q8b      # residual operator epic
```
