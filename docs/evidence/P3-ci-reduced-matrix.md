# P3.2 — Generated CI reduced matrix + Foundry CI (go-foundry-cli-cic)

- **Date:** 2026-07-31
- **Spec:** Section 16.6 / 47.1; FND-016; REQ-063, REQ-222, REQ-223; RSK-403
- **Bead:** `go-foundry-cli-cic`

## Generated projects (FND-016)

Embedded Core workflows (`catalog/core/files/workflows/`):

| Workflow | Role | Assertion |
| -------- | ---- | --------- |
| `ci.yml` | Exactly **one** required Linux PR/default job (`check`): gofmt, `go mod verify`, `go test -count=1 ./...`, vet, Staticcheck; concurrency cancel | `TestGeneratedCISingleRequiredJob`, `TestCoreCanonicalTreeInventory` workflow phase |
| `strict.yml` | Weekly + `workflow_dispatch` only: govulncheck, preflighted race, macOS representative, bounded fuzz | same tests; not a PR gate |

Risk-based promotion of scheduled checks into the PR gate is documented in
generated `docs/testing.md` (RSK-403).

Action pins: full commit SHAs from `catalog/versions.toml` (checkout, setup-go).

## Foundry repository (Section 47.1)

| Workflow | Jobs |
| -------- | ---- |
| `.github/workflows/ci.yml` | `unit` (ubuntu+macos: format/mod-verify/test/vet/staticcheck); `race` (linux preflighted); `catalog-digest`; `golden-matrix`; `sentinel`; `hostile-fsx`; `generate-e2e` (ubuntu+macos) |
| `.github/workflows/strict.yml` | weekly + manual: govulncheck, bounded fuzz, cold-cache baseline timings, full hostile suite |

Assertions: `TestFoundryCISection47` in `internal/archtest`. Golden discipline
(`CheckGoldenDiscipline`) remains clean (no auto-update env in workflows).

## Dogfood PR latency measurement path

1. Open a PR against `main` (or push to a PR branch).
2. Record wall-clock for the required generated-project job path:
   - Foundry: sum of required `ci.yml` jobs (unit + catalog-digest + golden-matrix
     + hostile + generate-e2e as configured).
   - Generated Core: single `check` job duration from Actions UI / `gh run view`.
3. Store notes under `docs/evidence/dogfood-*-logs/` when measuring dogfood PRs
   (`bm1` / later phases). No absolute millisecond gate until baselines exist
   (Section 49).

## Commands

```bash
GOTOOLCHAIN=go1.26.5 go test ./internal/archtest/ -run 'TestFoundryCI|TestGeneratedCI' -count=1
GOTOOLCHAIN=go1.26.5 go test ./internal/catalog/ -run TestCoreCanonicalTree -count=1
```
