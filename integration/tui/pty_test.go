//go:build tui_pty

package tui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// reapProcess always SIGTERM then Kill+Wait so timeout paths cannot leave
// orphan children or unjoined Wait goroutines (ipk.12).
func reapProcess(cmd *exec.Cmd, done <-chan error) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
		return
	case <-time.After(500 * time.Millisecond):
		_ = cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		// Best-effort; OS reaps eventually.
	}
}

// waitPTYStartup is an intentional wall-clock settle for alternate-screen
// entry before input. Real-PTY readiness is OS-bound (ipk.12 / ipk.15);
// e4 subprocess tests prefer waitForBytes markers where available.
func waitPTYStartup() {
	time.Sleep(200 * time.Millisecond)
}

// TestTUI_PTY_QuitRestore builds a generated TUI and checks the process exits
// 0 after sending 'q' under a PTY (Section 18.6 smoke). Full E4 matrices remain
// the reference for termios equality; this guards the generated shell binary.
func TestTUI_PTY_QuitRestore(t *testing.T) {
	log := testutil.New(t)
	log.Phase("pty_quit")
	dest := generateMinimalTUI(t, log)
	bin := buildTUIBinary(t, log, dest, "smoke-tui")

	cmd := exec.Command(bin)
	cmd.Dir = dest
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=xterm-256color")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fail("pty_start", err.Error())
	}
	defer func() { _ = ptmx.Close() }()

	waitPTYStartup()
	if _, err := ptmx.Write([]byte("q")); err != nil {
		log.Fail("write_q", err.Error())
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				log.Fail("wait", err.Error())
			}
		}
		log.Assert("exit_0", code == 0, 0, code)
		log.Step("pty_quit", testutil.OutcomeOK, "exit="+itoa(code))
	case <-time.After(10 * time.Second):
		reapProcess(cmd, done)
		log.Fail("timeout", "TUI did not exit after q")
	}
	log.PhaseEnd("pty_quit", testutil.OutcomeOK)
}

// TestTUI_PTY_SIGINT exits 130 under external interrupt.
func TestTUI_PTY_SIGINT(t *testing.T) {
	log := testutil.New(t)
	log.Phase("pty_sigint")
	dest := generateMinimalTUI(t, log)
	bin := buildTUIBinary(t, log, dest, "smoke-tui")

	cmd := exec.Command(bin)
	cmd.Dir = dest
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=xterm-256color")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fail("pty_start", err.Error())
	}
	defer func() { _ = ptmx.Close() }()
	waitPTYStartup()
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		log.Fail("sigint", err.Error())
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				log.Fail("wait", err.Error())
			}
		}
		// 130 is normative; some environments may surface 2 — accept 130 only.
		log.Assert("exit_130", code == 130, 130, code)
		log.Step("pty_sigint", testutil.OutcomeOK, "exit="+itoa(code)+" path="+filepath.Base(bin))
	case <-time.After(10 * time.Second):
		reapProcess(cmd, done)
		log.Fail("timeout", "TUI did not exit after SIGINT")
	}
	log.PhaseEnd("pty_sigint", testutil.OutcomeOK)
}

// TestTUI_PTY_DistributionQuitRestore is product E2E C4: generated public TUI
// with profiles=["distribution"] exits 0 after 'q' under a PTY
// (go-foundry-cli-79a.6).
func TestTUI_PTY_DistributionQuitRestore(t *testing.T) {
	log := testutil.New(t)
	log.Phase("pty_dist_quit")
	dest := generateDistributionTUI(t, log)
	bin := buildTUIBinary(t, log, dest, "dist-tui")

	cmd := exec.Command(bin)
	cmd.Dir = dest
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=xterm-256color")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fail("pty_start", err.Error())
	}
	defer func() { _ = ptmx.Close() }()

	waitPTYStartup()
	if _, err := ptmx.Write([]byte("q")); err != nil {
		log.Fail("write_q", err.Error())
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				log.Fail("wait", err.Error())
			}
		}
		log.Assert("exit_0", code == 0, 0, code)
		log.Step("pty_dist_quit", testutil.OutcomeOK, "exit="+itoa(code))
	case <-time.After(10 * time.Second):
		reapProcess(cmd, done)
		log.Fail("timeout", "dist TUI did not exit after q")
	}
	log.PhaseEnd("pty_dist_quit", testutil.OutcomeOK)
}
