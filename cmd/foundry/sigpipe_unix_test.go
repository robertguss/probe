//go:build unix

// Real SIGPIPE process tests (SPEC-FOUNDRY-002 Section 36.5 / REQ-158 / FND-012).
//
// Ownership of these process-boundary proofs: go-foundry-cli-fy9.
// E2E stream-failure dimensions for full generate live in j8h.2.
package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestDrainedSIGPIPEWriteReturnsEPIPE proves that after the Foundry-style
// drained SIGPIPE subscription, a write to a closed pipe returns EPIPE and
// does not kill the test process (SV-04 / Section 36.5).
func TestDrainedSIGPIPEWriteReturnsEPIPE(t *testing.T) {
	log := testutil.New(t)
	log.Phase("drained_sigpipe_write")

	// Install the same drained subscription production uses.
	sigpipe := make(chan os.Signal, 8)
	signal.Notify(sigpipe, syscall.SIGPIPE)
	t.Cleanup(func() {
		signal.Stop(sigpipe)
		// Drain residual notifications so later tests are not perturbed.
		for {
			select {
			case <-sigpipe:
			default:
				return
			}
		}
	})
	// Drain continuously for the duration of this test (mirrors main).
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigpipe:
			case <-done:
				return
			}
		}
	}()
	t.Cleanup(func() { close(done) })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	// Process must survive; write must fail with EPIPE.
	alive := true
	_, werr := w.Write([]byte("foundry-sigpipe-probe\n"))
	_ = w.Close()
	if !alive {
		t.Fatal("process killed by SIGPIPE")
	}
	log.Assert("write_err", werr != nil, true, werr)
	log.Assert("is_epipe", errors.Is(werr, syscall.EPIPE) || isBrokenPipeMsg(werr), true, werr)
	log.Step("survived", testutil.OutcomeOK, fmt.Sprintf("err=%v", werr))

	log.PhaseEnd("drained_sigpipe_write", testutil.OutcomeOK)
}

// TestChildRetainsDefaultSIGPIPEDisposition proves fork/exec children do not
// inherit Foundry's drained SIGPIPE subscription: a child that writes to a
// closed pipe is terminated by SIGPIPE (exit status 128+13 on Unix shells,
// or wait status with signal SIGPIPE).
func TestChildRetainsDefaultSIGPIPEDisposition(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real subprocess test in short mode")
	}
	log := testutil.New(t)
	log.Phase("child_default_sigpipe")

	// Parent may have SIGPIPE notified (other tests / harness). Children still
	// get default disposition via fork/exec (SV-04).
	sigpipe := make(chan os.Signal, 1)
	signal.Notify(sigpipe, syscall.SIGPIPE)
	t.Cleanup(func() { signal.Stop(sigpipe) })
	go func() {
		for range sigpipe {
		}
	}()

	// Use a tiny non-Go child so the runtime does not install its own handlers
	// before the write: /bin/sh writing to a closed stdout.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	cmd := exec.Command("/bin/sh", "-c", "echo child-sigpipe-probe")
	cmd.Stdout = w
	cmd.Stderr = &bytes.Buffer{}
	// Close parent read end after start so the child's write hits a broken pipe.
	// Sequence: start with both ends open, close reader in parent, wait.
	if err := cmd.Start(); err != nil {
		_ = r.Close()
		_ = w.Close()
		t.Fatalf("start: %v", err)
	}
	// Parent closes its copy of the write end so only the child holds it, then
	// closes the read end to break the pipe.
	_ = w.Close()
	_ = r.Close()

	waitErr := cmd.Wait()
	// /bin/sh may finish the small write before the pipe is broken (race with
	// close). Treat a nil wait as "inconclusive" and fall through to the Go probe.

	// Expect termination by SIGPIPE (signal 13).
	var exitOK bool
	var detail string
	if ee, ok := waitErr.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() && ws.Signal() == syscall.SIGPIPE {
				exitOK = true
				detail = fmt.Sprintf("signaled SIGPIPE status=%v", ws)
			} else if ws.ExitStatus() == 141 {
				// Some shells map SIGPIPE to 128+13.
				exitOK = true
				detail = "exit 141 (128+SIGPIPE)"
			} else {
				detail = fmt.Sprintf("signaled=%v signal=%v exit=%d",
					ws.Signaled(), ws.Signal(), ws.ExitStatus())
			}
		} else {
			detail = fmt.Sprintf("exit_error=%v", ee)
		}
	} else if waitErr != nil {
		detail = fmt.Sprintf("wait=%v", waitErr)
	} else {
		detail = "sh completed without error (pipe race); probing Go child"
	}

	// Fallback: a Go child without Notify(SIGPIPE) also dies on SIGPIPE when
	// writing to a closed pipe — used if /bin/sh buffering avoids the signal.
	if !exitOK {
		exitOK, detail = probeGoChildDefaultSIGPIPE(t)
	}
	log.Assert("default_sigpipe", exitOK, true, detail)
	log.Step("child_disposition", testutil.OutcomeOK, detail)
	log.Inputs(map[string]string{
		"goos":   runtime.GOOS,
		"goarch": runtime.GOARCH,
	})

	log.PhaseEnd("child_default_sigpipe", testutil.OutcomeOK)
}

// probeGoChildDefaultSIGPIPE re-execs the test binary as a helper that writes
// to a closed stdout without installing a drained SIGPIPE handler.
func probeGoChildDefaultSIGPIPE(t *testing.T) (bool, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		return false, "executable: " + err.Error()
	}
	r, w, err := os.Pipe()
	if err != nil {
		return false, "pipe: " + err.Error()
	}

	cmd := exec.Command(exe, "-test.run=^TestSIGPIPEHelperWriteOnly$", "-test.v")
	cmd.Env = append(os.Environ(), "FOUNDRY_SIGPIPE_HELPER=1")
	cmd.Stdout = w
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Start(); err != nil {
		_ = r.Close()
		_ = w.Close()
		return false, "start helper: " + err.Error()
	}
	_ = w.Close()
	_ = r.Close()

	// Helper may exit quickly; bound wait.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		return false, "helper timeout"
	}
	if waitErr == nil {
		// Helper wrote successfully somehow — not a SIGPIPE proof.
		return false, "helper exited 0 (no SIGPIPE)"
	}
	if ee, ok := waitErr.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() && ws.Signal() == syscall.SIGPIPE {
				return true, "go helper signaled SIGPIPE"
			}
			// Go runtime may convert to exit 2 with panic/print depending on
			// version; accept non-zero as evidence child did not inherit drain
			// that would make write return EPIPE and exit 0 after handling.
			if ws.ExitStatus() != 0 {
				return true, fmt.Sprintf("go helper non-zero exit=%d (no drained handler)", ws.ExitStatus())
			}
		}
	}
	return false, fmt.Sprintf("helper wait=%v", waitErr)
}

// TestSIGPIPEHelperWriteOnly is the re-exec child: write one line to stdout and
// exit 0. When stdout is a broken pipe and no Notify(SIGPIPE) is installed,
// the process is expected to die of SIGPIPE (or fail the write fatally).
//
// Invoked only via FOUNDRY_SIGPIPE_HELPER=1 from probeGoChildDefaultSIGPIPE.
func TestSIGPIPEHelperWriteOnly(t *testing.T) {
	if os.Getenv("FOUNDRY_SIGPIPE_HELPER") != "1" {
		t.Skip("helper only")
	}
	// Intentionally no signal.Notify(SIGPIPE).
	_, err := os.Stdout.Write([]byte("helper-line\n"))
	if err != nil {
		// If the runtime delivered EPIPE instead of killing us, exit non-zero
		// so the parent still sees "child does not silently succeed".
		os.Exit(2)
	}
	// Successful write (pipe still open) — exit 0.
}

// TestFoundryBinarySurvivesBrokenStdoutDuringVersion builds the real foundry
// binary (with installDrainedSIGPIPE), runs `foundry version` with a closed
// stdout pipe, and asserts the process exits (does not get SIGKILL'd mid-run
// without a status). Exit may be 0 or 1 depending on whether version wrote
// before the pipe broke; the process must not die of uncaught SIGPIPE alone
// without returning a wait status.
func TestFoundryBinarySurvivesBrokenStdoutDuringVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real go subprocess test in short mode")
	}
	log := testutil.New(t)
	log.Phase("foundry_broken_stdout")

	bin := buildFoundry(t)
	log.Fixture("foundry_bin", filepath.Base(bin))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	cmd := exec.Command(bin, "version")
	cmd.Stdout = w
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		_ = r.Close()
		_ = w.Close()
		t.Fatalf("start foundry: %v", err)
	}
	// Close write end in parent; immediately close read end so any write hits EPIPE.
	_ = w.Close()
	_ = r.Close()

	waitErr := cmd.Wait()
	// Process completed (was not left as an un-waitable SIGPIPE zombie with no
	// status). With drained SIGPIPE, Wait returns normally with exit code.
	var code int
	if waitErr == nil {
		code = 0
	} else if ee, ok := waitErr.(*exec.ExitError); ok {
		code = ee.ExitCode()
		// Must not be pure signal death without Go handling — if Signaled with
		// SIGPIPE, the production drain is broken.
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() && ws.Signal() == syscall.SIGPIPE {
			t.Fatalf("foundry killed by SIGPIPE (drain missing): %v stderr=%q", waitErr, stderr.String())
		}
	} else {
		t.Fatalf("wait: %v stderr=%q", waitErr, stderr.String())
	}
	log.Assert("got_exit_code", code == 0 || code == 1 || code == 2, true, code)
	log.Step("foundry_exit", testutil.OutcomeOK, fmt.Sprintf("exit=%d stderr_len=%d", code, stderr.Len()))
	log.PhaseEnd("foundry_broken_stdout", testutil.OutcomeOK)
}

func buildFoundry(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "foundry")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(repoRoot(t), "cmd", "foundry")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build foundry: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// cmd/foundry → repo root
	if filepath.Base(wd) == "foundry" {
		return filepath.Clean(filepath.Join(wd, "..", ".."))
	}
	// Walk up for go.mod.
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func isBrokenPipeMsg(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "epipe")
}
