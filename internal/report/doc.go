// Package report owns all human (text) and JSON formatting for Foundry
// commands (SPEC-FOUNDRY-002 Sections 36–38, 42.2; REQ-037, REQ-155–157,
// REQ-159, REQ-186).
//
// Responsibilities:
//   - Text: plain line-oriented stdout; diagnostics on stderr; NO_COLOR;
//     stable progress stage names (Section 29.2 / 36.2); no prompts or TTY
//     spinners.
//   - JSON: one versioned deterministic document on stdout
//     (schema, command, ok, result, error, warnings); empty stderr after
//     flag recognition (REQ-037 / Section 37).
//   - Agent-legible failures: error_id, message, remediation, optional
//     path/line/col, optional stage_path when a stage is preserved.
//   - Quiet: suppresses progress and summary lines only — never errors,
//     network disclosure, preserved-stage locations, or committed destination.
//   - Post-commit stream failure: absorbed so exit remains 0 (FND-012 /
//     Section 36.4); real OS-pipe encoder tests live in pipe_test.go; process
//     SIGPIPE drain + child disposition probes are cmd/foundry (fy9).
//
// report consumes typed results and GenerationEvent-like progress values only;
// it never performs generation work, filesystem mutation, or subprocesses.
//
// Layering (Section 42.3): may import diagnostic and version. Must not import
// generate (circular: generate is lower and cannot import report), cli, or
// testutil from production code. Progress stage IDs live here so generate can
// emit matching strings without importing report.
package report
