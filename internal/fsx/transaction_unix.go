//go:build unix

package fsx

import (
	"fmt"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"golang.org/x/sys/unix"
)

// Begin runs destination preflight (parent walk + custody + absent dest) then
// exclusive stage create relative to the retained parent (Section 31.2–31.5).
// On success the caller owns the Transaction and must Close it. On any error
// after stage creation the stage entry is preserved (no auto-delete).
//
// Begin never deletes a stage and never exposes a raw path for mutation.
func Begin(destination string, opts BeginOptions) (*Transaction, error) {
	log := opts.Log
	logStep(log, "transaction", "begin", "info",
		fmt.Sprintf("destination=%q project=%q", destination, opts.Project))

	parent, err := Preflight(destination, PreflightOptions{Log: log})
	if err != nil {
		logStep(log, "transaction", "preflight", "fail", err.Error())
		return nil, err
	}

	project := strings.TrimSpace(opts.Project)
	if project == "" {
		project = parent.Basename()
	}

	stage, err := CreateStage(parent, project)
	if err != nil {
		// Stage may or may not exist; CreateStage never deletes. Release parent.
		_ = parent.Close()
		logStep(log, "transaction", "stage_create", "fail", err.Error())
		return nil, err
	}
	if err := stage.VerifyIdentity(); err != nil {
		stagePath := stage.Path()
		_ = stage.Close()
		_ = parent.Close()
		logStep(log, "transaction", "identity", "fail",
			fmt.Sprintf("identity verify failed stage_path=%q rsk=%s", stagePath, RSK310))
		return nil, err
	}

	tx := &Transaction{
		parent: parent,
		stage:  stage,
		log:    log,
	}
	// "opened" is the integrated Transaction ready signal (E1-wired surface).
	logStep(log, "transaction", "opened", "pass",
		fmt.Sprintf("stage=%q stage_path=%q stage_identity=%s parent_identity=%s dest=%q",
			stage.Name(), stage.Path(), stage.Identity(), parent.Identity(), parent.Basename()))
	return tx, nil
}

// Parent returns the retained destination-parent handle. Nil when closed.
func (t *Transaction) Parent() *ParentHandle {
	if t == nil || t.closed {
		return nil
	}
	return t.parent
}

// Stage returns the retained stage handle. Nil when closed.
//
// Callers outside fsx must not use Stage for pathname-based mutation of the
// destination namespace. Prefer RootedWriter / DuplicateStageHandle / Commit.
func (t *Transaction) Stage() *Stage {
	if t == nil || t.closed {
		return nil
	}
	return t.stage
}

// StageName returns the stage basename under the parent (.foundry-<name>-<random>).
func (t *Transaction) StageName() string {
	if t == nil || t.stage == nil {
		return ""
	}
	return t.stage.Name()
}

// StagePath returns the diagnostic stage location (authored parent + basename).
// For reporting only — never use for pathname-based mutation.
func (t *Transaction) StagePath() string {
	if t == nil || t.stage == nil {
		return ""
	}
	return t.stage.Path()
}

// StageIdentity returns the recorded stage device/inode (SV-03 continuity).
func (t *Transaction) StageIdentity() FileID {
	if t == nil || t.stage == nil {
		return FileID{}
	}
	return t.stage.Identity()
}

// DestName returns the destination basename under the parent.
func (t *Transaction) DestName() string {
	if t == nil || t.parent == nil {
		return ""
	}
	return t.parent.Basename()
}

// RootedWriter returns the RootedWriter bound to the stage object. All writes
// are descriptor-relative; absolute paths and ".." are rejected. Nil when closed.
func (t *Transaction) RootedWriter() *RootedWriter {
	if t == nil || t.stage == nil || t.closed {
		return nil
	}
	return t.stage.Writer()
}

// Writer is an alias for RootedWriter (generate-facing convenience).
func (t *Transaction) Writer() *RootedWriter {
	return t.RootedWriter()
}

// DuplicateStageHandle returns a new CLOEXEC directory file descriptor that
// refers to the same stage object as the retained stage handle (Section 31 /
// 34.4). toolrun uses the returned FD for serialized fchdir child start and
// never re-opens the stage by pathname.
//
// The caller owns the returned FD and MUST close it (unix.Close / toolrun
// CloseFD). This method never closes the transaction's retained stage FD.
// Identity is re-verified before the dup so a mid-transaction stage swap is
// detected (REQ-125/184).
//
// On error the returned fd is -1.
func (t *Transaction) DuplicateStageHandle() (int, error) {
	if t == nil || t.closed {
		return -1, errStageIdentity("", "transaction is closed or nil; cannot duplicate stage handle")
	}
	if t.stage == nil || t.stage.fd < 0 {
		return -1, errStageIdentity(t.StagePath(), "stage handle is closed or nil; cannot duplicate stage handle")
	}
	if err := t.stage.VerifyIdentity(); err != nil {
		logStep(t.log, "transaction", "dup_stage", "fail",
			fmt.Sprintf("identity verify failed stage=%q stage_path=%q rsk=%s",
				t.stage.Name(), t.StagePath(), RSK310))
		return -1, err
	}

	// F_DUPFD_CLOEXEC: independent FD with CLOEXEC so it does not leak across
	// unintended execs; toolrun's bound start path owns/closes the duplicate.
	n, err := unix.FcntlInt(uintptr(t.stage.fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		// Fallback: Dup + explicit F_SETFD (older kernels without F_DUPFD_CLOEXEC).
		n, err = unix.Dup(t.stage.fd)
		if err != nil {
			stagePath := t.StagePath()
			logStep(t.log, "transaction", "dup_stage", "fail",
				fmt.Sprintf("dup failed stage=%q stage_path=%q errno=%s",
					t.stage.Name(), stagePath, errnoString(err)))
			return -1, wrapCause(
				errStageIdentity(stagePath, fmt.Sprintf("duplicate stage handle failed: %v", err)),
				err,
			)
		}
		// Set CLOEXEC via FcntlInt so we get error feedback (unix.CloseOnExec
		// is void and silently drops errors). Close the FD on failure to prevent
		// leaking it into child processes.
		if _, cloErr := unix.FcntlInt(uintptr(n), unix.F_SETFD, unix.FD_CLOEXEC); cloErr != nil {
			_ = unix.Close(n)
			stagePath := t.StagePath()
			logStep(t.log, "transaction", "dup_stage", "fail",
				fmt.Sprintf("F_SETFD CLOEXEC failed stage=%q stage_path=%q errno=%s",
					t.stage.Name(), stagePath, errnoString(cloErr)))
			return -1, wrapCause(
				errStageIdentity(stagePath, fmt.Sprintf("set CLOEXEC on duplicated handle failed: %v", cloErr)),
				cloErr,
			)
		}
	}

	// Confirm the dup refers to the same object (SV-03).
	dupID, idErr := fileIDFromFD(n)
	if idErr != nil {
		_ = unix.Close(n)
		return -1, wrapCause(
			errStageIdentity(t.StagePath(), "cannot fstat duplicated stage descriptor"),
			idErr,
		)
	}
	if !dupID.Equal(t.stage.id) {
		_ = unix.Close(n)
		logStep(t.log, "transaction", "dup_stage", "fail",
			fmt.Sprintf("identity mismatch recorded=%s dup=%s", t.stage.id, dupID))
		return -1, errStageIdentity(t.StagePath(),
			fmt.Sprintf("duplicated stage identity mismatch: recorded %s vs dup %s", t.stage.id, dupID))
	}

	logStep(t.log, "transaction", "dup_stage", "pass",
		fmt.Sprintf("stage=%q stage_path=%q stage_identity=%s dup_fd=%d",
			t.stage.Name(), t.StagePath(), t.stage.Identity(), n))
	return n, nil
}

// Commit performs the exclusive no-replace rename relative to the retained
// parent and classifies the outcome per Section 31.8–31.9. The stage is never
// deleted on failure (REQ-130/131). Safe to call at most once productively;
// a second call after committed success classifies fail-closed (stage gone).
func (t *Transaction) Commit() CommitResult {
	sysName := renameSyscallName()
	if t == nil || t.closed {
		return commitFailClosed(ClassAmbiguous, diagnostic.IDFSCommitAmbiguous,
			"transaction is closed or nil", "", "", "", sysName, nil, CommitObservation{})
	}
	if t.stage == nil {
		return commitFailClosed(ClassAmbiguous, diagnostic.IDFSCommitAmbiguous,
			"stage handle is nil", "", "", "", sysName, nil, CommitObservation{})
	}
	logStep(t.log, "transaction", "commit", "info",
		fmt.Sprintf("stage=%q stage_path=%q dest=%q", t.stage.Name(), t.StagePath(), t.DestName()))
	res := Commit(t.stage)
	if res.Committed() {
		t.committed = true
	}
	return res
}

// Committed reports whether Commit already classified the destination as
// holding the recorded stage identity.
func (t *Transaction) Committed() bool {
	return t != nil && t.committed
}

// Close releases stage and parent descriptors. It does NOT unlink or remove
// the stage entry (Section 31.6). Safe after Commit (stage may already be
// renamed away; closing the stage FD is still required). Idempotent.
func (t *Transaction) Close() error {
	if t == nil || t.closed {
		return nil
	}
	t.closed = true
	var first error
	if t.stage != nil {
		if err := t.stage.Close(); err != nil && first == nil {
			first = err
		}
		// Keep stage pointer for StageName/StagePath diagnostics after close;
		// DirFD is already -1 inside Stage.Close.
	}
	if t.parent != nil {
		if err := t.parent.Close(); err != nil && first == nil {
			first = err
		}
	}
	logStep(t.log, "transaction", "close", "pass",
		fmt.Sprintf("committed=%v stage=%q stage_path=%q (handles released; stage not deleted)",
			t.committed, t.StageName(), t.StagePath()))
	return first
}
