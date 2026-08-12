// Package toolrun owns closed subprocess environment construction, binary
// preflight, descriptor-bound child start, and plan-declared step execution
// for Foundry external tools (SPEC-FOUNDRY-002 Section 34, Appendix E;
// REQ-135, REQ-153, REQ-154, REQ-185).
//
// Scope of this package (P2.2 / umbrella go-foundry-cli-x7b):
//
//   - Exact go/git environments from an empty base plus allowlist (Section 34.2)
//   - Startup binary location and go version preflight (fail before staging)
//   - Descriptor-bound child start via serialized fchdir (Section 34.4)
//   - Plan-declared timeouts, per-stream 4 MiB caps, process-group kill on cancel
//   - StepResult with failure-only capture replay (Section 34.3)
//
// Environments are never inherited-minus-denylist. Host bleed is closed by
// construction; fixed keys (GOENV, GOFLAGS, GOCACHEPROG, GOTOOLCHAIN, GOWORK,
// GOVCS, GOAUTH, private-module vars, CGO_ENABLED) carry normative values only.
//
// Child cwd (Section 34.4): CaptureOriginalCWD once at init; BoundStarter
// serializes on a process-wide mutex: fchdir(stage_fd) → Start with empty
// pathname Dir → fchdir(original_fd). Stage FD comes from
// fsx.Transaction.DuplicateStageHandle (never re-opened by path for cwd).
// Pathname swap of the stage entry during start must not redirect the child.
// Restore failure is fail-closed (fail class tool.cwd_restore_failed;
// diagnostic id tool.failed).
//
// Caps / timeouts (Section 34.3 / REQ-154): each step carries a plan timeout
// and output_cap_bytes (default 4 MiB per stream). Truncation is explicit.
// Cancellation and deadline kill the child process group (Setpgid) — deadline
// kills immediately; cancel uses a bounded grace then SIGKILL. Captured output
// is replayed only on failure (StepResult.Replay).
//
// Preflight failures use diagnostic identifiers:
//
//	tool.missing       — go/git not found, not a file, or not executable
//	tool.wrong_version — go version is not exactly the pinned toolchain
//	tool.timeout       — plan-declared step timeout exceeded
//	tool.failed        — non-zero exit, cancel, start failure, cwd faults
//
// Pinned go discovery (FOUNDRY_GO_BIN / module-cache / mise / PATH) lives in
// FindPinnedGoBinary and ProbeGoVersionLocal. CLI pipelineHost prefers a
// matching pin before bare LookPath so agents can generate without hunting.
// tool.wrong_version remediation names FOUNDRY_GO_BIN and any nearby pin path.
//
// Logging contract: emit resolved binary paths, observed version strings,
// allowlist key names, and allowlist SHA-256 hash — never env values.
// Bound starts log binary basename, stage identity (dev/ino), lock wait ms,
// fchdir errno if any, restore ok/fail, child pid, output cap, kill grace —
// never full host homes. Step logs include step id, argv (basename only),
// duration, exit, truncation flags, fail class.
//
// Hostile env/config/template sentinel suite (REQ-214 / go-foundry-cli-bhn):
// unit isolation + process-tree audit (CompareProcessTree, IsolationDump,
// EmptyTemplateDir) live here; permanent CI job runs
// integration/hostile/sentinel (E2 plants against production Construct*) and
// keeps integration/hostile/e2 green as the promotion source.
//
// Layering (Section 42.3): may import diagnostic, plan, catalog, fsx. Must not
// import generate, cli, or testutil. Sole production package that starts
// go/git subprocesses.
package toolrun
