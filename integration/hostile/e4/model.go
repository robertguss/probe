package e4

import (
	"context"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

// SpikeModel is the minimal TUI model used by the E4 lifecycle spike.
// Patterns (cancel-before-quit, cooperative effects, alt-screen) feed
// generated TUI templates.
type SpikeModel struct {
	appCtx  context.Context
	cancel  context.CancelFunc
	tracker *EffectTracker
	log     *ProbeLog

	// Mode switches Init behaviour for specific probes.
	Mode ModelMode

	// EffectDuration for the cooperative long-running effect started on Init
	// when Mode is ModeWithEffect / ModeHold.
	EffectDuration time.Duration

	// quitClass is set when the model deliberately quits.
	quitClass QuitClass

	// ready is set true after the first View (program is live).
	ready atomic.Bool

	// status is rendered into the view for debugging.
	status string

	// UseAltScreen requests alternate-screen buffer (PTY restore proof).
	UseAltScreen bool
}

// ModelMode selects Init behaviour.
type ModelMode int

const (
	// ModeIdle: static help/quit screen, no effects (generated-shell shape).
	ModeIdle ModelMode = iota
	// ModeWithEffect: starts a long cooperative effect on Init.
	ModeWithEffect
	// ModeFailingEffect: starts an effect that reports an error.
	ModeFailingEffect
	// ModeHold: long effect + no auto-quit (for signal / external cancel tests).
	ModeHold
)

// SpikeModelConfig constructs a SpikeModel.
type SpikeModelConfig struct {
	AppCtx         context.Context
	Cancel         context.CancelFunc
	Tracker        *EffectTracker
	Log            *ProbeLog
	Mode           ModelMode
	EffectDuration time.Duration
	UseAltScreen   bool
}

// NewSpikeModel builds a model bound to the lifecycle application context.
func NewSpikeModel(cfg SpikeModelConfig) *SpikeModel {
	if cfg.Tracker == nil {
		cfg.Tracker = NewEffectTracker()
	}
	if cfg.Log == nil {
		cfg.Log = NewProbeLog()
	}
	if cfg.EffectDuration == 0 {
		cfg.EffectDuration = 30 * time.Second
	}
	if cfg.AppCtx == nil {
		cfg.AppCtx = context.Background()
	}
	return &SpikeModel{
		appCtx:         cfg.AppCtx,
		cancel:         cfg.Cancel,
		tracker:        cfg.Tracker,
		log:            cfg.Log,
		Mode:           cfg.Mode,
		EffectDuration: cfg.EffectDuration,
		UseAltScreen:   cfg.UseAltScreen,
		status:         "ready",
	}
}

// Ready reports whether View has run at least once.
func (m *SpikeModel) Ready() bool { return m.ready.Load() }

// Tracker returns the effect tracker.
func (m *SpikeModel) Tracker() *EffectTracker { return m.tracker }

// QuitClass returns the deliberate quit classification, if any.
func (m *SpikeModel) QuitClass() QuitClass { return m.quitClass }

// Init implements tea.Model.
func (m *SpikeModel) Init() tea.Cmd {
	switch m.Mode {
	case ModeWithEffect, ModeHold:
		return tea.Batch(
			func() tea.Msg { return initDoneMsg{} },
			CooperativeSleep(m.appCtx, m.tracker, "coop-sleep", m.EffectDuration),
		)
	case ModeFailingEffect:
		return tea.Batch(
			func() tea.Msg { return initDoneMsg{} },
			FailingEffect(m.tracker, "failing", "simulated effect failure"),
		)
	default:
		return func() tea.Msg { return initDoneMsg{} }
	}
}

// Update implements tea.Model.
//
// Normative quit pattern (Section 18.3 #4): cancel application context BEFORE
// returning tea.Quit / tea.Interrupt so in-flight cooperative effects observe
// cancellation promptly.
func (m *SpikeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case initDoneMsg:
		m.status = "init-done"
		return m, nil

	case effectDoneMsg:
		if msg.Cancelled {
			m.status = "effect-cancelled:" + msg.Name
		} else {
			m.status = "effect-done:" + msg.Name
		}
		return m, nil

	case effectErrorMsg:
		m.status = "effect-error:" + msg.Name
		m.quitClass = QuitClassEffectErr
		m.invokeCancel()
		return m, tea.Quit

	case panicMsg:
		// Framework default panic recovery catches this and restores the terminal.
		panic("e4 spike: framework-recovered panic probe")

	case startEffectMsg:
		d := msg.Duration
		if d == 0 {
			d = m.EffectDuration
		}
		return m, CooperativeSleep(m.appCtx, m.tracker, msg.Name, d)

	case startFailingEffectMsg:
		return m, FailingEffect(m.tracker, msg.Name, msg.Err)

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q":
			// Deliberate user quit → exit 0.
			m.quitClass = QuitClassQKey
			m.invokeCancel()
			return m, tea.Quit

		case "ctrl+c":
			// Ctrl-C delivered as a key event inside the program → exit 130.
			m.quitClass = QuitClassCtrlCKey
			m.invokeCancel()
			// Interrupt maps to tea.ErrInterrupted (distinct from external kill).
			return m, tea.Interrupt

		case "p":
			// Trigger framework-recovered panic (for PTY restore matrix).
			return m, func() tea.Msg { return panicMsg{} }
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m *SpikeModel) View() tea.View {
	m.ready.Store(true)
	content := "e4-lifecycle-spike\n"
	content += "status=" + m.status + "\n"
	content += "keys: q=quit  ctrl+c=interrupt  p=panic\n"
	v := tea.NewView(content)
	v.AltScreen = m.UseAltScreen
	return v
}

func (m *SpikeModel) invokeCancel() {
	if m.cancel != nil {
		m.cancel()
	}
}
