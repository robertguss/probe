//go:build unix

package e4

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/goleak"
)

// TestMain runs goleak after the e4 suite (ipk.10). Bubble Tea command
// goroutines are expected to finish via cancel+WaitUntilIdle in each test;
// ignore known non-joinable stdlib/runtime noise only.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// Bubble Tea's internal program loop can leave a blocked reader when
		// tests force-Kill after cancel; ignore only that package's top frame
		// if present after cooperative paths.
		goleak.IgnoreTopFunction("charm.land/bubbletea/v2.(*Program).eventLoop"),
		goleak.IgnoreTopFunction("charm.land/bubbletea/v2.(*Program).readInputs"),
		goleak.IgnoreTopFunction("charm.land/bubbletea/v2.(*Program).handleCommands"),
		goleak.IgnoreTopFunction("charm.land/bubbletea/v2.initContext"),
	)
}

func waitReady(t *testing.T, m *SpikeModel, timeout time.Duration) {
	t.Helper()
	// Poll with NewTicker+Stop (ipk.12); bubbletea View readiness is
	// event-driven but not fully joinable without OS timers.
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if m.Ready() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("model not ready within %s", timeout)
		}
		<-tick.C
	}
}

func pipeIO() (in *bytes.Buffer, out *bytes.Buffer) {
	return &bytes.Buffer{}, &bytes.Buffer{}
}

// TestE4_QuitKey_ZeroEffects proves q → exit 0, cancel-before-quit, zero effects.
func TestE4_QuitKey_ZeroEffects(t *testing.T) {
	t.Parallel()
	log := NewProbeLog()
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeWithEffect, EffectDuration: 30 * time.Second,
	})
	in, out := pipeIO()

	opts := RequiredProgramOptions(appCtx)
	opts = append(opts, tea.WithInput(in), tea.WithOutput(out))
	p := tea.NewProgram(model, opts...)

	errCh := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errCh <- err
	}()

	waitReady(t, model, 3*time.Second)
	// Effect should be active before quit.
	if !waitActive(tracker, 1, time.Second) {
		t.Fatalf("expected cooperative effect to be active before quit")
	}
	log.Record(ProbeEntry{
		Probe: "q", Step: "pre_quit", Outcome: "info",
		ActiveEffects: tracker.Active(), SignalOwner: SignalOwnerMain,
		Detail: "effect in-flight before q",
	})

	// Cancel-before-quit is done inside Update; inject key press message.
	p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})

	var runErr error
	select {
	case runErr = <-errCh:
		// Cancel-before-quit may surface ErrProgramKilled+context.Canceled;
		// model quit class remains authoritative (see MapExitCode).
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("timeout waiting for quit")
	}

	if !tracker.WaitUntilIdle(2 * time.Second) {
		t.Fatalf("effects still active: %d", tracker.Active())
	}
	active := tracker.Active()
	class := ClassifyRunError(runErr, model.quitClass)
	code := MapExitCode(runErr, class)
	log.Record(ProbeEntry{
		Probe: "q", Step: "return", Outcome: passFail(active == 0 && code == ExitOK && class == QuitClassQKey),
		ActiveEffects: active, SignalOwner: SignalOwnerMain, PTYRestore: "n/a",
		ExitCode: code, QuitClass: string(class),
		Errno:  errString(runErr),
		Detail: fmt.Sprintf("want exit=0 class=q active=0; got exit=%d class=%s active=%d run_err=%v", code, class, active, runErr),
	})
	if class != QuitClassQKey {
		t.Fatalf("quit class: got %s want q (run_err=%v)", class, runErr)
	}
	if code != ExitOK {
		t.Fatalf("exit code: got %d want 0", code)
	}
	if active != 0 {
		t.Fatalf("active effects: got %d want 0", active)
	}
}

// TestE4_CtrlCKey_Exit130 proves Ctrl-C-as-key → 130 and zero effects.
func TestE4_CtrlCKey_Exit130(t *testing.T) {
	t.Parallel()
	log := NewProbeLog()
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeWithEffect, EffectDuration: 30 * time.Second,
	})
	in, out := pipeIO()
	opts := RequiredProgramOptions(appCtx)
	opts = append(opts, tea.WithInput(in), tea.WithOutput(out))
	p := tea.NewProgram(model, opts...)

	errCh := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errCh <- err
	}()

	waitReady(t, model, 3*time.Second)
	_ = waitActive(tracker, 1, time.Second)

	// ctrl+c key: Code 'c' with ModCtrl — String() should be "ctrl+c".
	p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	var runErr error
	select {
	case runErr = <-errCh:
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("timeout")
	}

	if !tracker.WaitUntilIdle(2 * time.Second) {
		t.Fatalf("effects still active: %d", tracker.Active())
	}
	active := tracker.Active()
	class := ClassifyRunError(runErr, model.quitClass)
	code := MapExitCode(runErr, class)

	// Prefer ErrInterrupted; cancel-before-quit may race WithContext and
	// surface ErrProgramKilled+context.Canceled instead. Model quit class is
	// authoritative either way.
	if !errors.Is(runErr, tea.ErrInterrupted) && !errors.Is(runErr, tea.ErrProgramKilled) {
		t.Fatalf("want ErrInterrupted or ErrProgramKilled, got %v", runErr)
	}
	log.Record(ProbeEntry{
		Probe: "ctrl+c-key", Step: "return",
		Outcome:       passFail(active == 0 && code == ExitSignal && class == QuitClassCtrlCKey),
		ActiveEffects: active, SignalOwner: SignalOwnerMain, PTYRestore: "n/a",
		ExitCode: code, QuitClass: string(class),
		Errno:  errString(runErr),
		Detail: fmt.Sprintf("want exit=130 class=ctrl+c-key active=0; got exit=%d class=%s active=%d", code, class, active),
	})
	if class != QuitClassCtrlCKey {
		t.Fatalf("class: got %s (run_err=%v)", class, runErr)
	}
	if code != ExitSignal {
		t.Fatalf("exit: got %d want 130", code)
	}
	if active != 0 {
		t.Fatalf("active: %d", active)
	}
}

// TestE4_ContextCancel_SIGINTPath proves external cancel (NotifyContext path) → 130.
func TestE4_ContextCancel_SIGINTPath(t *testing.T) {
	t.Parallel()
	log := NewProbeLog()
	// Simulate signal.NotifyContext cancel without sending a real process signal
	// (real SIGINT/SIGTERM covered by subprocess PTY tests).
	appCtx, cancel := context.WithCancel(context.Background())

	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeHold, EffectDuration: 30 * time.Second,
	})
	in, out := pipeIO()
	opts := RequiredProgramOptions(appCtx)
	opts = append(opts, tea.WithInput(in), tea.WithOutput(out))
	p := tea.NewProgram(model, opts...)

	errCh := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errCh <- err
	}()

	waitReady(t, model, 3*time.Second)
	_ = waitActive(tracker, 1, time.Second)

	// External signal owner cancels appCtx (same as signal.NotifyContext on SIGINT).
	cancel()

	var runErr error
	select {
	case runErr = <-errCh:
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("timeout")
	}

	if !tracker.WaitUntilIdle(2 * time.Second) {
		t.Fatalf("effects still active: %d", tracker.Active())
	}
	active := tracker.Active()
	class := ClassifyRunError(runErr, model.quitClass)
	code := MapExitCode(runErr, class)

	if !errors.Is(runErr, tea.ErrProgramKilled) {
		t.Fatalf("want ErrProgramKilled, got %v", runErr)
	}
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("want context.Canceled wrapped, got %v", runErr)
	}
	log.Record(ProbeEntry{
		Probe: "signal-cancel", Step: "return",
		Outcome:       passFail(active == 0 && code == ExitSignal && class == QuitClassSignal),
		ActiveEffects: active, SignalOwner: SignalOwnerMain, PTYRestore: "n/a",
		ExitCode: code, QuitClass: string(class),
		Errno:  errString(runErr),
		Detail: "NotifyContext cancel → ErrProgramKilled+context.Canceled → exit 130",
	})
	if class != QuitClassSignal || code != ExitSignal || active != 0 {
		t.Fatalf("class=%s code=%d active=%d", class, code, active)
	}
}

// TestE4_SIGTERMPath documents that NotifyContext cancellation is identical for
// SIGINT and SIGTERM (both cancel appCtx). Real SIGTERM is covered by
// TestE4_Subprocess_RealSignals/SIGTERM.
func TestE4_SIGTERMPath(t *testing.T) {
	t.Parallel()
	log := NewProbeLog()
	appCtx, cancel := context.WithCancel(context.Background())
	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeHold, EffectDuration: 30 * time.Second,
	})
	in, out := pipeIO()
	opts := RequiredProgramOptions(appCtx)
	opts = append(opts, tea.WithInput(in), tea.WithOutput(out))
	p := tea.NewProgram(model, opts...)
	errCh := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errCh <- err
	}()
	waitReady(t, model, 3*time.Second)
	_ = waitActive(tracker, 1, time.Second)
	// Simulate SIGTERM delivery into NotifyContext (cancel).
	cancel()
	var runErr error
	select {
	case runErr = <-errCh:
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("timeout")
	}
	if !tracker.WaitUntilIdle(2 * time.Second) {
		t.Fatalf("effects still active: %d", tracker.Active())
	}
	active := tracker.Active()
	class := ClassifyRunError(runErr, model.quitClass)
	code := MapExitCode(runErr, class)
	log.Record(ProbeEntry{
		Probe: "sigterm-cancel", Step: "return",
		Outcome:       passFail(active == 0 && code == ExitSignal && class == QuitClassSignal),
		ActiveEffects: active, SignalOwner: SignalOwnerMain, PTYRestore: "n/a",
		ExitCode: code, QuitClass: string(class),
		Errno:  errString(runErr),
		Detail: "SIGTERM→NotifyContext cancel identical to SIGINT path",
	})
	if class != QuitClassSignal || code != ExitSignal || active != 0 {
		t.Fatalf("class=%s code=%d active=%d err=%v", class, code, active, runErr)
	}
}

// TestE4_StartupFailure_Exit1 proves startup error never starts the program.
func TestE4_StartupFailure_Exit1(t *testing.T) {
	t.Parallel()
	log := NewProbeLog()
	res := Run(Config{
		Log:        log,
		StartupErr: fmt.Errorf("invalid --debug-log basename"),
	})
	if res.ExitCode != ExitFailure {
		t.Fatalf("exit: %d", res.ExitCode)
	}
	if res.QuitClass != QuitClassStartup {
		t.Fatalf("class: %s", res.QuitClass)
	}
	if res.ActiveEffects != 0 {
		t.Fatalf("active: %d", res.ActiveEffects)
	}
	if res.PTYRestore != "n/a" {
		t.Fatalf("pty: %s", res.PTYRestore)
	}
	log.Record(ProbeEntry{
		Probe: "startup", Step: "assert", Outcome: "pass",
		ActiveEffects: 0, SignalOwner: SignalOwnerMain, PTYRestore: "n/a",
		ExitCode: ExitFailure, QuitClass: string(QuitClassStartup),
	})
}

// TestE4_EffectError_ZeroEffects proves effect error quit drains effects.
func TestE4_EffectError_ZeroEffects(t *testing.T) {
	t.Parallel()
	log := NewProbeLog()
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeFailingEffect,
	})
	in, out := pipeIO()
	res := Run(Config{
		AppCtx: appCtx, Cancel: cancel, Model: model,
		Input: in, Output: out, Log: log,
	})
	if res.ActiveEffects != 0 {
		t.Fatalf("active: %d", res.ActiveEffects)
	}
	if res.QuitClass != QuitClassEffectErr {
		t.Fatalf("class: %s want effect-error", res.QuitClass)
	}
	if res.ExitCode != ExitFailure {
		t.Fatalf("exit: %d want 1", res.ExitCode)
	}
}

// TestE4_FrameworkPanic_Recovered proves default panic recovery (no WithoutCatchPanics).
// ModeWithEffect starts a cooperative Cmd; post-panic the lifecycle owner cancels
// appCtx so the effect observes Done (framework does not join Cmd goroutines).
func TestE4_FrameworkPanic_Recovered(t *testing.T) {
	t.Parallel()
	log := NewProbeLog()
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeWithEffect, EffectDuration: 30 * time.Second,
	})
	in, out := pipeIO()
	opts := RequiredProgramOptions(appCtx)
	opts = append(opts, tea.WithInput(in), tea.WithOutput(out))
	p := tea.NewProgram(model, opts...)

	errCh := make(chan error, 1)
	go func() {
		_, err := p.Run()
		errCh <- err
	}()

	waitReady(t, model, 3*time.Second)
	_ = waitActive(tracker, 1, time.Second)
	p.Send(panicMsg{})

	var runErr error
	select {
	case runErr = <-errCh:
	case <-time.After(5 * time.Second):
		p.Kill()
		t.Fatal("timeout")
	}

	if !errors.Is(runErr, tea.ErrProgramPanic) {
		t.Fatalf("want ErrProgramPanic, got %v", runErr)
	}
	if !errors.Is(runErr, tea.ErrProgramKilled) {
		t.Fatalf("want ErrProgramKilled wrap, got %v", runErr)
	}
	// Lifecycle owner cancels after framework-recovered panic so cooperative
	// effects drain (main does not claim to recover panics from Cmd goroutines).
	cancel()
	if !tracker.WaitUntilIdle(2 * time.Second) {
		t.Fatalf("effects still active: %d", tracker.Active())
	}
	active := tracker.Active()
	class := ClassifyRunError(runErr, model.quitClass)
	code := MapExitCode(runErr, class)
	log.Record(ProbeEntry{
		Probe: "panic", Step: "return",
		Outcome:       passFail(active == 0 && code == ExitFailure && class == QuitClassPanic),
		ActiveEffects: active, SignalOwner: SignalOwnerMain, PTYRestore: "n/a-buffer",
		ExitCode: code, QuitClass: string(class),
		Errno:  errString(runErr),
		Detail: "framework recovered panic; WithoutCatchPanics not used; owner cancelled effects",
	})
	if class != QuitClassPanic || code != ExitFailure || active != 0 {
		t.Fatalf("class=%s code=%d active=%d", class, code, active)
	}
}

// TestE4_StaticOptions proves WithContext + WithoutSignalHandler present and
// WithoutCatchPanics absent in spike sources.
func TestE4_StaticOptions(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	res, err := StaticCheckOptions(dir)
	if err != nil {
		t.Fatal(err)
	}
	log := NewProbeLog()
	log.Record(ProbeEntry{
		Probe: "static", Step: "options",
		Outcome:     passFail(res.Pass()),
		SignalOwner: SignalOwnerMain,
		Detail:      res.Detail,
		Args:        fmt.Sprintf("files=%d", len(res.FilesScanned)),
	})
	if !res.Pass() {
		t.Fatalf("static check failed: %s", res.Detail)
	}
	t.Logf("static: %s scanned=%v", res.Detail, basenames(res.FilesScanned))
}

// TestE4_RunHelper_Integration exercises the Run() template helper end-to-end.
func TestE4_RunHelper_Integration(t *testing.T) {
	t.Parallel()
	log := NewProbeLog()
	appCtx, cancel := context.WithCancel(context.Background())
	// cancel will be invoked by model on q; also defer for safety.
	defer cancel()

	tracker := NewEffectTracker()
	model := NewSpikeModel(SpikeModelConfig{
		AppCtx: appCtx, Cancel: cancel, Tracker: tracker, Log: log,
		Mode: ModeIdle,
	})
	in, out := pipeIO()

	done := make(chan Result, 1)
	go func() {
		done <- Run(Config{
			AppCtx: appCtx, Cancel: cancel, Model: model,
			Input: in, Output: out, Log: log,
		})
	}()

	waitReady(t, model, 3*time.Second)
	// Find program? Run doesn't expose it. Send via a short-lived approach:
	// write 'q' won't parse easily. Use ModeIdle + Kill via cancel after ready.
	// For Run helper, cancel appCtx to simulate signal.
	cancel()

	select {
	case res := <-done:
		if res.ActiveEffects != 0 {
			t.Fatalf("active: %d", res.ActiveEffects)
		}
		if res.ExitCode != ExitSignal {
			t.Fatalf("exit: %d want 130", res.ExitCode)
		}
		if res.SignalOwner != SignalOwnerMain {
			t.Fatalf("owner: %s", res.SignalOwner)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// TestE4_RequiredOptions_Order documents the option set used by templates.
func TestE4_RequiredOptions_Order(t *testing.T) {
	opts := RequiredProgramOptions(context.Background())
	if len(opts) != 2 {
		t.Fatalf("want exactly 2 required options, got %d", len(opts))
	}
	// Construct a program to ensure options apply without panic.
	in, out := pipeIO()
	m := NewSpikeModel(SpikeModelConfig{Mode: ModeIdle})
	p := tea.NewProgram(m, append(opts, tea.WithInput(in), tea.WithOutput(out))...)
	go func() {
		waitReady(t, m, 2*time.Second)
		p.Quit()
	}()
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestE4_MapExitCode_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		err   error
		class QuitClass
		want  int
	}{
		{"q", nil, QuitClassQKey, 0},
		// cancel-before-quit race still maps deliberate q → 0
		{"q-killed-race", fmt.Errorf("%w: %w", tea.ErrProgramKilled, context.Canceled), QuitClassQKey, 0},
		{"ctrlc", tea.ErrInterrupted, QuitClassCtrlCKey, 130},
		{"signal", tea.ErrProgramKilled, QuitClassSignal, 130},
		{"panic", fmt.Errorf("%w: %w", tea.ErrProgramKilled, tea.ErrProgramPanic), QuitClassPanic, 1},
		{"startup", errors.New("boom"), QuitClassStartup, 1},
		{"effect", nil, QuitClassEffectErr, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MapExitCode(tc.err, tc.class); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func waitActive(tr *EffectTracker, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tr.Active() >= n {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return tr.Active() >= n
}

func passFail(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

// Ensure runtime import used for evidence meta in other tests.
var _ = runtime.GOOS
