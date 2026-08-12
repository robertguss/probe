package fsx

import (
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Convenience constructors for Appendix D fs.* errors. Location is always a
// path (Section 38.2). Remediation defaults come from the diagnostic registry.

func errUnsafePath(path, msg string) *diagnostic.FoundryError {
	return diagnostic.New(diagnostic.IDFSUnsafePath, msg, diagnostic.PathLocation(path))
}

func errParentMissing(path, msg string) *diagnostic.FoundryError {
	return diagnostic.New(diagnostic.IDFSParentMissing, msg, diagnostic.PathLocation(path))
}

func errNamespaceNotPrivate(path, msg string) *diagnostic.FoundryError {
	return diagnostic.New(diagnostic.IDFSNamespaceNotPrivate, msg, diagnostic.PathLocation(path))
}

func errDestinationExists(path, msg string) *diagnostic.FoundryError {
	return diagnostic.New(diagnostic.IDFSDestinationExists, msg, diagnostic.PathLocation(path))
}

func errParentMoved(path, msg string) *diagnostic.FoundryError {
	return diagnostic.New(diagnostic.IDFSParentMoved, msg, diagnostic.PathLocation(path))
}

// errStageCreate maps stage-creation failures (mkdirat, exhaust EEXIST retries,
// open/root bind) to fs.commit_failed (Appendix D exit 1; stage preserved when
// any entry was created). There is no separate stage_create_failed identifier
// in Appendix D; commit_failed is the fail-closed fs.* bucket for non-commit
// transaction failures that preserve staging state (step 9, Section 29.2).
//
// Remediation always includes RSK-310 manual inspect/remove text (Section 31.6).
func errStageCreate(path, msg string) *diagnostic.FoundryError {
	return attachPreserveRemediation(
		diagnostic.New(diagnostic.IDFSCommitFailed, msg, diagnostic.PathLocation(path)),
		path,
	)
}

// errStageIdentity maps mid-transaction stage identity mismatches to
// fs.commit_failed (exit 1; stop mutation; stage state reported).
//
// Remediation always includes RSK-310 manual inspect/remove text (Section 31.6).
func errStageIdentity(path, msg string) *diagnostic.FoundryError {
	base := diagnostic.New(diagnostic.IDFSCommitFailed, msg, diagnostic.PathLocation(path)).
		WithRemediation(
			"Stop and inspect the stage basename under the destination parent before any further mutation. " +
				"A stage identity change indicates a concurrent rename or replacement; Foundry will not delete the stage.",
		)
	return attachPreserveRemediation(base, path)
}

// wrapCause attaches a low-level cause without changing the identifier.
func wrapCause(fe *diagnostic.FoundryError, cause error) *diagnostic.FoundryError {
	if fe == nil || cause == nil {
		return fe
	}
	return diagnostic.Wrap(fe.ID(), fe.Message(), fe.Location(), cause)
}
