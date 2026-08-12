package generate_test

import (
	"context"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestGenerationEventShapeContract codifies the generate → report event
// contract (Section 29.1 / 43). Every emitted GenerationEvent must have a
// known Kind; progress events carry a known Stage; state/terminal events carry
// a known LifecycleState; terminal events appear exactly once and carry a
// known Outcome.
func TestGenerationEventShapeContract(t *testing.T) {
	log := testutil.New(t)
	log.Phase("event_shape_contract")

	cases := []struct {
		name        string
		cancel      bool
		wantOutcome generate.CommitOutcome
	}{
		{
			name:        "happy_path",
			cancel:      false,
			wantOutcome: generate.OutcomeCommitted,
		},
		{
			name:        "cancelled_before_run",
			cancel:      true,
			wantOutcome: generate.OutcomeCancelled,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase(tc.name)

			sink := &generate.CollectingSink{}
			m := generate.New(generate.Config{Sink: sink})

			ctx := context.Background()
			if tc.cancel {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			res := m.Run(ctx)

			if res.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %q, want %q", res.Outcome, tc.wantOutcome)
			}

			terminal := 0
			for i, ev := range sink.Events {
				switch ev.Kind {
				case generate.EventProgress:
					if ev.Stage == "" {
						t.Fatalf("event %d: progress event has empty Stage", i)
					}
					if !generate.ValidStageID(ev.Stage) {
						t.Fatalf("event %d: progress event has unknown Stage %q", i, ev.Stage)
					}
				case generate.EventState:
					if ev.State == "" {
						t.Fatalf("event %d: state event has empty State", i)
					}
					if !knownLifecycleState(ev.State) {
						t.Fatalf("event %d: state event has unknown State %q", i, ev.State)
					}
				case generate.EventNetwork:
					if ev.Stage != generate.StageReportPlanNetwork {
						t.Fatalf("event %d: network event Stage = %q, want %q", i, ev.Stage, generate.StageReportPlanNetwork)
					}
					if len(ev.Lines) == 0 {
						t.Fatalf("event %d: network event has no Lines", i)
					}
				case generate.EventSummary:
					if ev.Detail == "" {
						t.Fatalf("event %d: summary event has empty Detail", i)
					}
				case generate.EventTerminal:
					terminal++
					if ev.State == "" {
						t.Fatalf("event %d: terminal event has empty State", i)
					}
					if !generate.IsTerminal(ev.State) {
						t.Fatalf("event %d: terminal event State %q is not terminal", i, ev.State)
					}
					if !knownOutcome(ev.Outcome) {
						t.Fatalf("event %d: terminal event has unknown Outcome %q", i, ev.Outcome)
					}
				default:
					t.Fatalf("event %d: unknown Kind %q", i, ev.Kind)
				}
			}
			if terminal != 1 {
				t.Fatalf("expected exactly 1 terminal event, got %d", terminal)
			}
			sub.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("event_shape_contract", testutil.OutcomeOK)
}

func knownLifecycleState(s generate.LifecycleState) bool {
	for _, ls := range generate.LifecycleStates() {
		if ls == s {
			return true
		}
	}
	// Package-local terminal state used for cancellations.
	return s == generate.LifeCancelled
}

func knownOutcome(o generate.CommitOutcome) bool {
	switch o {
	case generate.OutcomeCommitted, generate.OutcomeConflicted, generate.OutcomeFailedPreserved,
		generate.OutcomeAmbiguous, generate.OutcomeCancelled, generate.OutcomeNotStarted:
		return true
	}
	return false
}
