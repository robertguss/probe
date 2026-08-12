# worktree-status — real TUI dogfood seed notes (P3.3 / bm1)

Foundry-owned disposable smoke fixture: `integration/fixtures/foundry-smoke-tui/`.

Real dogfood binary **worktree-status** (private TUI) is seeded after smoke-tui
is green (bead `go-foundry-cli-afh` → `go-foundry-cli-bm1`).

## Intent

- Git worktree status TUI with asynchronous refresh (Appendix B private TUI shape).
- **No** synthetic demos, greet services, or empty `effects.go` scaffolding.
- First real async feature adds cooperative `tea.Cmd` + typed messages deliberately
  (Section 18.7 rules in generated `docs/ui-architecture.md`).

## Spec sketch (not generated here)

```toml
schema = 1
name = "worktree-status"
module = "github.com/robertguss/worktree-status"
description = "Git worktree status TUI with asynchronous refresh"
archetype = "tui"
destination = "./worktree-status"
profiles = []
visibility = "private"

[git]
init = true
initial_branch = "main"
```

## Dogfood path

1. `foundry generate --spec <worktree-status.toml>`
2. Implement domain under `internal/` (not in `internal/tui` until boundary is real).
3. Keep single lifecycle owner in `cmd/worktree-status/main.go`.
4. Measure PR latency of generated Core CI (one Linux job) per `docs/evidence/P3-ci-reduced-matrix.md`.

Do not invent app-domain demos in the Foundry catalog; all domain growth is owner-owned.
