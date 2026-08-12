# Foundry integration fixtures (REQ-066 / REQ-010)

Foundry-owned fixtures for plan/tree goldens, double-generation determinism,
and **extension-path** proofs. These live in the Foundry repository only —
they are **never** generated into user projects.

**Authority:** [SPEC-FOUNDRY-002](../../docs/02-definitive-foundry-specification-revised-fable-5.md)
(Sections 17.2, 32.1, 42.1, REQ-010, REQ-066).

## Purpose

| Fixture | Role |
| ------- | ---- |
| [`foundry-smoke-cli/`](foundry-smoke-cli/) | Disposable CLI dogfood shell (`profiles = []`); plan + tree goldens; double-generation byte equality within the Section 32.1 envelope |
| [`extension-cli-subcommand/`](extension-cli-subcommand/) | Post-generate extension path: add a responsibility package + subcommand **by hand** after Foundry generate; proves growth recipe, not catalog content |

## Non-goals

- **Not** user-facing product samples (see [`examples/`](../../examples/) for copy-paste specs).
- **Not** catalog content — no greet/demo domain ships in generated CLI shells (FND-014).
- **Not** a profile or plugin surface — extension is manual overlay after generate.
- **Not** dogfood repositories themselves — dogfood (`repo-map`) evolves independently after smoke is green (Section 52).
- **Not** full hostile/cancel/stream matrices — those live under `integration/hostile/` and `j8h.2` generate e2e.

## How tests use these fixtures

```bash
# Pure plan/tree goldens + double-generation digests (no host tools required)
go test -count=1 ./integration/fixtures -run 'TestSmoke|TestDouble|TestAbsence'

# Real generate + extension overlay compile/test (requires pinned Go toolchain)
go test -count=1 ./integration/fixtures -run 'TestGenerate|TestExtension'

# Update goldens (never in CI)
UPDATE_GOLDEN=1 go test ./integration/fixtures -run TestSmokeCLIPlanGolden
UPDATE_GOLDEN=1 go test ./integration/fixtures -run TestSmokeCLITreeGolden
```

Step logging uses `internal/testutil` and records `plan_sha256` plus content digests
in golden meta. On tree mismatch tests dump the path list.

## Layout

```text
integration/fixtures/
  README.md                         # this file
  foundry-smoke-cli/
    foundry.toml                    # Project Spec for smoke-cli
  extension-cli-subcommand/
    README.md                       # extension purpose + non-goals
    testdata/overlay/               # files merged into a generated tree
      internal/ping/…               # domain package (no Cobra)
      internal/cli/ping.go…         # subcommand constructor
  testdata/
    foundry_smoke_cli_plan.golden
    foundry_smoke_cli_tree.golden
    foundry_smoke_cli_meta.golden
  *_test.go                         # goldens, double-gen, generate, extension
```

## Related beads

- **go-foundry-cli-wly** (this package) — fixtures + goldens + double-generation
- **go-foundry-cli-j8h.2** — full generate e2e matrix consuming smoke-cli (`integration/generate/` + `cmd/foundry/testdata/generate/`)
- **go-foundry-cli-vu8** — dogfood: smoke-cli then real `repo-map` (`integration/dogfood/`, `scripts/dogfood-cli.sh`, `dogfood/repo-map/`, evidence under `docs/evidence/dogfood-cli-*`)
