package generate

import (
	"errors"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Result is the total outcome of one Machine.Run (Section 29 / 31.9 / REQ-036).
type Result struct {
	// State is the terminal (or last) lifecycle state.
	State LifecycleState
	// Outcome is the commit/cancel classification for reporting.
	Outcome CommitOutcome
	// Exit is the process exit code the CLI boundary should use.
	Exit int
	// StagePreserved is true when a stage directory exists and must be kept.
	StagePreserved bool
	// StagePath is the preserved stage location (when StagePreserved).
	StagePath string
	// Destination is set when Outcome is committed.
	Destination string
	// FailedStage is the stage that failed or was active at cancel (empty on
	// clean commit success through report).
	FailedStage StageID
	// Err is the underlying error (nil on success / pure cancel).
	Err error
	// Events is the full emission sequence (also delivered to EventSink).
	Events []GenerationEvent
	// CommitDominated is true when cancel raced commit and the commit result
	// classified the exit (REQ-036 / Section 29.2 stage 18).
	CommitDominated bool
}

// CancelClass classifies a cancellation relative to stage/commit progress.
type CancelClass string

const (
	// CancelBeforeStage — stages 1–8 incomplete; nothing on disk; exit 130.
	CancelBeforeStage CancelClass = "before-stage"
	// CancelAfterStage — stage exists, pre-commit; preserve + exit 130.
	CancelAfterStage CancelClass = "after-stage"
	// CancelRacingCommit — concurrent with stage 18; commit result dominates.
	CancelRacingCommit CancelClass = "racing-commit"
)

// ClassifyCancel implements REQ-036 cancel points.
//
//	stageExists false → before-stage (130, nothing on disk)
//	stageExists true && !inCommit → after-stage (130, preserve)
//	inCommit true → racing-commit (commit result dominates)
func ClassifyCancel(stageExists, inCommit bool) CancelClass {
	if inCommit {
		return CancelRacingCommit
	}
	if stageExists {
		return CancelAfterStage
	}
	return CancelBeforeStage
}

// ApplyCancel builds a Result for a cancel that wins (not racing commit).
func ApplyCancel(class CancelClass, stagePath string, failed StageID, events []GenerationEvent) Result {
	r := Result{
		State:       LifeCancelled,
		Outcome:     OutcomeCancelled,
		Exit:        diagnostic.ExitCancelled,
		FailedStage: failed,
		Err:         diagnostic.ErrCancelled,
		Events:      events,
	}
	switch class {
	case CancelBeforeStage:
		r.StagePreserved = false
		r.StagePath = ""
	case CancelAfterStage:
		r.StagePreserved = true
		r.StagePath = stagePath
	case CancelRacingCommit:
		// Caller must not use ApplyCancel for racing-commit; commit dominates.
		r.CommitDominated = true
	}
	return r
}

// StageError is returned by a stage function to control exit and preserve.
type StageError struct {
	// Exit overrides DefaultFailureExit when non-zero.
	Exit int
	// Preserve forces stage preservation reporting when the runtime has a stage.
	Preserve bool
	// Outcome overrides the terminal CommitOutcome when non-empty (commit stage).
	Outcome CommitOutcome
	// Err is the underlying cause (may be *diagnostic.FoundryError).
	Err error
	// Detail is optional machine-facing detail (host-independent).
	Detail string
}

func (e *StageError) Error() string {
	if e == nil {
		return "generate: stage error"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Detail != "" {
		return e.Detail
	}
	return "generate: stage error"
}

func (e *StageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// AsStageError extracts a *StageError from err's chain.
func AsStageError(err error) (*StageError, bool) {
	var se *StageError
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}
