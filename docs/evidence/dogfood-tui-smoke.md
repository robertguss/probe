# TUI dogfood — foundry-smoke-tui (Section 52.1 / bm1)

- **Date:** 2026-07-31
- **Bead:** `go-foundry-cli-bm1` (P3.3)
- **REQs:** REQ-242, REQ-246, REQ-247; Section 60 earliest TUI dogfood
- **Raw logs:** [`dogfood-tui-logs/`](dogfood-tui-logs/)
- **Prerequisite matrices:** `go-foundry-cli-1yq` (lifecycle/PTY/debug-log), `go-foundry-cli-afh` (smoke fixture)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS |
| Kernel | Linux 6.8.0-134-generic |
| Arch | linux/amd64 |
| Go | go1.26.5 (`GOTOOLCHAIN=go1.26.5`) |
| Terminal | non-interactive dogfood host (no multiplexer) |
| Foundry | local `go run ./cmd/foundry` (session main) |

Source: [`dogfood-tui-logs/machine.txt`](dogfood-tui-logs/machine.txt).

## Exact commands

```bash
mkdir -p /home/rob/rrs-gen-tmp && chmod 700 /home/rob/rrs-gen-tmp
GOTOOLCHAIN=go1.26.5 go run ./cmd/foundry generate \
  --spec integration/fixtures/foundry-smoke-tui/foundry.toml \
  --dest /home/rob/rrs-gen-tmp/foundry-smoke-tui \
  --output json
cd /home/rob/rrs-gen-tmp/foundry-smoke-tui && go test -count=1 ./...
```

Automated support (same harness conventions as CLI dogfood / j8h.2):

```bash
go test -count=1 -timeout 10m ./integration/fixtures/ -run 'TestGenerateSmokeTUI|TestDoubleGenerationTUI'
go test -count=1 -timeout 10m ./integration/tui/ -run TestTUIDebugLogSafetyMatrix
go test -tags=tui_pty -count=1 ./integration/tui/ -run 'TestTUI_PTY'
```

## plan_sha256 sample

| Run | plan_sha256 | Notes |
| --- | ----------- | ----- |
| Dogfood generate | `7d11b07f6d2bbc571bfa6ede9561ac157bb567372fe85947c5d8e662fbdc4ad4` | dest `/home/rob/rrs-gen-tmp/foundry-smoke-tui` |

JSON: [`dogfood-tui-logs/dogfood-tui-generate.json`](dogfood-tui-logs/dogfood-tui-generate.json).

## Time-to-first-green

| Step | Result |
| ---- | ------ |
| Generate (default verify) | committed first attempt (~2s wall) |
| `go test -count=1 ./...` | green without edits (`internal/tui`, `internal/version`) |
| Files deleted post-generate | 0 non-test generated files |
| Synthetic demos | none (FND-014) |

## Lifecycle / PTY matrix reference (1yq)

| Suite | Status |
| ----- | ------ |
| Lifecycle owner static matrix | PASS (`TestTUILifecycleOwnerMatrix`) |
| Exit mapping matrix | PASS |
| Debug-log exclusive-create matrix | PASS |
| PTY quit (`-tags=tui_pty`) | PASS exit 0 |
| PTY SIGINT | PASS exit 130 |

## Friction (RSK-305 seed)

1. **Custody parents:** destination must live under a private (0700) parent; default `/tmp` and group-writable project roots refuse (`fs.namespace_not_private`).
2. **Debug-log ordering:** `--debug-log` must validate before `--version` short-circuit (fixed in 1yq/main template).
3. **Bubbles module path:** lock uses `charm.land/bubbles/v2` (actual go.mod path); Section 12.4 also names the GitHub mirror.

No new blocker beads filed for these (resolved or documented). Residual: real interactive terminal dogfood for worktree-status async refresh (next section).

## REQ-247 metrics seed (smoke)

| Metric | Value |
| ------ | ----- |
| Time-to-first-green | ~2s generate + package tests |
| Files touched outside generate | 0 |
| Retained non-test generated files | all (none deleted) |
| Agent observation | AGENTS.md single-owner section is legible |
