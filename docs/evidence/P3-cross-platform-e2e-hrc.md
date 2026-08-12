# P3.5 — Cross-platform e2e testscript suites (go-foundry-cli-hrc)

- **Date:** 2026-07-31
- **Spec:** REQ-218, Section 45; expands j8h.2 generate e2e + P1 write-free

## Matrix

| Cell | Linux (required) | macOS (CI matrix) |
| ---- | ---------------- | ----------------- |
| Write-free scripts (`TestWriteFreeE2E`) | yes | yes (same scripts) |
| Generate CLI default/strict | `success_*.txt` | generate-e2e job |
| Generate TUI smoke | `success_tui_default.txt` | generate-e2e job |
| Double-generation CLI/TUI | `double_generation*.txt` | generate-e2e job |
| Quiet/progress, pre-existing dest | existing scripts | generate-e2e job |

Foundry CI (`.github/workflows/ci.yml` job `generate-e2e`) runs:

```bash
go test -count=1 -timeout 15m -v ./integration/generate/
go test -count=1 -timeout 10m ./cmd/foundry -run 'TestGenerateE2E'
```

on **ubuntu-latest** and **macos-latest** (fail-fast false). Artifacts upload on
failure under `artifacts/generate-e2e/`.

## Skip policy

OS-specific skips must print an explicit reason (never silent pass). Current
scripts do not OS-skip TUI/CLI generate cells; PTY termios matrices remain under
`-tags=tui_pty` (optional, not required for generate e2e).

## Commands (local)

```bash
GOTOOLCHAIN=go1.26.5 go test ./cmd/foundry -run TestGenerateE2E -count=1 -timeout 20m
GOTOOLCHAIN=go1.26.5 go test ./cmd/foundry -run TestWriteFree -count=1
```
