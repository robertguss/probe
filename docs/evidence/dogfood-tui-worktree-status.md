# TUI dogfood — worktree-status begun (Section 52 / bm1)

- **Date:** 2026-07-31
- **Bead:** `go-foundry-cli-bm1`
- **Spec seed:** `dogfood/worktree-status-notes.md`
- **Raw logs:** [`dogfood-tui-logs/`](dogfood-tui-logs/)

## What was done

1. Generated a **private** TUI project `worktree-status` from Foundry using the
   Appendix B private-TUI shape (no profiles).
2. Verified default verify: `go test -count=1 ./...` green on the shell with no
   domain code yet.
3. Recorded that real async refresh / worktree domain work is **owner-owned**
   after generate (not catalog demos).

## Commands

```bash
# Spec used (visibility private; destination under 0700 parent):
# name=worktree-status module=github.com/robertguss/worktree-status archetype=tui

GOTOOLCHAIN=go1.26.5 go run ./cmd/foundry generate \
  --spec /path/to/worktree-status.toml --output json
cd /home/rob/rrs-gen-tmp/worktree-status && go test -count=1 ./...
```

Generate JSON: [`dogfood-tui-logs/dogfood-tui-worktree-gen.json`](dogfood-tui-logs/dogfood-tui-worktree-gen.json).

## Friction / follow-ups for domain work

| Item | Notes | Action |
| ---- | ----- | ------ |
| Async refresh | First real `tea.Cmd` must be context-cooperative (Section 18.7) | implement in project; keep E4 patterns |
| go-git / env isolation | Use Foundry E2 lessons if shelling to git | RSK tracking via bjt |
| Agent legibility | AGENTS.md + ui-architecture sufficient for shell | re-check after first feature |

## Metrics seed

| Metric | Value |
| ------ | ----- |
| Time-to-shell-green | generate + `go test` first try |
| Domain files added post-generate | 0 (begun only) |
| Catalog demos introduced | 0 |
