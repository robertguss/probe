package toolrun

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// Fail class tokens for step execution (mirror diagnostic identifiers).
const (
	FailClassTimeout = "tool.timeout"
	FailClassFailed  = "tool.failed"
)

// DefaultKillGrace is how long after context cancel/deadline the Foundry waits
// before SIGKILL'ing the child process group (Section 34.3).
const DefaultKillGrace = 2 * time.Second

// StepRequest is one plan-declared external step execution (Section 34 / REQ-154).
//
// Binary is the absolute path resolved at preflight. Args are argv[1:] (the
// plan's Argv without the leading binary basename when Binary is absolute;
// production passes plan Argv[1:] after locating the binary).
//
// Env is the exact constructed environment (Section 34.2). Production always
// passes a non-nil map from ConstructGoEnv / ConstructGitEnv / plan.ExternalStep.Env.
//
// StageFD binds child cwd via BoundStarter (Section 34.4). Preflight steps that
// run from the Foundry startup cwd use StageFD < 0 only with RunUnbound;
// transactional steps always use a live stage descriptor.
type StepRequest struct {
	// StepID is the Appendix E / plan external_steps id (e.g. go-mod-tidy).
	StepID string
	// Binary is the absolute executable path.
	Binary string
	// Args are argv[1:].
	Args []string
	// Env is KEY→value constructed environment (empty-base allowlist).
	Env map[string]string
	// StageFD is the stage directory descriptor for bound start; required for Run.
	StageFD int
	// Timeout is the plan-declared timeout. Zero uses a conservative default
	// of 60s (preflight-class); production always passes plan TimeoutS.
	Timeout time.Duration
	// OutputCapBytes is the per-stream cap. Zero uses DefaultOutputCapBytes (4 MiB).
	OutputCapBytes int
}

// StepResult is the outcome of one external step (toolrun → verify/report).
//
// Captured streams are always bounded. Output is replayed to operators only
// via Replay() when the step failed (Section 34.3 / REQ-154). Success leaves
// streams available to generate for mutation inspection but report encoding
// must not dump them.
type StepResult struct {
	StepID          string
	Argv            []string // binary basename + args (stable; no host homes)
	BinaryBase      string
	EnvHash         string
	EnvKeys         []string
	Duration        time.Duration
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
	StdoutBytes     int64 // total offered before cap (best-effort)
	StderrBytes     int64
	TimedOut        bool
	Cancelled       bool
	FailClass       string
	BoundLog        BoundStartLog
	// PID of the child when started; 0 if never started.
	PID int
	err error
}

// Err returns the step failure, if any.
func (r StepResult) Err() error { return r.err }

// OK reports whether the step completed with exit 0 and no fail class.
func (r StepResult) OK() bool {
	return r.err == nil && r.FailClass == "" && !r.TimedOut && !r.Cancelled && r.ExitCode == 0
}

// ShouldReplay reports whether captured output must be surfaced (failure only).
func (r StepResult) ShouldReplay() bool { return !r.OK() }

// Replay returns bounded stdout/stderr only on failure. On success both slices
// are nil so callers cannot accidentally dump success noise (Section 34.3).
func (r StepResult) Replay() (stdout, stderr []byte, outTrunc, errTrunc bool) {
	if r.OK() {
		return nil, nil, false, false
	}
	return r.Stdout, r.Stderr, r.StdoutTruncated, r.StderrTruncated
}

// LogFields returns structured non-secret fields for step/report logging.
// Never includes env values or full host home paths in binary fields.
func (r StepResult) LogFields() map[string]string {
	m := map[string]string{
		"step_id":          r.StepID,
		"binary":           r.BinaryBase,
		"argv":             strings.Join(r.Argv, " "),
		"env_hash":         r.EnvHash,
		"env_keys":         strings.Join(r.EnvKeys, ","),
		"duration_ms":      fmt.Sprintf("%d", r.Duration.Milliseconds()),
		"exit_code":        fmt.Sprintf("%d", r.ExitCode),
		"stdout_truncated": boolString(r.StdoutTruncated),
		"stderr_truncated": boolString(r.StderrTruncated),
		"stdout_bytes":     fmt.Sprintf("%d", r.StdoutBytes),
		"stderr_bytes":     fmt.Sprintf("%d", r.StderrBytes),
		"timed_out":        boolString(r.TimedOut),
		"cancelled":        boolString(r.Cancelled),
		"fail_class":       r.FailClass,
		"child_pid":        fmt.Sprintf("%d", r.PID),
	}
	if r.BoundLog.BinaryBase != "" {
		m["bound"] = r.BoundLog.LogDetail()
	}
	return m
}

// LogDetail is a single-line summary for steplog (no env values, no homes).
func (r StepResult) LogDetail() string {
	return fmt.Sprintf(
		"step=%s binary=%s exit=%d duration_ms=%d timed_out=%v cancelled=%v fail_class=%s stdout_trunc=%v stderr_trunc=%v env_hash=%s",
		r.StepID, r.BinaryBase, r.ExitCode, r.Duration.Milliseconds(),
		r.TimedOut, r.Cancelled, r.FailClass, r.StdoutTruncated, r.StderrTruncated, r.EnvHash,
	)
}

// Executor runs plan-declared external steps with closed env, bound cwd,
// plan timeout, per-stream caps, and process-group kill on cancel (REQ-154).
type Executor struct {
	starter *BoundStarter
	// KillGrace bounds the wait after cancel before SIGKILL of the process group.
	// Zero uses DefaultKillGrace.
	KillGrace time.Duration
}

// NewExecutor builds an Executor that uses starter for descriptor-bound starts.
// starter must be non-nil (from NewBoundStarter + CaptureOriginalCWD).
func NewExecutor(starter *BoundStarter) (*Executor, error) {
	if starter == nil {
		return nil, fmt.Errorf("toolrun: Executor requires BoundStarter")
	}
	return &Executor{starter: starter, KillGrace: DefaultKillGrace}, nil
}

// Run executes one transactional external step (Section 34).
//
// Protocol:
//  1. Build argv/env log fields (hash + keys, never values)
//  2. Apply plan timeout to ctx
//  3. BoundStarter: fchdir(stage) → Start(Dir="") → restore → Wait
//  4. Cap streams (explicit truncation flags)
//  5. Classify timeout / cancel / non-zero exit into fail classes
//
// No shell is used. Binary must already be an absolute preflight-resolved path.
func (e *Executor) Run(ctx context.Context, req StepRequest) StepResult {
	start := time.Now()
	res := StepResult{
		StepID:     req.StepID,
		BinaryBase: filepath.Base(req.Binary),
		Argv:       buildArgv(req.Binary, req.Args),
		ExitCode:   -1, // unknown until wait
	}
	if req.Env != nil {
		res.EnvHash = AllowlistHash(req.Env)
		res.EnvKeys = EnvKeys(req.Env)
	}

	if e == nil || e.starter == nil {
		res.FailClass = FailClassFailed
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"executor not initialized",
		)
		res.Duration = time.Since(start)
		return res
	}
	if req.Binary == "" {
		res.FailClass = FailClassFailed
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"empty binary for step %s", req.StepID,
		)
		res.Duration = time.Since(start)
		return res
	}
	if req.StageFD < 0 {
		res.FailClass = FailClassFailed
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"step %s requires stage descriptor fd (got %d)", req.StepID, req.StageFD,
		)
		res.Duration = time.Since(start)
		return res
	}

	capN := req.OutputCapBytes
	if capN <= 0 {
		capN = DefaultOutputCapBytes
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultPreflightTimeout
	}

	// Plan-declared timeout wraps the caller's context.
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	envSlice := EnvSlice(req.Env)
	bound := e.starter.Start(stepCtx, BoundStartRequest{
		StageFD:        req.StageFD,
		Binary:         req.Binary,
		Args:           req.Args,
		Env:            envSlice,
		StepID:         req.StepID,
		OutputCapBytes: capN,
		KillGrace:      e.killGrace(),
	})

	res.BoundLog = bound.Log
	res.PID = bound.PID
	res.Duration = time.Since(start)

	// Cap streams (production path may already have capped; re-cap is idempotent
	// when bound already truncated at the same limit).
	out, outTrunc := CapBytes(bound.Stdout, capN)
	errb, errTrunc := CapBytes(bound.Stderr, capN)
	res.Stdout = out
	res.Stderr = errb
	res.StdoutTruncated = outTrunc || bound.StdoutTruncated
	res.StderrTruncated = errTrunc || bound.StderrTruncated
	res.StdoutBytes = bound.StdoutBytes
	res.StderrBytes = bound.StderrBytes
	if res.StdoutBytes == 0 {
		res.StdoutBytes = int64(len(bound.Stdout))
	}
	if res.StderrBytes == 0 {
		res.StderrBytes = int64(len(bound.Stderr))
	}

	// Prefer cwd restore / fchdir / env-size failures as already classified by BoundStarter.
	if bound.FailClass == FailClassCWDRestore || bound.FailClass == FailClassFchdirStage || bound.FailClass == FailClassEnvSize {
		res.FailClass = bound.FailClass
		res.err = bound.Err()
		res.ExitCode = bound.ExitCode
		return res
	}

	// Classify timeout / cancellation from context first (even if wait error is
	// "signal: killed").
	if errors.Is(stepCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.FailClass = FailClassTimeout
		res.ExitCode = exitCodeFrom(bound)
		detail := fmt.Sprintf(
			"external step %s exceeded plan timeout %s (binary %s argv %v)",
			req.StepID, timeout, res.BinaryBase, res.Argv,
		)
		res.err = diagnostic.Newf(
			diagnostic.IDToolTimeout,
			diagnostic.StepLocation(req.StepID),
			"%s", detail,
		).WithRemediation(
			"Inspect the preserved stage and the step's bounded output. " +
				"Address the hang/slowness and re-run; do not delete the stage without inspection.",
		)
		return res
	}
	if errors.Is(stepCtx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		res.Cancelled = true
		res.FailClass = FailClassFailed
		res.ExitCode = exitCodeFrom(bound)
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"external step %s cancelled (binary %s)",
			req.StepID, res.BinaryBase,
		)
		return res
	}

	if bound.Err() != nil {
		res.FailClass = FailClassFailed
		res.ExitCode = exitCodeFrom(bound)
		if res.ExitCode < 0 {
			// start failure — no exit code
			res.err = bound.Err()
			if res.err != nil {
				// Re-wrap only if not already a FoundryError.
				var fe *diagnostic.FoundryError
				if !errors.As(res.err, &fe) {
					res.err = diagnostic.Wrapf(
						diagnostic.IDToolFailed,
						diagnostic.StepLocation(req.StepID),
						bound.Err(),
						"external step %s binary %s failed: %v",
						req.StepID, res.BinaryBase, bound.Err(),
					)
				}
			}
			return res
		}
		// Non-zero exit from tool.
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"external step %s binary %s exited %d",
			req.StepID, res.BinaryBase, res.ExitCode,
		).WithRemediation(
			"Read the step id, argv, and bounded output. Fix the tool failure in the preserved stage, then re-run generate.",
		)
		return res
	}

	// Success path.
	res.ExitCode = 0
	if bound.ExitCode > 0 {
		// Defensive: Wait returned nil but ExitCode set — treat as failure.
		res.ExitCode = bound.ExitCode
		res.FailClass = FailClassFailed
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"external step %s binary %s exited %d",
			req.StepID, res.BinaryBase, res.ExitCode,
		)
		return res
	}
	return res
}

// RunFromPlan executes a plan.ExternalStep against StageFD.
// Binary must already be absolute (plan may store "go"/"git" basenames during
// pure plan construction; generate resolves before calling toolrun).
func (e *Executor) RunFromPlan(ctx context.Context, step plan.ExternalStep, stageFD int) StepResult {
	args := step.Argv
	if len(args) > 0 {
		// plan Argv is full argv including basename at [0]; drop it when Binary is set.
		args = args[1:]
	}
	timeout := time.Duration(step.TimeoutS) * time.Second
	return e.Run(ctx, StepRequest{
		StepID:         step.ID,
		Binary:         step.Binary,
		Args:           args,
		Env:            step.Env,
		StageFD:        stageFD,
		Timeout:        timeout,
		OutputCapBytes: step.OutputCapBytes,
	})
}

func (e *Executor) killGrace() time.Duration {
	if e.KillGrace > 0 {
		return e.KillGrace
	}
	return DefaultKillGrace
}

func buildArgv(binary string, args []string) []string {
	base := filepath.Base(binary)
	out := make([]string, 0, 1+len(args))
	out = append(out, base)
	out = append(out, args...)
	return out
}

func exitCodeFrom(b BoundStartResult) int {
	if b.ExitCode != 0 {
		return b.ExitCode
	}
	if b.err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(b.err, &ee) {
		return ee.ExitCode()
	}
	// signal kill / start failure
	if b.PID != 0 {
		return -1
	}
	return -1
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
