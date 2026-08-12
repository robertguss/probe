# P3.1 — TUI minimal shell umbrella (go-foundry-cli-jr7)

- **Date:** 2026-07-31
- **Children closed:** rrs (tree), 1yq (lifecycle/PTY/debug-log matrices), afh (goldens + smoke-tui)

## Acceptance evidence

| Criterion | Evidence |
| --------- | -------- |
| Section 18.2 tree | `TestTUICanonicalTreeInventory`; catalog `archetypes/tui/` |
| Single lifecycle owner | generated main `signal.NotifyContext`; app `WithContext`+`WithoutSignalHandler` |
| No effects.go/messages.go | inventory absence + smoke absence tests |
| Debug-log exclusive-create | `TestTUIDebugLogSafetyMatrix` |
| PTY quit/SIGINT | `go test -tags=tui_pty ./integration/tui/` |
| Goldens + smoke-tui | `TestSmokeTUI*`, double-generation |
| E4 patterns | MapExitCode / ClassifyRunError / cancel-before-quit |

## Commands

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/catalog/ -run TestTUI -count=1
GOTOOLCHAIN=go1.26.5 go test ./integration/tui/ -count=1
GOTOOLCHAIN=go1.26.5 go test ./integration/fixtures/ -run 'SmokeTUI|DoubleGenerationTUI' -count=1
```
