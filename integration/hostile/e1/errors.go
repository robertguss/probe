package e1

import "fmt"

// Stable error identifiers matching Section 31 / Appendix D (subset used by E1).
const (
	ErrUnsafePath          = "fs.unsafe_path"
	ErrParentMissing       = "fs.parent_missing"
	ErrNamespaceNotPrivate = "fs.namespace_not_private"
	ErrDestinationExists   = "fs.destination_exists"
	ErrRenameUnsupported   = "fs.rename_unsupported"
	ErrCommitFailed        = "fs.commit_failed"
	ErrCommitAmbiguous     = "fs.commit_ambiguous"
	ErrParentMoved         = "fs.parent_moved"
	ErrStageCreateFailed   = "fs.stage_create_failed"
)

// SpikeError is a typed diagnostic used by the spike (promotable to diagnostic).
type SpikeError struct {
	ID      string
	Message string
	Err     error
}

func (e *SpikeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.ID, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.ID, e.Message)
}

func (e *SpikeError) Unwrap() error { return e.Err }

func spikeErr(id, msg string, err error) *SpikeError {
	return &SpikeError{ID: id, Message: msg, Err: err}
}
