//go:build unix

package e1

import (
	"bytes"
	"fmt"
	"os/exec"
	"sync"

	"golang.org/x/sys/unix"
)

// childStartMu serializes descriptor-bound child startup (Section 34.4 step 1).
var childStartMu sync.Mutex

// OriginalCWD holds the Foundry startup working-directory descriptor (34.4).
type OriginalCWD struct {
	FD int
}

// CaptureOriginalCWD opens "." once at startup for later fchdir restore.
func CaptureOriginalCWD() (*OriginalCWD, error) {
	fd, err := unix.Open(".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return &OriginalCWD{FD: fd}, nil
}

// Close releases the original-cwd descriptor.
func (o *OriginalCWD) Close() error {
	if o == nil || o.FD < 0 {
		return nil
	}
	err := unix.Close(o.FD)
	o.FD = -1
	return err
}

// RunInStage starts a child with working directory bound to stageFD via the
// serialized fchdir protocol (Section 34.4). Cmd.Dir is never set.
//
// If swapDuringStart is non-nil, it runs after fchdir(stage) and before
// cmd.Start — used to prove pathname swap cannot redirect the child (SV-03).
func RunInStage(orig *OriginalCWD, stageFD int, argv []string, swapDuringStart func() error, log *ProbeLog, fsLabel string) (stdout, stderr []byte, err error) {
	if len(argv) == 0 {
		return nil, nil, fmt.Errorf("empty argv")
	}
	childStartMu.Lock()
	defer childStartMu.Unlock()

	// 2. fchdir(stage_fd)
	if err := unix.Fchdir(stageFD); err != nil {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "fchdir_stage",
			Syscall: "fchdir", Args: fmt.Sprintf("stage_fd=%d", stageFD),
			Errno: errnoString(err), Outcome: "fail"})
		return nil, nil, err
	}
	log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "fchdir_stage",
		Syscall: "fchdir", Args: fmt.Sprintf("stage_fd=%d", stageFD), Outcome: "pass"})

	// Hostile pathname swap while our cwd is descriptor-bound.
	if swapDuringStart != nil {
		if err := swapDuringStart(); err != nil {
			// Always attempt restore before returning.
			_ = unix.Fchdir(orig.FD)
			log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "pathname_swap",
				Outcome: "fail", Detail: err.Error()})
			return nil, nil, err
		}
		log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "pathname_swap",
			Outcome: "pass", Detail: "pathname renamed under retained stage fd"})
	}

	// 3. Start child with no Dir field — inherits descriptor-bound cwd.
	cmd := exec.Command(argv[0], argv[1:]...)
	// Explicitly leave cmd.Dir empty (pathname Dir is prohibited).
	cmd.Dir = ""
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	startErr := cmd.Start()
	if startErr != nil {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "start",
			Syscall: "exec", Args: fmt.Sprintf("%v", argv), Errno: startErr.Error(), Outcome: "fail"})
	} else {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "start",
			Syscall: "exec", Args: fmt.Sprintf("%v Dir=<empty>", argv), Outcome: "pass"})
	}

	// 4. fchdir(original_fd) — restore before waiting so we never continue with unknown cwd.
	restoreErr := unix.Fchdir(orig.FD)
	if restoreErr != nil {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "fchdir_restore",
			Syscall: "fchdir", Args: fmt.Sprintf("orig_fd=%d", orig.FD),
			Errno: errnoString(restoreErr), Outcome: "fail"})
		// Fail closed after in-flight step (34.4 step 5).
	} else {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "fchdir_restore",
			Syscall: "fchdir", Args: fmt.Sprintf("orig_fd=%d", orig.FD), Outcome: "pass"})
	}

	if startErr != nil {
		if restoreErr != nil {
			return outBuf.Bytes(), errBuf.Bytes(), fmt.Errorf("start: %w; restore: %v", startErr, restoreErr)
		}
		return outBuf.Bytes(), errBuf.Bytes(), startErr
	}

	waitErr := cmd.Wait()
	if restoreErr != nil {
		// Prefer restore failure as fail-closed signal.
		return outBuf.Bytes(), errBuf.Bytes(), spikeErr(ErrCommitFailed, "cwd restore failed after child start", restoreErr)
	}
	if waitErr != nil {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "wait",
			Outcome: "fail", Detail: waitErr.Error()})
		return outBuf.Bytes(), errBuf.Bytes(), waitErr
	}
	log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "wait", Outcome: "pass"})
	return outBuf.Bytes(), errBuf.Bytes(), nil
}

// ProvePathnameSwap renames the stage basename under parent while a retained
// stage FD is the process cwd, then verifies the child still sees stage content.
func ProvePathnameSwap(parent *ParentHandle, stage *Stage, orig *OriginalCWD, log *ProbeLog, fsLabel string) error {
	// Place a sentinel file in the stage via os.Root.
	const sentinel = "e1-cwd-sentinel.txt"
	const payload = "descriptor-bound-cwd-ok\n"
	if err := stage.WriteFile(sentinel, []byte(payload), 0600); err != nil {
		return err
	}

	swappedName := stage.Name + ".swapped-path"
	swap := func() error {
		// Rename the directory entry relative to parent — pathname changes,
		// retained stage FD still identifies the same object (SV-03).
		err := unix.Renameat(parent.FD, stage.Name, parent.FD, swappedName)
		if err != nil {
			return err
		}
		// Update stage name so subsequent commit probes know the entry name.
		stage.Name = swappedName
		return nil
	}

	// Child: read the sentinel via relative path (inherits cwd = stage object).
	stdout, stderr, err := RunInStage(orig, stage.FD, []string{"/bin/cat", sentinel}, swap, log, fsLabel)
	if err != nil {
		return fmt.Errorf("child after pathname swap: %w stderr=%s", err, stderr)
	}
	if string(stdout) != payload {
		return fmt.Errorf("child cwd not bound to stage object: got %q want %q", stdout, payload)
	}
	log.Record(ProbeEntry{FS: fsLabel, Probe: "child_cwd", Step: "swap_proof",
		Outcome: "pass", Detail: "child read sentinel after pathname rename of stage entry"})
	return nil
}
