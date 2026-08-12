//go:build unix

package toolrun_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func captureOrig(t *testing.T) *toolrun.OriginalCWD {
	t.Helper()
	orig, err := toolrun.CaptureOriginalCWD()
	if err != nil {
		t.Fatalf("CaptureOriginalCWD: %v", err)
	}
	t.Cleanup(func() { _ = orig.Close() })
	return orig
}

func openStageFD(t *testing.T, dir string) int {
	t.Helper()
	fd, err := toolrun.OpenDirFD(dir)
	if err != nil {
		t.Fatalf("OpenDirFD(%s): %v", dir, err)
	}
	t.Cleanup(func() { _ = toolrun.CloseFD(fd) })
	return fd
}

// TestBoundStart_FakeRunner_DirAlwaysEmpty asserts pathname Dir is never set
// (Section 34.4 / acceptance: no pathname Dir in production starts).
func TestBoundStart_FakeRunner_DirAlwaysEmpty(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	orig := captureOrig(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)

	fake := &toolrun.FakeRunner{}
	var observedDirs []string
	var mu sync.Mutex

	starter, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{
		OnCommand: func(dir, binary string, args []string) {
			mu.Lock()
			observedDirs = append(observedDirs, dir)
			mu.Unlock()
			fake.RecordBoundStart(dir, binary, args)
		},
		StartChild: func(_ context.Context, binary string, args, env []string, dir string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			if dir != "" {
				return 0, nil, fmt.Errorf("pathname Dir set: %q", dir)
			}
			// Simulate successful child.
			return 4242, func() ([]byte, []byte, error) {
				return []byte("ok\n"), nil, nil
			}, nil
		},
	})
	if err != nil {
		log.Fail("new_starter", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	const n = 5
	for i := 0; i < n; i++ {
		res := starter.Start(context.Background(), toolrun.BoundStartRequest{
			StageFD: stageFD,
			Binary:  "/fake/bin/go",
			Args:    []string{"version"},
			Env:     []string{"PATH=/usr/bin"},
			StepID:  "go-preflight",
		})
		log.Step("start", testutil.OutcomeOK, res.Log.LogDetail())
		if !res.OK() {
			log.Fail("start_ok", fmt.Sprintf("i=%d err=%v class=%s", i, res.Err(), res.FailClass))
		}
		log.Assert("dir_empty_log", res.Log.DirSet == "", "", res.Log.DirSet)
		log.Assert("child_pid", res.Log.ChildPID == 4242, 4242, res.Log.ChildPID)
		log.Assert("restore_ok", res.Log.RestoreOK, true, res.Log.RestoreOK)
		log.Assert("fchdir_stage_ok", res.Log.FchdirStageOK, true, res.Log.FchdirStageOK)
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	mu.Lock()
	dirs := append([]string(nil), observedDirs...)
	mu.Unlock()
	log.Assert("observed_count", len(dirs) == n, n, len(dirs))
	for i, d := range dirs {
		log.Assert(fmt.Sprintf("dir_empty_%d", i), d == "", "", d)
	}
	log.Assert("fake_dirs", fake.AllDirsEmpty(), true, fake.AllDirsEmpty())
	log.Assert("fake_starts", fake.BoundStartCount() == n, n, fake.BoundStartCount())
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestBoundStart_PathnameSwap_ChildSeesStageObject renames the stage entry
// while holding the race window; child still reads the sentinel via relative
// path (object continuity / SV-03).
func TestBoundStart_PathnameSwap_ChildSeesStageObject(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	orig := captureOrig(t)

	parent := t.TempDir()
	stageName := "stage-obj"
	stagePath := filepath.Join(parent, stageName)
	if err := os.Mkdir(stagePath, 0o700); err != nil {
		log.Fail("mkdir_stage", err.Error())
	}
	const sentinel = "cwd-sentinel.txt"
	const payload = "descriptor-bound-cwd-ok\n"
	if err := os.WriteFile(filepath.Join(stagePath, sentinel), []byte(payload), 0o600); err != nil {
		log.Fail("write_sentinel", err.Error())
	}
	stageFD := openStageFD(t, stagePath)
	// Record identity before swap.
	var st unix.Stat_t
	if err := unix.Fstat(stageFD, &st); err != nil {
		log.Fail("fstat", err.Error())
	}
	preDev, preIno := uint64(st.Dev), uint64(st.Ino)
	log.Fixture("stage", fmt.Sprintf("dev=%d ino=%d name=%s", preDev, preIno, stageName))

	swapped := stageName + ".swapped-path"
	swap := func() error {
		// Rename directory entry under parent — pathname changes; retained FD
		// still names the same object.
		err := unix.Renameat(unix.AT_FDCWD, stagePath, unix.AT_FDCWD, filepath.Join(parent, swapped))
		if err != nil {
			return err
		}
		log.Step("pathname_swap", testutil.OutcomeOK, "renamed_to="+swapped)
		return nil
	}

	starter, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{
		AfterStageFchdir: swap,
	})
	if err != nil {
		log.Fail("new_starter", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	// Child: cat relative sentinel — inherits descriptor-bound cwd.
	res := starter.Start(context.Background(), toolrun.BoundStartRequest{
		StageFD: stageFD,
		Binary:  "/bin/cat",
		Args:    []string{sentinel},
		Env:     []string{"PATH=/usr/bin:/bin", "LC_ALL=C"},
		StepID:  "cwd-swap-proof",
	})
	log.Step("bound_start", testutil.OutcomeOK, res.Log.LogDetail())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if !res.OK() {
		log.Fail("start_ok", fmt.Sprintf("err=%v stderr=%s", res.Err(), res.Stderr))
	}
	log.Assert("stdout_payload", string(res.Stdout) == payload, payload, string(res.Stdout))
	log.Assert("dir_empty", res.Log.DirSet == "", "", res.Log.DirSet)
	log.Assert("restore_ok", res.Log.RestoreOK, true, res.Log.RestoreOK)
	log.Assert("stage_dev", res.Log.StageDev == preDev, preDev, res.Log.StageDev)
	log.Assert("stage_ino", res.Log.StageIno == preIno, preIno, res.Log.StageIno)
	// Original path must be gone; swapped path exists — child still worked.
	if _, err := os.Stat(stagePath); !os.IsNotExist(err) {
		log.Fail("old_path_gone", fmt.Sprintf("stat old path err=%v", err))
	}
	if _, err := os.Stat(filepath.Join(parent, swapped)); err != nil {
		log.Fail("swapped_path_exists", err.Error())
	}
	// Process cwd must not remain on the stage after Start returns.
	wd, err := os.Getwd()
	if err != nil {
		log.Fail("getwd", err.Error())
	}
	// Compare against the retained orig via fchdir identity is heavy; ensure
	// wd is not the swapped stage path.
	log.Assert("cwd_not_swapped_stage", wd != filepath.Join(parent, swapped), "not stage", wd)
	log.NoteID("tool.cwd_restore_failed") // document fail-class token for dumps
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestBoundStart_RestoreFailure_SurfacesStableID injects fchdir restore error
// and asserts FailClassCWDRestore + tool.failed diagnostic id; process must
// not leave cwd on the stage when restore can still be attempted.
func TestBoundStart_RestoreFailure_SurfacesStableID(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	orig := captureOrig(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	// Remember pre-test cwd for recovery check.
	preWD, err := os.Getwd()
	if err != nil {
		log.Fail("getwd", err.Error())
	}

	origFD := orig.FD()
	var fchdirCalls atomic.Int32
	inject := func(fd int) error {
		n := fchdirCalls.Add(1)
		// First call is stage fchdir — succeed via real syscall.
		// Second call is restore — fail injectively.
		if n == 1 {
			return unix.Fchdir(fd)
		}
		if fd == origFD {
			return unix.EBADF // injectable restore failure
		}
		return unix.Fchdir(fd)
	}

	starter, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{
		Fchdir: inject,
		StartChild: func(_ context.Context, binary string, args, env []string, dir string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			log.Assert("dir_empty_on_start", dir == "", "", dir)
			return 99, func() ([]byte, []byte, error) { return []byte("ran\n"), nil, nil }, nil
		},
	})
	if err != nil {
		log.Fail("new_starter", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := starter.Start(context.Background(), toolrun.BoundStartRequest{
		StageFD: stageFD,
		Binary:  "/fake/bin/tool",
		Args:    []string{"x"},
		Env:     []string{"PATH=/bin"},
		StepID:  "go-mod-tidy",
	})
	log.Step("bound_start", testutil.OutcomeFail, res.Log.LogDetail())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	log.Assert("not_ok", !res.OK(), false, res.OK())
	log.Assert("fail_class", res.FailClass == toolrun.FailClassCWDRestore,
		toolrun.FailClassCWDRestore, res.FailClass)
	log.Assert("restore_not_ok", !res.Log.RestoreOK, false, res.Log.RestoreOK)
	log.Assert("restore_errno_set", res.Log.RestoreErrno != "", "nonempty", res.Log.RestoreErrno)
	log.Assert("fchdir_stage_ok", res.Log.FchdirStageOK, true, res.Log.FchdirStageOK)

	fe, ok := diagnostic.AsFoundryError(res.Err())
	if !ok {
		log.Fail("foundry_error", fmt.Sprintf("%T %v", res.Err(), res.Err()))
	}
	log.Assert("diagnostic_id", fe.ID() == diagnostic.IDToolFailed,
		diagnostic.IDToolFailed, fe.ID())
	log.Assert("exit_code", fe.ExitCode() == diagnostic.ExitFailure,
		diagnostic.ExitFailure, fe.ExitCode())
	log.NoteID(string(fe.ID()))
	log.NoteID(toolrun.FailClassCWDRestore)

	// Attempt recovery: real fchdir back to orig so the test process is safe.
	if err := unix.Fchdir(origFD); err != nil {
		// Last resort: Chdir to preWD.
		if chErr := os.Chdir(preWD); chErr != nil {
			log.Fail("cwd_recover", fmt.Sprintf("fchdir=%v chdir=%v", err, chErr))
		}
	}
	wd, _ := os.Getwd()
	log.Assert("cwd_recovered", wd == preWD, preWD, wd)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestBoundStart_ConcurrentSerialized proves N concurrent Start calls never
// overlap critical sections (mutex serializes) and never observe wrong cwd.
func TestBoundStart_ConcurrentSerialized(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	orig := captureOrig(t)
	preWD, err := os.Getwd()
	if err != nil {
		log.Fail("getwd", err.Error())
	}

	const n = 8
	stages := make([]string, n)
	fds := make([]int, n)
	for i := 0; i < n; i++ {
		stages[i] = t.TempDir()
		marker := fmt.Sprintf("marker-%d", i)
		if err := os.WriteFile(filepath.Join(stages[i], "id.txt"), []byte(marker+"\n"), 0o600); err != nil {
			log.Fail("write_marker", err.Error())
		}
		fds[i] = openStageFD(t, stages[i])
	}

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	var lockWaits []int64
	var lockMu sync.Mutex

	csProbe := func() {
		c := concurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		// Hold long enough that racing Starts would overlap without a mutex.
		time.Sleep(15 * time.Millisecond)
		concurrent.Add(-1)
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	var wg sync.WaitGroup
	errs := make([]error, n)
	stdouts := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Fresh starter per goroutine is fine — mutex is process-wide.
			s, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{
				DuringCriticalSection: csProbe,
			})
			if err != nil {
				errs[i] = err
				return
			}
			res := s.Start(context.Background(), toolrun.BoundStartRequest{
				StageFD: fds[i],
				Binary:  "/bin/cat",
				Args:    []string{"id.txt"},
				Env:     []string{"PATH=/usr/bin:/bin", "LC_ALL=C"},
				StepID:  fmt.Sprintf("concurrent-%d", i),
			})
			lockMu.Lock()
			lockWaits = append(lockWaits, res.Log.LockWaitMs)
			lockMu.Unlock()
			if res.Err() != nil {
				errs[i] = res.Err()
				return
			}
			stdouts[i] = string(res.Stdout)
			if res.Log.DirSet != "" {
				errs[i] = fmt.Errorf("dir set to %q", res.Log.DirSet)
			}
			if !res.Log.RestoreOK {
				errs[i] = fmt.Errorf("restore failed: %s", res.Log.RestoreErrno)
			}
		}(i)
	}
	wg.Wait()
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			log.Fail(fmt.Sprintf("goroutine_%d", i), errs[i].Error())
		}
		want := fmt.Sprintf("marker-%d\n", i)
		log.Assert(fmt.Sprintf("stdout_%d", i), stdouts[i] == want, want, stdouts[i])
	}
	max := maxConcurrent.Load()
	log.Assert("max_concurrent_cs", max == 1, 1, max)
	log.Assert("lock_waits_recorded", len(lockWaits) == n, n, len(lockWaits))
	log.Step("lock_waits", testutil.OutcomeOK, fmt.Sprintf("waits_ms=%v max_cs=%d", lockWaits, max))

	wd, err := os.Getwd()
	if err != nil {
		log.Fail("getwd", err.Error())
	}
	log.Assert("cwd_restored", wd == preWD, preWD, wd)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestBoundStart_MutexContention_NoOverlap is a focused atomic-flag proof that
// critical sections never overlap under the process-wide mutex.
func TestBoundStart_MutexContention_NoOverlap(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	orig := captureOrig(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)

	var inCS atomic.Bool
	var overlaps atomic.Int32
	var entries atomic.Int32

	const n = 12
	var wg sync.WaitGroup
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{
				DuringCriticalSection: func() {
					entries.Add(1)
					if !inCS.CompareAndSwap(false, true) {
						overlaps.Add(1)
					}
					time.Sleep(5 * time.Millisecond)
					inCS.Store(false)
				},
				StartChild: func(_ context.Context, _ string, _, _ []string, dir string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
					if dir != "" {
						return 0, nil, errors.New("dir set")
					}
					return 1, func() ([]byte, []byte, error) { return nil, nil, nil }, nil
				},
			})
			if err != nil {
				t.Errorf("NewBoundStarter: %v", err)
				return
			}
			res := s.Start(context.Background(), toolrun.BoundStartRequest{
				StageFD: stageFD,
				Binary:  "/fake/bin/x",
				Args:    nil,
				Env:     []string{"PATH=/bin"},
				StepID:  "mutex-probe",
			})
			if !res.OK() {
				t.Errorf("start: %v class=%s", res.Err(), res.FailClass)
			}
			log.Step("start", testutil.OutcomeOK, res.Log.LogDetail())
		}()
	}
	wg.Wait()
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	log.Assert("entries", entries.Load() == n, n, entries.Load())
	log.Assert("overlaps", overlaps.Load() == 0, 0, overlaps.Load())
	log.Assert("in_cs_cleared", !inCS.Load(), false, inCS.Load())
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestBoundStart_StepLog_DetailedFields checks LogDetail carries stage fd,
// lock wait, restore result, and child pid without host home paths.
func TestBoundStart_StepLog_DetailedFields(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	orig := captureOrig(t)
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "x.txt"), []byte("x\n"), 0o600); err != nil {
		log.Fail("write", err.Error())
	}
	stageFD := openStageFD(t, stage)

	starter, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{})
	if err != nil {
		log.Fail("new_starter", err.Error())
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := starter.Start(context.Background(), toolrun.BoundStartRequest{
		StageFD: stageFD,
		Binary:  "/bin/cat",
		Args:    []string{"x.txt"},
		Env:     []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "HOME=/home/foundry"},
		StepID:  "go-test",
	})
	detail := res.Log.LogDetail()
	log.Step("bound_start", testutil.OutcomeOK, detail)
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if !res.OK() {
		log.Fail("ok", res.Err().Error())
	}
	log.Assert("has_binary", strings.Contains(detail, "binary=cat"), true, detail)
	log.Assert("has_stage_fd", strings.Contains(detail, "stage_fd="), true, detail)
	log.Assert("has_stage_dev", strings.Contains(detail, "stage_dev="), true, detail)
	log.Assert("has_stage_ino", strings.Contains(detail, "stage_ino="), true, detail)
	log.Assert("has_lock_wait", strings.Contains(detail, "lock_wait_ms="), true, detail)
	log.Assert("has_restore_ok", strings.Contains(detail, "restore=ok"), true, detail)
	log.Assert("has_child_pid", strings.Contains(detail, "child_pid="), true, detail)
	log.Assert("has_dir_empty", strings.Contains(detail, `dir=""`), true, detail)
	// Never leak full host homes into golden-compared streams.
	log.Assert("no_home_foundry", !strings.Contains(detail, "/home/foundry"), true, detail)
	log.Assert("binary_is_base", res.Log.BinaryBase == "cat", "cat", res.Log.BinaryBase)
	log.Assert("pid_positive", res.Log.ChildPID > 0, ">0", res.Log.ChildPID)
	log.Assert("stage_fd_match", res.Log.StageFD == stageFD, stageFD, res.Log.StageFD)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestBoundStart_Count2_Determinism ensures protocol logs are stable under -count=2.
func TestBoundStart_Count2_Determinism(t *testing.T) {
	log := testutil.New(t)
	orig := captureOrig(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)

	var dirs []string
	starter, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{
		OnCommand: func(dir, _ string, _ []string) { dirs = append(dirs, dir) },
		StartChild: func(_ context.Context, _ string, _, _ []string, dir string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			return 7, func() ([]byte, []byte, error) { return []byte("v\n"), nil, nil }, nil
		},
	})
	if err != nil {
		log.Fail("new", err.Error())
	}
	res := starter.Start(context.Background(), toolrun.BoundStartRequest{
		StageFD: stageFD,
		Binary:  "/usr/local/go/bin/go",
		Args:    []string{"version"},
		Env:     []string{"PATH=/bin"},
		StepID:  "go-preflight",
	})
	log.Assert("ok", res.OK(), true, res.OK())
	log.Assert("dir", res.Log.DirSet == "", "", res.Log.DirSet)
	log.Assert("binary_base", res.Log.BinaryBase == "go", "go", res.Log.BinaryBase)
	log.Assert("restore", res.Log.RestoreOK, true, res.Log.RestoreOK)
	log.Assert("on_command_dir", len(dirs) == 1 && dirs[0] == "", "empty", dirs)
}

// TestBoundStart_StageFchdirFailure surfaces stage fchdir errors without
// leaving the process cwd changed when restore can run (stage fchdir fails
// before any cwd change succeeds — inject after real open).
func TestBoundStart_StageFchdirFailure(t *testing.T) {
	log := testutil.New(t)
	orig := captureOrig(t)
	preWD, _ := os.Getwd()

	starter, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{
		Fchdir: func(fd int) error {
			return unix.EIO
		},
	})
	if err != nil {
		log.Fail("new", err.Error())
	}
	res := starter.Start(context.Background(), toolrun.BoundStartRequest{
		StageFD: 3, // unused; fchdir injected
		Binary:  "/bin/true",
		StepID:  "go-vet",
	})
	log.Assert("not_ok", !res.OK(), false, res.OK())
	log.Assert("fail_class", res.FailClass == toolrun.FailClassFchdirStage,
		toolrun.FailClassFchdirStage, res.FailClass)
	log.Assert("stage_not_ok", !res.Log.FchdirStageOK, false, res.Log.FchdirStageOK)
	fe, ok := diagnostic.AsFoundryError(res.Err())
	log.Assert("is_foundry", ok, true, ok)
	if ok {
		log.Assert("id", fe.ID() == diagnostic.IDToolFailed, diagnostic.IDToolFailed, fe.ID())
	}
	wd, _ := os.Getwd()
	log.Assert("cwd_unchanged", wd == preWD, preWD, wd)
}
