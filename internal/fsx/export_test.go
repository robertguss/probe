//go:build unix

package fsx

import (
	"os"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// SetStageSuffixForTest replaces the stage name suffix generator with a
// fixed sequence (for EEXIST retry / exhaust tests). Returns a restore func.
// Not part of the production API surface.
func SetStageSuffixForTest(t *testing.T, suffixes []string) (restore func()) {
	t.Helper()
	prev := stageSuffix
	i := 0
	stageSuffix = func() (string, error) {
		if i >= len(suffixes) {
			// Fall back to real randomness after the scripted sequence.
			return randomHex(8)
		}
		s := suffixes[i]
		i++
		return s, nil
	}
	return func() { stageSuffix = prev }
}

// SetStageSuffixErrForTest forces stageSuffix to return err (CreateStage error path).
func SetStageSuffixErrForTest(t *testing.T, err error) (restore func()) {
	t.Helper()
	prev := stageSuffix
	stageSuffix = func() (string, error) { return "", err }
	return func() { stageSuffix = prev }
}

// SetExclusiveRenameForTest replaces the exclusive no-replace rename primitive
// (for ENOTSUP/EXDEV/false-success injection). Returns a restore func.
// Not part of the production API surface.
func SetExclusiveRenameForTest(t *testing.T, fn func(parentFd int, stageName, destName string) error) (restore func()) {
	t.Helper()
	prev := exclusiveRenameFn
	exclusiveRenameFn = fn
	return func() { exclusiveRenameFn = prev }
}

// ObserveCommitForTest exposes observeCommit for classification injection tests.
func ObserveCommitForTest(parent *ParentHandle, stageName, destName string, recorded FileID, sysErr error) CommitObservation {
	return observeCommit(parent, stageName, destName, recorded, sysErr)
}

// MapUncommittedErrorForTest exposes errno → id mapping for unit tests.
func MapUncommittedErrorForTest(err error, stageName, destName string) (Identifier string, msg string) {
	id, m := mapUncommittedError(err, stageName, destName)
	return string(id), m
}

// RewriteDarwinDirAliasesForTest exposes rewriteDarwinDirAliases (no-op off Darwin).
func RewriteDarwinDirAliasesForTest(path string) string {
	return rewriteDarwinDirAliases(path)
}

// IsSystemPrefixForTest exposes isSystemPrefix custody helper.
func IsSystemPrefixForTest(path string) bool {
	return isSystemPrefix(path)
}

// SplitAbsForTest exposes splitAbs path component split.
func SplitAbsForTest(path string) []string {
	return splitAbs(path)
}

// NormalizeDestinationForTest exposes normalizeDestination.
func NormalizeDestinationForTest(destination string) (dest, parent, base string, err error) {
	return normalizeDestination(destination)
}

// WithPreserveRemediationForTest exposes withPreserveRemediation.
func WithPreserveRemediationForTest(err error, stagePath string) error {
	return withPreserveRemediation(err, stagePath)
}

// AttachPreserveRemediationForTest exposes attachPreserveRemediation.
func AttachPreserveRemediationForTest(fe *diagnostic.FoundryError, stagePath string) *diagnostic.FoundryError {
	return attachPreserveRemediation(fe, stagePath)
}

// WrapCauseForTest exposes wrapCause.
func WrapCauseForTest(fe *diagnostic.FoundryError, cause error) *diagnostic.FoundryError {
	return wrapCause(fe, cause)
}

// IsExistErrnoForTest / IsRenameUnsupportedForTest / IsEXDEVForTest expose
// commit errno classifiers.
func IsExistErrnoForTest(err error) bool        { return isExistErrno(err) }
func IsRenameUnsupportedForTest(err error) bool { return isRenameUnsupported(err) }
func IsEXDEVForTest(err error) bool             { return isEXDEV(err) }

// ErrnoStringForTest exposes errnoString.
func ErrnoStringForTest(err error) string { return errnoString(err) }

// ValidateRootedRelPathForTest exposes validateRootedRelPath.
func ValidateRootedRelPathForTest(relPath string) (string, error) {
	return validateRootedRelPath(relPath)
}

// ParseModeForTest exposes parseMode.
func ParseModeForTest(mode string) (os.FileMode, error) {
	return parseMode(mode)
}

// SplitRelForTest exposes splitRel.
func SplitRelForTest(rel string) []string { return splitRel(rel) }

// NewCommitErrorForTest exposes newCommitError location/remediation paths.
func NewCommitErrorForTest(id diagnostic.Identifier, msg string, parent *ParentHandle, stageName, destName string) error {
	return newCommitError(id, msg, parent, stageName, destName)
}

// CommitFailClosedForTest exposes commitFailClosed.
func CommitFailClosedForTest(class CommitClass, id diagnostic.Identifier, msg, stageName, stagePath, destName, sysName string, sysErr error, obs CommitObservation) CommitResult {
	return commitFailClosed(class, id, msg, stageName, stagePath, destName, sysName, sysErr, obs)
}

// FileIDFromFDForTest exposes fileIDFromFD.
func FileIDFromFDForTest(fd int) (FileID, error) { return fileIDFromFD(fd) }

// ErrorsIsNotExistForTest / ErrorsIsNotDirForTest / ErrorsIsLoopForTest expose errno helpers.
func ErrorsIsNotExistForTest(err error) bool { return errorsIsNotExist(err) }
func ErrorsIsNotDirForTest(err error) bool   { return errorsIsNotDir(err) }
func ErrorsIsLoopForTest(err error) bool     { return errorsIsLoop(err) }

// ClassifyParentOpenatFailForTest exposes classifyParentOpenatFail.
func ClassifyParentOpenatFailForTest(dirfd int, comp string, err error) diagnostic.Identifier {
	return classifyParentOpenatFail(dirfd, comp, err)
}
