//go:build !unix

package fsx

import "github.com/robertguss/go-foundry-cli/internal/diagnostic"

// Begin is unsupported off unix (Foundry supports macOS+Linux only).
func Begin(destination string, opts BeginOptions) (*Transaction, error) {
	return nil, unsupported("Begin")
}

// Parent implements the unix API surface.
func (t *Transaction) Parent() *ParentHandle {
	if t == nil || t.closed {
		return nil
	}
	return t.parent
}

// Stage implements the unix API surface.
func (t *Transaction) Stage() *Stage {
	if t == nil || t.closed {
		return nil
	}
	return t.stage
}

// StageName implements the unix API surface.
func (t *Transaction) StageName() string {
	if t == nil || t.stage == nil {
		return ""
	}
	return t.stage.Name()
}

// StagePath implements the unix API surface.
func (t *Transaction) StagePath() string {
	if t == nil || t.stage == nil {
		return ""
	}
	return t.stage.Path()
}

// StageIdentity implements the unix API surface.
func (t *Transaction) StageIdentity() FileID {
	if t == nil || t.stage == nil {
		return FileID{}
	}
	return t.stage.Identity()
}

// DestName implements the unix API surface.
func (t *Transaction) DestName() string {
	if t == nil || t.parent == nil {
		return ""
	}
	return t.parent.Basename()
}

// RootedWriter implements the unix API surface.
func (t *Transaction) RootedWriter() *RootedWriter {
	if t == nil || t.closed || t.stage == nil {
		return nil
	}
	return t.stage.Writer()
}

// Writer is an alias for RootedWriter.
func (t *Transaction) Writer() *RootedWriter {
	return t.RootedWriter()
}

// DuplicateStageHandle is unsupported off unix.
func (t *Transaction) DuplicateStageHandle() (int, error) {
	return -1, unsupported("DuplicateStageHandle")
}

// Commit is unsupported off unix.
func (t *Transaction) Commit() CommitResult {
	return CommitResult{
		Class:   ClassAmbiguous,
		Exit:    diagnostic.ExitFailure,
		Err:     unsupported("Transaction.Commit"),
		Message: "fsx.Transaction.Commit is only supported on macOS and Linux (unix)",
	}
}

// Committed implements the unix API surface.
func (t *Transaction) Committed() bool {
	return t != nil && t.committed
}

// Close implements the unix API surface (no deletion).
func (t *Transaction) Close() error {
	if t == nil || t.closed {
		return nil
	}
	t.closed = true
	return nil
}
