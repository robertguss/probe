# CLI dogfood — foundry-smoke-cli (Section 52.1 step 1)

- **Date:** 2026-07-31
- **Bead:** `go-foundry-cli-vu8` (P2.7)
- **REQs:** REQ-242, REQ-245, REQ-247; Section 49 baseline seed; Section 60 earliest dogfood
- **Raw logs:** [`dogfood-cli-logs/`](dogfood-cli-logs/)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS |
| Kernel | `Linux 6.8.0-134-generic` |
| Arch | `linux/amd64` |
| Go | `go1.26.5` (`GOTOOLCHAIN=local`; pinned toolchain under `$HOME/go/pkg/mod/golang.org/toolchain@…`) |
| Git | `git version 2.43.0` |
| Foundry | `v0.0.0-20260731034008-c95d6364b5c5+dirty` (local unstamped build during dogfood) |
| Catalog digest | `fba7cd432c37bbe7e30fe5e4ff46034d381a14ff6e48ee7085e957ab028985d8` |

Source: [`dogfood-cli-logs/machine.txt`](dogfood-cli-logs/machine.txt).

## Exact commands

```bash
export PATH="$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64/bin:$PATH"
export GOROOT="$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64"
export GOTOOLCHAIN=local
export FOUNDRY_DOGFOOD_ARTIFACT_DIR=docs/evidence/dogfood-cli-logs

# Automated support (script + step logger)
./scripts/dogfood-cli.sh

# Go tests with structured step logger
go test -count=1 -timeout 10m -v ./integration/dogfood/ -run TestDogfoodSmokeCLI
```

Public generate shape used by the script (custody-safe temp parent, mode `0700`):

```bash
foundry generate \
  --spec integration/fixtures/foundry-smoke-cli/foundry.toml \
  --dest "$PARENT/foundry-smoke-cli" \
  --verify default
# then: go test -count=1 ./... && go build -o foundry-smoke-cli ./cmd/foundry-smoke-cli
# then: ./foundry-smoke-cli --help && ./foundry-smoke-cli version
```

## plan_sha256 samples

| Run | plan_sha256 | Notes |
| --- | ----------- | ----- |
| Script cold | `69756aa01974063afd0544893387108739e53c1c41996a6da8193983096f8dde` | dest under `/tmp/foundry-dogfood-ve9g0B/` |
| Script warm sibling | `3eca1fd910063abcb7c126373a702616eca80b4176729c8dcfe4d8c6ea1799f3` | different abs dest → different plan hash (host path in plan envelope) |
| Go test cold | `aa5e34cd4c78a1ab6d6f1264b409679724c63379547ff1f6bf1ba0dec2eaed49` | `TestDogfoodSmokeCLI` |

Plan identity includes absolute destination; compare content digests / tree goldens for byte equality (see `integration/fixtures` double-generation).

## Section 49 baseline seed (not a gate)

| Metric | Value (ms) | Cache state |
| ------ | ---------- | ----------- |
| Cold generate+default verify | **1465** | first placement into fresh parent; warm host module cache |
| Warm generate+default verify (sibling) | **1468** | second generate; host caches warm |
| Go-test measured cold | 1459 | `TestDogfoodSmokeCLI` |
| Go-test measured warm | 1445 | `TestDogfoodSmokeCLI` |

Full cold-module-cache captures and write-free p50/p95 are owned by **`go-foundry-cli-j8h.1`** and live under [`perf/`](perf/) (`baselines-latest.json`, `summary.md`, harness `scripts/perf/capture-baselines.sh`). These dogfood rows only **seed** baselines from real dogfood (REQ-165). P2.8 cites [`perf/P2.8-citation.md`](perf/P2.8-citation.md).

Raw: [`dogfood-cli-logs/timings.env`](dogfood-cli-logs/timings.env), [`dogfood-cli-logs/script-run.txt`](dogfood-cli-logs/script-run.txt).

## Time-to-first-green

- Generate (default verify) committed successfully on first attempt (~1.5s).
- Generated project `go test -count=1 ./...` green without edits.
- Binary help/version exercised successfully.
- **No preserved-stage paths** encountered on the happy path.
- **Files deleted post-generate:** 0 non-test generated files (FND-014).

## REQ-247 measurement seed (smoke)

| Measurement | Observation |
| ----------- | ----------- |
| Orientation files | `AGENTS.md`, `docs/architecture.md`, `docs/commands.md`, `README.md` present; smoke needed no first edit |
| Attempts to first correct edit | N/A (disposable; no domain feature) |
| Package-placement errors | 0 |
| Retained vs deleted non-test files | retained all generated non-test files; deleted = 0 |
| First-feature diff | none (disposable smoke) |
| Gen+verify latency warm/cold | see Section 49 seed above |
| Median PR CI latency (FND-016) | not measured yet (seeded row only) |
| Default vs strict failure split | default-only this run |
| Bypass demand | none |
| Escaped defects (RSK-403) | none observed |
| Config/persistence patterns (RSK-401) | none (stateless smoke) |
| Preserved-stage cleanup (RSK-310) | none encountered |
| Agent observations | Minimal shell is immediately testable; help/version work; growth path documented |

## Simplification triggers

| Trigger | Status |
| ------- | ------ |
| >1/4 generated non-test files deleted | **Not fired** (0 deletions) |
| Same file deleted in both real projects | N/A for smoke-only |
| Repeated agent boundary confusion | **Not observed** |

## Friction list

| Friction | Severity | Disposition |
| -------- | -------- | ----------- |
| Host `PATH` often has go1.26.4 via mise while Foundry preflight requires exact go1.26.5 | Medium | **Resolved** (`go-foundry-cli-0wc`): `FOUNDRY_GO_BIN` + auto-discovery of module-cache pin; preflight remediation names exact binary path; documented in root README + `dogfood/README.md`; scripts probe with `GOTOOLCHAIN=local` |
| `plan_sha256` varies with absolute destination path | Low | By design; double-gen goldens use content digests |

## Independence check

- Generated tree lived under `/tmp/foundry-dogfood-*` (mode `0700`), not inside the Foundry repo.
- Fixture spec remains under `integration/fixtures/foundry-smoke-cli/` only.

## Related

- Repo-map dogfood: [`dogfood-cli-repo-map.md`](dogfood-cli-repo-map.md)
- Fixtures / goldens: `integration/fixtures/` (`go-foundry-cli-wly`)
- Full e2e matrix still required for Phase 2 exit (`5pr`); dogfood does **not** waive `j8h.2` / `qd8`
