package toolrun

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// FailClassCWDRestore is the stable fail-class token for cwd restore failure
// (P2.2.b / Section 34.4). Diagnostic registry id remains tool.failed
// (Appendix D); this class appears in step logs and BoundStartResult.FailClass.
const FailClassCWDRestore = "tool.cwd_restore_failed"

// FailClassFchdirStage classifies failure to fchdir into the stage descriptor.
const FailClassFchdirStage = "tool.fchdir_stage_failed"

// FailClassEnvSize classifies an environment block that exceeds ARG_MAX.
const FailClassEnvSize = "tool.env_size_exceeded"

// envSliceByteSize returns an upper-bound byte count for an environment
// vector, counting the null terminator for each entry.
func envSliceByteSize(env []string) int64 {
	var n int64
	for _, e := range env {
		n += int64(len(e)) + 1
	}
	return n
}

// argMaxBytes returns the host ARG_MAX limit. It can be overridden by tests
// through BoundStarterOptions.EnvSizeLimit.
func argMaxBytes(s *BoundStarter) int64 {
	if s.envSizeLimit > 0 {
		return s.envSizeLimit
	}
	return sysconfARGMAX()
}

// childStartMu is the process-wide child-start mutex (Section 34.4 step 1).
// All BoundStarter instances serialize on this lock so fchdir pairs never interleave.
var childStartMu sync.Mutex

// OriginalCWD holds the Foundry process startup working-directory descriptor
// retained for fchdir restore after every bound child start (Section 34.4).
//
// Capture once at toolrun/foundry init via CaptureOriginalCWD; never re-open
// by pathname for restore purposes.
type OriginalCWD struct {
	fd int
}

// FD returns the retained directory file descriptor (read-only for callers).
// Returns -1 when closed or unset.
func (o *OriginalCWD) FD() int {
	if o == nil {
		return -1
	}
	return o.fd
}

// BoundStartRequest is one descriptor-bound child start (Section 34.4).
//
// StageFD is a directory file descriptor owned by the transaction (from
// fsx.Transaction.DuplicateStageHandle when fsx is wired). toolrun never
// closes StageFD and never re-opens the stage by pathname for cwd purposes.
type BoundStartRequest struct {
	// StageFD is an open O_DIRECTORY descriptor for the stage object.
	StageFD int
	// Binary is the absolute path of the executable (preflight-resolved).
	Binary string
	// Args are argv[1:] (binary path is separate).
	Args []string
	// Env is the full constructed environment slice (Section 34.2); nil means
	// inherit is prohibited — production always passes an explicit slice.
	Env []string
	// StepID is the Appendix E step id for diagnostic location (e.g. go-mod-tidy).
	StepID string
	// OutputCapBytes is the per-stream capture cap (Section 34.3). Zero uses
	// DefaultOutputCapBytes (4 MiB). Applied during capture on the production
	// path and re-applied by Executor as a safety net.
	OutputCapBytes int
	// KillGrace is the cancel→SIGKILL grace for the child process group.
	// Zero uses DefaultKillGrace. Executor fills this from its KillGrace field.
	KillGrace time.Duration
}

// BoundStartLog is the detailed, non-secret record of one bound start.
// Never includes full host home paths — BinaryBase is filepath.Base only.
type BoundStartLog struct {
	BinaryBase    string
	StageFD       int
	StageDev      uint64
	StageIno      uint64
	LockWaitMs    int64
	FchdirStageOK bool
	FchdirErrno   string // stage fchdir errno text if any
	RestoreOK     bool
	RestoreErrno  string
	ChildPID      int
	// DirSet is the pathname Dir applied to the child (must always be empty).
	DirSet string
	// OutputCap is the per-stream cap applied (bytes).
	OutputCap int
	// KillGraceMs is the configured process-group kill grace (ms).
	KillGraceMs int64
}

// BoundStartResult is the outcome of Start (wait included).
type BoundStartResult struct {
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
	StdoutBytes     int64 // total offered before cap (best-effort)
	StderrBytes     int64
	PID             int
	ExitCode        int // 0 on success; -1 when unknown / signal; else process exit
	Log             BoundStartLog
	FailClass       string
	err             error
}

// Err returns the start/wait/restore failure, if any.
func (r BoundStartResult) Err() error { return r.err }

// OK reports whether the child started, restore succeeded, and wait returned nil.
func (r BoundStartResult) OK() bool {
	return r.err == nil && r.FailClass == ""
}

// LogDetail returns a stable single-line detail for step logging (no host homes).
func (l BoundStartLog) LogDetail() string {
	restore := "ok"
	if !l.RestoreOK {
		restore = "fail"
		if l.RestoreErrno != "" {
			restore += ":" + l.RestoreErrno
		}
	}
	stage := "ok"
	if !l.FchdirStageOK {
		stage = "fail"
		if l.FchdirErrno != "" {
			stage += ":" + l.FchdirErrno
		}
	}
	return fmt.Sprintf(
		"binary=%s stage_fd=%d stage_dev=%d stage_ino=%d lock_wait_ms=%d fchdir_stage=%s restore=%s child_pid=%d dir=%q output_cap=%d kill_grace_ms=%d",
		l.BinaryBase, l.StageFD, l.StageDev, l.StageIno, l.LockWaitMs,
		stage, restore, l.ChildPID, l.DirSet, l.OutputCap, l.KillGraceMs,
	)
}

// childProc is a started child waiting to be Wait'ed after cwd restore.
type childProc interface {
	PID() int
	Wait() (stdout, stderr []byte, stdoutTrunc, stderrTrunc bool, stdoutN, stderrN int64, err error)
}

// startFunc starts a child with Dir exactly "" and returns a wait handle.
// Production uses exec.Cmd; tests inject fakes that assert Dir empty.
// capBytes/killGrace come from BoundStartRequest (plan-declared caps / Section 34.3).
type startFunc func(ctx context.Context, binary string, args, env []string, dir string, capBytes int, killGrace time.Duration) (childProc, error)

// BoundStarter runs the Section 34.4 fchdir protocol for transactional children.
//
// Production path:
//  1. Acquire process-wide child-start mutex
//  2. fchdir(stage_fd)
//  3. Start child with empty pathname Dir (inherits descriptor-bound cwd)
//  4. fchdir(original_fd) — restore; failure is fail-closed
//  5. Release mutex
//  6. Wait for child (outside the fchdir critical section)
//
// Test hooks (BoundStarterOptions) inject fchdir errors, pathname swaps during
// the race window, and critical-section probes for mutex serialization tests.
type BoundStarter struct {
	orig *OriginalCWD

	// fchdirFn defaults to platform fchdir.
	fchdirFn func(fd int) error
	// afterStage runs after successful fchdir(stage) and before Start (tests).
	afterStage func() error
	// duringCS runs while holding the mutex after stage fchdir (mutex tests).
	duringCS func()
	// onCommand observes Dir/binary/args immediately before Start (tests).
	onCommand func(dir, binary string, args []string)
	// startChild runs the child Start only; nil uses real exec (production).
	startChild startFunc
	// envSizeLimit overrides ARG_MAX detection for env-size tests. Zero uses
	// the host sysconf value.
	envSizeLimit int64
}

// BoundStarterOptions configures test hooks. All fields are nil in production.
type BoundStarterOptions struct {
	// Fchdir overrides the platform fchdir (inject restore / stage failures).
	Fchdir func(fd int) error
	// AfterStageFchdir runs after fchdir(stage) and before child Start —
	// used to prove pathname swap cannot redirect the child (SV-03).
	AfterStageFchdir func() error
	// DuringCriticalSection runs while holding the child-start mutex after
	// stage fchdir (mutex contention / exclusivity probes).
	DuringCriticalSection func()
	// OnCommand is invoked with the Dir that will be applied (must be "")
	// plus binary and args, immediately before Start.
	OnCommand func(dir, binary string, args []string)
	// StartChild replaces real exec.Start for pure unit tests of Dir/mutex
	// plumbing. When set, the real OS process is not started. dir is always "".
	// The returned child's Wait is invoked after restore + mutex release.
	// capBytes is the plan-declared per-stream cap (for tests that emit large output).
	StartChild func(ctx context.Context, binary string, args, env []string, dir string, capBytes int, killGrace time.Duration) (pid int, wait func() (stdout, stderr []byte, err error), err error)
	// EnvSizeLimit overrides ARG_MAX detection for env-size tests. Zero uses the
	// host sysconf value.
	EnvSizeLimit int64
}

// NewBoundStarter builds a starter bound to a retained OriginalCWD.
// orig must be non-nil with a live FD (from CaptureOriginalCWD).
func NewBoundStarter(orig *OriginalCWD, opts BoundStarterOptions) (*BoundStarter, error) {
	if orig == nil || orig.fd < 0 {
		return nil, fmt.Errorf("toolrun: OriginalCWD is required for bound child start")
	}
	s := &BoundStarter{
		orig:         orig,
		fchdirFn:     opts.Fchdir,
		afterStage:   opts.AfterStageFchdir,
		duringCS:     opts.DuringCriticalSection,
		onCommand:    opts.OnCommand,
		envSizeLimit: opts.EnvSizeLimit,
	}
	if s.fchdirFn == nil {
		s.fchdirFn = platformFchdir
	}
	if opts.StartChild != nil {
		s.startChild = func(ctx context.Context, binary string, args, env []string, dir string, capBytes int, killGrace time.Duration) (childProc, error) {
			pid, wait, err := opts.StartChild(ctx, binary, args, env, dir, capBytes, killGrace)
			if err != nil {
				return nil, err
			}
			return &fakeChild{pid: pid, wait: wait, capBytes: capBytes}, nil
		}
	} else {
		s.startChild = platformStartChild
	}
	return s, nil
}

type fakeChild struct {
	pid      int
	capBytes int
	wait     func() (stdout, stderr []byte, err error)
}

func (c *fakeChild) PID() int { return c.pid }
func (c *fakeChild) Wait() (stdout, stderr []byte, stdoutTrunc, stderrTrunc bool, stdoutN, stderrN int64, err error) {
	if c.wait == nil {
		return nil, nil, false, false, 0, 0, nil
	}
	out, errb, err := c.wait()
	stdoutN = int64(len(out))
	stderrN = int64(len(errb))
	capN := c.capBytes
	if capN <= 0 {
		capN = DefaultOutputCapBytes
	}
	out, stdoutTrunc = CapBytes(out, capN)
	errb, stderrTrunc = CapBytes(errb, capN)
	return out, errb, stdoutTrunc, stderrTrunc, stdoutN, stderrN, err
}

// Start runs the descriptor-bound child protocol and waits for completion.
//
// Pathname Dir is never set. Restore failure surfaces as FailClassCWDRestore
// with diagnostic id tool.failed; the process cwd is always restored (or the
// attempt is made) before the mutex is released so callers never continue
// with the process cwd left on the stage.
func (s *BoundStarter) Start(ctx context.Context, req BoundStartRequest) BoundStartResult {
	capN := req.OutputCapBytes
	if capN <= 0 {
		capN = DefaultOutputCapBytes
	}
	grace := req.KillGrace
	if grace <= 0 {
		grace = DefaultKillGrace
	}
	res := BoundStartResult{
		ExitCode: -1,
		Log: BoundStartLog{
			BinaryBase:  filepath.Base(req.Binary),
			StageFD:     req.StageFD,
			DirSet:      "", // invariant: never a pathname
			OutputCap:   capN,
			KillGraceMs: grace.Milliseconds(),
		},
	}
	if s == nil || s.orig == nil || s.orig.fd < 0 {
		res.FailClass = FailClassFchdirStage
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"bound child start: OriginalCWD not initialized",
		)
		return res
	}
	if req.StageFD < 0 {
		res.FailClass = FailClassFchdirStage
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"bound child start: invalid stage fd %d", req.StageFD,
		)
		return res
	}
	if req.Binary == "" {
		res.FailClass = FailClassFchdirStage
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"bound child start: empty binary",
		)
		return res
	}
	if envBytes := envSliceByteSize(req.Env); envBytes > argMaxBytes(s) {
		res.FailClass = FailClassEnvSize
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"environment size %d exceeds ARG_MAX %d for step %s",
			envBytes, argMaxBytes(s), req.StepID,
		).WithRemediation(
			"Reduce the size of the tool environment block (fewer/larger values " +
				"are not under user control). This is a host-limit error, not a tool bug.",
		)
		return res
	}

	// Stage identity for logs (best-effort; failure does not abort start).
	if id, err := platformFileID(req.StageFD); err == nil {
		res.Log.StageDev = id.Dev
		res.Log.StageIno = id.Ino
	}

	// --- critical section under process-wide mutex ---
	var (
		child     childProc
		startErr  error
		restoreOK bool
	)
	func() {
		waitStart := time.Now()
		childStartMu.Lock()
		defer childStartMu.Unlock()
		res.Log.LockWaitMs = time.Since(waitStart).Milliseconds()

		// 2. fchdir(stage_fd)
		if err := s.fchdirFn(req.StageFD); err != nil {
			res.Log.FchdirStageOK = false
			res.Log.FchdirErrno = errnoString(err)
			res.FailClass = FailClassFchdirStage
			res.err = diagnostic.Wrapf(
				diagnostic.IDToolFailed,
				diagnostic.StepLocation(req.StepID),
				err,
				"fchdir to stage fd %d failed (binary %s)",
				req.StageFD, res.Log.BinaryBase,
			).WithRemediation(
				"Stage directory descriptor is unusable for child cwd binding. " +
					"Inspect the preserved stage and re-run; do not set exec.Cmd.Dir to a pathname.",
			)
			return
		}
		res.Log.FchdirStageOK = true

		// Test hooks: pathname swap / contention while cwd is descriptor-bound.
		if s.duringCS != nil {
			s.duringCS()
		}
		if s.afterStage != nil {
			if err := s.afterStage(); err != nil {
				// Always attempt restore before leaving the critical section.
				res.err = s.restoreOrFail(&res, req.StepID, err, "after_stage_hook")
				return
			}
		}

		// 3. Start child with no pathname Dir.
		const dir = ""
		res.Log.DirSet = dir
		if s.onCommand != nil {
			s.onCommand(dir, req.Binary, req.Args)
		}

		child, startErr = s.startChild(ctx, req.Binary, req.Args, req.Env, dir, capN, grace)
		if child != nil {
			res.PID = child.PID()
			res.Log.ChildPID = res.PID
		}

		// 4. fchdir(original_fd) — restore before unlock / wait so the
		// Foundry process never continues with cwd left on the stage.
		restoreErr := s.fchdirFn(s.orig.fd)
		if restoreErr != nil {
			res.Log.RestoreOK = false
			res.Log.RestoreErrno = errnoString(restoreErr)
			res.FailClass = FailClassCWDRestore
			if startErr != nil {
				res.err = diagnostic.Wrapf(
					diagnostic.IDToolFailed,
					diagnostic.StepLocation(req.StepID),
					restoreErr,
					"cwd restore failed after child start (binary %s): restore: %v; start: %v",
					res.Log.BinaryBase, restoreErr, startErr,
				).WithRemediation(cwdRestoreRemediation())
			} else {
				res.err = diagnostic.Wrapf(
					diagnostic.IDToolFailed,
					diagnostic.StepLocation(req.StepID),
					restoreErr,
					"cwd restore failed after child start (binary %s, pid %d)",
					res.Log.BinaryBase, res.PID,
				).WithRemediation(cwdRestoreRemediation())
			}
			// Still wait below if child started, so we do not leak zombies.
			return
		}
		res.Log.RestoreOK = true
		restoreOK = true
	}()

	// If stage fchdir / hook failed without starting a child, we are done.
	if child == nil {
		if res.err == nil && startErr != nil {
			res.FailClass = string(diagnostic.IDToolFailed)
			res.err = diagnostic.Wrapf(
				diagnostic.IDToolFailed,
				diagnostic.StepLocation(req.StepID),
				startErr,
				"external step %s binary %s failed to start: %v",
				req.StepID, res.Log.BinaryBase, startErr,
			)
		}
		return res
	}

	// 5. Wait outside the fchdir critical section (mutex already released).
	stdout, stderr, outTrunc, errTrunc, outN, errN, waitErr := child.Wait()
	res.Stdout = stdout
	res.Stderr = stderr
	res.StdoutTruncated = outTrunc
	res.StderrTruncated = errTrunc
	res.StdoutBytes = outN
	res.StderrBytes = errN

	// Prefer restore failure as the fail-closed signal even if wait succeeds.
	if res.FailClass == FailClassCWDRestore && res.err != nil {
		return res
	}
	if startErr != nil {
		res.FailClass = string(diagnostic.IDToolFailed)
		res.err = diagnostic.Wrapf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			startErr,
			"external step %s binary %s failed to start: %v",
			req.StepID, res.Log.BinaryBase, startErr,
		)
		return res
	}
	if waitErr != nil {
		res.FailClass = string(diagnostic.IDToolFailed)
		res.err = waitErr
		// Prefer raw ExitError so Executor can extract exit code; wrap only when needed.
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			res.ExitCode = ee.ExitCode()
		}
		return res
	}
	res.ExitCode = 0
	if !restoreOK && res.err == nil {
		// Defensive: restore path should have set res.err already.
		res.FailClass = FailClassCWDRestore
		res.err = diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(req.StepID),
			"cwd restore failed after child start (binary %s)", res.Log.BinaryBase,
		).WithRemediation(cwdRestoreRemediation())
	}
	return res
}

// restoreOrFail attempts orig fchdir after a mid-protocol failure.
// Must be called while holding childStartMu.
func (s *BoundStarter) restoreOrFail(res *BoundStartResult, stepID string, cause error, where string) error {
	restoreErr := s.fchdirFn(s.orig.fd)
	if restoreErr != nil {
		res.Log.RestoreOK = false
		res.Log.RestoreErrno = errnoString(restoreErr)
		res.FailClass = FailClassCWDRestore
		return diagnostic.Wrapf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(stepID),
			restoreErr,
			"cwd restore failed after %s: restore: %v; prior: %v",
			where, restoreErr, cause,
		).WithRemediation(cwdRestoreRemediation())
	}
	res.Log.RestoreOK = true
	res.FailClass = string(diagnostic.IDToolFailed)
	return diagnostic.Wrapf(
		diagnostic.IDToolFailed,
		diagnostic.StepLocation(stepID),
		cause,
		"bound child start aborted at %s: %v", where, cause,
	)
}

func cwdRestoreRemediation() string {
	return "Foundry failed closed after cwd restore failure (Section 34.4). " +
		"Do not continue with an unknown process working directory. " +
		"Inspect the preserved stage, restart the Foundry process, and re-run. " +
		"Never set exec.Cmd.Dir to a stage pathname as a workaround."
}

// FileID is a device/inode pair for stage identity logging.
type FileID struct {
	Dev uint64
	Ino uint64
}
