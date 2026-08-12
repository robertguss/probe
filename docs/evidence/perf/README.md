# Performance baselines (Section 49 / REQ-165)

Evidence pack for **`go-foundry-cli-j8h.1`**. Warm/cold generate timings and
write-free p50/p95 from a real dogfood machine. **No absolute pass/fail gates.**

| File | Role |
| ---- | ---- |
| [`baselines-latest.json`](baselines-latest.json) | Latest machine-readable capture |
| `baselines-<stamp>-<machine_class>.json` | Timestamped archive copy |
| [`summary.md`](summary.md) | Human summary of the latest capture |
| [`schema.json`](schema.json) | Stable format for later thresholding |
| [`P2.8-citation.md`](P2.8-citation.md) | Link target for Phase 2 exit review (`5pr`) |

Reproduce:

```bash
./scripts/perf/capture-baselines.sh
go test ./internal/archtest -run TestPerfBaselineSchema
```

Optional CI (not required): `.github/workflows/perf-baselines.yml`.
