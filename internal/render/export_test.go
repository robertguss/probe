package render

import (
	"text/template/parse"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// ForbiddenFuncReasonForTest exposes forbiddenFuncReason.
func ForbiddenFuncReasonForTest(ident string) string {
	return forbiddenFuncReason(ident)
}

// ParseNodeIsNilForTest exposes parseNodeIsNil for typed-nil interface values.
func ParseNodeIsNilForTest(n parse.Node) bool {
	return parseNodeIsNil(n)
}

// TemplateJoinForTest exposes templateJoin.
func TemplateJoinForTest(sep string, parts ...any) (string, error) {
	return templateJoin(sep, parts...)
}

// TemplateErrorForTest exposes templateError (empty name branch).
func TemplateErrorForTest(name, msg string) *diagnostic.FoundryError {
	return templateError(name, msg)
}

// WrapWriteErrorForTest exposes wrapWriteError.
func WrapWriteErrorForTest(path string, err error) error {
	return wrapWriteError(path, err)
}

// IsGoSourcePathForTest exposes isGoSourcePath.
func IsGoSourcePathForTest(p string) bool {
	return isGoSourcePath(p)
}

// IsExactVersionForTest exposes isExactVersion.
func IsExactVersionForTest(v string) bool {
	return isExactVersion(v)
}

// NewInventoryForTest exposes newInventory.
func NewInventoryForTest(entries []Entry, content map[string][]byte) *Inventory {
	return newInventory(entries, content)
}

// SafeOutputPathForTest exposes safeOutputPath.
func SafeOutputPathForTest(p string) (string, error) {
	return safeOutputPath(p)
}

// SourcePathUnsafeForTest exposes sourcePathUnsafe.
func SourcePathUnsafeForTest(src string) string {
	return sourcePathUnsafe(src)
}
