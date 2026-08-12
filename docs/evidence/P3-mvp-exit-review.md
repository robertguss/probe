# P3.6 — MVP exit review (Section 50 / go-foundry-cli-66w)

- **Date:** 2026-07-31
- **Status:** CONFIRMS (pending k0o/bjt close in same session)

## Section 50 checklist

| # | Item | Evidence | Status |
| - | ---- | -------- | ------ |
| 1 | Six commands complete; both archetypes `profiles=[]` | plan/generate CLI+TUI smoke | **PASS** |
| 2 | Verified + Git-init + atomic commit | generate e2e committed | **PASS** |
| 3 | Reduced generated CI asserted | cic / core_tree workflow phase | **PASS** |
| 4 | TUI lifecycle/PTY/debug-log; jr7 complete | 1yq, jr7 evidence | **PASS** |
| 5 | Cross-platform e2e hrc | P3-cross-platform-e2e-hrc.md; TestGenerateE2E | **PASS** |
| 6 | Three-agent acceptance k0o | three-agent-acceptance.md | **PASS** |
| 7 | Dogfood CLI+TUI + REQ-247 metrics | dogfood-cli-*, dogfood-tui-* | **PASS** |
| 8 | REQ traceability P3 slice | req-traceability (dlv.1) | **PASS** (pre-closed) |
| 9 | E4 CONFIRMS before TUI | E4-bubbletea-lifecycle.md | **PASS** |

## Commands

```bash
GOTOOLCHAIN=go1.26.5 go test ./integration/fixtures/ -run 'Smoke|DoubleGeneration' -count=1
GOTOOLCHAIN=go1.26.5 go test ./cmd/foundry -run TestGenerateE2E -count=1
GOTOOLCHAIN=go1.26.5 go test ./internal/archtest/ -run 'FoundryCI|GeneratedCI' -count=1
```

## Simplification triggers

None requiring pre-P4 spec revision. Darwin live evidence remains open as
follow-up (not MVP scope).
