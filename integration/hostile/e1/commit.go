//go:build unix

package e1

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// CommitResult is the classified outcome of an exclusive no-replace commit.
type CommitResult struct {
	Class      CommitClass
	ErrorID    string // empty on committed
	Message    string
	SyscallErr error
	StageName  string
	DestName   string
	Obs        CommitObservation
}

// Commit renames stage → dest relative to the retained parent handle with
// exclusive no-replace semantics (31.7) and classifies via identity (31.8).
// There is no pathname fallback and no stage deletion on any outcome.
func Commit(parent *ParentHandle, stage *Stage, destName string, log *ProbeLog, fsLabel string) CommitResult {
	if log == nil {
		log = NewProbeLog()
	}

	// Diagnostic parent reobservation (31.7) — not a pathname mutation path.
	if err := reobserveParent(parent, log, fsLabel); err != nil {
		return CommitResult{
			Class:     ClassUncommitted,
			ErrorID:   ErrParentMoved,
			Message:   err.Error(),
			StageName: stage.Name,
			DestName:  destName,
		}
	}

	sysErr := exclusiveRename(parent.FD, stage.Name, destName)
	obs := observeCommit(parent, stage.Name, destName, stage.ID, sysErr)
	class, detail := ClassifyCommit(obs)

	res := CommitResult{
		Class:      class,
		SyscallErr: sysErr,
		StageName:  stage.Name,
		DestName:   destName,
		Obs:        obs,
		Message:    detail,
	}

	switch class {
	case ClassCommitted:
		res.ErrorID = ""
		log.Record(ProbeEntry{FS: fsLabel, Probe: "commit", Step: "classified",
			Syscall: renameSyscallName(), Args: fmt.Sprintf("%s -> %s", stage.Name, destName),
			Errno: errnoString(sysErr), Outcome: "pass", Detail: detail})
	case ClassUncommitted:
		res.ErrorID = MapUncommittedError(sysErr)
		log.Record(ProbeEntry{FS: fsLabel, Probe: "commit", Step: "classified",
			Syscall: renameSyscallName(), Args: fmt.Sprintf("%s -> %s", stage.Name, destName),
			Errno: errnoString(sysErr), Outcome: "fail",
			Detail: fmt.Sprintf("%s: %s (stage preserved: %s)", res.ErrorID, detail, stage.Name)})
	case ClassConflict:
		res.ErrorID = ErrDestinationExists
		log.Record(ProbeEntry{FS: fsLabel, Probe: "commit", Step: "classified",
			Syscall: renameSyscallName(), Args: fmt.Sprintf("%s -> %s", stage.Name, destName),
			Errno: errnoString(sysErr), Outcome: "fail",
			Detail: fmt.Sprintf("%s: %s (stage preserved: %s)", res.ErrorID, detail, stage.Name)})
	case ClassAmbiguous:
		res.ErrorID = ErrCommitAmbiguous
		log.Record(ProbeEntry{FS: fsLabel, Probe: "commit", Step: "classified",
			Syscall: renameSyscallName(), Args: fmt.Sprintf("%s -> %s", stage.Name, destName),
			Errno: errnoString(sysErr), Outcome: "fail",
			Detail: fmt.Sprintf("%s: %s", res.ErrorID, detail)})
	}
	return res
}

func reobserveParent(parent *ParentHandle, log *ProbeLog, fsLabel string) error {
	// Re-open authored path and compare identity with retained handle.
	fd, err := unix.Open(parent.Authored, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_reobserve", Step: "open_authored",
			Syscall: "open", Args: parent.Authored, Errno: errnoString(err), Outcome: "fail"})
		return spikeErr(ErrParentMoved, "cannot re-open authored parent", err)
	}
	defer unix.Close(fd)
	id, err := fileIDFromFD(fd)
	if err != nil {
		return err
	}
	if !id.Equal(parent.ID) {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_reobserve", Step: "identity",
			Outcome: "fail", Detail: fmt.Sprintf("retained=%s observed=%s", parent.ID, id)})
		return spikeErr(ErrParentMoved, fmt.Sprintf("retained %s vs observed %s", parent.ID, id), nil)
	}
	log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_reobserve", Step: "identity",
		Outcome: "pass", Detail: id.String()})
	return nil
}

func observeCommit(parent *ParentHandle, stageName, destName string, recorded FileID, sysErr error) CommitObservation {
	obs := CommitObservation{
		SyscallOK:     sysErr == nil,
		SyscallErrno:  sysErr,
		RecordedStage: recorded,
	}
	if exists, id, err := parent.ChildLookup(stageName); err == nil {
		obs.StagePresent = exists
		obs.StageID = id
	}
	if exists, id, err := parent.ChildLookup(destName); err == nil {
		obs.DestPresent = exists
		obs.DestID = id
	}
	return obs
}
