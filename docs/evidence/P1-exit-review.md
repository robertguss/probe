# P1 — Phase 1 exit review + E5 gate check

- **Date:** 2026-07-31
- **Gated phase:** Phase 1 **exit** (Section 51.1, REQ-241)
- **Bead:** `go-foundry-cli-0z4`
- **Parent epic:** `go-foundry-cli-5an` (close after this exit)
- **E5 record:** [`E5-version-action-lock.md`](E5-version-action-lock.md) — **CONFIRMS**
- **REQ matrix:** [`req-traceability.md`](req-traceability.md) / [`req-traceability.json`](req-traceability.json) — P1 slice **verified** (53/53)
- **Raw logs:** [`docs/evidence/p1-logs/`](p1-logs/)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS |
| Kernel | `Linux 6.8.0-134-generic #134-Ubuntu SMP PREEMPT_DYNAMIC x86_64` |
| Arch | `linux/amd64` |
| Go | `go1.26.5` (`GOTOOLCHAIN=go1.26.5`) |
| Module | `github.com/robertguss/go-foundry-cli` |

Source: [`p1-logs/machine.txt`](p1-logs/machine.txt).

## Exit checklist (Section 51.1 + REQ-241)

| # | Criterion | Result | Evidence |
| - | --------- | ------ | -------- |
| 1 | `validate` / `plan` / `catalog list` / `catalog show` / `version` complete with golden plans | **PASS** | `internal/cli/surface_test.go`, `cmd/foundry/testdata/writefree/*`, `internal/plan/testdata/*.golden` (mvp cli/tui + Appendix B) |
| 2 | Full applicable error-identifier coverage (Appendix D P1 domains) | **PASS** | `TestP1RegistryDomainsComplete` — 17 P1 ids; domains `spec,resolve,plan,render,report,catalog,usage,internal` |
| 3 | Zero writes / subprocesses / network proven (unit + P1.8.a e2e) | **PASS** | `TestP1QualitySuperSuite` purity; `TestWriteFree` + `TestWriteFreePurityAudit` (strace: no network, no child exec, no project writes) |
| 4 | Unimplemented profile IDs rejected (`configuration`, `local-persistence`, typos) | **PASS** | `internal/catalog/recipe_test.go`, `internal/resolve/resolve_test.go`, `cmd/foundry/testdata/writefree/profiles.txt` |
| 5 | E5 evidence **CONFIRMS** | **PASS** | [`E5-version-action-lock.md`](E5-version-action-lock.md) Result: **CONFIRMS** (2026-07-30) |
| 6 | Architecture / purity / golden discipline green (P1.8) | **PASS** | `TestP1QualitySuperSuite`, `TestArchitecture`, `TestSection58MechanicalGreen` |
| 7 | REQ traceability: all Phase-1 REQs verified with test paths | **PASS** | `FOUNDRY_MATRIX_PHASE_EXIT=P1 go test ./internal/archtest/ -run TestReqTraceabilityPhaseExit` — 53 P1 rows `verified` |
| 8 | No Phase-2 transaction/tool code reachable from generate product path | **PASS** | Packages absent: `internal/generate`, `internal/fsx`, `internal/toolrun`, `internal/gitinit`, `internal/verify`; `generate` hard-stops via `generatePhase1HardStop()` (`usage.invalid`, exit 2) |

## Exact commands

```bash
export GOTOOLCHAIN=go1.26.5
export CI=true

# P1 REQ matrix phase-exit gate (strict paths)
FOUNDRY_MATRIX_PHASE_EXIT=P1 go test -count=1 ./internal/archtest/ -run 'TestReqTraceabilityPhaseExit' -v

# P1.8 quality super-suite (arch + purity + golden + registry)
go test -count=1 ./internal/archtest/ -run 'TestP1QualitySuperSuite|TestArchitecture|TestWriteFreePurityStatic|TestP1RegistryDomainsComplete|TestSection58MechanicalGreen' -v

# Write-free e2e (P1.8.a)
go test -count=1 ./cmd/foundry/ -run 'TestWriteFree' -v

# Golden plans + determinism
go test -count=1 ./internal/plan/ -run 'TestGoldenPlans|TestDeterminismCount2' -v

# Full module regression
go test ./... -count=1 -timeout 180s
```

## Sample digests

| Artifact | Value |
| -------- | ----- |
| Catalog digest | `66478b609dc15339efe2e88c747342c61b0b7be28c074c81cbeea75b8ea10c1a` |
| plan_sha256 `mvp_minimal_cli` | `b78ff87a029329cd7e6d80a2412a71d33c52a5b862591c157f8d2b9cb2b4d370` |
| plan_sha256 `mvp_minimal_tui` | `00c7caebf0f006980ec3f3035d01224e6575cac767451a18ad75b0a6ff0f699d` |
| plan_sha256 `appendix_b_private_cli` | `d3a94c1e534bf9e6a0e36098a4f499784c75d00213ff4c47fa6707607a904e54` |
| plan_sha256 `appendix_b_private_tui` | `08eb2f041f93fad809a716147f3a6d16a183251ed657c6305663597fb4c4b9a8` |
| Foundry version output | `foundry 0.1.0` / `go1.26.5` / matching catalog_digest |

Sources: [`p1-logs/catalog-digest.txt`](p1-logs/catalog-digest.txt), [`p1-logs/plan-sha256-samples.txt`](p1-logs/plan-sha256-samples.txt).

## P1 REQ matrix citation

All **53** Phase-1 rows in `docs/evidence/req-traceability.json` are `status=verified` with on-disk primary test paths (strict path check). Representative owners:

| Package / surface | Representative REQs | Primary tests |
| ----------------- | ------------------- | ------------- |
| diagnostic | REQ-157, REQ-159, REQ-188 | `internal/diagnostic/*_test.go` |
| spec | REQ-038–043, REQ-210 | `internal/spec/*_test.go` |
| catalog | REQ-011, REQ-090–092, REQ-073–075 | `internal/catalog/*_test.go` |
| resolve | REQ-005, REQ-077, REQ-098–101, REQ-093 | `internal/resolve/*_test.go` |
| render | (via plan/catalog path safety) | `internal/render/*_test.go` |
| plan | REQ-120–122, REQ-211 | `internal/plan/*_test.go`, goldens |
| report + cli | REQ-030–033, REQ-035, REQ-037, REQ-155–156 | `internal/cli/*`, `internal/report/*`, writefree e2e |
| arch / purity | REQ-008, REQ-012, REQ-180–187, REQ-219 | `internal/archtest/*` |
| exit | REQ-241 | this document + writefree e2e + matrix gate |

Full table: [`req-traceability.md`](req-traceability.md). Checker: `FOUNDRY_MATRIX_PHASE_EXIT=P1`.

## Generate product path (no P2 txn/tool code)

| Check | Observation |
| ----- | ----------- |
| Package tree | No `internal/generate`, `internal/fsx`, `internal/toolrun`, `internal/gitinit`, `internal/verify` |
| CLI `generate` | Parses flags then `generatePhase1HardStop()` → `usage.invalid`, exit 2, zero filesystem mutation |
| e2e | `generate_hardstop` + purity audit: dest not created; strace clean for write-free commands |
| Architecture layers | `internal/archtest` encodes future generate above fsx/toolrun; packages not yet present |

## Failure classes exercised (write-free e2e)

| Class | Script / test | Exit |
| ----- | ------------- | ---- |
| Valid examples validate/plan | `writefree/validate_examples.txt`, `plan.txt` | 0 |
| Invalid specs | `writefree/validate_invalid.txt` | non-zero + registry ids |
| Unimplemented / recipe-only profiles | `writefree/profiles.txt` | `resolve.unknown_profile` + remediation |
| Catalog list/show | `writefree/catalog.txt` | 0 |
| Version + digest | `writefree/version.txt` | 0 |
| Stdin `--spec -` | `writefree/stdin.txt` | 0 |
| Generate hard-stop | `writefree/generate_hardstop.txt` | 2, no writes |
| Purity snapshot / strace | `TestWriteFreePurityAudit` | no network / child / project write |

## Blocking dependencies (all closed before this review)

`4hi` (P1.8), `5an.1` (write-free e2e), `dlv.1` (matrix seed+checker), `j1m` (E5), package roots `jq1`/`di6`/`wkz`/`s45`/`3ms`/`9a7`/`80g`, plus `d0i`/`w4n`.

## Result

**CONFIRMS** Phase 1 exit criteria (Section 51.1) and **REQ-241**:

1. Write-free surface complete with golden plans
2. P1 error-identifier coverage complete
3. Zero writes/subprocesses/network proven
4. Unimplemented profile IDs rejected with agent remediation
5. E5 **CONFIRMS**
6. Architecture/purity/golden discipline green
7. All P1 REQ matrix rows **verified** with test paths
8. No Phase-2 transaction/tool code on the generate product path

Phase 1 product exit is **met**. Parent epic `go-foundry-cli-5an` may close after this bead. Phase 2 entry still requires E1–E3 (already recorded) before any transaction/tool package lands.
