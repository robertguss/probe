package report

import (
	"fmt"
	"io"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Encoder formats command results and failures for one invocation.
//
// It owns stdout/stderr writing only — no generation work, no FS mutation.
// Safe for sequential use from a single goroutine (typical CLI Run path).
//
// Post-commit: call MarkCommitted after a successful exclusive rename so
// subsequent stream write failures are absorbed (exit 0; FND-012 / §36.4).
type Encoder struct {
	stdout io.Writer
	stderr io.Writer
	opts   Options

	committed    bool
	streamFailed bool
	// lastStreamErr is the first absorbed post-commit write error (tests).
	lastStreamErr error
}

// New constructs an Encoder. nil writers become io.Discard-like no-ops via
// guarded writes (Write methods tolerate nil).
func New(stdout, stderr io.Writer, opts Options) *Encoder {
	return &Encoder{
		stdout: stdout,
		stderr: stderr,
		opts:   opts.normalized(),
	}
}

// Options returns a copy of the encoder options.
func (e *Encoder) Options() Options {
	if e == nil {
		return Options{}
	}
	return e.opts
}

// MarkCommitted records that the exclusive rename succeeded. Subsequent
// Success / progress write failures are absorbed: StreamFailed becomes true
// and methods return nil so the CLI can exit 0.
func (e *Encoder) MarkCommitted() {
	if e != nil {
		e.committed = true
	}
}

// Committed reports whether MarkCommitted was called.
func (e *Encoder) Committed() bool {
	return e != nil && e.committed
}

// StreamFailed reports whether a required write failed after MarkCommitted.
func (e *Encoder) StreamFailed() bool {
	return e != nil && e.streamFailed
}

// LastStreamError returns the first absorbed post-commit stream error, if any.
func (e *Encoder) LastStreamError() error {
	if e == nil {
		return nil
	}
	return e.lastStreamErr
}

// absorb records a post-commit write failure and returns nil so callers keep
// exit 0. Pre-commit write failures are returned as-is (report.failed / CLI).
func (e *Encoder) absorb(err error) error {
	if err == nil {
		return nil
	}
	if e != nil && e.committed {
		if !e.streamFailed {
			e.streamFailed = true
			e.lastStreamErr = err
		}
		return nil
	}
	return err
}

// Success encodes a successful command result.
//
// JSON: one Envelope on stdout; stderr untouched (must stay empty after flags).
// Text: summary on stdout unless Quiet; empty summary is a no-op.
//
// After MarkCommitted, write errors are absorbed (return nil) and a best-effort
// stderr notice is attempted once.
func (e *Encoder) Success(command string, result any, textSummary string) error {
	return e.SuccessWithWarnings(command, result, textSummary, nil)
}

// SuccessWithWarnings is Success with an optional stable warnings list
// (JSON only; text ignores warnings unless folded into textSummary).
func (e *Encoder) SuccessWithWarnings(command string, result any, textSummary string, warnings []string) error {
	if e == nil {
		return nil
	}
	switch e.opts.Mode {
	case ModeJSON:
		env := SuccessEnvelope(command, result, warnings)
		err := WriteJSON(e.stdout, env)
		if err != nil {
			if e.committed {
				WritePostCommitStreamNotice(e.stderr, destinationFromResult(result))
			}
			return e.absorb(err)
		}
		return nil
	default:
		err := WriteTextSuccess(e.stdout, textSummary, e.opts.Quiet)
		if err != nil {
			if e.committed {
				WritePostCommitStreamNotice(e.stderr, destinationFromResult(result))
			}
			return e.absorb(err)
		}
		return nil
	}
}

// Failure encodes a command failure and returns the process exit code that
// should be used at the CLI boundary (Section 38.1).
//
// JSON: failure Envelope on stdout; empty stderr.
// Text: diagnostic (+ remediation + stage_path) on stderr.
//
// stagePath is always surfaced when non-empty (never suppressed by Quiet).
//
// Post-commit: Failure must not be used for a committed repository (FND-012).
// If Committed is true, Failure still encodes but returns ExitSuccess so a
// miswired caller cannot flip exit 0 — prefer Success for post-commit reports.
func (e *Encoder) Failure(command string, err error, stagePath string) (exitCode int, writeErr error) {
	if e == nil {
		return diagnostic.ExitCode(err), nil
	}
	code := diagnostic.ExitCode(err)
	if e.committed {
		// Commit dominates: never report failure exit after successful rename.
		code = diagnostic.ExitSuccess
	}

	switch e.opts.Mode {
	case ModeJSON:
		env := FailureEnvelope(command, err, stagePath, nil)
		// After commit, never emit ok=false (FND-012 / REQ-156).
		if e.committed {
			// Best-effort success-shaped notice is wrong here — caller should
			// use Success. Still refuse error+committed lie by rewriting to
			// a minimal success with stream notice.
			WritePostCommitStreamNotice(e.stderr, stagePath)
			return diagnostic.ExitSuccess, nil
		}
		writeErr = WriteJSON(e.stdout, env)
		return code, writeErr
	default:
		WriteTextError(e.stderr, err, stagePath, e.opts.colorEnabled())
		return code, nil
	}
}

// Progress emits a stable progress line for generate (text mode only).
// Suppressed when Quiet or JSON mode. Never written to stderr.
func (e *Encoder) Progress(id StageID) error {
	if e == nil {
		return nil
	}
	if e.opts.Mode != ModeText {
		return nil
	}
	err := WriteProgress(e.stdout, id, e.opts.Quiet)
	return e.absorb(err)
}

// NetworkDisclosure emits network-capable step disclosure (Section 13.5).
// Text mode only; never suppressed by Quiet. JSON mode embeds disclosure in
// the plan/result document instead.
func (e *Encoder) NetworkDisclosure(lines []string) error {
	if e == nil || e.opts.Mode != ModeText {
		return nil
	}
	err := WriteNetworkDisclosure(e.stdout, lines)
	return e.absorb(err)
}

// Summary writes a summary line (text mode). Suppressed by Quiet.
func (e *Encoder) Summary(text string) error {
	if e == nil || e.opts.Mode != ModeText {
		return nil
	}
	err := WriteTextSuccess(e.stdout, text, e.opts.Quiet)
	return e.absorb(err)
}

// Verbose writes a verbose detail line to stderr (text mode). No-op when
// Verbose is false or mode is JSON.
func (e *Encoder) Verbose(detail string) error {
	if e == nil || e.opts.Mode != ModeText {
		return nil
	}
	return WriteVerbose(e.stderr, detail, e.opts.Verbose)
}

// destinationFromResult tries to extract a destination string from known
// result shapes for post-commit notices.
func destinationFromResult(result any) string {
	switch r := result.(type) {
	case GenerateResult:
		return r.Destination
	case *GenerateResult:
		if r != nil {
			return r.Destination
		}
	case map[string]any:
		if d, ok := r["destination"].(string); ok {
			return d
		}
	}
	return ""
}

// FormatValidateText is the default text summary for validate success.
func FormatValidateText(spec string) string {
	return fmt.Sprintf("validate: ok (spec=%s)", spec)
}

// FormatPlanText is a short human summary; full plan JSON is ModeJSON result.
func FormatPlanText(projectName, dest string, fileCount int) string {
	return fmt.Sprintf("plan: project=%s destination=%s files=%d", projectName, dest, fileCount)
}

// FormatVersionText formats version.Info-like fields for text mode.
func FormatVersionText(version, commit, goVer, catalogDigest string) string {
	if commit == "" {
		commit = "-"
	}
	if catalogDigest == "" {
		catalogDigest = "-"
	}
	return fmt.Sprintf(
		"foundry %s\ncommit: %s\ngo: %s\ncatalog_digest: %s",
		version, commit, goVer, catalogDigest,
	)
}
