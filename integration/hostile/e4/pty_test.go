//go:build unix

package e4

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/creack/pty"
)

// openPTY creates a master/slave PTY pair sized 24x80.
func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	m, s, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open: %v", err)
	}
	if err := pty.Setsize(m, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("pty.Setsize: %v", err)
	}
	t.Cleanup(func() {
		_ = m.Close()
		_ = s.Close()
	})
	return m, s
}

// drainMaster continuously reads master so the slave writer does not block.
// Uses deadline-based reads (not io.Copy) so cleanup can always unblock.
func drainMaster(t *testing.T, master *os.File) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	var mu sync.Mutex
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tmp := make([]byte, 1024)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = master.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			n, err := master.Read(tmp)
			if n > 0 {
				mu.Lock()
				buf.Write(tmp[:n])
				mu.Unlock()
			}
			if err != nil {
				if isTimeout(err) || errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		_ = master.SetReadDeadline(time.Now())
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
	})
	return buf
}

// TestE4_PTY_RestoreOnQuit proves termios restored after normal q quit + alt-screen.
func TestE4_PTY_RestoreOnQuit(t *testing.T) {
	log := NewProbeLog()
	master, slave := openPTY(t)
	before, err := SnapshotTermios(int(slave.Fd()))
	if err != nil {
		t.Fatalf("snapshot before: %v", err)
	}
	if !before.Cooked() {
		t.Fatalf("expected cooked termios before start: %s", before)
	}
	outBuf := drainMaster(t, master)

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeWithEffect, EffectDuration: 30 * time.Second,
		UseAltScreen: true,
	})

	// Same slave for input+output so MakeRaw/Restore apply to this fd.
	opts := RequiredProgramOptions(appCtx)
	opts = append(opts,
		tea.WithInput(slave),
		tea.WithOutput(slave),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
	)
	p := tea.NewProgram(model, opts...)

	errCh := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errCh <- err
	}()

	waitReady(t, model, 5*time.Second)
	_ = waitActive(tracker, 1, time.Second)

	// During run, slave should be raw (MakeRaw clears ICANON|ECHO).
	during, err := SnapshotTermios(int(slave.Fd()))
	if err != nil {
		t.Fatalf("snapshot during: %v", err)
	}
	log.Record(ProbeEntry{
		Probe: "pty-quit", Step: "during_raw", Outcome: passFail(!during.Cooked()),
		Detail: during.String(), SignalOwner: SignalOwnerMain,
	})
	if during.Cooked() {
		t.Fatalf("expected raw mode during run: %s", during)
	}

	p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})

	var runErr error
	select {
	case runErr = <-errCh:
		// cancel-before-quit may wrap ErrProgramKilled; class remains q.
		_ = runErr
	case <-time.After(8 * time.Second):
		p.Kill()
		t.Fatal("timeout")
	}
	if model.quitClass != QuitClassQKey {
		t.Fatalf("quit class: %s (run_err=%v)", model.quitClass, runErr)
	}

	if !tracker.WaitUntilIdle(2 * time.Second) {
		t.Fatalf("active effects: %d", tracker.Active())
	}

	after, err := SnapshotTermios(int(slave.Fd()))
	if err != nil {
		t.Fatalf("snapshot after: %v", err)
	}
	restored := after.Equal(before) && after.Cooked()
	status := "restored"
	if !restored {
		status = "not-restored"
	}
	// Alt-screen exit sequence (CSI ? 1049 l) expected when AltScreen was used.
	out := outBuf.String()
	altExit := bytes.Contains([]byte(out), []byte("\x1b[?1049l")) ||
		bytes.Contains([]byte(out), []byte("\x1b[?1049l"))
	// Some stacks use 47/1047; accept any reset-mode alt sequence.
	if !altExit {
		altExit = bytes.Contains([]byte(out), []byte("1049l")) ||
			bytes.Contains([]byte(out), []byte("?47l")) ||
			bytes.Contains([]byte(out), []byte("?1047l"))
	}

	log.Record(ProbeEntry{
		Probe: "pty-quit", Step: "return",
		Outcome:       passFail(restored && tracker.Active() == 0),
		ActiveEffects: tracker.Active(), SignalOwner: SignalOwnerMain,
		PTYRestore: status, ExitCode: ExitOK, QuitClass: string(QuitClassQKey),
		Detail: fmt.Sprintf("before=%s after=%s alt_exit=%v out_len=%d", before, after, altExit, len(out)),
	})
	if !restored {
		t.Fatalf("termios not restored: before=%s after=%s", before, after)
	}
	if tracker.Active() != 0 {
		t.Fatalf("active: %d", tracker.Active())
	}
}

// TestE4_PTY_RestoreOnContextCancel proves restore after WithContext kill.
func TestE4_PTY_RestoreOnContextCancel(t *testing.T) {
	log := NewProbeLog()
	master, slave := openPTY(t)
	before, err := SnapshotTermios(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	_ = drainMaster(t, master)

	appCtx, cancel := context.WithCancel(context.Background())
	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeHold, EffectDuration: 30 * time.Second,
		UseAltScreen: true,
	})
	opts := RequiredProgramOptions(appCtx)
	opts = append(opts,
		tea.WithInput(slave), tea.WithOutput(slave),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
	)
	p := tea.NewProgram(model, opts...)
	errCh := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errCh <- err
	}()
	waitReady(t, model, 5*time.Second)
	cancel() // signal-owner path

	var runErr error
	select {
	case runErr = <-errCh:
	case <-time.After(8 * time.Second):
		p.Kill()
		t.Fatal("timeout")
	}
	if !errors.Is(runErr, tea.ErrProgramKilled) {
		t.Fatalf("want killed, got %v", runErr)
	}
	_ = tracker.WaitUntilIdle(2 * time.Second)
	after, err := SnapshotTermios(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	restored := after.Equal(before) && after.Cooked()
	status := "restored"
	if !restored {
		status = "not-restored"
	}
	log.Record(ProbeEntry{
		Probe: "pty-signal", Step: "return",
		Outcome:       passFail(restored && tracker.Active() == 0),
		ActiveEffects: tracker.Active(), SignalOwner: SignalOwnerMain,
		PTYRestore: status, ExitCode: ExitSignal, QuitClass: string(QuitClassSignal),
		Errno:  errString(runErr),
		Detail: fmt.Sprintf("before=%s after=%s", before, after),
	})
	if !restored {
		t.Fatalf("termios not restored: before=%s after=%s", before, after)
	}
}

// TestE4_PTY_RestoreOnPanic proves framework panic recovery restores termios.
func TestE4_PTY_RestoreOnPanic(t *testing.T) {
	log := NewProbeLog()
	master, slave := openPTY(t)
	before, err := SnapshotTermios(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	_ = drainMaster(t, master)

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeIdle, UseAltScreen: true,
	})
	opts := RequiredProgramOptions(appCtx)
	opts = append(opts,
		tea.WithInput(slave), tea.WithOutput(slave),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
	)
	p := tea.NewProgram(model, opts...)
	errCh := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errCh <- err
	}()
	waitReady(t, model, 5*time.Second)
	p.Send(panicMsg{})

	var runErr error
	select {
	case runErr = <-errCh:
	case <-time.After(8 * time.Second):
		p.Kill()
		t.Fatal("timeout")
	}
	if !errors.Is(runErr, tea.ErrProgramPanic) {
		t.Fatalf("want panic, got %v", runErr)
	}
	after, err := SnapshotTermios(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	restored := after.Equal(before) && after.Cooked()
	status := "restored"
	if !restored {
		status = "not-restored"
	}
	log.Record(ProbeEntry{
		Probe: "pty-panic", Step: "return",
		Outcome:       passFail(restored),
		ActiveEffects: tracker.Active(), SignalOwner: SignalOwnerMain,
		PTYRestore: status, ExitCode: ExitFailure, QuitClass: string(QuitClassPanic),
		Errno:  errString(runErr),
		Detail: fmt.Sprintf("before=%s after=%s", before, after),
	})
	if !restored {
		t.Fatalf("termios not restored after panic: before=%s after=%s", before, after)
	}
}

// TestE4_Subprocess_RealSignals builds e4spike and delivers real SIGINT/SIGTERM.
func TestE4_Subprocess_RealSignals(t *testing.T) {
	bin := buildSpike(t)
	for _, sig := range []struct {
		name string
		sig  os.Signal
	}{
		{"SIGINT", syscall.SIGINT},
		{"SIGTERM", syscall.SIGTERM},
	} {
		sig := sig
		t.Run(sig.name, func(t *testing.T) {
			log := NewProbeLog()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, bin, "-mode=hold")
			cmd.Env = append(os.Environ(), "E4_SPIKE=1")
			ptmx, err := pty.Start(cmd)
			if err != nil {
				t.Fatalf("pty.Start: %v", err)
			}
			defer func() { _ = ptmx.Close() }()
			_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

			// Wait until spike prints ready marker.
			if err := waitForBytes(ptmx, []byte("e4-lifecycle-spike"), 5*time.Second); err != nil {
				t.Fatalf("ready: %v", err)
			}
			time.Sleep(100 * time.Millisecond)

			if err := cmd.Process.Signal(sig.sig); err != nil {
				t.Fatalf("signal: %v", err)
			}

			err = cmd.Wait()
			exitCode := exitCodeOf(err)
			// signal.NotifyContext cancel → ErrProgramKilled → os.Exit(130)
			ok := exitCode == ExitSignal
			log.Record(ProbeEntry{
				Probe: "subprocess-" + sig.name, Step: "return",
				Outcome:     passFail(ok),
				SignalOwner: SignalOwnerMain, PTYRestore: "subprocess-exited",
				ExitCode: exitCode, QuitClass: string(QuitClassSignal),
				Detail: fmt.Sprintf("signal=%v wait_err=%v", sig.sig, err),
			})
			if !ok {
				t.Fatalf("exit code %d want 130 (err=%v)", exitCode, err)
			}
		})
	}
}

// TestE4_Subprocess_QuitKey drives 'q' over a real PTY to the spike binary.
func TestE4_Subprocess_QuitKey(t *testing.T) {
	bin := buildSpike(t)
	log := NewProbeLog()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-mode=idle")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptmx.Close() }()
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

	if err := waitForBytes(ptmx, []byte("e4-lifecycle-spike"), 5*time.Second); err != nil {
		t.Fatalf("ready: %v", err)
	}
	if _, err := ptmx.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	code := exitCodeOf(err)
	log.Record(ProbeEntry{
		Probe: "subprocess-q", Step: "return",
		Outcome:     passFail(code == ExitOK),
		SignalOwner: SignalOwnerMain, PTYRestore: "subprocess-exited",
		ExitCode: code, QuitClass: string(QuitClassQKey),
		Detail: fmt.Sprintf("wait_err=%v", err),
	})
	if code != ExitOK {
		t.Fatalf("exit %d want 0 (err=%v)", code, err)
	}
}

func buildSpike(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "e4spike")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/e4spike")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.26.5")
	// Package-relative: tests run with cwd = package dir.
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build e4spike: %v\n%s", err, outBytes)
	}
	return out
}

func waitForBytes(r io.Reader, want []byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var buf []byte
	tmp := make([]byte, 256)
	for time.Now().Before(deadline) {
		// Set short read via type assertion if possible.
		if f, ok := r.(*os.File); ok {
			_ = f.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		}
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if bytes.Contains(buf, want) {
				return nil
			}
		}
		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) && !isTimeout(err) {
			// keep draining on temporary errors
			if err == io.EOF {
				return fmt.Errorf("eof before marker; got %q", buf)
			}
		}
	}
	return fmt.Errorf("timeout waiting for %q; got %q", want, buf)
}

func isTimeout(err error) bool {
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
