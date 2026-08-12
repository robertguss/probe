# Foundry testing guide

Maintainer guide for tests in the Foundry repository (SPEC-FOUNDRY-002
Sections 45–47, REQ-210–REQ-219).

Implementation authority:
[`docs/02-definitive-foundry-specification-revised-fable-5.md`](../02-definitive-foundry-specification-revised-fable-5.md)
(**SPEC-FOUNDRY-002**).

## Layers

| Layer                                       | Where                                                                            | How to run                                                                                                                                                                                          |
| ------------------------------------------- | -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Product E2E plan (commands / generators)    | [`product-e2e-plan.md`](product-e2e-plan.md) + `make product-e2e` (when wired)  | Local/agent only — **no CI job** for this suite; macOS via operator MBP checklist in that doc                                                                                                       |
| Unit                                        | beside code (`*_test.go`)                                                        | `go test -count=1 ./...`                                                                                                                                                                            |
| Golden                                      | `testdata/*.golden`                                                              | same; update with `UPDATE_GOLDEN=1` (never in CI)                                                                                                                                                   |
| Architecture                                | `internal/archtest`                                                              | included in `./...`                                                                                                                                                                                 |
| Section 58 red-lines                        | `internal/archtest` (`CheckRedlines`)                                            | included in `./...`; register at `docs/evidence/section-58-redlines.md`                                                                                                                             |
| REQ→test matrix (DoD #2)                    | `internal/archtest` (`TestReqTraceability*`)                                     | always-on completeness vs §53; phase-exit: `FOUNDRY_MATRIX_PHASE_EXIT=P1\|P2\|P3\|P4\|S6\|DoD`; register at `docs/evidence/req-traceability.md`                                                     |
| Profile admission process (REQ-078 / §19.4) | `internal/archtest` (`TestProfileAdmissionDocHeadings`)                          | always-on heading/content check; process doc at `docs/dev/profile-admission.md`                                                                                                                     |
| Section 21 recipes (REQ-073–075)            | `internal/archtest` (`TestSection21Recipe*`) + `internal/catalog/recipe_test.go` | recipe docs at `docs/recipes/`; remediation paths; not catalog profiles                                                                                                                             |
| testscript e2e (write-free)                 | `cmd/foundry/testdata/writefree/`                                                | `go test ./cmd/foundry -run TestWriteFree` (5an.1)                                                                                                                                                  |
| Generate e2e matrix (j8h.2)                 | `integration/generate/` + `cmd/foundry/testdata/generate/`                       | CI jobs `*-generate-e2e`; `go test ./integration/generate/`; `go test ./cmd/foundry -run TestGenerateE2E`; artifacts via `FOUNDRY_GENERATE_E2E_ARTIFACT_DIR`                                        |
| Hostile / evidence                          | `integration/hostile/e{1..5}`                                                    | `go test -count=1 ./integration/...`                                                                                                                                                                |
| REQ-214 sentinel (permanent)                | `integration/hostile/sentinel` + `internal/toolrun` Sentinel\*                   | CI job `linux-hostile-sentinel`; `FOUNDRY_SENTINEL_ARTIFACT_DIR` for redacted dumps                                                                                                                 |
| REQ-213 hostile FS (permanent)              | `integration/hostile/fsx` + `internal/fsx` (`//go:build hostile`)                | CI job `linux-hostile-fsx`; `go test -tags=hostile ./integration/hostile/fsx/`; `FOUNDRY_HOSTILE_FSX_ARTIFACT_DIR` for probe JSON; multi-FS via `FOUNDRY_FSX_ROOTS` / `scripts/fsx-mount-matrix.sh` |
| Race                                        | CI / explicit                                                                    | `CGO_ENABLED=1 go test -race` after compiler preflight                                                                                                                                              |
| Perf baselines (REQ-165)                    | script + schema test                                                             | `./scripts/perf/capture-baselines.sh`; `go test ./internal/archtest -run TestPerfBaselineSchema`                                                                                                    |
| Perf benchmarks                             | `internal/{spec,resolve,plan,render}`                                            | `go test -bench=. -count=5 -benchtime=200ms ./internal/spec ./internal/resolve ./internal/plan ./internal/render`                                                                                   |

### Build tags

| Tag           | Used in `//go:build` today? | Purpose |
| ------------- | --------------------------- | ------- |
| `hostile`     | Yes (`integration/hostile/fsx`, `internal/fsx` hostile cases) | Hostile FS mounts / ENOSPC probes; `make hostile` |
| `unix`        | Yes (fsx, verify, toolrun, sentinel, gitinit, …) | Unix-only product paths |
| `tui_pty`     | Yes (`integration/tui/pty_test.go`) | Scripted PTY TUI; `make product-e2e` |
| `foundrydev`  | Yes (`internal/catalog` load-dir surface) | Dev-only catalog directory load |
| `integration` | **No** | Not applied: heavy generate/dogfood stay default (ipk.4 decision A). Do not document as active. |
| `perf`        | **No** | Benchmarks use `go test -bench`, not a build tag. |
| `race`        | **No** | Race uses `go test -race` / `CGO_ENABLED=1`, not a build tag. |

Default `go test ./...` must stay green without extra tags. Product builds use
`CGO_ENABLED=0`; race is the sole CGO exception (Section 44.3).

### Default-suite decision: generate / dogfood e2e (ipk.4)

**Decision A (2026-08-01):** Heavy real-generate paths stay in the **default**
`go test ./...` suite — **not** behind `//go:build integration`.

| Package / path | Default `./...`? | Notes |
| -------------- | ---------------- | ----- |
| `integration/generate` | Yes | Multi-minute; CI also has dedicated `*-generate-e2e` with retries/artifacts |
| `integration/dogfood` | Yes | CI golden-matrix dogfood slice re-runs with retries for isolation |
| `integration/fixtures` (incl. real-generate cells) | Yes | Prefer `-short` skip on heavy cells (ipk.5) |
| `integration/tui` (non-PTY) | Yes | debug-log matrix already honors `-short` |
| `integration/tui` (`tui_pty`) | No | Requires `-tags=tui_pty`; product-e2e only |
| `integration/hostile/fsx` | No | `-tags=hostile` only |

**Why A (not B):** Agent/CI fast loops already use `./internal/...` + write-free;
full `./...` is the honesty bar that product generate still works. Option B
(tag everything `integration` and shrink unit CI) was rejected for this audit
to avoid silent e2e drops and large CI/Makefile churn. Dedicated CI jobs that
re-run generate/dogfood are intentional isolation (artifacts, OS matrix,
retries), not a license to remove them from default.

**Budgets (honest):**

| Path | Command | Budget |
| ---- | ------- | ------ |
| Fast agent | `go test -count=1 ./internal/...` + write-free | seconds–low minutes |
| Full default | `go test -count=1 ./...` | multi-minute (includes generate/dogfood) |
| Short full | `go test -short -count=1 ./...` | skips heavy untagged e2e cells (ipk.5) |
| Medium generate | `go test -count=1 -timeout 15m ./integration/generate/` | ≤15m |
| Product E2E | `make product-e2e` | ~15–40m warm; **not** a CI job |

### Default-suite policy: hostile fsx & sentinel (`wet.4.6`)

**Decision:** the REQ-214 sentinel suite (`integration/hostile/sentinel`,
`integration/hostile/e2`, and the `Sentinel*`/`ProcessTree*` cases in
`internal/toolrun`) stays in the **default** `go test ./...` suite. It is
constrained only by `//go:build unix`, needs no elevated privileges or
special mounts, and runs in ~1s on both macOS and Linux — keeping it default
maximizes coverage on every `go test ./...` invocation without hurting the
fast feedback loop. The REQ-213 hostile filesystem suite
(`integration/hostile/fsx` and the `hostile`-tagged cases in
`internal/fsx`) stays **behind the `hostile` build tag** and is excluded
from the default suite: it probes ENOSPC/read-only/multi-filesystem
behavior (`FOUNDRY_FSX_ROOTS`) that on Linux CI runners is exercised via
scratch mounts, and on developer macOS machines may require `sudo`
(`hdiutil`/loopback mounts) that we do not want to force on every
`go test ./...` run.

Given that split:

- Run everything fast-and-default: `go test -count=1 ./...` (includes
  sentinel/e1-e5, excludes hostile-tagged fsx).
- Run the hostile fsx suite locally in one command: `make hostile`.
- Run just the sentinel/E2 slice in isolation: `make sentinel`.
- CI keeps its dedicated tagged jobs regardless of this policy: `sentinel`
  (`linux-hostile-sentinel`) and `hostile-fsx` (`linux-hostile-fsx`) in
  [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml); the `unit`
  job's `go test -count=1 ./...` on `ubuntu-latest`/`macos-latest` already
  covers sentinel/e1-e5 as part of the default suite on both platforms.

## export_test / white-box policy (ipk.9)

`export_test.go` and `*ForTest` helpers exist only for **behavior injection**
or **classification seams** needed by real scenario tests (pipeline hosts,
stream classify, stage FDs). Rules:

| Prefer | Avoid |
| ------ | ----- |
| Public APIs and package-local white-box tests (`package foo`) | Cross-package reflect/unsafe on unexported fields |
| `toolrun.NewStepResultForTest` (or similar named test constructors) | Residual coverage unlockers that only hit lines without contract asserts |
| Deleting a `*ForTest` when public tests already cover the branch | Growing export surface for line floors alone |

## Residual and coverage-chasing tests (ipk.3)

Files named `*residual*_test.go` or `*coverage*_test.go` under `internal/` /
`cmd/` are **quarantined inventory**, not a preferred place for new tests.

| Rule | Detail |
| ---- | ------ |
| Prefer public contracts | New behavior is covered by suite/scenario/property/golden tests that exercise exported APIs and real inputs. |
| No pure line-chasing | Do not add residual/coverage tests whose only job is to touch branches via `*ForTest` / `export_test` without a failing assertion on a contract. |
| Allowlist growth gate | Every residual/coverage file path must appear in `archtest.ResidualCoverageAllowlist` with a short rationale. `TestResidualCoverageInventory` fails if a new file appears or an allowlisted file is deleted without updating the map. |
| Rewrite or delete | When cleaning: rewrite padding into real contract asserts, or delete the test if package floors still hold via public scenarios. |
| Coverage floors | CI total ≥86% and package floors (see CI table) must remain met by **real** behavior tests — residual files may help, but are not license to invent always-true asserts (see ipk.2). |

Inventory (2026-08-01 audit): ~4k LOC across catalog/cli/diagnostic/fsx/plan/
render/report/resolve/spec/testutil/verify/version residual and coverage
files, plus `export_test.go` surfaces in cli/fsx/render/verify (reduced
separately under ipk.9).

```bash
# Gate runs as part of archtest (default suite)
go test -count=1 ./internal/archtest -run TestResidualCoverageInventory
```

## Step logger (`internal/testutil`)

Multi-step tests **must** use the structured step logger. Trivial table rows
may use plain `t.Run`.

```go
log := testutil.New(t)
log.Phase("arrange")
log.Inputs(map[string]string{
    "spec":      "demo.toml", // basenames only — no absolute host homes
    "archetype": "cli",
    "verify":    "default",
})
log.Fixture("catalog", "embedded")
log.PhaseEnd("arrange", testutil.OutcomeOK)

log.Phase("act")
log.Step("parse", testutil.OutcomeOK, "fields=3")
log.PhaseEnd("act", testutil.OutcomeOK)

log.Phase("assert")
log.Assert("field_count", got == 3, 3, got)
log.PhaseEnd("assert", testutil.OutcomeOK)
```

### Log line format

Every line includes: `package`, `test`, `idx`, `elapsed_ms`, `level`, `name`,
`outcome`, optional `detail` / `expected` / `actual`.

Levels: `PHASE`, `STEP`, `ASSERT`, `FIXTURE`, `SKIP`, `FAIL`.

On failure, the logger dumps the last N steps plus attached paths/IDs/exit
codes. Secret-like keys (`PASSWORD`, `*_TOKEN`, `API_KEY`, …) are redacted.

### Subprocess logging

Record argv, env-allowlist hash (not full env), exit code, stream byte
lengths, and first/last lines **on failure only**.

## Golden files (REQ-219)

- Goldens are checked-in files with normalized LF paths/modes and no timestamps.
- Update **one named suite at a time**:

  ```bash
  UPDATE_GOLDEN=1 go test ./internal/foo -run TestPlanGolden
  ```

- **CI must never auto-update.** `internal/testutil` refuses updates when
  `CI=true` even if `UPDATE_GOLDEN=1`. CI workflows must set `CI: "true"` and
  must never set `UPDATE_GOLDEN` (enforced by `archtest.CheckGoldenDiscipline`).
- Print every changed path (done by `CompareGolden`).
- Bulk updates above the documented suite threshold
  (`archtest.GoldenBulkThreshold`, currently 20 files) require a second
  explicit opt-in (suite-specific; not automatic).

### CLI surface snapshots

Help text and representative command/flag-combination outputs are snapshotted
in `internal/cli/testdata/`:

- Help: `TestHelpGoldens` captures root and subcommand `--help`.
- Flag combinations: `TestFlagCombinationGoldens` captures stable outputs such
  as `version --output json`, `catalog list`, and `catalog show cli --output json`.

Both suites normalize line endings to LF and trim trailing whitespace with
`normalizeHelp` so cross-platform diffs stay stable. The `golden-matrix` CI job
validates them and rejects updates when `CI=true`.

## Architecture / purity / quality tests (P1.8)

`internal/archtest` enforces:

- No `internal/compose` / `internal/structured` (deleted Stage 4 machinery)
- No production import of `internal/testutil`
- Single `os.Exit` site: `cmd/foundry/main.go`
- Cobra only from `cmd/foundry` / `internal/cli`
- Section 42.3 layer direction when packages exist
- Section 58 red-line negatives (`CheckRedlines`, consumed by `Check`)
- Write-free purity (`CheckPurity`): P1 packages ban `net`/`os/exec`/`syscall`;
  `internal/cli` allows `exec.LookPath` only (no `Command`/`Start`) — complements
  write-free e2e purity audits
- Golden CI discipline (`CheckGoldenDiscipline`)
- P1 Appendix D registry completeness (`CheckP1RegistryCompleteness`)

Super-suite: `go test ./internal/archtest -run TestP1QualitySuperSuite`.

## CI (current matrix)

Primary workflow: [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml).
Actions are full-SHA pinned with version comments (`catalog/versions.toml`).
Never set `UPDATE_GOLDEN` in CI; always set `CI=true` (REQ-219).

| Job | OS | What it gates |
| --- | -- | ------------- |
| `unit` | ubuntu + macos | `gofmt`, `go mod verify`, `go vet`, `go test -count=1 ./...`, property tests `-count=10`, Staticcheck |
| `windows-unit` | windows-latest | **Hard gate** for compile/vet hygiene only — not a product-support claim. Product platforms are **Linux + macOS**. Unix-only tests are build-tagged out. |
| `coverage` | ubuntu | Total ≥86%, `cmd/foundry` ≥50% (honest floor; main via `testscript.Main`), plus package floors: generate/fsx/plan ≥80%, cli ≥85%, resolve ≥90% |
| `race` | ubuntu | `CGO_ENABLED=1 go test -race` after compiler preflight; **fail-closed** if preflight fails (ipk.11 — no soft-skip green on ubuntu-latest) |
| `catalog-digest` | ubuntu | Embedded catalog digest matches on-disk `catalog/` |
| `golden-matrix` | ubuntu | Plan/render/CLI goldens + dogfood slices; refuses golden updates |
| `sentinel` / `hostile-fsx` | ubuntu | REQ-214 / REQ-213 permanent hostile suites |
| `*-generate-e2e` | ubuntu + macos | Generate e2e matrix |

Additional workflows (not all PR-required):

| Workflow | Role |
| -------- | ---- |
| `bench-regression.yml` | Microbench vs `docs/evidence/perf/microbench-baseline.json` (PR) |
| `perf-regression.yml` / `perf-baselines.yml` | Warm generate p50 gate / weekly capture |
| `flaky.yml` | Flake detector (`-count=5`); continue-on-error while baseline stabilizes — see flake policy below |
| `strict.yml` | Weekly: govulncheck, bounded fuzz, full hostile |
| `darwin-evidence.yml` | macOS E1 APFS + E3 race harnesses |
| `docs-validate.yml` / `lint-testscripts.yml` | Docs recipes + testscript lint |

### Flake policy: retries and flaky detection (ipk.17)

| Mechanism | Policy |
| --------- | ------ |
| `flaky.yml` | Still `continue-on-error: true` while the suite baseline stabilizes. Always uploads `flaky-detection-report` artifact (JSON + summary). |
| When flaky detector flags a test | **Required:** open a bead (`bd create`) naming the package/test and link the artifact run URL in the description. Do not ignore intermittent failures. |
| CI generate-e2e / dogfood retries (3 attempts) | Retries are for throughput on long suites. **When attempt > 1 succeeds after a failure:** the job must keep uploaded artifacts; file a bead if the same test retries twice in 7 days. Prefer fixing root cause over raising retry counts. |
| Race on flaky sample | Optional: run flaky detector with `-race` on `toolrun`/`generate`/`plan`/`resolve` locally when investigating concurrency flakes. |
| Goal | Drive retries toward reporting-only once flake rate is near zero; never use retries as a substitute for fixing races/timeouts. |

### Intentional CI gaps: product-e2e and `tui_pty` (ipk.18)

| Surface | In PR CI? | Where it runs | Evidence / DoD |
| ------- | --------- | ------------- | -------------- |
| `make product-e2e` | **No** | Local/agent only (Linux + macOS MBP) | [`product-e2e-plan.md`](product-e2e-plan.md), [`docs/evidence/product-e2e-macos.md`](../evidence/product-e2e-macos.md) |
| `integration/tui` with `-tags=tui_pty` | **No** | Local/`make product-e2e` only | Same plan + PTY sections; never claimed green by unit/generate-e2e jobs |

These gaps are **intentional** (long wall time, real PTY, host go1.26 toolchain).
Do **not** treat green PR CI as proof that product-e2e or scripted TUI PTY
passed. Phase-exit / release DoD must cite local or agent evidence paths above.

**Product E2E** (`make product-e2e`) is **not** a CI job — local/agent only.
See [`product-e2e-plan.md`](product-e2e-plan.md).

### Supported platforms

| Platform | Product | CI role |
| -------- | ------- | ------- |
| Linux | Full support | Primary unit, race, hostile, generate-e2e |
| macOS | Full support | unit matrix + generate-e2e + darwin-evidence; product-e2e green on MBP (`docs/evidence/product-e2e-macos.md`) |
| Windows | **Not a product platform** | Hard-gated compile/vet of stubs only |

### Suite budget (agent guidance)

| Path | Command | When |
| ---- | ------- | ---- |
| **Fast** | `go test -count=1 ./internal/...` + `go test ./cmd/foundry -run TestWriteFree` | Default agent loop |
| **Short full tree** | `go test -short -count=1 ./...` | Full module under `-short`: multi-minute generate/dogfood/real-generate fixture cells skip (ipk.5); pure unit + goldens still run |
| **Medium** | `go test -count=1 ./integration/generate/ -timeout 15m` | Generate-path changes |
| **Full product** | `make product-e2e` (~15–40 min warm) | Release bar / phase exit / inventory claims |
| **Hostile** | `make hostile` / `make sentinel` | FS/env changes |

## Write-free e2e (`TestWriteFree`)

Process-boundary scripts under `cmd/foundry/testdata/writefree/` exercise the
real binary (via `testscript.Main` → `foundry`) for:

- `version` / `catalog list|show` / `validate` / `plan`
- examples/ + `testdata/specs/` fixtures (including invalids)
- `--spec -` stdin, flag matrix, unimplemented profiles
- `generate` Phase-1 hard-stop (zero filesystem mutation)
- purity snapshots (no destination writes)

Verbose step logs and Linux `strace` purity live in
`cmd/foundry/writefree_e2e_test.go` (`TestWriteFreeVerboseDiagnostics`,
`TestWriteFreePurityAudit`). Failure output includes argv, exit, stream
digests, plan_sha256, and error ids so agents can fix without a debugger.

```bash
go test ./cmd/foundry -run TestWriteFree -count=1
go test ./cmd/foundry -run 'TestWriteFreeVerbose|TestWriteFreePurity' -count=1 -v
```

## Plan/generate equality (Section 13.3 / `j8h.4`)

Plan/generate divergence is a named defect class: the plan a user inspects
MUST be byte-equal to the plan `generate` executes under the same flags
(REQ-033). Coverage layers:

| Layer                         | Path                                                         | Run                                                                                |
| ----------------------------- | ------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| CLI property (fake lifecycle) | `internal/cli` `TestPlanGenerateByteEquality*`               | `go test ./internal/cli -run TestPlanGenerateByteEquality -count=2`                |
| Package dual Pipeline         | `integration/fixtures` `TestPlanGeneratePackageByteEquality` | `go test ./integration/fixtures -run TestPlanGeneratePackageByteEquality -count=2` |
| Process e2e (real generate)   | `cmd/foundry/testdata/plan_generate/`                        | `go test ./cmd/foundry -run TestPlanGenerateEquality -count=1`                     |

On inequality, tests dump a redacted unified plan JSON diff
(`testutil.AssertPlanJSONEqual` / `RedactPlanJSONForDiff`).

## Generate e2e matrix (`j8h.2` / P2.5.d)

Permanent regression shield for real `generate` (default+strict, git on/off,
commit outcomes, cancel, stream failure, disclosure, double-generation,
quiet/progress, process-tree ids, REQ-133 scans). Soft-related to dogfood
(`vu8`); **hard-blocks Phase 2 exit (`5pr`)**.

| Layer               | Path                             | Run                                                     |
| ------------------- | -------------------------------- | ------------------------------------------------------- |
| Integration matrix  | `integration/generate/`          | `go test -count=1 -timeout 15m ./integration/generate/` |
| Process testscripts | `cmd/foundry/testdata/generate/` | `go test ./cmd/foundry -run TestGenerateE2E -count=1`   |

On intentional failure fixtures, step logs and optional artifacts under
`FOUNDRY_GENERATE_E2E_ARTIFACT_DIR` name **stage + cause** (filename
`*__stage-<id>__cause-<token>__*.txt`). Never logs secret-bearing env values.

CI: `.github/workflows/ci.yml` jobs `ubuntu-latest-generate-e2e` and
`macos-latest-generate-e2e` (matrix OS), artifact upload on failure.

```bash
FOUNDRY_GENERATE_E2E_ARTIFACT_DIR=/tmp/gen-e2e \
  go test -count=1 -timeout 15m -v ./integration/generate/
go test ./cmd/foundry -run TestGenerateE2E -count=1
```

## CLI dogfood (`vu8` / P2.7)

Earliest real-use exercise after one green generate fixture (Section 52 / 60,
REQ-242 / REQ-245 / REQ-247). Complements — does **not** replace — the generate
e2e matrix or hostile suite. Phase 2 exit (`5pr`) still requires both.

| Layer       | Path                                | Run                                                    |
| ----------- | ----------------------------------- | ------------------------------------------------------ |
| Script      | `scripts/dogfood-cli.sh`            | `./scripts/dogfood-cli.sh` / `--with-repo-map`         |
| Integration | `integration/dogfood/`              | `go test -count=1 -timeout 10m ./integration/dogfood/` |
| Inputs      | `dogfood/repo-map/` + smoke fixture | specs + post-generate overlays only                    |
| Evidence    | `docs/evidence/dogfood-cli-*.md`    | plan_sha256, timings, REQ-247 seeds                    |

Requires pinned **go1.26.5** (catalog exact) for the _generated project's_
toolchain pin. The Go integration tests' own toolchain-discovery helper
(`pinnedGoBinary()` in `integration/fixtures/helpers_test.go` and
`integration/dogfood/helpers_test.go`, bead `go-foundry-cli-wet.3.1`)
resolves in this order:

- `FOUNDRY_GO_BIN` absolute override — trusted directly, no version probing.
- toolchain module-cache glob under `$GOMODCACHE` (any `GOOS`/`GOARCH`, any
  `go1.26.x` patch — not only `.5`), then `/usr/local/go/bin/go`,
  `/opt/homebrew/bin/go`, `/usr/local/bin/go`, then `PATH` (`exec.LookPath`).
- accepts any **go1.26.x** patch (not only the exact `.5`) to avoid spurious
  skips on typical dev machines running a slightly different 1.26 patch;
  `catalog/versions.toml` + `go.mod` remain the single source of truth for
  the exact generated-project pin.

GOROOT for the discovered binary is derived by resolving symlinks first
(`filepath.EvalSymlinks`) before taking two directories up — naively doing
that on a symlink (e.g. Homebrew's `/opt/homebrew/bin/go` → Cellar) yields a
bogus GOROOT and breaks `go vet`/`go test` with `no such tool "vet"`.

CI: `actions/setup-go` with `go-version-file: go.mod` already installs the
exact pinned toolchain for every job (including `golden-matrix`, which now
also runs the dogfood suite — see below), so these tests exercise the real
go1.26.5 in CI rather than skipping.

See root README “Exact go1.26.5” and [`dogfood/README.md`](../../dogfood/README.md).
Set `FOUNDRY_DOGFOOD_ARTIFACT_DIR` to capture script logs.

```bash
# Optional when PATH has only a nearby version (e.g. mise 1.26.4):
# export FOUNDRY_GO_BIN=$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64/bin/go

FOUNDRY_DOGFOOD_ARTIFACT_DIR=docs/evidence/dogfood-cli-logs \
  ./scripts/dogfood-cli.sh --with-repo-map
go test -count=1 -timeout 10m -v ./integration/dogfood/
```

## Performance baselines (`j8h.1` / P2.7.b / REQ-165 / Section 49)

Scripted warm/cold generate + write-free p50/p95 captures. **No absolute
pass/fail gates** until multi-machine history exists. Format locked by
`TestPerfBaselineSchema` and `docs/evidence/perf/schema.json`.

| Layer           | Path                                         | Run                                                                                   |
| --------------- | -------------------------------------------- | ------------------------------------------------------------------------------------- |
| Capture harness | `scripts/perf/capture-baselines.sh`          | `./scripts/perf/capture-baselines.sh` (`--quick`, `--skip-cold`)                      |
| Evidence pack   | `docs/evidence/perf/`                        | `baselines-latest.json`, `summary.md`, `P2.8-citation.md`                             |
| Format lock     | `internal/archtest` `TestPerfBaselineSchema` | `go test ./internal/archtest -run TestPerfBaselineSchema`                             |
| Optional CI     | `.github/workflows/perf-baselines.yml`       | `workflow_dispatch` / weekly; **not** a required gate                                 |
| Regression gate | `.github/workflows/perf-regression.yml`      | PR-only; fails on `warm_host_cache` p50 regression >10% (bead go-foundry-cli-wet.5.3) |

```bash
./scripts/perf/capture-baselines.sh
# FOUNDRY_PERF_OUT=/tmp/perf WRITE_FREE_N=10 WARM_N=2 COLD_N=1 \
#   ./scripts/perf/capture-baselines.sh
go test -count=1 ./internal/archtest/ -run TestPerfBaselineSchema
```

P2.8 exit (`5pr`) cites [`docs/evidence/perf/P2.8-citation.md`](../evidence/perf/P2.8-citation.md).

## Microbenchmark regression gate (bead go-foundry-cli-wet.5.7)

Hot-path benchmarks live in `internal/{spec,resolve,plan,render}/bench_test.go`:

```bash
go test ./internal/spec ./internal/resolve ./internal/plan ./internal/render \
  -run=^$ -bench=. -count=5 -benchtime=200ms
```

The stored baseline is at `docs/evidence/perf/microbench-baseline.json`. Update
it intentionally after a verified performance improvement:

```bash
python3 scripts/perf/bench-capture.py 5 200ms > docs/evidence/perf/microbench-baseline.json
```

Comparison method (implemented in `scripts/perf/bench-compare.py`):

- Reference value: median of the stored baseline `ns/op` values.
- Observed value: median of the current run `ns/op` values.
- Regression flag: observed median > reference median × (1 + threshold).
- Default threshold is **20%** (`FOUNDRY_BENCH_REGRESSION_THRESHOLD`).

CI job: `.github/workflows/bench-regression.yml` runs on every PR and fails
when a statistically significant regression is detected. Current and baseline
artifacts are uploaded with 14-day retention for offline analysis.

## Phase exit reviews

| Phase | Bead                 | Evidence                                                           | Matrix gate                    |
| ----- | -------------------- | ------------------------------------------------------------------ | ------------------------------ |
| P1    | `go-foundry-cli-0z4` | [`docs/evidence/P1-exit-review.md`](../evidence/P1-exit-review.md) | `FOUNDRY_MATRIX_PHASE_EXIT=P1` |
| P2    | `go-foundry-cli-5pr` | [`docs/evidence/P2-exit-review.md`](../evidence/P2-exit-review.md) | `FOUNDRY_MATRIX_PHASE_EXIT=P2` |

```bash
FOUNDRY_MATRIX_PHASE_EXIT=P2 go test -count=1 ./internal/archtest/ \
  -run TestReqTraceabilityPhaseExit
```

## Canonical commands

```bash
go test -count=1 ./...
go test ./cmd/foundry -run TestWriteFree -count=1
go test ./cmd/foundry -run TestPlanGenerateEquality -count=1
go test ./cmd/foundry -run TestGenerateE2E -count=1
go test -count=1 -timeout 15m ./integration/generate/
go test -count=1 -timeout 10m ./integration/dogfood/
go test ./internal/archtest -run TestPerfBaselineSchema -count=1
go vet ./...
gofmt -l .
CGO_ENABLED=0 go build -o /tmp/foundry ./cmd/foundry
./scripts/dogfood-cli.sh --with-repo-map
./scripts/perf/capture-baselines.sh
```
