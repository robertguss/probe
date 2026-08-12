package fsx

import "fmt"

// CommitClass is the post-syscall classification (Section 31.8).
// Classification is by identity inspection relative to the retained parent
// handle, not by the rename errno alone.
type CommitClass string

const (
	// ClassCommitted — destination holds the recorded stage identity.
	ClassCommitted CommitClass = "committed"
	// ClassUncommitted — stage still present with recorded identity; dest absent.
	ClassUncommitted CommitClass = "uncommitted"
	// ClassConflict — destination present with a different identity; stage preserved.
	ClassConflict CommitClass = "conflict"
	// ClassAmbiguous — contradictory or incomplete identities; stop mutation.
	ClassAmbiguous CommitClass = "ambiguous"
)

// String returns the stable class token.
func (c CommitClass) String() string { return string(c) }

// CommitObservation is the post-rename identity state relative to the parent
// handle (Section 31.8).
type CommitObservation struct {
	// SyscallOK is true when the exclusive rename returned a nil error.
	SyscallOK bool
	// SyscallErrno is the raw rename error (nil when SyscallOK).
	SyscallErrno error
	// StagePresent is true when the stage basename still exists under the parent.
	StagePresent bool
	// StageID is the observed stage child identity (when StagePresent).
	StageID FileID
	// DestPresent is true when the destination basename exists under the parent.
	DestPresent bool
	// DestID is the observed destination child identity (when DestPresent).
	DestID FileID
	// RecordedStage is the stage identity captured at CreateStage time.
	RecordedStage FileID
}

// ClassifyCommit implements Section 31.8 totally from identity inspection.
// It does not trust the syscall error value alone (false-negative acknowledgments).
func ClassifyCommit(obs CommitObservation) (CommitClass, string) {
	stageMatch := obs.StagePresent && obs.StageID.Equal(obs.RecordedStage)
	destIsStage := obs.DestPresent && obs.DestID.Equal(obs.RecordedStage)

	if obs.SyscallOK {
		if destIsStage {
			return ClassCommitted, "syscall ok; destination identity matches recorded stage"
		}
		return ClassAmbiguous, "syscall ok but destination identity does not match recorded stage"
	}

	// Syscall reported failure — inspect children relative to the parent handle.
	switch {
	case stageMatch && !obs.DestPresent:
		return ClassUncommitted, "stage present with recorded identity; destination absent"
	case !obs.StagePresent && destIsStage:
		return ClassCommitted, "false-negative: stage gone, destination holds recorded stage identity"
	case obs.DestPresent && !destIsStage:
		return ClassConflict, "destination present with different identity; stage preserved if present"
	default:
		return ClassAmbiguous, fmt.Sprintf(
			"contradictory identities stage_present=%v stage_id=%s dest_present=%v dest_id=%s recorded=%s",
			obs.StagePresent, obs.StageID, obs.DestPresent, obs.DestID, obs.RecordedStage,
		)
	}
}
