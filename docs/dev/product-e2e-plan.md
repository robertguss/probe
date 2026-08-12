# Product end-to-end (E2E) plan and guide

**Bead epic:** `go-foundry-cli-79a`  
**Authority:** [SPEC-FOUNDRY-002](../02-definitive-foundry-specification-revised-fable-5.md)  
**Related:** [testing.md](testing.md) (layer catalog), [user-guide.md](../user-guide.md) (product surface)

This document is the **product-facing** E2E plan: every public command, flag,
archetype/profile generator path, and safety behavior we claim to verify, plus
how to re-run that verification on **Linux** (agent-primary) and **macOS**
(MacBook Pro checklist).

It is intentionally **not** a re-statement of the full REQ matrix or hostile
filesystem program. Those stay under [testing.md](testing.md) and
`internal/archtest`.

---

## 1. Goals

| Goal | Detail |
| ---- | ------ |
| Written plan | This file — inventory, pass criteria, how to run, coverage map |
| Durable automation | Go tests / testscripts under the repo; extend existing suites first |
| Single entrypoint | `make product-e2e` (local/agent only) |
| Dogfood bar | After `generate`: `go build`, `go test`, run key workflows |
| TUI interaction | **Scripted PTY only** (no human keyboard required) |
| Platforms | **Linux** (executed in-repo by agents) + **macOS** (you run on MBP) |

### Explicit non-goals

- **No CI wiring** for this product E2E suite (no new GitHub Actions jobs).
- Full SPEC §53 REQ→test matrix as black-box CLI (see `internal/archtest` /
  `docs/evidence/req-traceability.md`).
- Maintainer tooling as primary rows: `make hostile`, perf baselines, flaky
  detector, docs-validate internals (covered elsewhere; linked in §8).
- Human-driven interactive TUI sessions as a gate.
- Inventing profiles beyond catalog `distribution`.

**CI coverage statement (ipk.18):** PR CI never runs `make product-e2e` and
never builds `//go:build tui_pty` tests. A green unit/generate-e2e matrix is
**not** evidence that product-e2e or scripted TUI PTY passed. Local/agent
evidence: this plan §2–3, `docs/evidence/product-e2e-macos.md`, and the
intentional-gap table in [`testing.md`](testing.md#intentional-ci-gaps-product-e2e-and-tui_pty-ipk18).

---

## 2. How to run

### Prerequisites

- Repo checkout with a working Go toolchain for **building Foundry**.
- For **`generate` / dogfood**: **exact** catalog-pinned Go (`go1.26.5` under
  `GOTOOLCHAIN=local`). See README / user guide (`FOUNDRY_GO_BIN` or auto
  discovery). Nearby versions fail closed with `tool.wrong_version`.
- `git` on `PATH` (generate runs isolated `git init` only — no commits).
- Optional for TUI PTY: unix PTY support (Linux/macOS; not Windows).

### One command

```bash
# From repo root — agent and local developers (no CI job for this target)
make product-e2e
```

This runs write-free command tests, CLI surface/flag suites, generate e2e,
dogfood (including `TestDogfoodProductMatrix`), default TUI lifecycle tests,
scripted PTY (`-tags=tui_pty`), and integration fixtures. Budget ~20–40
minutes on a warm module cache depending on host.

### Linux (agent / primary automation)

Run `make product-e2e` (or the approximation above). Record failures with
enough context (command, exit code, first/last log lines). Prefer fixing
product bugs over weakening assertions.

### macOS (MacBook Pro — operator checklist)

This suite is **not** CI-gated. On your MBP:

1. Install/use **go1.26.5** exactly for generate paths (`GOTOOLCHAIN=local`,
   or set `FOUNDRY_GO_BIN` to that binary).
2. From a clean checkout of the same commit you want to verify:

   ```bash
   git status   # clean tree preferred
   make product-e2e
   ```

3. Note any macOS-only failures (paths, PTY, git template, case sensitivity)
   as beads; do not silently skip.

Optional artifact dir (when tests honor it):

```bash
export FOUNDRY_DOGFOOD_ARTIFACT_DIR="$PWD/docs/evidence/product-e2e-macos"
export FOUNDRY_GENERATE_E2E_ARTIFACT_DIR="$PWD/docs/evidence/product-e2e-macos-generate"
mkdir -p "$FOUNDRY_DOGFOOD_ARTIFACT_DIR" "$FOUNDRY_GENERATE_E2E_ARTIFACT_DIR"
make product-e2e
```

---

## 3. Pass criteria (product bar)

A matrix cell **passes** only when all applicable steps succeed:

1. **Command contract** — expected exit code; stable error `domain.reason` when
   specified; no unexpected writes for write-free commands.
2. **Generate** (success cells) — destination created; exclusive commit;
   refused if destination already exists.
3. **Dogfood** (success generate cells):
   - `go build` in destination (with pinned toolchain as required by generated
     `go.mod` / module policy).
   - `go test ./...` in destination.
   - **CLI:** run produced binary with at least `--help` (and version if
     present).
   - **TUI:** scripted PTY smoke (quit restore, SIGINT) — not a human session.
4. **Distribution profile cells** — expected scaffolding files present (e.g.
   `.github/workflows/release.yml`, releasing/CONTRIBUTING/SECURITY docs per
   catalog), then dogfood as above.
5. **Platform** — Linux green in automation; macOS green via MBP checklist for
   the same `make product-e2e` entrypoint.

---

## 4. Inventory and coverage map

**Coverage status legend**

| Status | Meaning |
| ------ | ------- |
| `covered` | Existing automated test is enough for this row |
| `partial` | Some automation; dogfood or flag matrix incomplete |
| `gap` | Needs new automation (`go-foundry-cli-79a.*` children) |
| `manual-macos` | Same automation; human runs on MBP |
| `env-gated` | Skips unless environment can exercise it |

**Gap analysis completed** (`go-foundry-cli-79a.2`, 2026-08-01). Status below is
authoritative for follow-up beads.

### A. Public commands

| ID | Case | Pass criteria | Automation | Status | Follow-up |
| -- | ---- | ------------- | ---------- | ------ | --------- |
| A1 | Root / command `--help` | Exit 0; public commands; plan = dry-run | `TestPublicCommandsRegistered`, `TestHelpGoldens`, writefree `help.txt` | **covered** | — |
| A2 | `validate --spec` success | Exit 0; plan_sha256 | writefree `validate_examples.txt` | **covered** | — |
| A3 | `plan --spec` text + json | Exit 0; no writes | writefree `plan.txt`; purity snapshot | **covered** | — |
| A4 | `generate --spec` success | Commit + dest | `integration/generate` matrices | **covered** (generate) | dogfood depth → C\* |
| A5 | `catalog list` text/json | cli/core/tui/distribution | writefree `catalog.txt`; CLI goldens | **covered** | — |
| A6 | `catalog show <id>` | Known units + unknown id | `catalog.txt` + `catalog_show_extended.txt` (tui, distribution, unknown) | **covered** | — |
| A7 | `version` text/json | Fields present | writefree `version.txt`; surface tests | **covered** | — |

### B. Global flags

| ID | Case | Automation | Status | Follow-up |
| -- | ---- | ---------- | ------ | --------- |
| B1–B4 | output / quiet / verbose / color contracts | writefree `flags.txt`, `flag_combinations.txt` | **covered** | — |
| B5 | `generate --quiet` | `TestMatrixQuietAndProgressNames` | **covered** | — |

**Bead 79a.3 outcome:** no new command/flag tests required unless gap analysis
regressions appear; bead may close as “verified covered” after a quick re-run
of writefree + surface suites.

### C. Generator matrix (archetype × profile) — primary gaps

| ID | Spec shape | What exists today | Gap vs product bar | Follow-up bead |
| -- | ---------- | ----------------- | ------------------ | -------------- |
| C1 | `cli` + `profiles = []` | generate matrix + `goTestGenerated`; **full** dogfood in `TestDogfoodSmokeCLI` (build, test, `--help`, `version`) | Meet bar for smoke fixture | **covered** for smoke; examples minimal-cli generate covered without binary run → optional polish in 79a.5 |
| C2 | `tui` + `profiles = []` | generate matrix + `goTestGenerated` on `minimal-tui`; PTY in `integration/tui` **behind `-tags=tui_pty`** | PTY not in default `go test`; no `make` entry; dogfood package lacks TUI smoke sibling | **79a.6** (+ include tag in **79a.8**) |
| C3 | `cli` + `distribution` | generate + artifact file asserts (`TestMatrixDistributionProfile_PublicSpec`) | **No** post-generate `go test` / build / run binary | **79a.5** |
| C4 | `tui` + `distribution` | none found | Full cell missing | **79a.5** + **79a.6** |
| C5 | private appendix B CLI/TUI | validate only (`validate_examples.txt`) | No generate/dogfood of appendix-b private fixtures | **79a.4** / **79a.5** |
| C6 | recipe-only profile reject | `TestMatrixRecipeOnlyProfile_Rejection`; writefree `profiles.txt` | — | **covered** |

### D. Spec I/O modes

| ID | Case | Automation | Status | Follow-up |
| -- | ---- | ---------- | ------ | --------- |
| D1 | `--spec path` | ubiquitous | **covered** | — |
| D2 | `--spec -` | writefree `stdin.txt`; surface stdin tests | **covered** | — |
| D3 | `--dest` override | used throughout generate e2e; flag_combinations | **covered** | — |
| D4 | name / basename rules | invalid-bad-name; writefree invalid | **covered** | — |

### E. Negatives and generate safety

| ID | Case | Automation | Status | Follow-up |
| -- | ---- | ---------- | ------ | --------- |
| E1–E3 | invalid examples | writefree `validate_invalid.txt` + examples | **covered** | — |
| E4 | dest exists | `TestMatrixPreExistingDestination` | **covered** | — |
| E5 | second generate same dest | pre-existing dest + mutation tests; double-gen is **sibling dest** equality not same-dest refuse | **covered** via E4 | — |
| E6 | usage errors | writefree `error_ids.txt`, `exit_codes.txt`, flags | **covered** | — |
| E7 | concurrent dest | `TestGenerateConcurrentDestinationConflict` | **covered** | — |
| E8 | wrong Go version | `TestGenerateWrongGoVersionProductProbe` (fake `go` via PipelineHost / FOUNDRY_GO_BIN path) | **covered** | closed `b0z` / `n0y.1` |
| E9 | mid-stage / cancel | generate matrix failure tests | **covered** | — |

**Bead 79a.7:** mostly documentation + optional E8; little new code expected.

### F. Plan ↔ generate equality

| ID | Automation | Status |
| -- | ---------- | ------ |
| F1–F2 | `TestMatrixPlanGenerateEqualitySmoke`, fixtures plan equality, cmd plan_generate | **covered** |

### G. Examples tree

| ID | Fixture | validate/plan | generate | full dogfood | Follow-up |
| -- | ------- | ------------- | -------- | ------------ | --------- |
| G1 | `minimal-cli.toml` | yes | yes (matrix) | via smoke fixture sibling | **covered** / light 79a.4 |
| G2 | `minimal-tui.toml` | yes | yes (tui matrix + go test) | PTY only with tag | **79a.6** |
| G3 | `appendix-b-private-cli.toml` | validate yes | **gap** | **gap** | **79a.4** + **79a.5** |
| G4 | `appendix-b-private-tui.toml` | validate yes | **gap** | **gap** | **79a.4** + **79a.5**/**79a.6** |
| G5 | `appendix-b-public-cli.toml` | validate yes | near: synthetic dist spec in matrix (not this file path) | dogfood **gap** | **79a.4** + **79a.5** |
| G6 | `invalid-*.toml` | fail + ids | n/a | n/a | **covered** |

### H. Platforms

| ID | Platform | Status | Follow-up |
| -- | -------- | ------ | --------- |
| H1 | Linux | **`make product-e2e` green** (~11.5m); evidence `product-e2e-linux.md` | closed |
| H2 | macOS MBP | **`make product-e2e` green** (~3.0m warm); evidence `product-e2e-macos.md` | **closed** (`go-foundry-cli-mpc`) |

### Gap summary (implementation status)

| Work | Bead | Status |
| ---- | ---- | ------ |
| Commands/flags verified covered | 79a.3 | **done** |
| Examples valid/invalid + appendix generate dogfood | 79a.4 | **done** (`TestDogfoodProductMatrix`, `TestDogfoodExamplesInvalidMatrix`) |
| Distribution CLI/TUI full dogfood | 79a.5 | **done** (same matrix) |
| Scripted PTY bare + distribution TUI | 79a.6 | **done** (`TestTUI_PTY_*`, including `TestTUI_PTY_DistributionQuitRestore`) |
| Safety/negatives | 79a.7 + n0y.1 | **done** (E1–E9; E8 automated product probe) |
| `make product-e2e` | 79a.8 | **done** |
| Linux full execute | 79a.9 | **done** — `make product-e2e` PASS ~11.5m; evidence `docs/evidence/product-e2e-linux.md` |
| macOS MBP execute | 79a.10 + mpc | **done** — `make product-e2e` PASS ~3.0m on MBP; evidence `docs/evidence/product-e2e-macos.md` |

---

## 5. Work breakdown (beads)

### 5.1 Implementation epic — **closed** (`go-foundry-cli-79a`)

Delivered the plan doc, gap fill, `make product-e2e`, product matrix dogfood,
TUI PTY, and Linux green run.

| Bead | Title | Role | Status |
| ---- | ----- | ---- | ------ |
| `go-foundry-cli-79a` | Product E2E plan + durable suite | Epic | closed |
| `go-foundry-cli-79a.1` | Write product E2E plan doc | This file | closed |
| `go-foundry-cli-79a.2` | Gap analysis | Finalize §4 status column | closed |
| `go-foundry-cli-79a.3` | Commands + global flags gaps | A/B | closed |
| `go-foundry-cli-79a.4` | Examples matrix | G rows | closed |
| `go-foundry-cli-79a.5` | Generate matrix full dogfood | C1–C5 | closed |
| `go-foundry-cli-79a.6` | TUI scripted PTY dogfood | C2/C4 PTY | closed |
| `go-foundry-cli-79a.7` | Safety / negatives | E rows | closed |
| `go-foundry-cli-79a.8` | `make product-e2e` | Entrypoint (no CI) | closed |
| `go-foundry-cli-79a.9` | Execute on Linux | Green + evidence | closed |
| `go-foundry-cli-79a.10` | macOS MBP checklist (instructions) | Operator handoff text | closed |
| `go-foundry-cli-79a.11` | Consistency closeout | Doc ↔ suite | closed |

### 5.2 Residual / operator epic — **closed** (`go-foundry-cli-q8b`)

Plan items that needed human or optional env work after 79a — **all closed**.

| Bead | Title | Role | Status |
| ---- | ----- | ---- | ------ |
| `go-foundry-cli-q8b` | Product E2E residual / operator verification | Epic | **closed** |
| `go-foundry-cli-mpc` | Verify `make product-e2e` on macOS (MBP) | H2 | **closed** — evidence `docs/evidence/product-e2e-macos.md` |
| `go-foundry-cli-b0z` | Wrong-Go (E8) product probe | E8 | **closed** — `TestGenerateWrongGoVersionProductProbe` |
| `go-foundry-cli-tuo` | Link residual beads into this plan §7 | Doc sync | closed |

### 5.3 Fine-grained inventory checklist — **closed** (`go-foundry-cli-874`)

One bead per plan §4 inventory ID (A1…H2). All sections closed after Linux +
macOS `make product-e2e` green.

| Section bead | Plan IDs | Leaf beads | Status |
| ------------ | -------- | ---------- | ------ |
| `874.2` | A1–A7 | `874.2.1`…`874.2.7` | closed (all covered) |
| `874.3` | B1–B5 | `874.3.1`…`874.3.5` | closed (all covered) |
| `874.4` | C1–C6 | `874.4.1`…`874.4.6` | closed (all covered) |
| `874.5` | D1–D4 | `874.5.1`…`874.5.4` | closed (all covered) |
| `874.6` | E1–E9 | `874.6.1`…`874.6.9` | closed (E8 via `TestGenerateWrongGoVersionProductProbe`) |
| `874.7` | F1–F2 | `874.7.1`…`874.7.2` | closed (all covered) |
| `874.8` | G1–G6 | `874.8.1`…`874.8.6` | closed (all covered) |
| `874.9` | H1–H2 | `874.9.1`, `874.9.2` | **closed** (H2 via mpc) |

Browse: `bd children go-foundry-cli-874`

---

## 6. Implementation principles

1. **Extend first** — prefer `integration/generate`, `integration/dogfood`,
   `integration/tui`, `cmd/foundry` writefree testscripts, `internal/cli`
   goldens over a parallel framework.
2. **Black-box where it matters** — product claims should exercise the
   `foundry` binary or public CLI entry (`cli.NewRoot`) the way users do.
3. **Custody-safe temps** — generate into temp parents with restrictive modes;
   never write examples’ `./destination` into the repo tree.
4. **Step logger** — multi-step dogfood uses `internal/testutil` step logging.
5. **No CI changes** in this epic — local/`make` only.
6. **Pin Go for generate** — tests that shell to real `go` must honor catalog
   pin discovery (`FOUNDRY_GO_BIN` / foundry’s own resolution).

---

## 7. Residual / manual

| Item | Who | Bead | Notes |
| ---- | --- | ---- | ----- |
| macOS full `make product-e2e` | MBP operator | **`go-foundry-cli-mpc` closed** | PASS ~3.0m; evidence `docs/evidence/product-e2e-macos.md` |
| Wrong-Go preflight (E8) | Automated | **`go-foundry-cli-b0z` closed** | `integration/generate/wrong_go_test.go` injects fake go binary |
| “Does the TUI feel good?” | Optional human | — | **Not** a gate; PTY automation is the bar; no bead |

---

## 8. Covered elsewhere (do not re-test as product E2E)

| Area | Where |
| ---- | ----- |
| Unit / golden / purity | `go test ./...`, [testing.md](testing.md) |
| REQ traceability | `internal/archtest`, `docs/evidence/req-traceability.md` |
| Hostile FS / sentinel | `make hostile`, `make sentinel`, `integration/hostile/*` |
| Perf | `make perf`, `scripts/perf/` |
| Section 58 red-lines | `internal/archtest` |
| Profile admission process doc | `docs/dev/profile-admission.md` |
| Recipes (not profiles) | `docs/recipes/*` |

---

## 9. Changelog

| Date | Change |
| ---- | ------ |
| 2026-08-01 | Initial plan (`go-foundry-cli-79a.1`); no CI; macOS = MBP checklist |
| 2026-08-01 | Gap analysis (`go-foundry-cli-79a.2`): inventory statuses + primary gaps (distribution dogfood, tui+dist, appendix generate, `tui_pty` make wiring) |
| 2026-08-01 | Automation: `TestDogfoodProductMatrix`, invalid examples matrix, dist TUI PTY, `make product-e2e`; fixed `toolrun` Linux `sysconfARGMAX` build break |
| 2026-08-01 | Residual beads epic `go-foundry-cli-q8b` (macOS verify `mpc`, E8 optional `b0z`); §5/§7 updated |
| 2026-08-01 | Fine-grained inventory checklist epic `go-foundry-cli-874` (A1–H2 leaves); covered closed; E8/H2 open |
| 2026-08-01 | E8 automated (`TestGenerateWrongGoVersionProductProbe`); residual operator work is H2/`mpc` only; post-hardening `n0y` follow-through |
| 2026-08-01 | macOS MBP `make product-e2e` PASS ~3.0m; closed `mpc` + `874` + `q8b`; evidence `product-e2e-macos.md` |
|
