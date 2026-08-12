# E3 — Race detector prerequisite probe

- **Date:** 2026-07-30
- **Gated phase:** Phase 2 entry (Section 51.2, REQ-240)
- **Resolves:** FND-004 (executable confirmation of Section 44.3 / 35.4 / REQ-004 / REQ-217)
- **Spike code:** [`integration/hostile/e3/`](../../integration/hostile/e3/)
- **Known-race fixture:** [`integration/hostile/e3/knownrace/`](../../integration/hostile/e3/knownrace/)
- **Raw logs:** [`docs/evidence/e3-logs/`](e3-logs/)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS (`PRETTY_NAME`) |
| Kernel | `Linux 6.8.0-134-generic #134-Ubuntu SMP PREEMPT_DYNAMIC x86_64` |
| Arch | `linux/amd64` |
| Go | `go1.26.5` (`GOTOOLCHAIN=go1.26.5`; pin matches Section 12) |
| Host C compiler (preflight) | `/usr/bin/gcc` — `gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0` |
| Also present | `clang` 18.1.3; `/usr/bin/cc` → alternatives |
| Default `go env CGO_ENABLED` | `1` (host default; product builds still force `0`) |
| Default `go env CC` | `gcc` |

Source: [`e3-logs/machine.txt`](e3-logs/machine.txt).

### macOS / Darwin

| Item | Status |
| ---- | ------ |
| Preflight prefers `clang` then `gcc` then `cc` on `GOOS=darwin` | Present in `preflight.go` |
| `GOOS=darwin GOARCH=arm64 go test -c` of `e3` + `knownrace` | **PASS** ([`e3-logs/darwin-crosscompile.txt`](e3-logs/darwin-crosscompile.txt)) |
| Live race harness | Ready: [`scripts/e3-darwin-race.sh`](../../integration/hostile/e3/scripts/e3-darwin-race.sh) → logs under [`e3-logs/darwin/`](e3-logs/darwin/) |
| Live race run on owner macOS (CLT/Xcode compiler + `go test -race`) | **BLOCKED** 2026-07-30 — Tailscale peer `roberts-macbook-pro` offline (see below) |
| Equivalent CI macOS runner | Documented: GitHub-hosted `macos-latest` (Section 47 full matrix bead P3.2 `cic`); same commands as harness |

Portable logic is identical on both supported platforms; only compiler discovery order differs. Live macOS DATA RACE proof is a promotion/CI obligation, not a Section 44.3 redesign trigger (same pattern as E1 Darwin/APFS). Tracked by bead `go-foundry-cli-o60.1`.

#### Live Darwin procedure (owner Mac or CI macOS runner)

On a macOS host with Xcode CLT (Tailscale peer `roberts-macbook-pro` when online):

```bash
# From repository root on Darwin:
./integration/hostile/e3/scripts/e3-darwin-race.sh
```

Manual equivalent:

```bash
GOTOOLCHAIN=go1.26.5 go test ./integration/hostile/e3/ -count=1 -v -timeout 180s
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=1 FOUNDRY_KNOWN_RACE=1 \
  go test -race -count=1 ./integration/hostile/e3/knownrace/
```

Harness writes:

| File | Content |
| ---- | ------- |
| `e3-logs/darwin/machine.txt` | hostname, `sw_vers`, Go, clang/gcc, xcode-select |
| `e3-logs/darwin/go-test.txt` | full `-v` E3 package log; must show preflight `PATH lookup clang` (or gcc) |
| `e3-logs/darwin/knownrace-race.txt` | standalone `-race` fixture; must contain `DATA RACE` and non-zero exit |
| `e3-logs/darwin/summary.txt` | exit codes + `acceptance=PASS` when preflight + DATA RACE both seen |

After a green live run: flip the macOS table row to **PASS**, paste probe excerpts below, set bead `go-foundry-cli-o60.1` closed.

**Live Darwin (2026-07-31):** Recorded on GitHub-hosted `macos-latest` via `.github/workflows/darwin-evidence.yml` (run `30626462718`, job `macos-e3-race`). Owner Tailscale Mac remained offline; CI path is the documented alternate in bead `go-foundry-cli-o60.1`. Logs under `e3-logs/darwin/` with `acceptance=PASS`.

**Equivalent CI macOS runner (documented path):** when Section 47 expands (P3.2), a `runs-on: macos-latest` job should invoke the same harness (or the two `go test` commands above) and upload `e3-logs/darwin/*` as artifacts. That is the non-owner-Mac acceptance alternate in bead `go-foundry-cli-o60.1`. Current minimal CI (`.github/workflows/ci.yml`) is Linux-only unit tests and does not yet exercise race.

## Exact commands

```bash
# From repository root — full E3 matrix (preflight + skip notices + known race + gate):
GOTOOLCHAIN=go1.26.5 go test ./integration/hostile/e3/ -count=1 -v -timeout 180s

# Known-race fixture alone under the race detector (must FAIL with DATA RACE).
# FOUNDRY_KNOWN_RACE=1 arms the intentional body so module-wide -race stays green:
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=1 FOUNDRY_KNOWN_RACE=1 \
  go test -race -count=1 ./integration/hostile/e3/knownrace/

# Same fixture without -race / with product CGO posture (must PASS):
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 FOUNDRY_KNOWN_RACE=1 \
  go test -count=1 ./integration/hostile/e3/knownrace/

# Full module regression:
GOTOOLCHAIN=go1.26.5 go test ./... -count=1 -timeout 180s

# Darwin compile of spike (no live race without macOS host):
GOTOOLCHAIN=go1.26.5 GOOS=darwin GOARCH=arm64 \
  go test -c -o /tmp/e3-darwin.test ./integration/hostile/e3/
```

Captured verbose run: [`e3-logs/go-test.txt`](e3-logs/go-test.txt) (`EXIT:0`).

## Expected behavior (specification)

| Contract | Spec | Expectation |
| -------- | ---- | ----------- |
| 1 | §44.3 / FND-004 | Race is the sole CGO exception: `CGO_ENABLED=1` only after compiler preflight |
| 2 | §44.3 | Missing/broken compiler → **explicit skip notice**, never silent pass |
| 3 | REQ-217 | Known-race fixture under `-race` reports `DATA RACE` (instrumentation active) |
| 4 | §35.1–35.2 / §35.4 / App. E | Default + strict **generation** verification never include `go test -race` |
| 5 | REQ-004 / DEC-013 | Product/analysis/release builds use `CGO_ENABLED=0` |
| 6 | §16.6 / §47 | Race belongs to Foundry CI and generated **strict** workflow, not generation |

**Forbidden:** silent green when race was not run; race as a generation-gate step; global CGO=1 product builds.

## Observed behavior

### Pass/fail table (run 2026-07-30T20:39:41Z)

| Probe | Path | Result |
| ----- | ---- | ------ |
| Host compiler preflight | `PATH` → `/usr/bin/gcc`; compile trivial `.c` → object | **PASS** |
| Missing compiler skip notice | `PATH=/nonexistent…` | **PASS** (golden notice) |
| `CGO_ENABLED=0` skip notice | forced env | **PASS** (golden notice) |
| Broken compiler skip notice | executable `badcc` exits 1 | **PASS** (same missing-compiler golden) |
| Known race under `-race` | subprocess `go test -race ./…/knownrace/` | **PASS** (`DATA RACE` + non-zero exit) |
| Known race without `-race` | `CGO_ENABLED=0 go test` | **PASS** (ok; no DATA RACE) |
| Generation gate inventory | default + strict steps | **PASS** (no `-race`) |
| Product `CGO_ENABLED=0` compile | `go test -c` of e3 package | **PASS** |
| Skip-notice never silent | table empty-path / cgo-zero | **PASS** |
| End-to-end matrix | preflight + race + gate + cgo0 + golden | **PASS** (5/5) |
| Darwin cross-compile | `GOOS=darwin GOARCH=arm64 go test -c` | **PASS** |
| Live macOS race | GitHub Actions `macos-latest` (run 30626462718) + harness | **PASS** — preflight clang; DATA RACE under FOUNDRY_KNOWN_RACE=1 (see `e3-logs/darwin/`) |

All `E3PROBE` outcomes for required Linux contracts: **pass** (0 fail). Full log: [`e3-logs/go-test.txt`](e3-logs/go-test.txt).

### Skip-notice goldens (promotable)

```
race detector skipped: host C compiler unavailable (requires CGO_ENABLED=1 and a working C compiler); this is not a silent pass

race detector skipped: CGO_ENABLED=0 (race requires CGO_ENABLED=1 after compiler preflight); this is not a silent pass
```

Constants: `e3.SkipNoticeMissingCompiler`, `e3.SkipNoticeCGODisabled`.

### Generation gate (Section 35 / Appendix E)

| Step id | When | Contains `-race`? |
| ------- | ---- | ----------------- |
| `gofmt-conformance` | Always (in-process) | no |
| `tidy-mutation-set` | Always (in-process) | no |
| `go-mod-verify` | Always | no |
| `go-test` (`-count=1 -buildvcs=false …`) | Always | **no** |
| `go-vet` | Always | no |
| `final-conformance` | Always (in-process) | no |
| `go-staticcheck` | Strict only | no |
| `go-govulncheck` | Strict only | no |

Race placement is CI/strict-workflow only (`go test -race` after preflight), never a generation step.

### Raw log excerpts

Host preflight:

```
E3PROBE	os=linux arch=amd64 probe=preflight step=host_compiler
  args="PreflightWithEnv(host)" outcome=pass cgo=1 compiler="/usr/bin/gcc" skip=""
  detail="compiler preflight ok; PATH lookup gcc; version=gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0"
```

Missing compiler (never silent):

```
E3PROBE	… probe=preflight step=missing_compiler
  outcome=pass cgo=1 skip="race detector skipped: host C compiler unavailable
  (requires CGO_ENABLED=1 and a working C compiler); this is not a silent pass"
  detail="no C compiler on PATH or via CC; PATH has no gcc/clang/cc"
```

Known-race fixture under instrumentation:

```
WARNING: DATA RACE
Read at 0x… by goroutine 10:
  …/knownrace.(*RacyCounter).Inc.func2()
Previous write at 0x… by goroutine 9:
  …/knownrace.(*RacyCounter).Inc.func1()
…
E3PROBE	… probe=known_race step=expect_data_race outcome=pass
  detail="DATA RACE reported; race instrumentation active"
```

Without `-race` (product posture):

```
E3PROBE	… probe=known_race step=without_race_flag
  args="go test -count=1 (no -race) CGO_ENABLED=0" outcome=pass cgo=0
  detail="ok  github.com/robertguss/go-foundry-cli/integration/hostile/e3/knownrace …"
```

Generation gate:

```
E3PROBE	… probe=generation_gate step=default_and_strict outcome=pass
  detail="Section 35.1/35.2 steps exclude -race; race is CI/strict-workflow only (35.4)"
```

## Architecture audit (spike)

| Check | Result |
| ----- | ------ |
| Silent pass when race unavailable | **Absent** — all `!OK` paths emit golden skip notice |
| Race in generation gate inventory | **Absent** — `GenerationGateContainsRace` false for default+strict |
| Product CGO ban | **Honored** in probe — `CGO_ENABLED=0` compile succeeds; race path forces `1` only after preflight |
| Production CI templates (`strict.yml`) | **Not created** (Phase 2/3); preflight + goldens are the promotion source |
| Known-race fixture rewritable for promotion | **No rewrite needed** — keep `knownrace` package as permanent REQ-217 fixture |

## P2 / CI promotion path (fixtures without rewrite)

| Spike path | Promotion target |
| ---------- | ---------------- |
| `integration/hostile/e3/preflight.go` | Foundry CI race job + generated `strict.yml` compiler preflight shell/Go helper |
| `SkipNoticeMissingCompiler` / `SkipNoticeCGODisabled` | Exact CI log strings; fail the job messaging layer if race was required-and-skipped incorrectly; scheduled strict may skip with notice |
| `integration/hostile/e3/knownrace/` | Permanent Foundry CI known-race package (REQ-217); arm with `FOUNDRY_KNOWN_RACE=1` so module-wide `-race` stays green |
| `DefaultGenerationGate` / `StrictGenerationGate` | Seed for `internal/verify` step tables (P2) — still no race |
| `SanitizeEnvForRace` | Race job env construction (`CGO_ENABLED=1`, explicit `CC`) |
| `E3PROBE` log format | CI probe logging for race preflight steps |

**Do not rewrite:** skip-notice goldens, known-race concurrent write shape, generation-gate exclusion of `-race`.

## Result

**CONFIRMS** Section 44.3 (CGO policy / race exception), Section 35.4 (race not in generation gate), FND-004, REQ-004, and REQ-217 on **Linux** (`linux/amd64`) with a real host C compiler (`gcc 13.3.0`):

1. Compiler preflight discovers and probes a working C compiler before race.
2. Missing, disabled-CGO, and broken-compiler paths emit **explicit skip notices** (golden text includes “not a silent pass”).
3. Known-race fixture under `go test -race` reports **DATA RACE** (instrumentation active).
4. Default and strict **generation** verification inventories exclude `-race`.
5. Product-style `CGO_ENABLED=0` compilation succeeds for the same packages.

**Operational refinements recorded (not contradictions of §44.3):**

1. **Live owner-macOS race execution** awaits a Darwin host with Xcode CLT (bead `go-foundry-cli-o60.1`); Darwin-oriented discovery order, cross-compile, and `scripts/e3-darwin-race.sh` harness are ready. Equivalent path: CI `macos-latest` running the same harness.
2. **Shell alias `cc`→interactive tool** on this host does not affect preflight: resolution uses PATH binary lookup (prefers `gcc`/`clang`), never shell aliases.

**FND-004:** split CGO ban is executable — product stays pure-Go (`CGO_ENABLED=0`); race is an explicit, preflighted development/CI exception. Proceed to Phase 2 using this preflight + fixture as the race-job template.
