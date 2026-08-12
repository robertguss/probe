package e1

import (
	"fmt"
	"io/fs"
)

// FileID is a device/inode identity (SV-03 object continuity).
type FileID struct {
	Dev uint64
	Ino uint64
}

func (id FileID) String() string {
	return fmt.Sprintf("dev=%d ino=%d", id.Dev, id.Ino)
}

func (id FileID) Equal(other FileID) bool {
	return id.Dev == other.Dev && id.Ino == other.Ino
}

// CommitClass is the post-syscall classification (Section 31.8).
type CommitClass string

const (
	ClassCommitted   CommitClass = "committed"
	ClassUncommitted CommitClass = "uncommitted"
	ClassConflict    CommitClass = "conflict"
	ClassAmbiguous   CommitClass = "ambiguous"
)

// CommitObservation is the post-rename identity state relative to the parent handle.
type CommitObservation struct {
	SyscallOK     bool
	SyscallErrno  error
	StagePresent  bool
	StageID       FileID
	DestPresent   bool
	DestID        FileID
	RecordedStage FileID
}

// ClassifyCommit implements Section 31.8 totally from identity inspection.
// It does not trust the syscall error value alone.
func ClassifyCommit(obs CommitObservation) (CommitClass, string) {
	stageMatch := obs.StagePresent && obs.StageID.Equal(obs.RecordedStage)
	destIsStage := obs.DestPresent && obs.DestID.Equal(obs.RecordedStage)

	if obs.SyscallOK {
		if destIsStage {
			return ClassCommitted, "syscall ok; destination identity matches recorded stage"
		}
		return ClassAmbiguous, "syscall ok but destination identity does not match recorded stage"
	}

	// Syscall reported failure — inspect children.
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

// MapUncommittedError maps a failed exclusive rename errno to a stable id (31.7).
func MapUncommittedError(err error) string {
	if err == nil {
		return ErrCommitFailed
	}
	if isExist(err) {
		return ErrDestinationExists
	}
	if isRenameUnsupported(err) {
		return ErrRenameUnsupported
	}
	if isEXDEV(err) {
		return ErrCommitFailed // unexpected EXDEV — fail closed
	}
	return ErrCommitFailed
}

func isExist(err error) bool {
	return err != nil && (isErrno(err, errExist) || isPathExist(err))
}

func isPathExist(err error) bool {
	return err != nil && (err == fs.ErrExist || underlyingIsExist(err))
}
