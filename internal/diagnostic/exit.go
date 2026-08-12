package diagnostic

import (
	"context"
	"errors"
)

// Process exit codes (Section 38.1 / Appendix D).
const (
	// ExitSuccess is success, including successful commit with failed report stream.
	ExitSuccess = 0
	// ExitFailure is runtime/system/tool/verification failure.
	ExitFailure = 1
	// ExitUsage is input error: specification, selection, destination, usage.
	ExitUsage = 2
	// ExitCancelled is SIGINT/SIGTERM before the commit point (Appendix D).
	// Cancellation is not a domain.reason registry identifier.
	ExitCancelled = 130
)

// ErrCancelled is the sentinel for pre-commit cancellation (exit 130).
// It is not a registered domain.reason identifier.
var ErrCancelled = errors.New("foundry: cancelled before commit")

// ExitCode maps an error to a process exit code for the CLI boundary.
// nil → 0; cancellation (ErrCancelled / context.Canceled) → 130;
// *FoundryError → registry exit; other errors → 1.
//
// Does not store context; only inspects the error chain (REQ-188).
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, ErrCancelled) || errors.Is(err, context.Canceled) {
		return ExitCancelled
	}
	var fe *FoundryError
	if errors.As(err, &fe) {
		return fe.ExitCode()
	}
	return ExitFailure
}

// IsCancelled reports whether err represents pre-commit cancellation.
func IsCancelled(err error) bool {
	return err != nil && (errors.Is(err, ErrCancelled) || errors.Is(err, context.Canceled))
}
