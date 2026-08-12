package e4

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

// SignalOwnerMain is the identity string for the single lifecycle owner.
// Logged on every quit path for evidence.
const SignalOwnerMain = "main.signal.NotifyContext"

// QuitClass classifies how the program left the event loop (Section 18.3 #6).
type QuitClass string

const (
	QuitClassQKey      QuitClass = "q"
	QuitClassCtrlCKey  QuitClass = "ctrl+c-key"
	QuitClassSignal    QuitClass = "signal" // external SIGINT/SIGTERM / ctx cancel
	QuitClassStartup   QuitClass = "startup-failure"
	QuitClassRuntime   QuitClass = "runtime-failure"
	QuitClassEffectErr QuitClass = "effect-error"
	QuitClassPanic     QuitClass = "framework-recovered-panic"
)

// Exit codes per Section 18.3 #6 / REQ-070.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitSignal  = 130 // 128 + SIGINT
)

// Config configures a lifecycle-owned program run.
type Config struct {
	// AppCtx is optional; when nil, Run builds signal.NotifyContext itself.
	AppCtx context.Context
	Cancel context.CancelFunc
	Model  *SpikeModel
	Input  io.Reader
	Output io.Writer
	Log    *ProbeLog
	// StartupErr, when non-nil, short-circuits before NewProgram (startup failure path).
	StartupErr error
	// ExtraOpts appended after the required WithContext + WithoutSignalHandler.
	ExtraOpts []tea.ProgramOption
	// EffectIdleTimeout bounds waiting for cooperative effects after Run.
	EffectIdleTimeout time.Duration
}

// Result is the outcome of a lifecycle-owned Run.
type Result struct {
	Err           error
	ExitCode      int
	QuitClass     QuitClass
	ActiveEffects int
	SignalOwner   string
	PTYRestore    string // "n/a" | "restored" | "not-restored" | "unchecked"
	Model         *SpikeModel
}

// NewLifecycleContext creates the single signal-owned application context.
// Callers MUST defer stop().
func NewLifecycleContext(parent context.Context) (ctx context.Context, stop context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

// RequiredProgramOptions returns the normative program options for Section 18.3.
// Default panic recovery stays enabled: WithoutCatchPanics is intentionally absent.
func RequiredProgramOptions(appCtx context.Context) []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithContext(appCtx),
		tea.WithoutSignalHandler(),
		// DO NOT add tea.WithoutCatchPanics() — framework owns terminal restore on panic.
	}
}

// MapExitCode maps a Run error and quit class to the three exit codes.
//
// Cancel-before-quit (Section 18.3 #4) cancels appCtx so cooperative effects
// observe Done. Because the program is also constructed with
// tea.WithContext(appCtx), that cancel races the Quit/Interrupt message and
// often surfaces as ErrProgramKilled+context.Canceled. Model-reported
// QuitClass is therefore authoritative for deliberate user paths.
func MapExitCode(err error, class QuitClass) int {
	switch class {
	case QuitClassQKey:
		// Deliberate user quit → 0 even when cancel races WithContext.
		return ExitOK
	case QuitClassCtrlCKey, QuitClassSignal:
		return ExitSignal
	case QuitClassPanic, QuitClassStartup, QuitClassRuntime, QuitClassEffectErr:
		return ExitFailure
	}
	if err == nil {
		return ExitOK
	}
	// Prefer panic/failure over the wrapped ErrProgramKilled.
	if errors.Is(err, tea.ErrProgramPanic) {
		return ExitFailure
	}
	if errors.Is(err, tea.ErrInterrupted) {
		return ExitSignal
	}
	if errors.Is(err, tea.ErrProgramKilled) {
		return ExitSignal
	}
	return ExitFailure
}

// ClassifyRunError derives QuitClass from the error and model-reported class.
func ClassifyRunError(err error, modelClass QuitClass) QuitClass {
	if modelClass != "" {
		switch modelClass {
		case QuitClassQKey, QuitClassCtrlCKey, QuitClassEffectErr:
			return modelClass
		}
	}
	if err == nil {
		if modelClass != "" {
			return modelClass
		}
		return QuitClassQKey
	}
	if errors.Is(err, tea.ErrProgramPanic) {
		return QuitClassPanic
	}
	if errors.Is(err, tea.ErrInterrupted) {
		// Model may have set CtrlCKey; if not, still treat as interrupt class.
		if modelClass == QuitClassCtrlCKey {
			return QuitClassCtrlCKey
		}
		return QuitClassCtrlCKey
	}
	if errors.Is(err, tea.ErrProgramKilled) {
		return QuitClassSignal
	}
	return QuitClassRuntime
}

// Run executes one lifecycle-owned Bubble Tea program and records evidence.
//
// Pattern for generated TUI main:
//
//	appCtx, stop := e4.NewLifecycleContext(context.Background())
//	defer stop()
//	res := e4.Run(e4.Config{AppCtx: appCtx, Cancel: stop, Model: m, ...})
//	os.Exit(res.ExitCode)
func Run(cfg Config) Result {
	log := cfg.Log
	if log == nil {
		log = NewProbeLog()
	}

	res := Result{
		SignalOwner: SignalOwnerMain,
		PTYRestore:  "unchecked",
	}

	// Startup failure path: never construct the program.
	if cfg.StartupErr != nil {
		res.Err = cfg.StartupErr
		res.QuitClass = QuitClassStartup
		res.ExitCode = ExitFailure
		res.ActiveEffects = 0
		res.PTYRestore = "n/a"
		log.Record(ProbeEntry{
			Probe: "lifecycle", Step: "startup_failure", Outcome: "pass",
			ActiveEffects: 0, SignalOwner: SignalOwnerMain, PTYRestore: "n/a",
			ExitCode: ExitFailure, QuitClass: string(QuitClassStartup),
			Detail: cfg.StartupErr.Error(),
		})
		return res
	}

	appCtx := cfg.AppCtx
	cancel := cfg.Cancel
	ownsCtx := false
	if appCtx == nil {
		appCtx, cancel = NewLifecycleContext(context.Background())
		ownsCtx = true
	}
	if ownsCtx {
		defer cancel()
	}

	model := cfg.Model
	if model == nil {
		model = NewSpikeModel(SpikeModelConfig{
			AppCtx:  appCtx,
			Cancel:  cancel,
			Tracker: NewEffectTracker(),
			Log:     log,
		})
	} else {
		model.appCtx = appCtx
		model.cancel = cancel
		if model.tracker == nil {
			model.tracker = NewEffectTracker()
		}
		if model.log == nil {
			model.log = log
		}
	}
	res.Model = model

	opts := RequiredProgramOptions(appCtx)
	if cfg.Input != nil {
		opts = append(opts, tea.WithInput(cfg.Input))
	}
	if cfg.Output != nil {
		opts = append(opts, tea.WithOutput(cfg.Output))
	}
	opts = append(opts, cfg.ExtraOpts...)

	log.Record(ProbeEntry{
		Probe: "lifecycle", Step: "construct", Outcome: "info",
		SignalOwner: SignalOwnerMain,
		Detail:      "tea.WithContext(appCtx)+tea.WithoutSignalHandler(); panic recovery default ON",
		Args:        "options=WithContext,WithoutSignalHandler",
	})

	p := tea.NewProgram(model, opts...)
	_, runErr := p.Run()

	// After return (including framework-recovered panic), ensure appCtx is
	// cancelled so cooperative effects observe Done. Safe to call twice.
	if cancel != nil {
		cancel()
	}

	// Join cooperative effects (framework does not join Cmd goroutines).
	idleTimeout := cfg.EffectIdleTimeout
	if idleTimeout == 0 {
		idleTimeout = 2 * time.Second
	}
	tracker := model.tracker
	idle := tracker.WaitUntilIdle(idleTimeout)
	active := tracker.Active()
	res.ActiveEffects = active
	res.Err = runErr
	res.QuitClass = ClassifyRunError(runErr, model.quitClass)
	res.ExitCode = MapExitCode(runErr, res.QuitClass)

	outcome := "pass"
	detail := fmt.Sprintf("run_err=%v idle=%v", runErr, idle)
	if active != 0 {
		outcome = "fail"
		detail = fmt.Sprintf("ACTIVE_EFFECTS_NONZERO=%d run_err=%v", active, runErr)
	}
	_, started, finished := tracker.Snapshot()
	detail = fmt.Sprintf("%s started=%d finished=%d", detail, started, finished)

	log.Record(ProbeEntry{
		Probe: "lifecycle", Step: "return", Outcome: outcome,
		ActiveEffects: active, SignalOwner: SignalOwnerMain,
		PTYRestore: res.PTYRestore, ExitCode: res.ExitCode,
		QuitClass: string(res.QuitClass),
		Detail:    detail,
		Errno:     errString(runErr),
	})

	return res
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
