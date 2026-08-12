package generate

import "fmt"

// LifecycleState is a Section 29.1 state-machine state name. Spellings match
// report.LifecycleState for stable event encoding.
type LifecycleState string

// Section 29.1 state names (stable).
const (
	LifePlanned         LifecycleState = "planned"
	LifeParentAcquired  LifecycleState = "parent-acquired"
	LifeStageCreated    LifecycleState = "stage-created"
	LifeRendered        LifecycleState = "rendered"
	LifeNormalized      LifecycleState = "normalized"
	LifeFrozen          LifecycleState = "frozen"
	LifeVerified        LifecycleState = "verified"
	LifeConformed       LifecycleState = "conformed"
	LifeGitInitialized  LifecycleState = "git-initialized"
	LifeCommitting      LifecycleState = "committing"
	LifeCommitted       LifecycleState = "committed"
	LifeConflicted      LifecycleState = "conflicted"
	LifeFailedPreserved LifecycleState = "failed-preserved"
	LifeAmbiguous       LifecycleState = "ambiguous"
	// LifeCancelled is a package-local terminal used when cancel wins before
	// commit. Not listed in Section 29.1's success path; emitted as a terminal
	// GenerationEvent with outcome "cancelled" (report.OutcomeCancelled).
	LifeCancelled LifecycleState = "cancelled"
)

// LifecycleStates returns every Section 29.1 normative state name in table
// order (excludes package-local LifeCancelled).
func LifecycleStates() []LifecycleState {
	return []LifecycleState{
		LifePlanned,
		LifeParentAcquired,
		LifeStageCreated,
		LifeRendered,
		LifeNormalized,
		LifeFrozen,
		LifeVerified,
		LifeConformed,
		LifeGitInitialized,
		LifeCommitting,
		LifeCommitted,
		LifeConflicted,
		LifeFailedPreserved,
		LifeAmbiguous,
	}
}

// TerminalStates are states from which no further lifecycle transition is legal.
func TerminalStates() []LifecycleState {
	return []LifecycleState{
		LifeCommitted,
		LifeConflicted,
		LifeFailedPreserved,
		LifeAmbiguous,
		LifeCancelled,
	}
}

// IsTerminal reports whether s is a terminal lifecycle state.
func IsTerminal(s LifecycleState) bool {
	switch s {
	case LifeCommitted, LifeConflicted, LifeFailedPreserved, LifeAmbiguous, LifeCancelled:
		return true
	default:
		return false
	}
}

// legalTransitions is the table-driven Section 29.1 success path plus failure
// and cancel terminals. Keys are "from" states; values are allowed "to" states.
// REQ-123: no skip/reorder — only these edges are legal.
var legalTransitions = map[LifecycleState][]LifecycleState{
	// Success path.
	LifePlanned:        {LifeParentAcquired, LifeFailedPreserved, LifeCancelled},
	LifeParentAcquired: {LifeStageCreated, LifeFailedPreserved, LifeCancelled},
	LifeStageCreated:   {LifeRendered, LifeFailedPreserved, LifeCancelled},
	LifeRendered:       {LifeNormalized, LifeFailedPreserved, LifeCancelled},
	LifeNormalized:     {LifeFrozen, LifeFailedPreserved, LifeCancelled},
	LifeFrozen:         {LifeVerified, LifeFailedPreserved, LifeCancelled},
	LifeVerified:       {LifeConformed, LifeFailedPreserved, LifeCancelled},
	LifeConformed:      {LifeGitInitialized, LifeCommitting, LifeFailedPreserved, LifeCancelled},
	LifeGitInitialized: {LifeCommitting, LifeFailedPreserved, LifeCancelled},
	LifeCommitting:     {LifeCommitted, LifeConflicted, LifeFailedPreserved, LifeAmbiguous},
	// Terminals have no outgoing edges.
	LifeCommitted:       nil,
	LifeConflicted:      nil,
	LifeFailedPreserved: nil,
	LifeAmbiguous:       nil,
	LifeCancelled:       nil,
}

// LegalTransition reports whether from→to is a documented edge.
func LegalTransition(from, to LifecycleState) bool {
	for _, next := range legalTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// TransitionError is returned when an illegal lifecycle transition is attempted.
type TransitionError struct {
	From LifecycleState
	To   LifecycleState
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("generate: illegal lifecycle transition %q → %q (REQ-123)", e.From, e.To)
}

// MustTransition validates from→to and returns a TransitionError when illegal.
// Used by the machine and by tests that probe API misuse / internal bugs.
func MustTransition(from, to LifecycleState) error {
	if LegalTransition(from, to) {
		return nil
	}
	return &TransitionError{From: from, To: to}
}

// AllLegalEdges returns every (from, to) pair in the transition table.
func AllLegalEdges() [][2]LifecycleState {
	var out [][2]LifecycleState
	// Deterministic order: LifecycleStates then package-local cancelled source edges.
	order := append(LifecycleStates(), LifeCancelled)
	for _, from := range order {
		for _, to := range legalTransitions[from] {
			out = append(out, [2]LifecycleState{from, to})
		}
	}
	return out
}

// stageCompletesState maps a successfully completed stage to the lifecycle
// state that becomes current after the stage. Empty means "no state change".
func stageCompletesState(id StageID) LifecycleState {
	switch id {
	case StageReportPlanNetwork:
		return LifePlanned
	case StageAcquireParent:
		return LifeParentAcquired
	case StageCreateStage:
		return LifeStageCreated
	case StageRender:
		return LifeRendered
	case StageGoModTidy:
		return LifeNormalized
	case StageFreezeTree:
		return LifeFrozen
	case StageVerifyTools:
		return LifeVerified
	case StageFinalConformance:
		return LifeConformed
	case StageGitTemplateCleanup:
		// Stages 15–16 complete optional git; always land on git-initialized
		// even when git.init is a no-op (stage still runs, no skip).
		return LifeGitInitialized
	case StageCommit:
		// Entering commit moves to committing; terminal is set from CommitResult.
		return LifeCommitting
	default:
		return ""
	}
}
