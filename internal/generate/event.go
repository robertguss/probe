package generate

// EventKind distinguishes GenerationEvent categories (Section 29.1 / 43).
// Values are stable strings for goldens and report mapping.
type EventKind string

const (
	// EventProgress is emitted when a Section 29.2 stage begins.
	EventProgress EventKind = "progress"
	// EventState is emitted on a lifecycle state transition.
	EventState EventKind = "state"
	// EventNetwork is emitted for pre-staging network disclosure lines.
	EventNetwork EventKind = "network"
	// EventSummary is optional human detail (report encodes; machine rarely emits).
	EventSummary EventKind = "summary"
	// EventTerminal is emitted once with the final outcome.
	EventTerminal EventKind = "terminal"
)

// CommitOutcome is the generate-side terminal/commit classification string.
// Values match report.CommitOutcome for stable JSON field values (REQ-156).
type CommitOutcome string

const (
	OutcomeCommitted       CommitOutcome = "committed"
	OutcomeConflicted      CommitOutcome = "conflicted"
	OutcomeFailedPreserved CommitOutcome = "failed-preserved"
	OutcomeAmbiguous       CommitOutcome = "ambiguous"
	OutcomeCancelled       CommitOutcome = "cancelled"
	OutcomeNotStarted      CommitOutcome = "not-started"
)

// GenerationEvent is a typed lifecycle unit emitted by the state machine and
// consumed by report (Section 29.1 / 43). The machine performs no output
// encoding; report owns text/JSON formatting.
type GenerationEvent struct {
	// Kind is progress, state, network, summary, or terminal.
	Kind EventKind
	// Stage is a Section 29.2 StageID when Kind is progress.
	Stage StageID
	// State is a Section 29.1 LifecycleState when Kind is state (or terminal).
	State LifecycleState
	// Detail is optional human text (must be host-independent in goldens).
	Detail string
	// Lines carries network disclosure lines when Kind is network.
	Lines []string
	// Outcome is set for terminal events.
	Outcome CommitOutcome
	// StagePath is set when a stage is preserved (basename-safe in tests).
	StagePath string
	// Destination is set on committed terminal events (basename-safe in tests).
	Destination string
}

// EventSink receives typed lifecycle events. Implementations must not encode
// for the user-facing report stream here — that is report's job. A nil sink
// is a no-op.
type EventSink interface {
	// OnEvent is invoked for every GenerationEvent in emission order.
	OnEvent(ev GenerationEvent)
}

// NopSink discards events.
type NopSink struct{}

// OnEvent implements EventSink.
func (NopSink) OnEvent(GenerationEvent) {}

// CollectingSink records events for tests and orchestration.
type CollectingSink struct {
	Events []GenerationEvent
}

// OnEvent implements EventSink.
func (s *CollectingSink) OnEvent(ev GenerationEvent) {
	if s == nil {
		return
	}
	s.Events = append(s.Events, ev)
}

// EventSequence formats events as stable golden lines (no wall-clock).
// Format per line: kind|stage|state|outcome|detail
func EventSequence(events []GenerationEvent) string {
	var b []byte
	for i, ev := range events {
		if i > 0 {
			b = append(b, '\n')
		}
		line := string(ev.Kind) + "|" + string(ev.Stage) + "|" + string(ev.State) + "|" + string(ev.Outcome)
		if ev.Detail != "" {
			line += "|" + ev.Detail
		}
		if ev.StagePath != "" {
			line += "|stage_path=" + ev.StagePath
		}
		if ev.Destination != "" {
			line += "|dest=" + ev.Destination
		}
		b = append(b, line...)
	}
	if len(events) > 0 {
		b = append(b, '\n')
	}
	return string(b)
}

// OutcomeFromLifecycle maps a terminal lifecycle state to CommitOutcome.
func OutcomeFromLifecycle(s LifecycleState) CommitOutcome {
	switch s {
	case LifeCommitted:
		return OutcomeCommitted
	case LifeConflicted:
		return OutcomeConflicted
	case LifeFailedPreserved:
		return OutcomeFailedPreserved
	case LifeAmbiguous:
		return OutcomeAmbiguous
	case LifeCancelled:
		return OutcomeCancelled
	default:
		return OutcomeNotStarted
	}
}

// LifecycleFromCommitOutcome maps a commit classification to terminal state.
func LifecycleFromCommitOutcome(o CommitOutcome) LifecycleState {
	switch o {
	case OutcomeCommitted:
		return LifeCommitted
	case OutcomeConflicted:
		return LifeConflicted
	case OutcomeFailedPreserved:
		return LifeFailedPreserved
	case OutcomeAmbiguous:
		return LifeAmbiguous
	case OutcomeCancelled:
		return LifeCancelled
	default:
		return LifeFailedPreserved
	}
}
