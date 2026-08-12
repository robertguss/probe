# P2 — Phase 2 exit review

- **Date:** 2026-07-31
- **Gated phase:** Phase 2 **exit** (Section 51.1, REQ-242)
- **Bead:** `go-foundry-cli-5pr`
- **Parent epic:** `go-foundry-cli-j8h` (close after this exit; bd forbids epic←task blocks)
- **REQ matrix:** [`req-traceability.md`](req-traceability.md) / [`req-traceability.json`](req-traceability.json) — P2 slice **verified** (49/49)
- **Perf pack:** [`perf/`](perf/) — see [`perf/P2.8-citation.md`](perf/P2.8-citation.md)
- **Dogfood:** [`dogfood-cli-smoke.md`](dogfood-cli-smoke.md), [`dogfood-cli-repo-map.md`](dogfood-cli-repo-map.md)
- **Raw logs:** [`docs/evidence/p2-logs/`](p2-logs/)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS |
| Kernel | `Linux 6.8.0-134-generic #134-Ubuntu SMP PREEMPT_DYNAMIC x86_64` |
| Arch | `linux/amd64` |
| Go | `go1.26.5` (`GOTOOLCHAIN=go1.26.5`) |
| Git | `git version 2.43.0` |
| Module | `github.com/robertguss/go-foundry-cli` |

Source: [`p2-logs/machine.txt`](p2-logs/machine.txt).

## Exit checklist (Section 51.1 + REQ-242 + bead `5pr`)

| # | Criterion | Result | Evidence |
| - | --------- | ------ | -------- |
| 1 | CLI generation default+strict green (Linux host; macOS via CI matrix job) | **PASS** | `integration/generate/` success matrix; `cmd/foundry/testdata/generate/success_{default,strict}.txt`; CI `ubuntu-latest-generate-e2e` + `macos-latest-generate-e2e` |
| 2 | Hostile FS + env sentinel + race preflight suites green | **PASS** | `integration/hostile/fsx` (`//go:build hostile`); `integration/hostile/sentinel`; E1/E2 promotion sources; E3 race preflight + knownrace; CI `linux-hostile-fsx`, `linux-hostile-sentinel` |
| 3 | Stage preservation + commit-matrix green | **PASS** | `internal/fsx/preserve_test.go`, `preservation_audit_test.go`, `commit_test.go`, `classify_test.go`; generate failure matrix asserts preserved stage + no destination |
| 4 | Generate e2e matrix (j8h.2) green with detailed failure artifacts | **PASS** | `integration/generate/` + `cmd/foundry` `TestGenerateE2E`; artifact naming via `FOUNDRY_GENERATE_E2E_ARTIFACT_DIR` |
| 5 | smoke-cli + repo-map exercised; dogfood measurements started | **PASS** | [`dogfood-cli-smoke.md`](dogfood-cli-smoke.md), [`dogfood-cli-repo-map.md`](dogfood-cli-repo-map.md); `integration/dogfood/`; `scripts/dogfood-cli.sh` |
| 6 | Performance baselines recorded (j8h.1); **no hard gates** | **PASS** | [`perf/`](perf/) — `baselines-latest.json`, `summary.md`, [`P2.8-citation.md`](perf/P2.8-citation.md); `gates.absolute_thresholds=null` |
| 7 | Network disclosure before stage proven | **PASS** | `internal/generate/network_test.go` (default+strict goldens, event order); e2e disclosure dimension in generate matrix |
| 8 | Threat-to-control-to-test mapping for Section 46.2 reviewed | **PASS** | [Section below](#section-462-threat-to-control-to-test-mapping) |
| 9 | REQ traceability: Phase-2 REQs verified in matrix (dlv.1) | **PASS** | `FOUNDRY_MATRIX_PHASE_EXIT=P2 go test ./internal/archtest -run TestReqTraceabilityPhaseExit` — 49/49 `verified` with on-disk paths |

## Exact commands

```bash
export GOTOOLCHAIN=go1.26.5
export CI=true
export CGO_ENABLED=0

# P2 REQ matrix phase-exit gate (strict paths)
FOUNDRY_MATRIX_PHASE_EXIT=P2 go test -count=1 ./internal/archtest/ \
  -run 'TestReqTraceabilityPhaseExit' -v

# Perf pack schema lock (no absolute gates)
go test -count=1 ./internal/archtest/ -run 'TestPerfBaselineSchema' -v

# Unit + architecture (default tags)
go test -count=1 ./... -timeout 180s

# Generate e2e matrix
FOUNDRY_GENERATE_E2E_ARTIFACT_DIR=/tmp/gen-e2e \
  go test -count=1 -timeout 15m -v ./integration/generate/
go test -count=1 -timeout 10m ./cmd/foundry -run 'TestGenerateE2E'

# Hostile FS (REQ-213)
go test -tags=hostile -count=1 -timeout 180s ./integration/hostile/fsx/
go test -tags=hostile -count=1 -timeout 120s ./internal/fsx/ \
  -run 'Hostile|Commit|Transaction|Preserve|Static|Parent|Stage|Classify'
go test -count=1 -timeout 120s ./integration/hostile/e1/

# Hostile env/config/template sentinel (REQ-214)
go test -count=1 -timeout 180s ./internal/toolrun/ \
  -run 'Sentinel|ProcessTree|ConstructGoEnv|ConstructGitEnv|Allowlist'
go test -count=1 -timeout 180s ./integration/hostile/sentinel/
go test -count=1 -timeout 180s ./integration/hostile/e2/

# Race preflight evidence (REQ-004/217; E3)
go test -count=1 -timeout 120s ./integration/hostile/e3/

# Dogfood (smoke + repo-map)
go test -count=1 -timeout 10m ./integration/dogfood/

# Plan/generate equality
go test -count=2 ./internal/cli -run TestPlanGenerateByteEquality
go test -count=2 ./integration/fixtures -run TestPlanGeneratePackageByteEquality
```

## Sample digests / identifiers

| Artifact | Value / path |
| -------- | ------------ |
| Perf baselines | [`perf/baselines-latest.json`](perf/baselines-latest.json) (captured 2026-07-31T03:57:23Z) |
| Warm generate p50 (smoke-cli default) | ~1463 ms (`perf/summary.md`) |
| Cold GOMODCACHE generate | ~27012 ms (`perf/summary.md`) — recorded, **not** a gate |
| Dogfood smoke plan_sha256 (script cold) | see `dogfood-cli-smoke.md` |
| Catalog digest (dogfood run) | `fba7cd432c37bbe7e30fe5e4ff46034d381a14ff6e48ee7085e957ab028985d8` |

## P2 REQ matrix citation

All **49** Phase-2 rows in `docs/evidence/req-traceability.json` are `status=verified` with on-disk primary test paths (strict path check). Representative owners:

| Package / surface | Representative REQs | Primary tests |
| ----------------- | ------------------- | ------------- |
| fsx / transaction | REQ-003/044/124–131/134/184/213 | `internal/fsx/*_test.go`, `integration/hostile/fsx/`, `integration/hostile/e1/` |
| toolrun / isolation | REQ-135/153–154/185/214 | `internal/toolrun/*_test.go`, `integration/hostile/sentinel/`, `integration/hostile/e2/` |
| gitinit | REQ-128/160 | `internal/gitinit/*_test.go` |
| verify | REQ-010/127/150–152 | `internal/verify/*_test.go`, generate e2e strict/default |
| generate lifecycle | REQ-034/036/123/158 | `internal/generate/*_test.go`, `internal/cli/generate_test.go` |
| render | REQ-006/095–097/126/212 | `internal/render/*_test.go` |
| CLI archetype | REQ-060–067/064–066 | `internal/catalog/cli_tree_test.go`, fixtures, dogfood |
| plan equality | REQ-010/132 | `internal/cli/plan_generate_equality_test.go`, `integration/fixtures/` |
| e2e matrix | REQ-133/212/242 | `integration/generate/`, `cmd/foundry/testdata/generate/` |
| dogfood + perf | REQ-165/242/245/247 | dogfood evidence + `docs/evidence/perf/` |
| race / CGO | REQ-004/217 | `integration/hostile/e3/` |
| security mapping | REQ-220 | this document §46.2 table + hostile suites |
| exit | REQ-242 | this document + matrix gate |

Full table: [`req-traceability.md`](req-traceability.md). Checker: `FOUNDRY_MATRIX_PHASE_EXIT=P2`.

## Section 46.2 threat-to-control-to-test mapping

Each control in SPEC-FOUNDRY-002 §46.2 maps to at least one attacking or regression test:

| Control (§46.2) | Threat class | Primary automated tests |
| --------------- | ------------ | ----------------------- |
| Descriptor-relative transaction (§31) | Parent/stage swap, pathname TOCTOU, custody escape | `integration/hostile/fsx/suite_test.go`, `internal/fsx/parent_test.go`, `stage_test.go`, `transaction_test.go`, `integration/hostile/e1/` |
| No automatic stage deletion (§31.6) | Silent destroy of forensic stage; unreported leftovers | `internal/fsx/preservation_audit_test.go`, `preserve_test.go`, `integration/hostile/fsx/audit_test.go`, generate failure matrix |
| Closed subprocess environments (§34.2) | Host GIT_*/GO* leak, credential helpers, hostile templates | `integration/hostile/sentinel/sentinel_test.go`, `internal/toolrun/sentinel_test.go`, `env_test.go`, `integration/hostile/e2/` |
| SIGPIPE containment (§36.5) | Broken pipe mid-transaction / falsified exit | `internal/generate/stream_test.go`, `cmd/foundry/sigpipe_unix_test.go`, `internal/report/pipe_test.go` |
| Restricted rendering (§26) | Template escape to FS/env/network/exec | `internal/render/template_test.go`, `template_fuzz_test.go`, `dispatch_test.go`, `purity_test.go` |
| Final conformance (§35.3) | Generated tests mutate committed bytes | `internal/verify/mutation_test.go`, `baseline_test.go`, `e2e_test.go` |
| Catalog validation (§24.4) | Path/binary smuggling in embedded catalog | `internal/catalog/*_test.go`, `internal/archtest/` purity/redlines, render path safety |
| Secret hygiene | Env/proxy/credential leakage in diagnostics | `internal/diagnostic/redact_test.go`, toolrun sentinel redaction (key names only) |
| Generated CI least privilege / full-SHA pins | Untrusted workflow execution / floating tags | Catalog versions + generated workflow goldens (`internal/catalog/`, `internal/render/`) |

**Risk register notes (non-blocking):** multi-FS loopbacks (FAT/exFAT/xfs) remain optional via `FOUNDRY_FSX_ROOTS` / `scripts/fsx-mount-matrix.sh` when mounts are available; Linux host FS hostile suite is the required gate. Absolute performance thresholds intentionally **not** invented from a single-machine capture (Section 49 / REQ-165).

## Failure classes exercised (generate e2e + hostile)

| Class | Location | Outcome contract |
| ----- | -------- | ---------------- |
| Success default+strict × git on/off | `integration/generate/matrix_success_test.go` | exit 0, dest placed, conformance |
| Double-generation | generate matrix + fixtures | second run refuses / byte-equal plan path |
| Pre-existing dest | `preexisting_dest.txt` | fail before stage |
| Cancel / SIGINT | failure matrix | 130; preserve or no-stage |
| Verify failure | failure matrix | no dest; stage preserved 0700 |
| Commit conflict / EEXIST | fsx commit + hostile | classified; stage preserved |
| Stream / SIGPIPE | stream_test + sigpipe | commit dominates exit |
| Network disclosure | network_test + e2e | before stage create |
| Quiet / progress | quiet_progress.txt + CLI tests | quiet never hides errors/network/stage_path |
| Process-tree = plan | toolrun + generate | no extra git/go subcommands |
| Hostile env sentinels | sentinel suite | no host leak execution |
| Hostile FS races | hostile/fsx | parent/stage swap, custody, EXDEV shapes |

## Blocking dependencies (all closed before this review)

Package roots: `qb0` (fsx), `x7b` (toolrun), `8jp` (gitinit), `sui` (verify), `dd1` (generate).  
Wiring: `j8h.3` (CLI generate), `j8h.4` (plan equality), `ybi` (network disclosure), `fy9` (SIGPIPE).  
Suites: `j8h.2` (generate e2e), `qd8` (hostile FS), `bhn` (sentinel), `j8h.1` (perf).  
Dogfood: `vu8` (smoke-cli + repo-map).  
Matrix: `dlv.1`. Archetype content: `4a6`.

Parent epic `go-foundry-cli-j8h` remains open until manually closed after this exit (bd forbids epic←task blocks).

## Simplification triggers

No blocking simplification trigger was encountered during Phase 2 exit. Dogfood measurement seeds (REQ-247) are recorded; follow-on dogfood depth (TUI smoke, worktree-sync) is Phase 3+ scope (`bm1` et al.), not a P2 exit blocker.

## Result

**CONFIRMS** Phase 2 exit criteria (Section 51.1) and **REQ-242**:

1. CLI generation passes default+strict verification with e2e matrix coverage
2. Hostile FS, env sentinel, and race preflight suites green on Linux (CI also matrices generate e2e on macOS)
3. Stage preservation and commit classification green; no automatic stage delete
4. Generate e2e failure artifacts name stage + cause
5. foundry-smoke-cli + repo-map dogfood exercised with measurement seeds
6. Performance baselines recorded without absolute gates
7. Network disclosure before staging proven; no `--offline`
8. Section 46.2 threat→control→test mapping reviewed
9. All Phase-2 REQ matrix rows **verified** with test paths

Phase 2 product exit is **met**. Parent epic `go-foundry-cli-j8h` may close after this bead.
