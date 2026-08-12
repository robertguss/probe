//go:build !unix

package fsx

import "github.com/robertguss/go-foundry-cli/internal/diagnostic"

// CommitResult is the Section 31.9 classified outcome (stub surface).
type CommitResult struct {
	Class       CommitClass
	Exit        int
	Err         error
	StageName   string
	StagePath   string
	DestName    string
	Message     string
	Syscall     string
	SyscallErr  error
	Observation CommitObservation
}

// Committed reports whether the destination holds the recorded stage identity.
func (r CommitResult) Committed() bool {
	return r.Class == ClassCommitted
}

// ErrorID returns the Appendix D identifier when Err is a FoundryError.
func (r CommitResult) ErrorID() diagnostic.Identifier {
	if r.Err == nil {
		return ""
	}
	if fe, ok := diagnostic.AsFoundryError(r.Err); ok {
		return fe.ID()
	}
	return ""
}

// Commit is unsupported off unix.
func Commit(stage *Stage) CommitResult {
	return CommitResult{
		Class:   ClassAmbiguous,
		Exit:    diagnostic.ExitFailure,
		Err:     unsupported("Commit"),
		Message: "fsx.Commit is only supported on macOS and Linux (unix)",
	}
}

// RenameSyscallName is a stub label off unix.
func RenameSyscallName() string {
	return "unsupported"
}
