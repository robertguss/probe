# REQ → test traceability matrix (DoD #2)

Living matrix for all **125** active normative requirements in
[SPEC-FOUNDRY-002 §53](../02-definitive-foundry-specification-revised-fable-5.md).

**Authority:** the specification wins on any conflict. This matrix is the
Section 59 Definition-of-Done #2 artifact, not a second requirements source.

**Owning bead:** `go-foundry-cli-dlv.1`  
**Seeded:** 2026-07-30 (after P1.0 scaffold `zhm`)  
**Last update:** 2026-08-01  
**Machine-readable twin:** [`req-traceability.json`](req-traceability.json)

## Lifecycle protocol

1. **Seed early** (this file): every `REQ-###` listed with owning bead(s) and planned test layer(s); status `planned` (or `implemented` when tests already exist).
2. **Update at each package completion:** mark rows `implemented` and replace planned paths with concrete `*_test.go` / testscript names.
3. **Phase exit reviews hard-block** on `dlv.1` (`0z4`, `5pr`, `66w`, `7fm`): exit evidence must cite the phase slice with `verified` rows for that phase's REQs.
4. **Never close `7fm`** while any row remains `planned` or contains `TBD` cells — DoD mode of the checker enforces this.
5. Status values only: `planned` | `implemented` | `verified`. **Status must never be the placeholder string `TBD` (use `planned` until work lands).**

## Checker

```bash
# Completeness: every Section 53 REQ appears exactly once (always on in CI via go test)
go test -count=1 ./internal/archtest/ -run 'TestReqTraceability'

# Phase-exit hard gate: all REQs for that phase must be verified
FOUNDRY_MATRIX_PHASE_EXIT=P1 go test -count=1 ./internal/archtest/ -run 'TestReqTraceabilityPhaseExit'
FOUNDRY_MATRIX_PHASE_EXIT=P2 go test -count=1 ./internal/archtest/ -run 'TestReqTraceabilityPhaseExit'

# Definition of Done (7fm): zero planned or placeholder rows
FOUNDRY_MATRIX_PHASE_EXIT=DoD go test -count=1 ./internal/archtest/ -run 'TestReqTraceabilityPhaseExit'
```

On failure the checker prints **sorted** missing and extra REQ ids and exits non-zero.
`verified` rows without an existing test path warn until Phase 2, then fail when
`FOUNDRY_MATRIX_STRICT_PATHS=1` (default on for phase-exit invocations).

## Status legend

| Status | Meaning |
| ------ | ------- |
| `planned` | Owning bead + test layer assigned; implementation not yet landed |
| `implemented` | Code + primary tests exist; not yet phase-exit verified |
| `verified` | Phase-exit review confirmed tests cover the REQ for its phase |

## Counts (living)

- Total REQs: **125**
- By status: `planned`=0, `implemented`=0, `verified`=125
- By phase: `S6`=2, `P1`=53, `P2`=49, `P3`=14, `P4`=7


## Matrix

| REQ | Phase | Status | Owning beads | Tests | Last update |
| --- | --- | --- | --- | --- | --- |
| REQ-001 | P1 | `verified` | `go-foundry-cli-5an`, `go-foundry-cli-j8h`, `go-foundry-cli-vu8` | `cmd/foundry/writefree_e2e_test.go`, `cmd/foundry/testdata/writefree/`, `examples/` | 2026-07-31 |
| REQ-002 | P1 | `verified` | `go-foundry-cli-zhm`, `go-foundry-cli-j1m`, `go-foundry-cli-wkz` | `go.mod`, `catalog/versions.toml`, `integration/hostile/e5/lock_test.go` | 2026-07-31 |
| REQ-003 | P2 | `verified` | `go-foundry-cli-2fe`, `go-foundry-cli-qb0`, `go-foundry-cli-j8h.2` | `internal/fsx/destination_test.go`, `cmd/foundry/testdata/generate/preexisting_dest.txt`, `integration/generate/matrix_failure_test.go` | 2026-07-31 |
| REQ-004 | P2 | `verified` | `go-foundry-cli-ig3`, `go-foundry-cli-x7b`, `go-foundry-cli-cic` | `integration/hostile/e3/e3_test.go`, `integration/hostile/e3/preflight_test.go`, `integration/hostile/e3/knownrace/race_test.go`, `go.mod` | 2026-07-31 |
| REQ-005 | P1 | `verified` | `go-foundry-cli-s45`, `go-foundry-cli-wkz`, `go-foundry-cli-4a6`, `go-foundry-cli-jr7` | `internal/resolve/resolve_test.go`, `internal/catalog/catalog_test.go`, `internal/plan/golden_test.go` | 2026-07-31 |
| REQ-006 | P2 | `verified` | `go-foundry-cli-4a6`, `go-foundry-cli-wly`, `go-foundry-cli-j8h.2` | `internal/render/gomod_test.go`, `integration/fixtures/smoke_cli_test.go`, `integration/dogfood/smoke_test.go`, `cmd/foundry/testdata/generate/success_default.txt` | 2026-07-31 |
| REQ-007 | P1 | `verified` | `go-foundry-cli-80g`, `go-foundry-cli-5an.1`, `go-foundry-cli-8jp` | `internal/cli/surface_test.go`, `cmd/foundry/testdata/writefree/stdin.txt`, `cmd/foundry/writefree_e2e_test.go` | 2026-07-31 |
| REQ-008 | P1 | `verified` | `go-foundry-cli-4hi`, `go-foundry-cli-5an.1`, `go-foundry-cli-2y7` | `internal/archtest/redline_test.go`, `internal/archtest/quality_test.go`, `cmd/foundry/testdata/writefree/purity_snapshot.txt` | 2026-07-31 |
| REQ-009 | P3 | `verified` | `go-foundry-cli-a77`, `go-foundry-cli-k0o` | `catalog/core/files/AGENTS.md.tmpl`, `internal/catalog/core_tree_test.go`, `docs/evidence/three-agent-acceptance.md` | 2026-07-31 |
| REQ-010 | P2 | `verified` | `go-foundry-cli-j8h.4`, `go-foundry-cli-wly`, `go-foundry-cli-sui` | `internal/cli/plan_generate_equality_test.go`, `integration/fixtures/plan_generate_equality_test.go`, `integration/generate/matrix_success_test.go`, `internal/verify/verify_test.go`, `cmd/foundry/testdata/generate/double_generation.txt` | 2026-07-31 |
| REQ-011 | P1 | `verified` | `go-foundry-cli-wkz`, `go-foundry-cli-s45`, `go-foundry-cli-w4n`, `go-foundry-cli-chf` | `internal/catalog/catalog_test.go`, `internal/resolve/resolve_test.go`, `cmd/foundry/testdata/writefree/catalog.txt` | 2026-07-31 |
| REQ-012 | P1 | `verified` | `go-foundry-cli-2y7`, `go-foundry-cli-4hi`, `go-foundry-cli-c6b` | `internal/archtest/redline_test.go`, `docs/evidence/section-58-redlines.md`, `internal/cli/surface_test.go` | 2026-07-31 |
| REQ-030 | P1 | `verified` | `go-foundry-cli-c6b`, `go-foundry-cli-80g`, `go-foundry-cli-5an.1`, `go-foundry-cli-2y7` | `internal/cli/surface_test.go`, `cmd/foundry/testdata/writefree/`, `internal/archtest/redline_test.go` | 2026-07-31 |
| REQ-031 | P1 | `verified` | `go-foundry-cli-4hi`, `go-foundry-cli-5an.1`, `go-foundry-cli-80g` | `internal/cli/surface_test.go`, `cmd/foundry/testdata/writefree/`, `internal/archtest/quality_test.go`, `cmd/foundry/writefree_e2e_test.go` | 2026-07-31 |
| REQ-032 | P1 | `verified` | `go-foundry-cli-6wj`, `go-foundry-cli-di6`, `go-foundry-cli-9a7`, `go-foundry-cli-d0i` | `internal/spec/suite_test.go`, `internal/cli/pipeline_test.go`, `internal/plan/plan_test.go`, `cmd/foundry/testdata/writefree/validate_examples.txt` | 2026-07-31 |
| REQ-033 | P1 | `verified` | `go-foundry-cli-6wj`, `go-foundry-cli-9a7`, `go-foundry-cli-j8h.4`, `go-foundry-cli-5an.1` | `internal/plan/golden_test.go`, `internal/cli/pipeline_test.go`, `cmd/foundry/testdata/writefree/plan.txt` | 2026-07-31 |
| REQ-034 | P2 | `verified` | `go-foundry-cli-dd1`, `go-foundry-cli-j8h.3`, `go-foundry-cli-j8h.2`, `go-foundry-cli-ybi` | `internal/generate/network_test.go`, `internal/generate/machine_test.go`, `internal/cli/generate_test.go`, `integration/generate/matrix_success_test.go`, `cmd/foundry/testdata/generate/success_default.txt` | 2026-07-31 |
| REQ-035 | P1 | `verified` | `go-foundry-cli-11l`, `go-foundry-cli-wkz`, `go-foundry-cli-5an.1` | `internal/cli/catalog_test.go`, `internal/catalog/listshow_test.go`, `cmd/foundry/testdata/writefree/catalog.txt`, `internal/version/version_test.go` | 2026-07-31 |
| REQ-036 | P2 | `verified` | `go-foundry-cli-dd1`, `go-foundry-cli-nx1`, `go-foundry-cli-qb0`, `go-foundry-cli-j8h.2` | `internal/generate/machine_test.go`, `internal/generate/state_test.go`, `internal/fsx/preserve_test.go`, `integration/generate/matrix_failure_test.go`, `internal/cli/generate_test.go` | 2026-07-31 |
| REQ-037 | P1 | `verified` | `go-foundry-cli-41p`, `go-foundry-cli-80g`, `go-foundry-cli-5an.1` | `internal/report/encoder_test.go`, `internal/report/golden_test.go`, `cmd/foundry/testdata/writefree/flags.txt` | 2026-07-31 |
| REQ-038 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-i8b`, `go-foundry-cli-d0i` | `internal/spec/decode_test.go`, `internal/spec/suite_test.go`, `internal/spec/fuzz_test.go` | 2026-07-31 |
| REQ-039 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-d0i` | `internal/spec/suite_test.go`, `internal/spec/decode_test.go` | 2026-07-31 |
| REQ-040 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-i8b`, `go-foundry-cli-d0i` | `internal/spec/decode_test.go`, `internal/spec/suite_test.go` | 2026-07-31 |
| REQ-041 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-5yn` | `internal/spec/validate_test.go`, `internal/spec/suite_test.go`, `docs/02-definitive-foundry-specification-revised-fable-5.md` | 2026-07-31 |
| REQ-042 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-5yn`, `go-foundry-cli-d0i` | `internal/spec/validate_test.go`, `internal/spec/suite_test.go` | 2026-07-31 |
| REQ-043 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-5yn`, `go-foundry-cli-d0i` | `internal/spec/validate_test.go`, `internal/spec/suite_test.go` | 2026-07-31 |
| REQ-044 | P2 | `verified` | `go-foundry-cli-2fe`, `go-foundry-cli-qb0`, `go-foundry-cli-qd8` | `internal/fsx/destination_test.go`, `internal/fsx/parent_test.go`, `internal/fsx/transaction_test.go`, `integration/hostile/e1/e1_test.go`, `integration/hostile/fsx/suite_test.go` | 2026-07-31 |
| REQ-045 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-s45`, `go-foundry-cli-w4n` | `internal/spec/validate_test.go`, `internal/resolve/resolve_test.go`, `internal/catalog/recipe_test.go`, `cmd/foundry/testdata/writefree/profiles.txt` | 2026-07-31 |
| REQ-046 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-5yn`, `go-foundry-cli-8jp` | `internal/spec/validate_test.go`, `internal/spec/suite_test.go`, `internal/plan/schema_test.go` | 2026-07-31 |
| REQ-060 | P2 | `verified` | `go-foundry-cli-a77`, `go-foundry-cli-4a6`, `go-foundry-cli-wly` | `internal/catalog/core_tree_test.go`, `internal/catalog/cli_tree_test.go`, `integration/fixtures/smoke_cli_test.go`, `integration/fixtures/testdata/foundry_smoke_cli_tree.golden` | 2026-07-31 |
| REQ-061 | P2 | `verified` | `go-foundry-cli-a77`, `go-foundry-cli-4a6`, `go-foundry-cli-jr7` | `internal/catalog/core_tree_test.go`, `internal/catalog/cli_tree_test.go`, `integration/fixtures/smoke_cli_test.go`, `catalog/core/` | 2026-07-31 |
| REQ-062 | P2 | `verified` | `go-foundry-cli-a77`, `go-foundry-cli-cic` | `internal/catalog/core_tree_test.go`, `internal/catalog/cli_tree_test.go`, `catalog/core/` | 2026-07-31 |
| REQ-063 | P3 | `verified` | `go-foundry-cli-a77`, `go-foundry-cli-cic` | `internal/archtest/foundry_ci_test.go`, `docs/evidence/P3-ci-reduced-matrix.md`, `catalog/core/files/docs/testing.md.tmpl` | 2026-07-31 |
| REQ-064 | P2 | `verified` | `go-foundry-cli-4a6`, `go-foundry-cli-2gi`, `go-foundry-cli-wly` | `internal/catalog/cli_tree_test.go`, `integration/fixtures/smoke_cli_test.go`, `integration/dogfood/smoke_test.go`, `integration/fixtures/foundry-smoke-cli/foundry.toml` | 2026-07-31 |
| REQ-065 | P2 | `verified` | `go-foundry-cli-4a6`, `go-foundry-cli-2gi` | `internal/catalog/cli_tree_test.go`, `internal/catalog/foundrydev_surface_test.go`, `integration/fixtures/smoke_cli_test.go` | 2026-07-31 |
| REQ-066 | P2 | `verified` | `go-foundry-cli-4a6`, `go-foundry-cli-wly` | `integration/fixtures/smoke_cli_test.go`, `integration/fixtures/extension_test.go`, `integration/dogfood/smoke_test.go` | 2026-07-31 |
| REQ-067 | P2 | `verified` | `go-foundry-cli-4a6`, `go-foundry-cli-a77` | `internal/catalog/cli_tree_test.go`, `internal/catalog/core_tree_test.go`, `integration/fixtures/smoke_cli_test.go`, `docs/evidence/dogfood-cli-smoke.md` | 2026-07-31 |
| REQ-068 | P3 | `verified` | `go-foundry-cli-jr7`, `go-foundry-cli-rrs`, `go-foundry-cli-afh` | `internal/catalog/tui_tree_test.go`, `catalog/archetypes/tui/manifest.toml`, `cmd/foundry/testdata/generate/success_tui_default.txt`, `integration/fixtures/smoke_tui_test.go` | 2026-07-31 |
| REQ-069 | P3 | `verified` | `go-foundry-cli-jr7`, `go-foundry-cli-afh` | `internal/catalog/tui_tree_test.go`, `integration/tui/view_matrix_test.go`, `catalog/archetypes/tui/files/internal/tui/update.go.static` | 2026-07-31 |
| REQ-070 | P3 | `verified` | `go-foundry-cli-jr7`, `go-foundry-cli-1yq`, `go-foundry-cli-e0x` | `integration/tui/lifecycle_matrix_test.go`, `integration/hostile/e4/lifecycle_test.go`, `catalog/archetypes/tui/files/internal/tui/lifecycle_test.go.static`, `docs/evidence/E4-bubbletea-lifecycle.md` | 2026-07-31 |
| REQ-071 | P3 | `verified` | `go-foundry-cli-1yq`, `go-foundry-cli-jr7` | `integration/tui/debuglog_matrix_test.go`, `integration/tui/lifecycle_matrix_test.go`, `docs/evidence/dogfood-tui-smoke.md` | 2026-07-31 |
| REQ-072 | P3 | `verified` | `go-foundry-cli-1yq`, `go-foundry-cli-afh`, `go-foundry-cli-jr7` | `integration/tui/lifecycle_matrix_test.go`, `integration/tui/view_matrix_test.go`, `integration/tui/pty_test.go`, `integration/tui/debuglog_matrix_test.go`, `integration/fixtures/smoke_tui_test.go` | 2026-07-31 |
| REQ-073 | P1 | `verified` | `go-foundry-cli-w4n`, `go-foundry-cli-wkz`, `go-foundry-cli-s45`, `go-foundry-cli-6hp` | `internal/catalog/recipe_test.go`, `internal/resolve/resolve_test.go`, `cmd/foundry/testdata/writefree/profiles.txt`, `docs/recipes/configuration.md`, `internal/archtest/recipes_test.go` | 2026-07-31 |
| REQ-074 | P1 | `verified` | `go-foundry-cli-6hp`, `go-foundry-cli-w4n` | `docs/recipes/configuration.md`, `internal/archtest/recipes_test.go`, `internal/catalog/recipe_test.go`, `internal/catalog/recipe.go` | 2026-07-31 |
| REQ-075 | P1 | `verified` | `go-foundry-cli-w4n`, `go-foundry-cli-wkz`, `go-foundry-cli-s45`, `go-foundry-cli-6hp` | `docs/recipes/local-persistence.md`, `internal/archtest/recipes_test.go`, `internal/catalog/recipe_test.go`, `internal/resolve/resolve_test.go`, `cmd/foundry/testdata/writefree/profiles.txt` | 2026-07-31 |
| REQ-076 | P4 | `verified` | `go-foundry-cli-chf`, `go-foundry-cli-1wl`, `go-foundry-cli-7fm` | `internal/catalog/distribution_tree_test.go`, `docs/evidence/P4-distribution-profile.md`, `docs/evidence/self-distribution.md`, `internal/cli/catalog_test.go` | 2026-07-31 |
| REQ-077 | P1 | `verified` | `go-foundry-cli-s45`, `go-foundry-cli-wkz`, `go-foundry-cli-2y7` | `internal/resolve/resolve_test.go`, `internal/resolve/purity_test.go`, `internal/catalog/manifest_test.go`, `internal/archtest/redline_test.go` | 2026-07-31 |
| REQ-078 | P4 | `verified` | `go-foundry-cli-tpa`, `go-foundry-cli-vu8`, `go-foundry-cli-bjt` | `docs/dev/profile-admission.md`, `internal/archtest/profile_admission_test.go`, `docs/evidence/P4-exit-review.md` | 2026-07-31 |
| REQ-090 | P1 | `verified` | `go-foundry-cli-wkz`, `go-foundry-cli-mpu`, `go-foundry-cli-j1m` | `internal/catalog/catalog_test.go`, `catalog/versions.toml`, `integration/hostile/e5/lock_test.go` | 2026-07-31 |
| REQ-091 | P1 | `verified` | `go-foundry-cli-wkz`, `go-foundry-cli-mpu` | `internal/catalog/foundrydev_surface_test.go`, `internal/catalog/load_dir_test.go` | 2026-07-31 |
| REQ-092 | P1 | `verified` | `go-foundry-cli-95a`, `go-foundry-cli-wkz` | `internal/catalog/manifest_test.go`, `internal/catalog/catalog_test.go` | 2026-07-31 |
| REQ-093 | P1 | `verified` | `go-foundry-cli-s45`, `go-foundry-cli-9a7` | `internal/resolve/collision_test.go`, `internal/plan/plan_test.go` | 2026-07-31 |
| REQ-094 | P1 | `verified` | `go-foundry-cli-95a`, `go-foundry-cli-wkz`, `go-foundry-cli-qb0` | `internal/catalog/manifest_test.go`, `internal/render/path.go`, `internal/render/dispatch_test.go` | 2026-07-31 |
| REQ-095 | P2 | `verified` | `go-foundry-cli-5su`, `go-foundry-cli-3ms` | `internal/render/gomod_test.go`, `internal/render/gomod_fuzz_test.go`, `internal/render/dispatch_test.go` | 2026-07-31 |
| REQ-096 | P2 | `verified` | `go-foundry-cli-3ms`, `go-foundry-cli-l81`, `go-foundry-cli-qm0`, `go-foundry-cli-5su` | `internal/render/dispatch_test.go`, `internal/render/static_test.go`, `internal/render/template_test.go`, `internal/render/gomod_test.go` | 2026-07-31 |
| REQ-097 | P2 | `verified` | `go-foundry-cli-qm0`, `go-foundry-cli-3ms` | `internal/render/template_test.go`, `internal/render/template_fuzz_test.go`, `internal/render/dispatch_test.go` | 2026-07-31 |
| REQ-098 | P1 | `verified` | `go-foundry-cli-s45`, `go-foundry-cli-4hi` | `internal/resolve/purity_test.go`, `internal/resolve/resolve_test.go`, `internal/archtest/quality_test.go` | 2026-07-31 |
| REQ-099 | P1 | `verified` | `go-foundry-cli-s45` | `internal/resolve/resolve_test.go`, `internal/resolve/constraints_test.go` | 2026-07-31 |
| REQ-100 | P1 | `verified` | `go-foundry-cli-s45`, `go-foundry-cli-9a7` | `internal/resolve/resolve_test.go`, `internal/plan/plan_test.go`, `internal/plan/golden_test.go` | 2026-07-31 |
| REQ-101 | P1 | `verified` | `go-foundry-cli-s45`, `go-foundry-cli-41p` | `internal/resolve/resolve_test.go`, `internal/catalog/recipe_test.go`, `internal/report/encoder_test.go` | 2026-07-31 |
| REQ-120 | P1 | `verified` | `go-foundry-cli-9a7`, `go-foundry-cli-6wj`, `go-foundry-cli-j8h.4` | `internal/plan/plan_test.go`, `internal/cli/pipeline_test.go`, `internal/plan/schema_test.go` | 2026-07-31 |
| REQ-121 | P1 | `verified` | `go-foundry-cli-9a7` | `internal/plan/schema_test.go`, `internal/plan/golden_test.go`, `internal/plan/testdata/` | 2026-07-31 |
| REQ-122 | P1 | `verified` | `go-foundry-cli-9a7`, `go-foundry-cli-5an.1` | `internal/plan/plan_test.go`, `internal/plan/golden_test.go`, `internal/archtest/quality_test.go` | 2026-07-31 |
| REQ-123 | P2 | `verified` | `go-foundry-cli-dd1`, `go-foundry-cli-nx1`, `go-foundry-cli-j8h.2` | `internal/generate/machine_test.go`, `internal/generate/state_test.go`, `integration/generate/matrix_success_test.go`, `integration/generate/matrix_failure_test.go` | 2026-07-31 |
| REQ-124 | P2 | `verified` | `go-foundry-cli-2fe`, `go-foundry-cli-qb0`, `go-foundry-cli-qd8`, `go-foundry-cli-97c` | `internal/fsx/parent_test.go`, `internal/fsx/destination_test.go`, `internal/fsx/transaction_test.go`, `integration/hostile/e1/e1_test.go`, `integration/hostile/fsx/suite_test.go` | 2026-07-31 |
| REQ-125 | P2 | `verified` | `go-foundry-cli-2o3`, `go-foundry-cli-qb0`, `go-foundry-cli-qd8` | `internal/fsx/stage_test.go`, `internal/fsx/transaction_test.go`, `integration/hostile/fsx/suite_test.go` | 2026-07-31 |
| REQ-126 | P2 | `verified` | `go-foundry-cli-3ms`, `go-foundry-cli-sui`, `go-foundry-cli-dd1` | `internal/render/dispatch_test.go`, `internal/render/static_test.go`, `internal/verify/mutation_test.go`, `internal/verify/baseline_test.go`, `internal/generate/tree_test.go` | 2026-07-31 |
| REQ-127 | P2 | `verified` | `go-foundry-cli-sui`, `go-foundry-cli-dd1` | `internal/verify/verify_test.go`, `internal/verify/e2e_test.go`, `internal/generate/machine_test.go`, `integration/generate/matrix_failure_test.go` | 2026-07-31 |
| REQ-128 | P2 | `verified` | `go-foundry-cli-8jp`, `go-foundry-cli-bhn` | `internal/gitinit/init_test.go`, `internal/gitinit/hostile_test.go`, `internal/gitinit/semantic_test.go`, `internal/gitinit/surface_test.go` | 2026-07-31 |
| REQ-129 | P2 | `verified` | `go-foundry-cli-npy`, `go-foundry-cli-qb0`, `go-foundry-cli-qd8`, `go-foundry-cli-97c` | `internal/fsx/commit_test.go`, `internal/fsx/classify_test.go`, `integration/hostile/e1/e1_test.go`, `integration/hostile/fsx/suite_test.go` | 2026-07-31 |
| REQ-130 | P2 | `verified` | `go-foundry-cli-e79`, `go-foundry-cli-qb0`, `go-foundry-cli-2y7`, `go-foundry-cli-4hi` | `internal/fsx/preserve_test.go`, `internal/fsx/preservation_audit_test.go`, `internal/archtest/redline_test.go`, `integration/hostile/fsx/audit_test.go`, `integration/generate/matrix_failure_test.go` | 2026-07-31 |
| REQ-131 | P2 | `verified` | `go-foundry-cli-npy`, `go-foundry-cli-e79`, `go-foundry-cli-qd8` | `internal/fsx/classify_test.go`, `internal/fsx/commit_test.go`, `internal/fsx/preserve_test.go`, `integration/hostile/fsx/suite_test.go` | 2026-07-31 |
| REQ-132 | P2 | `verified` | `go-foundry-cli-j8h.4`, `go-foundry-cli-wly`, `go-foundry-cli-dd1` | `internal/cli/plan_generate_equality_test.go`, `integration/fixtures/plan_generate_equality_test.go`, `cmd/foundry/testdata/generate/double_generation.txt`, `integration/generate/matrix_success_test.go` | 2026-07-31 |
| REQ-133 | P2 | `verified` | `go-foundry-cli-j8h.2`, `go-foundry-cli-wly`, `go-foundry-cli-4hi` | `integration/generate/matrix_success_test.go`, `internal/generate/tree_test.go`, `internal/render/static_test.go`, `integration/fixtures/smoke_cli_test.go`, `internal/archtest/redline_test.go` | 2026-07-31 |
| REQ-134 | P2 | `verified` | `go-foundry-cli-qb0`, `go-foundry-cli-qd8`, `go-foundry-cli-3ms` | `internal/fsx/stage_test.go`, `internal/render/static_test.go`, `integration/hostile/fsx/suite_test.go`, `internal/generate/tree_test.go` | 2026-07-31 |
| REQ-135 | P2 | `verified` | `go-foundry-cli-x7b`, `go-foundry-cli-075`, `go-foundry-cli-ni0`, `go-foundry-cli-j1m` | `internal/toolrun/preflight_test.go`, `internal/toolrun/env_test.go`, `integration/hostile/e2/e2_test.go`, `integration/hostile/sentinel/sentinel_test.go`, `catalog/versions.toml` | 2026-07-31 |
| REQ-150 | P2 | `verified` | `go-foundry-cli-sui`, `go-foundry-cli-j8h.2` | `internal/verify/verify_test.go`, `internal/verify/mutation_test.go`, `internal/verify/gofmt_test.go`, `internal/verify/e2e_test.go`, `integration/generate/matrix_success_test.go` | 2026-07-31 |
| REQ-151 | P2 | `verified` | `go-foundry-cli-sui` | `internal/verify/mode_test.go`, `internal/verify/steps_test.go`, `internal/verify/e2e_test.go`, `cmd/foundry/testdata/generate/success_strict.txt`, `integration/generate/matrix_success_test.go` | 2026-07-31 |
| REQ-152 | P2 | `verified` | `go-foundry-cli-sui`, `go-foundry-cli-c6b`, `go-foundry-cli-2y7` | `internal/verify/mode_test.go`, `internal/cli/flags_test.go`, `internal/cli/surface_test.go`, `internal/archtest/redline_test.go` | 2026-07-31 |
| REQ-153 | P2 | `verified` | `go-foundry-cli-x7b`, `go-foundry-cli-075` | `internal/toolrun/run_test.go`, `internal/toolrun/preflight_test.go`, `internal/toolrun/env_test.go` | 2026-07-31 |
| REQ-154 | P2 | `verified` | `go-foundry-cli-0gy`, `go-foundry-cli-x7b`, `go-foundry-cli-ni0`, `go-foundry-cli-bhn` | `internal/toolrun/env_test.go`, `internal/toolrun/cwd_test.go`, `internal/toolrun/cap_test.go`, `internal/toolrun/sentinel_test.go`, `integration/hostile/e2/e2_test.go`, `integration/hostile/sentinel/sentinel_test.go` | 2026-07-31 |
| REQ-155 | P1 | `verified` | `go-foundry-cli-41p`, `go-foundry-cli-j8h.3`, `go-foundry-cli-fy9` | `internal/report/stages_test.go`, `internal/report/encoder_test.go`, `cmd/foundry/testdata/writefree/` | 2026-07-31 |
| REQ-156 | P1 | `verified` | `go-foundry-cli-41p`, `go-foundry-cli-80g`, `go-foundry-cli-fy9` | `internal/report/encoder_test.go`, `internal/report/golden_test.go`, `internal/cli/pipeline_test.go` | 2026-07-31 |
| REQ-157 | P1 | `verified` | `go-foundry-cli-jq1` | `internal/diagnostic/registry_test.go`, `internal/diagnostic/error_test.go`, `internal/archtest/quality_test.go` | 2026-07-31 |
| REQ-158 | P2 | `verified` | `go-foundry-cli-fy9`, `go-foundry-cli-dd1`, `go-foundry-cli-zhm` | `internal/generate/stream.go`, `internal/generate/stream_test.go`, `internal/generate/machine.go`, `internal/report/pipe_test.go`, `internal/report/encoder_test.go`, `cmd/foundry/sigpipe.go`, `cmd/foundry/main.go`, `cmd/foundry/sigpipe_unix_test.go` | 2026-07-31 |
| REQ-159 | P1 | `verified` | `go-foundry-cli-jq1`, `go-foundry-cli-41p` | `internal/diagnostic/redact_test.go`, `internal/diagnostic/error_test.go`, `internal/report/encoder_test.go` | 2026-07-31 |
| REQ-160 | P2 | `verified` | `go-foundry-cli-8jp`, `go-foundry-cli-bhn` | `internal/gitinit/init_test.go`, `internal/gitinit/hostile_test.go`, `internal/gitinit/semantic_test.go`, `internal/gitinit/surface_test.go`, `integration/hostile/sentinel/sentinel_test.go` | 2026-07-31 |
| REQ-161 | P2 | `verified` | `go-foundry-cli-a77`, `go-foundry-cli-2y7`, `go-foundry-cli-4hi` | `internal/catalog/core_tree_test.go`, `internal/archtest/redline_test.go`, `integration/fixtures/smoke_cli_test.go`, `internal/generate/tree_test.go` | 2026-07-31 |
| REQ-162 | P1 | `verified` | `go-foundry-cli-c6b`, `go-foundry-cli-zhm` | `internal/version/version_test.go`, `internal/cli/surface_test.go`, `cmd/foundry/testdata/writefree/version.txt` | 2026-07-31 |
| REQ-163 | P4 | `verified` | `go-foundry-cli-cf1` | `.github/dependabot.yml`, `catalog/versions.toml`, `internal/archtest/action_sha_pins_test.go`, `.github/workflows/ci.yml` | 2026-07-31 |
| REQ-164 | P4 | `verified` | `go-foundry-cli-1wl`, `go-foundry-cli-chf` | `docs/evidence/self-distribution.md`, `docs/evidence/P4-distribution-profile.md`, `docs/evidence/OQ-300-module-path.md`, `README.md` | 2026-07-31 |
| REQ-165 | P2 | `verified` | `go-foundry-cli-j8h.1`, `go-foundry-cli-vu8`, `go-foundry-cli-5an.1` | `docs/evidence/perf/baselines-latest.json`, `docs/evidence/perf/summary.md`, `docs/evidence/perf/P2.8-citation.md`, `scripts/perf/capture-baselines.sh`, `internal/archtest/perf_baseline.go`, `internal/archtest/perf_baseline_test.go`, `docs/evidence/dogfood-cli-smoke.md` | 2026-07-31 |
| REQ-180 | P1 | `verified` | `go-foundry-cli-zhm`, `go-foundry-cli-4hi`, `go-foundry-cli-2y7` | `internal/archtest/arch_test.go`, `internal/archtest/rules.go`, `internal/archtest/redline_test.go` | 2026-07-31 |
| REQ-181 | P1 | `verified` | `go-foundry-cli-zhm`, `go-foundry-cli-4hi`, `go-foundry-cli-c6b` | `cmd/foundry/main.go`, `internal/archtest/arch_test.go`, `internal/cli/surface_test.go` | 2026-07-31 |
| REQ-182 | P1 | `verified` | `go-foundry-cli-di6`, `go-foundry-cli-wkz`, `go-foundry-cli-4hi` | `internal/spec/suite_test.go`, `internal/catalog/catalog_test.go`, `internal/archtest/arch_test.go` | 2026-07-31 |
| REQ-183 | P1 | `verified` | `go-foundry-cli-s45`, `go-foundry-cli-9a7`, `go-foundry-cli-4hi` | `internal/resolve/purity_test.go`, `internal/plan/purity_test.go`, `internal/archtest/arch_test.go` | 2026-07-31 |
| REQ-184 | P2 | `verified` | `go-foundry-cli-qb0`, `go-foundry-cli-3ms`, `go-foundry-cli-2y7`, `go-foundry-cli-4hi` | `internal/fsx/transaction_test.go`, `internal/fsx/preservation_audit_test.go`, `internal/archtest/arch_test.go`, `internal/archtest/quality_test.go`, `internal/render/purity_test.go` | 2026-07-31 |
| REQ-185 | P2 | `verified` | `go-foundry-cli-x7b`, `go-foundry-cli-dd1`, `go-foundry-cli-4hi` | `internal/toolrun/run_test.go`, `internal/toolrun/cwd_test.go`, `internal/archtest/arch_test.go`, `internal/archtest/quality_test.go`, `internal/generate/machine_test.go` | 2026-07-31 |
| REQ-186 | P1 | `verified` | `go-foundry-cli-41p`, `go-foundry-cli-jq1`, `go-foundry-cli-c6b` | `internal/report/encoder_test.go`, `internal/diagnostic/registry_test.go`, `internal/version/version_test.go` | 2026-07-31 |
| REQ-187 | P1 | `verified` | `go-foundry-cli-zhm`, `go-foundry-cli-4hi`, `go-foundry-cli-2y7` | `internal/archtest/arch_test.go`, `internal/archtest/rules.go`, `internal/archtest/quality_test.go` | 2026-07-31 |
| REQ-188 | P1 | `verified` | `go-foundry-cli-jq1`, `go-foundry-cli-4hi` | `internal/diagnostic/error_test.go`, `internal/diagnostic/registry_test.go`, `internal/archtest/arch_test.go` | 2026-07-31 |
| REQ-210 | P1 | `verified` | `go-foundry-cli-d0i`, `go-foundry-cli-di6`, `go-foundry-cli-zhm.1` | `internal/spec/suite_test.go`, `internal/spec/fuzz_test.go`, `internal/testutil/steplog_test.go` | 2026-07-31 |
| REQ-211 | P1 | `verified` | `go-foundry-cli-s45`, `go-foundry-cli-9a7`, `go-foundry-cli-5an.1` | `internal/resolve/resolve_test.go`, `internal/plan/golden_test.go`, `internal/plan/plan_test.go`, `cmd/foundry/testdata/writefree/plan.txt` | 2026-07-31 |
| REQ-212 | P2 | `verified` | `go-foundry-cli-3ms`, `go-foundry-cli-j8h.2`, `go-foundry-cli-wly` | `internal/render/static_test.go`, `internal/render/template_test.go`, `internal/render/gomod_test.go`, `internal/render/dispatch_test.go`, `integration/fixtures/smoke_cli_test.go` | 2026-07-31 |
| REQ-213 | P2 | `verified` | `go-foundry-cli-qd8`, `go-foundry-cli-qb0`, `go-foundry-cli-97c`, `go-foundry-cli-j8h.2` | `integration/hostile/fsx/suite_test.go`, `integration/hostile/fsx/audit_test.go`, `internal/fsx/hostile_test.go`, `integration/hostile/e1/e1_test.go`, `internal/fsx/preservation_audit_test.go` | 2026-07-31 |
| REQ-214 | P2 | `verified` | `go-foundry-cli-bhn`, `go-foundry-cli-x7b`, `go-foundry-cli-ni0`, `go-foundry-cli-fy9` | `integration/hostile/sentinel/sentinel_test.go`, `internal/toolrun/sentinel_test.go`, `integration/hostile/e2/e2_test.go`, `cmd/foundry/sigpipe_unix_test.go`, `internal/generate/stream_test.go` | 2026-07-31 |
| REQ-215 | P3 | `verified` | `go-foundry-cli-wly`, `go-foundry-cli-afh`, `go-foundry-cli-cic`, `go-foundry-cli-1yq` | `integration/fixtures/smoke_tui_test.go`, `integration/tui/lifecycle_matrix_test.go`, `integration/tui/pty_test.go`, `cmd/foundry/testdata/generate/success_tui_default.txt`, `integration/generate/matrix_success_test.go` | 2026-07-31 |
| REQ-216 | P4 | `verified` | `go-foundry-cli-chf`, `go-foundry-cli-s45`, `go-foundry-cli-wly` | `internal/resolve/constraints_test.go`, `internal/cli/catalog_test.go`, `internal/catalog/distribution_tree_test.go`, `cmd/foundry/testdata/writefree/profiles.txt` | 2026-07-31 |
| REQ-217 | P2 | `verified` | `go-foundry-cli-ig3`, `go-foundry-cli-cic`, `go-foundry-cli-d0i` | `integration/hostile/e3/e3_test.go`, `integration/hostile/e3/knownrace/race_test.go`, `integration/hostile/e3/preflight_test.go`, `internal/spec/fuzz_test.go`, `internal/render/gomod_fuzz_test.go` | 2026-07-31 |
| REQ-218 | P3 | `verified` | `go-foundry-cli-hrc`, `go-foundry-cli-5an.1`, `go-foundry-cli-j8h.2`, `go-foundry-cli-zhm` | `cmd/foundry/script_test.go`, `cmd/foundry/testdata/generate/success_default.txt`, `integration/generate/matrix_success_test.go`, `integration/generate/matrix_failure_test.go`, `docs/evidence/P3-cross-platform-e2e-hrc.md` | 2026-07-31 |
| REQ-219 | P1 | `verified` | `go-foundry-cli-zhm.1`, `go-foundry-cli-4hi` | `internal/testutil/golden_test.go`, `internal/testutil/golden.go`, `internal/archtest/quality_test.go` | 2026-07-31 |
| REQ-220 | P2 | `verified` | `go-foundry-cli-qd8`, `go-foundry-cli-bhn`, `go-foundry-cli-2y7`, `go-foundry-cli-bjt` | `integration/hostile/fsx/suite_test.go`, `integration/hostile/sentinel/sentinel_test.go`, `internal/render/template_test.go`, `internal/archtest/redline_test.go`, `internal/fsx/preservation_audit_test.go`, `docs/evidence/P2-exit-review.md` | 2026-07-31 |
| REQ-221 | P3 | `verified` | `go-foundry-cli-jq1`, `go-foundry-cli-x7b`, `go-foundry-cli-1yq` | `internal/diagnostic/redact_test.go`, `internal/toolrun/env_test.go`, `internal/testutil/redact_test.go` | 2026-07-31 |
| REQ-222 | P3 | `verified` | `go-foundry-cli-cic`, `go-foundry-cli-zhm` | `.github/workflows/ci.yml`, `internal/archtest/foundry_ci_test.go`, `docs/evidence/P3-ci-reduced-matrix.md` | 2026-07-31 |
| REQ-223 | P3 | `verified` | `go-foundry-cli-cic`, `go-foundry-cli-a77` | `.github/workflows/strict.yml`, `.github/workflows/ci.yml`, `docs/evidence/P3-ci-reduced-matrix.md` | 2026-07-31 |
| REQ-224 | P4 | `verified` | `go-foundry-cli-cf1`, `go-foundry-cli-zhm`, `go-foundry-cli-j1m` | `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `internal/archtest/action_sha_pins_test.go`, `integration/hostile/e5/lock_test.go`, `catalog/versions.toml` | 2026-07-31 |
| REQ-240 | S6 | `verified` | `go-foundry-cli-o60`, `go-foundry-cli-97c`, `go-foundry-cli-ni0`, `go-foundry-cli-ig3`, `go-foundry-cli-e0x`, `go-foundry-cli-j1m` | `docs/evidence/E1-descriptor-relative-transaction.md`, `docs/evidence/E2-go-git-env-isolation.md`, `docs/evidence/E3-race-prerequisites.md`, `docs/evidence/E4-bubbletea-lifecycle.md`, `docs/evidence/E5-version-action-lock.md` | 2026-07-31 |
| REQ-241 | P1 | `verified` | `go-foundry-cli-0z4`, `go-foundry-cli-5an`, `go-foundry-cli-5an.1` | `docs/evidence/P1-exit-review.md`, `docs/evidence/req-traceability.json`, `cmd/foundry/writefree_e2e_test.go`, `cmd/foundry/testdata/writefree/`, `internal/archtest/quality_test.go` | 2026-07-31 |
| REQ-242 | P2 | `verified` | `go-foundry-cli-5pr`, `go-foundry-cli-j8h`, `go-foundry-cli-vu8` | `docs/evidence/P2-exit-review.md`, `docs/evidence/dogfood-cli-smoke.md`, `docs/evidence/dogfood-cli-repo-map.md`, `docs/evidence/perf/P2.8-citation.md`, `integration/generate/matrix_success_test.go`, `cmd/foundry/testdata/generate/` | 2026-07-31 |
| REQ-243 | P3 | `verified` | `go-foundry-cli-66w`, `go-foundry-cli-1b7`, `go-foundry-cli-bm1` | `docs/evidence/P3-mvp-exit-review.md`, `internal/archtest/p3_exit_evidence_test.go`, `integration/fixtures/smoke_tui_test.go`, `docs/evidence/P3-tui-shell-jr7.md` | 2026-07-31 |
| REQ-244 | P4 | `verified` | `go-foundry-cli-7fm`, `go-foundry-cli-sek`, `go-foundry-cli-chf` | `docs/evidence/P4-exit-review.md`, `docs/evidence/P4-distribution-profile.md`, `docs/evidence/self-distribution.md`, `internal/catalog/distribution_tree_test.go` | 2026-07-31 |
| REQ-245 | P2 | `verified` | `go-foundry-cli-vu8`, `go-foundry-cli-bm1` | `docs/evidence/dogfood-cli-smoke.md`, `docs/evidence/dogfood-cli-repo-map.md`, `integration/dogfood/smoke_test.go`, `integration/dogfood/repo_map_test.go`, `scripts/dogfood-cli.sh` | 2026-07-31 |
| REQ-246 | P3 | `verified` | `go-foundry-cli-k0o` | `docs/evidence/three-agent-acceptance.md`, `docs/evidence/three-agent/k0o-cli-change.txt`, `docs/evidence/dogfood-tui-smoke.md` | 2026-07-31 |
| REQ-247 | P2 | `verified` | `go-foundry-cli-vu8`, `go-foundry-cli-bm1`, `go-foundry-cli-bjt`, `go-foundry-cli-j8h.1` | `docs/evidence/dogfood-cli-smoke.md`, `docs/evidence/dogfood-cli-repo-map.md`, `docs/evidence/perf/summary.md`, `integration/dogfood/smoke_test.go`, `integration/dogfood/repo_map_test.go` | 2026-07-31 |
| REQ-248 | S6 | `verified` | `go-foundry-cli-o60`, `go-foundry-cli-hme`, `go-foundry-cli-dlv.1` | `docs/02-definitive-foundry-specification-revised-fable-5.md`, `AGENTS.md`, `docs/evidence/P4-exit-review.md`, `docs/evidence/P3-mvp-exit-review.md` | 2026-07-31 |
