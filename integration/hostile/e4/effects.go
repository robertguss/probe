package e4

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

// EffectTracker counts concurrent tea.Cmd effects. Bubble Tea does not join
// command goroutines, so cooperation + observed completion is mandatory
// (Section 18.3 #5, 18.7).
type EffectTracker struct {
	mu            sync.Mutex
	active        int
	totalStarted  int
	totalFinished int
	names         map[string]int // name → in-flight count
}

// NewEffectTracker constructs a zeroed tracker.
func NewEffectTracker() *EffectTracker {
	return &EffectTracker{names: make(map[string]int)}
}

// Start records the beginning of a named effect.
func (t *EffectTracker) Start(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active++
	t.totalStarted++
	t.names[name]++
}

// Finish records the end of a named effect.
func (t *EffectTracker) Finish(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active > 0 {
		t.active--
	}
	t.totalFinished++
	if n := t.names[name]; n > 0 {
		t.names[name] = n - 1
	}
}

// Active returns the number of in-flight effects.
func (t *EffectTracker) Active() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

// Snapshot returns active, started, finished counts.
func (t *EffectTracker) Snapshot() (active, started, finished int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active, t.totalStarted, t.totalFinished
}

// WaitUntilIdle blocks until Active()==0 or timeout. Returns true if idle.
// Uses NewTicker so testing/synctest can advance the fake clock when every
// bubble goroutine is durably blocked (ipk.12 / ipk.15).
func (t *EffectTracker) WaitUntilIdle(timeout time.Duration) bool {
	if t.Active() == 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if t.Active() == 0 {
			return true
		}
		if !time.Now().Before(deadline) {
			return t.Active() == 0
		}
		<-tick.C
	}
}

// effectDoneMsg reports cooperative-effect completion to Update.
type effectDoneMsg struct {
	Name      string
	Cancelled bool
	Err       string
}

// effectErrorMsg is an effect that completed with an application error.
type effectErrorMsg struct {
	Name string
	Err  string
}

// CooperativeSleep is a long-running tea.Cmd that blocks only on ctx or timer.
// It is the worked example for generated docs/ui-architecture.md (Section 18.7).
// Uses NewTimer+Stop so cancelled paths do not leak timers (ipk.12).
func CooperativeSleep(ctx context.Context, tracker *EffectTracker, name string, d time.Duration) tea.Cmd {
	return func() tea.Msg {
		tracker.Start(name)
		defer tracker.Finish(name)
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return effectDoneMsg{Name: name, Cancelled: true}
		case <-timer.C:
			return effectDoneMsg{Name: name, Cancelled: false}
		}
	}
}

// FailingEffect finishes promptly and reports an application error.
func FailingEffect(tracker *EffectTracker, name, errText string) tea.Cmd {
	return func() tea.Msg {
		tracker.Start(name)
		defer tracker.Finish(name)
		return effectErrorMsg{Name: name, Err: errText}
	}
}

// readyMsg is emitted once the model has rendered at least once.
type readyMsg struct{}

// panicMsg triggers a framework-recovered panic inside Update.
type panicMsg struct{}

// startEffectMsg requests starting a cooperative long effect.
type startEffectMsg struct {
	Name     string
	Duration time.Duration
}

// startFailingEffectMsg requests a failing effect.
type startFailingEffectMsg struct {
	Name string
	Err  string
}

// Signal that Init completed; used by tests waiting for program readiness.
type initDoneMsg struct{}

// atomicReady is set when View has run (program is pumping).
type atomicReady = atomic.Bool
