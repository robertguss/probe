//go:build unix

package toolrun

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// CaptureOriginalCWD opens "." once as an O_DIRECTORY|O_CLOEXEC descriptor
// for later fchdir restore (Section 34.4). Call at toolrun/foundry init.
func CaptureOriginalCWD() (*OriginalCWD, error) {
	fd, err := unix.Open(".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("toolrun: capture original cwd: %w", err)
	}
	return &OriginalCWD{fd: fd}, nil
}

// Close releases the original-cwd descriptor.
func (o *OriginalCWD) Close() error {
	if o == nil || o.fd < 0 {
		return nil
	}
	err := unix.Close(o.fd)
	o.fd = -1
	return err
}

func platformFchdir(fd int) error {
	return unix.Fchdir(fd)
}

func platformFileID(fd int) (FileID, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return FileID{}, err
	}
	return FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, nil
}

// execChild wraps *exec.Cmd after Start for Wait outside the fchdir critical section.
// Streams are captured through cappedWriter so the plan-declared output cap is
// applied during capture (Section 34.3 / REQ-154).
type execChild struct {
	cmd    *exec.Cmd
	outCap *cappedWriter
	errCap *cappedWriter
	pid    int
	// killGrace is retained for process-group kill on cancel (CommandContext
	// already signals the process; Setpgid ensures the group is addressable).
	killGrace time.Duration
	// done is closed when Wait() completes so the cancel-kill goroutine exits
	// promptly on the happy path (no context cancellation).
	done chan struct{}
}

func (c *execChild) PID() int { return c.pid }

func (c *execChild) Wait() (stdout, stderr []byte, stdoutTrunc, stderrTrunc bool, stdoutN, stderrN int64, err error) {
	err = c.cmd.Wait()
	// Signal the cancel-kill goroutine to exit (no-op if already closed).
	if c.done != nil {
		close(c.done)
	}
	stdout, stdoutTrunc = c.outCap.Bytes()
	stderr, stderrTrunc = c.errCap.Bytes()
	stdoutN = c.outCap.TotalOffered()
	stderrN = c.errCap.TotalOffered()
	return stdout, stderr, stdoutTrunc, stderrTrunc, stdoutN, stderrN, err
}

// platformStartChild starts a child with Dir exactly "" (pathname Dir prohibited).
// The child's cwd is whatever the process currently has via fchdir.
// Wait is invoked by BoundStarter after restore + mutex release.
//
// capBytes bounds each stream during capture. killGrace is retained for
// process-group kill on cancel (CommandContext already signals the process;
// Setpgid ensures the group is addressable for grandchild cleanup).
func platformStartChild(ctx context.Context, binary string, args, env []string, dir string, capBytes int, killGrace time.Duration) (childProc, error) {
	if dir != "" {
		// Hard guard: production and tests must never pass a pathname Dir.
		return nil, fmt.Errorf("toolrun: pathname Dir %q prohibited (Section 34.4)", dir)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	// Explicit empty Dir — inherits descriptor-bound cwd.
	cmd.Dir = ""
	cmd.Env = env
	// Own process group so cancel can SIGTERM/SIGKILL the whole tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if killGrace <= 0 {
		killGrace = DefaultKillGrace
	}
	outCap := newCappedWriter(capBytes)
	errCap := newCappedWriter(capBytes)
	cmd.Stdout = outCap
	cmd.Stderr = errCap

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := 0
	done := make(chan struct{})
	if cmd.Process != nil {
		pid = cmd.Process.Pid
		// On context cancel after Start: kill the process group with grace.
		// The goroutine exits when either the context is cancelled (and kill
		// completes) or the child exits normally (done is closed in Wait).
		go func(p int, grace time.Duration) {
			select {
			case <-ctx.Done():
				if ctx.Err() == nil {
					return
				}
				killProcessGroup(p, grace)
			case <-done:
				return
			}
		}(pid, killGrace)
	}
	return &execChild{
		cmd:       cmd,
		outCap:    outCap,
		errCap:    errCap,
		pid:       pid,
		killGrace: killGrace,
		done:      done,
	}, nil
}

func errnoString(err error) string {
	if err == nil {
		return ""
	}
	var errno unix.Errno
	if errors.As(err, &errno) {
		return errno.Error()
	}
	var errno2 syscall.Errno
	if errors.As(err, &errno2) {
		return errno2.Error()
	}
	return err.Error()
}

// OpenDirFD opens path as an O_DIRECTORY|O_CLOEXEC descriptor for tests and
// for bridging until fsx.DuplicateStageHandle is available. Production
// transactional starts must pass the stage FD from fsx, never a re-opened path
// of a live stage under race (tests use this only for fixture setup).
func OpenDirFD(path string) (int, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	return fd, nil
}

// CloseFD closes a directory FD opened by OpenDirFD (or test fixtures).
func CloseFD(fd int) error {
	if fd < 0 {
		return nil
	}
	return unix.Close(fd)
}
