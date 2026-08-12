# Performance baselines (Section 49 / REQ-165)

- **Bead:** `go-foundry-cli-j8h.1`
- **Captured (UTC):** 2026-07-31T03:57:23Z
- **Machine class:** `dev`
- **Host:** `dev-box` linux/amd64 kernel `6.8.0-134-generic`
- **Go:** `go version go1.26.5 linux/amd64`
- **Git:** `2.43.0`
- **Foundry:** `foundry v0.0.0-20260731035123-14c96fd85afb+dirty`
- **Machine JSON:** [`baselines-20260731T035723Z-dev.json`](baselines-20260731T035723Z-dev.json) · [`baselines-latest.json`](baselines-latest.json)

## Absolute gates

**None.** This capture does not invent pass/fail thresholds (Section 49 / REQ-165).
P2.8 exit review cites this pack as evidence that baselines were measured.

## Write-free commands (tool-free; SHOULD feel immediate)

| Command | n | p50 (ms) | p95 (ms) | min | max |
| ------- | - | -------- | -------- | --- | --- |
| `version` | 20 | 8 | 9 | 8 | 9 |
| `catalog list` | 20 | 9 | 11 | 8 | 12 |
| `validate` | 20 | 12 | 13 | 11 | 14 |
| `plan` | 20 | 12 | 13 | 11 | 14 |

## Generate `foundry-smoke-cli` (default verify)

| Label | Cache state | Aggregate |
| ----- | ----------- | --------- |
| `warm_host_cache` | warm host GOMODCACHE/GOCACHE | n=3 p50=1463ms p95=1481ms min=1442 max=1481 |
| `cold_gomodcache_cleared` | cold empty GOMODCACHE+GOCACHE | n=1 p50=27012ms p95=27012ms min=27012 max=27012 |
| `cold_parent_warm_cache` | fresh parent, warm host caches | n=1 p50=1506ms p95=1506ms min=1506 max=1506 |

### Samples

- `cold_01` label=`cold_gomodcache_cleared` exit=0 elapsed_ms=27012 gomodcache=cleared stages=19
  - top stages: go-mod-tidy=26986ms, tool-preflight=8ms, git-init=5ms, render=3ms
- `cold_parent_01` label=`cold_parent_warm_cache` exit=0 elapsed_ms=1506 gomodcache=host_warm stages=19
  - top stages: go-mod-tidy=1480ms, tool-preflight=8ms, git-init=5ms, render=3ms
- `warm_01` label=`warm_host_cache` exit=0 elapsed_ms=1442 gomodcache=host_warm stages=19
  - top stages: go-mod-tidy=1414ms, tool-preflight=8ms, git-init=4ms, render=3ms, git-template-cleanup=1ms
- `warm_02` label=`warm_host_cache` exit=0 elapsed_ms=1481 gomodcache=host_warm stages=19
  - top stages: go-mod-tidy=1455ms, tool-preflight=8ms, git-init=5ms, render=3ms
- `warm_03` label=`warm_host_cache` exit=0 elapsed_ms=1463 gomodcache=host_warm stages=19
  - top stages: go-mod-tidy=1436ms, tool-preflight=7ms, render=5ms, git-init=5ms

### Stage timing method

Per-stage `elapsed_ms` is the wall-clock delta between consecutive
`progress: <stage-id>` lines emitted by Foundry (Section 29.2 / 36.2).
This uses Foundry's own progress events as boundaries; it is not a
substitute for internal step-logger durations, but is stable for baselines.

## Reproduce

```bash
./scripts/perf/capture-baselines.sh
# optional: WRITE_FREE_N=20 WARM_N=3 COLD_N=1
# optional: FOUNDRY_PERF_OUT=/tmp/perf-out ./scripts/perf/capture-baselines.sh --quick
```

Optional CI: `.github/workflows/perf-baselines.yml` (`workflow_dispatch` only; not a required gate).

