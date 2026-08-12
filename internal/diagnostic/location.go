package diagnostic

import (
	"fmt"
	"strconv"
)

// Location names where a failure occurred (Section 38.2):
//   - Spec errors: file:line:column
//   - Tool errors: step id
//   - Filesystem errors: path
//
// Zero Location is valid (unknown/not applicable). Immutable after construction.
type Location struct {
	// File is a specification or source path (basename preferred in user text).
	File string
	// Line is 1-based; 0 means unset.
	Line int
	// Column is 1-based; 0 means unset.
	Column int
	// Path is a filesystem path for fs.* errors.
	Path string
	// StepID is an external-step identifier for tool.* / verify.* errors.
	StepID string
}

// SpecLocation builds a file:line:column location for specification errors.
func SpecLocation(file string, line, column int) Location {
	return Location{File: file, Line: line, Column: column}
}

// PathLocation builds a path location for filesystem errors.
func PathLocation(path string) Location {
	return Location{Path: path}
}

// StepLocation builds a step-id location for tool/verify errors.
func StepLocation(stepID string) Location {
	return Location{StepID: stepID}
}

// IsZero reports whether no location component is set.
func (l Location) IsZero() bool {
	return l.File == "" && l.Line == 0 && l.Column == 0 && l.Path == "" && l.StepID == ""
}

// String formats the location for human and agent consumption.
// Spec form: file:line:column (column omitted when 0).
// Path form: path. Step form: step=<id>. Empty when zero.
func (l Location) String() string {
	switch {
	case l.File != "":
		if l.Line <= 0 {
			return l.File
		}
		if l.Column <= 0 {
			return l.File + ":" + strconv.Itoa(l.Line)
		}
		return fmt.Sprintf("%s:%d:%d", l.File, l.Line, l.Column)
	case l.Path != "":
		return l.Path
	case l.StepID != "":
		return "step=" + l.StepID
	default:
		return ""
	}
}

// Equal reports structural equality (for tests).
func (l Location) Equal(o Location) bool {
	return l.File == o.File && l.Line == o.Line && l.Column == o.Column &&
		l.Path == o.Path && l.StepID == o.StepID
}
