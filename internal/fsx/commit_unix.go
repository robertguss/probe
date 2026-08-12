//go:build unix

package fsx

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"golang.org/x/sys/unix"
)

// exclusiveRenameFn is the exclusive no-replace rename primitive. Production
// uses exclusiveRename (platform-specific). Tests may replace it to inject
// ENOTSUP/EXDEV/false-success acknowledgments (REQ-129/131).
var exclusiveRenameFn = exclusiveRename

// CommitResult is the Section 31.9 classified outcome of an exclusive
// no-replace commit. It carries identity-classification detail and the
// preserved stage location when the stage was not consumed by a successful
// rename. There is never automatic stage deletion (Section 31.6 / REQ-130).
//
// Reporting hooks (human + JSON): callers pass StagePath into
// report.Encoder.Failure / GenerateResult.StagePath. Use Report() for the
// full PreserveReport (RSK-310 remediation + class + ids).
type CommitResult struct {
	// Class is the Section 31.8 identity classification.
	Class CommitClass
	// Exit is the Section 31.9 exit code for this outcome (0, 1, or 2).
	// Cancellation (130) is classified by generate before Commit is called.
	Exit int
	// Err is a *diagnostic.FoundryError when Class is not committed; nil on success.
	// Non-committed errors always carry RSK-310 manual-removal remediation.
	Err error
	// StageName is the stage basename under the parent. Always set for logging;
	// when Class is not committed the stage entry is preserved at this name
	// (when still present — see Observation).
	StageName string
	// StagePath is the authored parent path joined with StageName (Section 31.6).
	// Set whenever StageName is known so every uncommitted outcome reports the
	// exact preserved location. Reporting surfaces must emit this field.
	StagePath string
	// DestName is the destination basename under the parent.
	DestName string
	// Message is the classification detail string (no host home paths).
	Message string
	// Syscall is the exclusive-rename primitive name (platform-specific).
	Syscall string
	// SyscallErr is the raw rename error (may be non-nil even when Class is
	// committed — false-negative acknowledgments).
	SyscallErr error
	// Observation is the post-syscall identity snapshot.
	Observation CommitObservation
}

// Committed reports whether the destination holds the recorded stage identity.
func (r CommitResult) Committed() bool {
	return r.Class == ClassCommitted
}

// ErrorID returns the Appendix D identifier when Err is a FoundryError;
// empty string on committed success.
func (r CommitResult) ErrorID() diagnostic.Identifier {
	if r.Err == nil {
		return ""
	}
	if fe, ok := diagnostic.AsFoundryError(r.Err); ok {
		return fe.ID()
	}
	return ""
}

// Commit renames the stage entry to the destination basename relative to the
// retained parent handle with exclusive no-replace semantics (Section 31.7)
// and classifies the result by post-syscall identity inspection (Section 31.8).
//
// Destination basename is parent.Basename() (the project name per Section 15.3).
//
// On every non-committed outcome the stage is preserved (REQ-130/131). This
// package never deletes a stage. There is no pathname fallback and no
// link/unlink/check-then-rename substitute when exclusive rename is unsupported.
func Commit(stage *Stage) CommitResult {
	sysName := renameSyscallName()
	if stage == nil || stage.fd < 0 {
		return commitFailClosed(ClassAmbiguous, diagnostic.IDFSCommitAmbiguous,
			"stage handle is closed or nil", "", "", "", sysName, nil, CommitObservation{})
	}
	parent := stage.parent
	parentPath := ""
	if parent != nil {
		parentPath = parent.authored
	}
	stagePath := StageLocation(parentPath, stage.name)
	if parent == nil || parent.fd < 0 {
		return commitFailClosed(ClassAmbiguous, diagnostic.IDFSCommitAmbiguous,
			"parent handle is closed or nil", stage.name, stagePath, "", sysName, nil, CommitObservation{
				RecordedStage: stage.id,
			})
	}

	destName := parent.base
	stageName := stage.name
	log := stage.log
	if log == nil {
		log = parent.log
	}

	// Diagnostic parent reobservation (31.7) — not a pathname mutation path.
	if err := parent.Reobserve(); err != nil {
		// Preserve stage; report exact location + RSK-310 (Section 31.6).
		err = withPreserveRemediation(err, stagePath)
		logStep(log, "commit", "parent_reobserve", "fail",
			fmt.Sprintf("stage=%q stage_path=%q dest=%q id=%s rsk=%s",
				stageName, stagePath, destName, diagnostic.IDFSParentMoved, RSK310))
		return CommitResult{
			Class:     ClassUncommitted,
			Exit:      diagnostic.ExitFailure,
			Err:       err,
			StageName: stageName,
			StagePath: stagePath,
			DestName:  destName,
			Message:   "parent reobservation failed; stage preserved; rename not issued",
			Syscall:   sysName,
		}
	}

	logStep(log, "commit", "rename", "info",
		fmt.Sprintf("syscall=%s stage=%q stage_path=%q dest=%q stage_identity=%s",
			sysName, stageName, stagePath, destName, stage.id))

	sysErr := exclusiveRenameFn(parent.fd, stageName, destName)
	obs := observeCommit(parent, stageName, destName, stage.id, sysErr)
	class, detail := ClassifyCommit(obs)

	res := CommitResult{
		Class:       class,
		StageName:   stageName,
		StagePath:   stagePath,
		DestName:    destName,
		Message:     detail,
		Syscall:     sysName,
		SyscallErr:  sysErr,
		Observation: obs,
	}

	switch class {
	case ClassCommitted:
		res.Exit = diagnostic.ExitSuccess
		res.Err = nil
		// Stage consumed by rename; StagePath retained for logs only.
		logStep(log, "commit", "classified", "pass",
			fmt.Sprintf("syscall=%s errno=%s class=%s outcome=committed exit=0 detail=%s dest=%q",
				sysName, errnoString(sysErr), class, detail, destName))
	case ClassUncommitted:
		id, msg := mapUncommittedError(sysErr, stageName, destName)
		res.Exit = diagnostic.ExitCodeFor(id)
		res.Err = newCommitError(id, msg, parent, stageName, destName)
		logStep(log, "commit", "classified", "fail",
			fmt.Sprintf("syscall=%s errno=%s class=%s id=%s exit=%d stage_preserved=%q stage_path=%q rsk=%s detail=%s",
				sysName, errnoString(sysErr), class, id, res.Exit, stageName, stagePath, RSK310, detail))
	case ClassConflict:
		// Destination present with different identity — concurrent winner or
		// pre-existing object; losing stage preserved (exit 2).
		id := diagnostic.IDFSDestinationExists
		msg := fmt.Sprintf(
			"destination %q exists (commit-time conflict); losing stage %q preserved",
			destName, stageName,
		)
		res.Exit = diagnostic.ExitCodeFor(id)
		res.Err = newCommitError(id, msg, parent, stageName, destName)
		logStep(log, "commit", "classified", "fail",
			fmt.Sprintf("syscall=%s errno=%s class=%s id=%s exit=%d stage_preserved=%q stage_path=%q rsk=%s detail=%s",
				sysName, errnoString(sysErr), class, id, res.Exit, stageName, stagePath, RSK310, detail))
	case ClassAmbiguous:
		id := diagnostic.IDFSCommitAmbiguous
		msg := fmt.Sprintf(
			"ambiguous commit outcome for stage %q → dest %q: %s",
			stageName, destName, detail,
		)
		res.Exit = diagnostic.ExitCodeFor(id)
		res.Err = newCommitError(id, msg, parent, stageName, destName)
		logStep(log, "commit", "classified", "fail",
			fmt.Sprintf("syscall=%s errno=%s class=%s id=%s exit=%d stage=%q stage_path=%q dest=%q rsk=%s detail=%s",
				sysName, errnoString(sysErr), class, id, res.Exit, stageName, stagePath, destName, RSK310, detail))
	default:
		// Defensive: unknown class → fail closed as ambiguous.
		id := diagnostic.IDFSCommitAmbiguous
		msg := fmt.Sprintf("unknown commit class %q: %s", class, detail)
		res.Class = ClassAmbiguous
		res.Exit = diagnostic.ExitCodeFor(id)
		res.Err = newCommitError(id, msg, parent, stageName, destName)
		logStep(log, "commit", "classified", "fail",
			fmt.Sprintf("syscall=%s class=%s id=%s exit=%d stage_path=%q rsk=%s detail=%s",
				sysName, res.Class, id, res.Exit, stagePath, RSK310, detail))
	}
	return res
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

// mapUncommittedError maps a failed exclusive rename errno for the uncommitted
// class (stage present, dest absent) per Section 31.7 / 31.9.
//
//	EEXIST              → fs.destination_exists (exit 2)
//	ENOTSUP/EOPNOTSUPP/EINVAL → fs.rename_unsupported (exit 1)
//	EXDEV               → fs.commit_failed (exit 1; unexpected, fail closed)
//	other               → fs.commit_failed (exit 1)
func mapUncommittedError(err error, stageName, destName string) (diagnostic.Identifier, string) {
	switch {
	case err == nil:
		// Uncommitted with nil errno is contradictory; still fail closed.
		return diagnostic.IDFSCommitFailed, fmt.Sprintf(
			"commit uncommitted with nil errno for stage %q → dest %q; stage preserved",
			stageName, destName,
		)
	case isExistErrno(err):
		return diagnostic.IDFSDestinationExists, fmt.Sprintf(
			"destination %q exists (EEXIST); losing stage %q preserved",
			destName, stageName,
		)
	case isRenameUnsupported(err):
		return diagnostic.IDFSRenameUnsupported, fmt.Sprintf(
			"filesystem lacks exclusive no-replace rename (errno=%s); stage %q preserved — manually mv after identity check",
			errnoString(err), stageName,
		)
	case isEXDEV(err):
		// Section 31.9 matrix: unexpected EXDEV → fs.commit_failed (fail closed).
		return diagnostic.IDFSCommitFailed, fmt.Sprintf(
			"unexpected EXDEV on exclusive rename (parent/stage crossed filesystems); stage %q preserved",
			stageName,
		)
	default:
		return diagnostic.IDFSCommitFailed, fmt.Sprintf(
			"commit rename failed (errno=%s); stage %q preserved",
			errnoString(err), stageName,
		)
	}
}

func newCommitError(id diagnostic.Identifier, msg string, parent *ParentHandle, stageName, destName string) error {
	// Location: authored parent + dest basename for destination_exists;
	// stage path for preservation-oriented errors.
	parentPath := ""
	if parent != nil {
		parentPath = parent.authored
	}
	stagePath := StageLocation(parentPath, stageName)
	var locPath string
	if id == diagnostic.IDFSDestinationExists && parentPath != "" && destName != "" {
		locPath = filepath.Join(parentPath, destName)
	} else if stagePath != "" {
		locPath = stagePath
	} else if destName != "" {
		locPath = destName
	} else {
		locPath = stageName
	}
	fe := diagnostic.New(id, msg, diagnostic.PathLocation(locPath))
	// Every uncommitted outcome carries RSK-310 manual inspect/remove text
	// naming the preserved stage (Section 31.6 / REQ-130).
	return attachPreserveRemediation(fe, stagePath)
}

func commitFailClosed(class CommitClass, id diagnostic.Identifier, msg, stageName, stagePath, destName, sysName string, sysErr error, obs CommitObservation) CommitResult {
	exit := diagnostic.ExitCodeFor(id)
	if stagePath == "" {
		stagePath = StageLocation("", stageName)
	}
	loc := stagePath
	if loc == "" {
		loc = stageName
	}
	fe := diagnostic.New(id, msg, diagnostic.PathLocation(loc))
	if class != ClassCommitted && stageName != "" {
		fe = attachPreserveRemediation(fe, stagePath)
	}
	return CommitResult{
		Class:       class,
		Exit:        exit,
		Err:         fe,
		StageName:   stageName,
		StagePath:   stagePath,
		DestName:    destName,
		Message:     msg,
		Syscall:     sysName,
		SyscallErr:  sysErr,
		Observation: obs,
	}
}

func isExistErrno(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, unix.EEXIST) || errors.Is(err, syscall.EEXIST) || errorsIsExist(err)
}

func isRenameUnsupported(err error) bool {
	if err == nil {
		return false
	}
	// ENOTSUP / EOPNOTSUPP / EINVAL — filesystems without no-replace (Section 31.7).
	return errors.Is(err, unix.ENOTSUP) ||
		errors.Is(err, unix.EOPNOTSUPP) ||
		errors.Is(err, unix.EINVAL) ||
		errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.EINVAL)
}

func isEXDEV(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, unix.EXDEV) || errors.Is(err, syscall.EXDEV)
}

// RenameSyscallName returns the platform exclusive-rename primitive label
// (for tests and agent logs).
func RenameSyscallName() string {
	return renameSyscallName()
}
