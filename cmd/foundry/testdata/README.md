# testscript fixtures (`cmd/foundry/testdata`)

End-to-end `testscript` suites drive the real `foundry` binary (REQ-218 /
Section 45.1).

## Write-free e2e (`writefree/`) — P1.8.a / `5an.1`

| Script                  | Covers                                            |
| ----------------------- | ------------------------------------------------- |
| `version.txt`           | version text/JSON fields, quiet                   |
| `catalog.txt`           | list/show goldens-shape, unknown id               |
| `validate_examples.txt` | examples/ + Appendix B fixtures                   |
| `validate_invalid.txt`  | exact error ids (examples + specs/invalid)        |
| `plan.txt`              | dry-run plan, verify modes, plan_sha256 stability |
| `flags.txt`             | quiet/verbose/color/JSON; banned tokens           |
| `stdin.txt`             | `--spec -` validate/plan                          |
| `profiles.txt`          | configuration / local-persistence / typos         |
| `generate_hardstop.txt` | Phase-1 refusal, zero FS mutation                 |
| `purity_snapshot.txt`   | no destination files from write-free cmds         |

Run:

```bash
go test ./cmd/foundry -run TestWriteFree -count=1
go test ./cmd/foundry -run 'TestWriteFree' -count=1 -v   # step logs
```

Companion Go e2e (step logger + optional Linux strace purity):

```bash
go test ./cmd/foundry -run 'TestWriteFreeVerbose|TestWriteFreePurity' -count=1 -v
```

## Spec fixtures (`specs/`)

Project Specification fixtures for write-free e2e were **exported by
P1.2.c (`go-foundry-cli-d0i`)**:

- `specs/valid/` — Appendix B + minimal examples
- `specs/invalid/` — one fixture per applicable Appendix D `spec.*` id
  (plus mirrors of `examples/invalid-*.toml`)

See [`specs/README.md`](specs/README.md). Product copy-paste surface remains
under [`examples/`](../../../examples/).

## Plan/generate equality (`plan_generate/`) — P2.5.f / `j8h.4`

Section 13.3 defect class: the plan a user inspects is byte-equal (via
`plan_sha256`) to the plan `generate` executes under the same flags.

| Script         | Covers                                                                   |
| -------------- | ------------------------------------------------------------------------ |
| `equality.txt` | plan JSON vs generate `plan_sha256`; default/strict; `--dest`; smoke-cli |

Run:

```bash
go test ./cmd/foundry -run TestPlanGenerateEquality -count=1
```

Package + CLI property suites (write-free, fake lifecycle):

```bash
go test ./internal/cli -run TestPlanGenerateByteEquality -count=2
go test ./integration/fixtures -run TestPlanGeneratePackageByteEquality -count=2
```

## Generate e2e (`generate/`) — P2.5.d / `j8h.2`

Process-boundary scripts for real `foundry generate`. Full matrix (cancel,
stream injection, REQ-133, go test of generated tree, failure artifacts) lives
in [`integration/generate/`](../../../integration/generate/).

| Script                  | Covers                                                    |
| ----------------------- | --------------------------------------------------------- |
| `success_default.txt`   | clean commit, default verify, network disclosure          |
| `success_strict.txt`    | strict verify + govulncheck disclosure                    |
| `double_generation.txt` | plan_sha256 equality across two generates (REQ-010 smoke) |
| `preexisting_dest.txt`  | exit 2 `fs.destination_exists`                            |
| `quiet_progress.txt`    | `--quiet` + Section 29.2 progress names                   |

Run:

```bash
go test ./cmd/foundry -run TestGenerateE2E -count=1
go test -count=1 -timeout 15m ./integration/generate/
```

CI: `ubuntu-latest-generate-e2e` / `macos-latest-generate-e2e` with
`FOUNDRY_GENERATE_E2E_ARTIFACT_DIR` failure dumps (stage+cause in filenames).

Phase 3 — cross-platform expansion (REQ-218 full matrix).

`scaffold.txt` is a historical placeholder and is not executed.

## Custom testscript commands

All suites registered in `cmd/foundry/script_test.go` share the same command
set (`writeFreeCmds`). Each command logs a `STEPLOG` line so failures name the
check without a debugger.

| Command                 | Args                                    | Purpose                                                                                                                |
| ----------------------- | --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `dump_digest`           | `<file\|stdout\|stderr>`                | Print SHA-256 + byte length of a file or the last `exec` output.                                                       |
| `assert_plan_sha256_eq` | `<a.json> <b.json>`                     | Assert two plan JSON outputs have equal `plan_sha256`.                                                                 |
| `assert_plan_sha256_ne` | `<a.json> <b.json>`                     | Assert two plan JSON outputs have different `plan_sha256` (e.g. default vs strict).                                    |
| `assert_error_id`       | `[!] <file\|stdout\|stderr> <error_id>` | Assert the named output contains (or with `!`, does not contain) an error id.                                          |
| `list_tree`             | `<dir>`                                 | Log sorted relative entries under a directory.                                                                         |
| `assert_tree_unchanged` | `<dir> <snapshot_file>`                 | Purity check: directory contents must match the saved snapshot exactly.                                                |
| `assert_exit_code`      | `<want_code> foundry <args...>`         | Run the real `foundry` process and assert an exact numeric exit code.                                                  |
| `write_size`            | `<path> <bytes>`                        | Write a file of exactly N arbitrary bytes (synthesizes `spec.too_large` fixtures without checking in multi-MiB files). |
| `snapshot_tree`         | `<dir> <out_file>`                      | Write a sorted relative listing of a directory for later `assert_tree_unchanged`.                                      |

## Fixture policy

Three fixture sources are used across the suites. Keep their roles distinct:

1. **`examples/`** — user-facing Project Spec copy-paste surface. These specs
   must stay valid for `foundry validate` and are referenced by `README.md`.
   The testscript harness copies them into `$WORK` so scripts can use relative
   paths and host-independent golden output.

2. **`cmd/foundry/testdata/specs/valid/`** — write-free e2e fixtures. Mirrors
   `examples/` (Appendix B shapes) and may add narrow edge cases. Keep in sync
   with `examples/` invalid shapes through `specs/invalid/`.

3. **`cmd/foundry/testdata/specs/invalid/`** — one fixture per applicable
   `spec.*` error id from Appendix D, plus mirrors of `examples/invalid-*.toml`.
   Use these when an exact `assert_error_id` check is required.

Prefer generated or copied fixtures over inline bytes. Use `write_size` when a
script needs a file larger than the 1 MiB spec cap, and `snapshot_tree` /
`assert_tree_unchanged` to prove write-free commands produce no destination
files. Avoid hard-coding host paths; use `$EXAMPLES`, `$SPECS`, `$SENTINEL`,
`$REPO`, `$GOOS`, and `$GOARCH` set by the harness.

## Host setup rationale (`generateHostSetup`)

`testscript` defaults to `HOME=/no-home`, which empties module caches and
breaks `go mod tidy` / `govulncheck` during strict verify. The setup restores a
real `HOME`, `GOMODCACHE`, and `GOCACHE` from the host so generate e2e can run
real go subprocesses.

For custody safety (fsx Section 31.3), generated destinations must sit under a
sticky `/tmp` parent with mode `0700`. `$WORK` is typically `0775`, so
`generateHostSetup` creates a private temp parent (`foundry-gen-*` or
`foundry-eq-*`) and exposes it as `$GEN_ROOT`. Scripts must place `--dest`
values under `$GEN_ROOT`, not directly in `$WORK`.
