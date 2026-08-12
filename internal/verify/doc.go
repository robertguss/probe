// Package verify owns staging verification and final tree conformance for
// Foundry generate (SPEC-FOUNDRY-002 Section 35, REQ-126–127, REQ-150–152;
// FND-004, FND-006).
//
// Scope of this package (P2.4 / go-foundry-cli-sui):
//
//   - Mode enum exactly default|strict (REQ-152); no none; no env weaken
//   - Default gate: gofmt (in-process), tidy mutation-set + exact pin reparse,
//     go mod verify, go test -count=1 -buildvcs=false ./..., go vet
//     -buildvcs=false ./..., final non-.git path/type/mode/byte conformance
//   - Strict adds go tool staticcheck ./... and go tool govulncheck ./...
//   - Race is NEVER part of the generation gate (FND-004 / Section 35.4)
//   - After go mod tidy: freeze ConformanceBaseline; any later drift is
//     verify.unplanned_mutation (FND-006)
//   - Final conformance after every subsequent external tool
//
// Pipeline (stages 11–14 of Section 29.2, owned here):
//
//  1. Snapshot pre-tidy inventory
//  2. Run go mod tidy (toolrun StepRunner; mutates go.mod/go.sum only)
//  3. Mutation-set: only go.mod/go.sum may change; reparse pins; reject
//     replace/exclude/workspace → verify.module_mutation
//  4. Freeze ConformanceBaseline (paths, types, modes, digests + aggregate)
//  5. gofmt conformance of every staged .go file (go/format idempotence)
//  6. For each remaining external step: run via toolrun; on success, re-check
//     non-.git tree against baseline; drift → verify.unplanned_mutation
//  7. Explicit final-conformance check (same compare; named step for logs)
//
// Optional git init and post-git re-conformance remain internal/gitinit
// authority (stages 15–16); generate calls Conform against the baseline
// after git scratch removal when desired.
//
// Subprocess starts are delegated exclusively to internal/toolrun (REQ-185).
// This package never imports os/exec for product starts.
//
// Failures use diagnostic identifiers:
//
//	verify.failed             — in-process or gate failure (gofmt, step list)
//	verify.module_mutation    — tidy mutation-set / pin reparse failure
//	verify.unplanned_mutation — post-freeze tree drift (paths named)
//	tool.failed / tool.timeout — external step failures (propagated)
//
// The stage is never deleted here (Section 31.6 / REQ-130).
//
// Logging contract: per verify step — name, argv (basename), start/end,
// duration, exit, stream sizes, baseline digest before/after, mutation
// paths if any. On failure: which check, expected vs actual path list,
// remediation identifier. Never env values or full host homes.
//
// Layering (Section 42.3): may import diagnostic, plan, toolrun. Must not
// import generate, gitinit, cli, or testutil in production sources.
// (gitinit is a sibling side-effect package; verify must not depend on it
// so gitinit can re-check non-.git trees without an import cycle risk —
// ConformanceBaseline lives here and is the sole freeze authority.)
package verify
