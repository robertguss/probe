// Package generate owns the total post-plan generation lifecycle state machine
// (SPEC-FOUNDRY-002 Section 29, REQ-034, REQ-036, REQ-123, REQ-158, REQ-185).
//
// Scope of this package (P2.5 / umbrella go-foundry-cli-dd1; leaves nx1, fy9, ybi):
//
//   - Ordered Section 29.2 stages (1–19) with table-driven legal transitions only
//   - Typed GenerationEvent emissions for every progress/state/terminal change
//   - Cancel points (REQ-036): before stage → exit 130 nothing on disk; after
//     stage creation → stop tools, preserve stage, report location, exit 130;
//     cancel racing commit → commit result dominates (Section 31.9)
//   - Failure injection leaves the system in the documented preserve/exit state
//   - Stream failure policy (Section 36.5 / FND-012 / REQ-158 / fy9): pre-commit
//     blocks placement (exit 1, preserve stage); post-commit report failure is
//     absorbed so DominatedExit(commit) holds; step logs phase/stream/errno/exit
//   - Pre-staging network disclosure (REQ-034 / FND-007 / ybi): pure plan-driven
//     formatting after tool-preflight, before acquire-parent / create-stage
//   - Process-tree contract (REQ-120 / REQ-185): tool stages map 1:1 onto plan
//     external_steps ids (default/strict ± git-init); no invented extras; full
//     argv/env audit remains toolrun.CompareProcessTree
//   - No output encoding (report owns human/JSON encoding; Section 42.2)
//
// The process-wide drained SIGPIPE handler is registered in cmd/foundry
// (Section 42.2). This package owns stream classification + commit-dominates-
// reporting package tests; real SIGPIPE process/child disposition probes live
// under cmd/foundry. Public CLI wiring is j8h.3; E2E matrix is j8h.2.
//
// The machine orchestrates lower packages (fsx, toolrun, gitinit, verify) via
// injectable stage functions. It never mutates the filesystem itself, never
// starts subprocesses, and never encodes stdout/stderr.
//
// Stage and lifecycle name strings match internal/report's progress/state
// tables so logs and goldens stay diffable. generate does not import report
// (Section 42.3 layering; report/doc.go decoupling note).
//
// Logging contract: each transition records state, event kind/name, result,
// and duration_ms. Stream decisions log phase (pre/post commit), stream,
// errno, and exit chosen. Never secret env values or absolute host homes in goldens.
//
// Layering (Section 42.3): may import diagnostic and lower side-effect
// packages. Must not import report, cli, or testutil in production sources.
package generate
