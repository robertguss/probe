package diagnostic

import (
	"errors"
	"fmt"
	"strings"
)

// FoundryError is the stable product error type (Section 43).
//
// Fields:
//   - ID: registry-unique domain.reason identifier
//   - Message: human/agent message (what failed) — never a stack dump as primary text
//   - Location: file:line:column, step id, or path (Section 38.2)
//   - Remediation: smallest correct fix; defaults from registry when empty
//   - cause: optional wrapped error (Unwrap); not part of primary message unless verbose
//
// Invariants (REQ-188): does not store context.Context; immutable after
// construction (no setters); safe for concurrent read.
type FoundryError struct {
	id          Identifier
	message     string
	location    Location
	remediation string
	cause       error
}

// New builds a FoundryError for a known identifier. Unknown identifiers are
// still constructible but ExitCode falls back to ExitFailure; prefer registry
// constants. Message and remediation are redacted for known secret patterns.
func New(id Identifier, message string, loc Location) *FoundryError {
	return newError(id, message, loc, "", nil)
}

// Newf is New with fmt.Sprintf for the message.
func Newf(id Identifier, loc Location, format string, args ...any) *FoundryError {
	return newError(id, fmt.Sprintf(format, args...), loc, "", nil)
}

// Wrap builds a FoundryError that unwraps to cause.
func Wrap(id Identifier, message string, loc Location, cause error) *FoundryError {
	return newError(id, message, loc, "", cause)
}

// Wrapf is Wrap with fmt.Sprintf for the message.
func Wrapf(id Identifier, loc Location, cause error, format string, args ...any) *FoundryError {
	return newError(id, fmt.Sprintf(format, args...), loc, "", cause)
}

// WithRemediation returns a shallow copy with an explicit remediation override.
// The original error is not mutated.
func (e *FoundryError) WithRemediation(remediation string) *FoundryError {
	if e == nil {
		return nil
	}
	cp := *e
	cp.remediation = Redact(remediation)
	return &cp
}

func newError(id Identifier, message string, loc Location, remediation string, cause error) *FoundryError {
	msg := Redact(message)
	rem := Redact(remediation)
	if rem == "" {
		rem = RemediationFor(id)
	}
	return &FoundryError{
		id:          id,
		message:     msg,
		location:    loc,
		remediation: rem,
		cause:       cause,
	}
}

// ID returns the domain.reason identifier.
func (e *FoundryError) ID() Identifier {
	if e == nil {
		return ""
	}
	return e.id
}

// Message returns the primary human/agent message (no stack dump).
func (e *FoundryError) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

// Location returns the source location.
func (e *FoundryError) Location() Location {
	if e == nil {
		return Location{}
	}
	return e.location
}

// Remediation returns the agent-actionable fix text.
func (e *FoundryError) Remediation() string {
	if e == nil {
		return ""
	}
	if e.remediation != "" {
		return e.remediation
	}
	return RemediationFor(e.id)
}

// ExitCode returns the Appendix D exit code for this error's identifier.
func (e *FoundryError) ExitCode() int {
	if e == nil {
		return ExitFailure
	}
	return ExitCodeFor(e.id)
}

// Error implements the error interface. Primary text is
// "id: message (at location)" without stack dumps. Cause is not inlined
// (verbose/report layers may attach it separately).
func (e *FoundryError) Error() string {
	if e == nil {
		return "foundry: <nil>"
	}
	var b strings.Builder
	b.WriteString(string(e.id))
	if e.message != "" {
		b.WriteString(": ")
		b.WriteString(e.message)
	}
	if loc := e.location.String(); loc != "" {
		b.WriteString(" (at ")
		b.WriteString(loc)
		b.WriteString(")")
	}
	return b.String()
}

// Unwrap returns the wrapped cause, if any.
func (e *FoundryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Fields is the stable machine-readable projection for report/JSON layers.
// Field names are stable; report owns envelope encoding (REQ-186).
type Fields struct {
	ID          string `json:"id"`
	Message     string `json:"message"`
	Location    string `json:"location,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	ExitCode    int    `json:"exit_code"`
}

// Fields returns redacted stable fields for text and JSON consumers.
func (e *FoundryError) Fields() Fields {
	if e == nil {
		return Fields{}
	}
	return Fields{
		ID:          string(e.id),
		Message:     e.message,
		Location:    e.location.String(),
		Remediation: e.Remediation(),
		ExitCode:    e.ExitCode(),
	}
}

// AsFoundryError extracts a *FoundryError from err's chain.
func AsFoundryError(err error) (*FoundryError, bool) {
	var fe *FoundryError
	if err == nil {
		return nil, false
	}
	if errors.As(err, &fe) {
		return fe, true
	}
	return nil, false
}
