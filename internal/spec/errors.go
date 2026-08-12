package spec

import (
	"fmt"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// ValidationError is one or more independent field diagnostics from a single
// validation stage, ordered by source location (Section 14.5).
//
// It implements error and multi-unwrap (Unwrap []error) so errors.As can
// extract any *diagnostic.FoundryError in the set.
type ValidationError struct {
	// Errs is non-empty and ordered by source position (line, column, then
	// field name for stability). Callers must not mutate the slice or elements.
	Errs []*diagnostic.FoundryError
}

// Error joins all diagnostics; primary text is multi-line when n > 1.
func (e *ValidationError) Error() string {
	if e == nil || len(e.Errs) == 0 {
		return "spec: validation failed"
	}
	if len(e.Errs) == 1 {
		return e.Errs[0].Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "spec: %d field errors:", len(e.Errs))
	for _, fe := range e.Errs {
		b.WriteByte('\n')
		b.WriteString(fe.Error())
	}
	return b.String()
}

// Unwrap returns the underlying errors for errors.Is / errors.As (Go 1.20+).
func (e *ValidationError) Unwrap() []error {
	if e == nil || len(e.Errs) == 0 {
		return nil
	}
	out := make([]error, len(e.Errs))
	for i, fe := range e.Errs {
		out[i] = fe
	}
	return out
}

// First returns the first diagnostic, or nil.
func (e *ValidationError) First() *diagnostic.FoundryError {
	if e == nil || len(e.Errs) == 0 {
		return nil
	}
	return e.Errs[0]
}

// Len returns the number of diagnostics.
func (e *ValidationError) Len() int {
	if e == nil {
		return 0
	}
	return len(e.Errs)
}

// AsValidationError extracts a *ValidationError from err's chain.
func AsValidationError(err error) (*ValidationError, bool) {
	if err == nil {
		return nil, false
	}
	var ve *ValidationError
	if asValidationError(err, &ve) {
		return ve, true
	}
	return nil, false
}

func asValidationError(err error, target **ValidationError) bool {
	// Local errors.As without importing for clarity on pointer type.
	type unwrapper interface{ Unwrap() error }
	type multi interface{ Unwrap() []error }

	switch e := err.(type) {
	case *ValidationError:
		*target = e
		return true
	case multi:
		for _, u := range e.Unwrap() {
			if asValidationError(u, target) {
				return true
			}
		}
	case unwrapper:
		return asValidationError(e.Unwrap(), target)
	}
	return false
}

// CollectFoundryErrors returns all *FoundryError values from err, expanding
// ValidationError and multi-unwrap chains. Order is preserved for ValidationError.
func CollectFoundryErrors(err error) []*diagnostic.FoundryError {
	if err == nil {
		return nil
	}
	if ve, ok := err.(*ValidationError); ok {
		out := make([]*diagnostic.FoundryError, len(ve.Errs))
		copy(out, ve.Errs)
		return out
	}
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		return []*diagnostic.FoundryError{fe}
	}
	type multi interface{ Unwrap() []error }
	if m, ok := err.(multi); ok {
		var out []*diagnostic.FoundryError
		for _, u := range m.Unwrap() {
			out = append(out, CollectFoundryErrors(u)...)
		}
		return out
	}
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return CollectFoundryErrors(u.Unwrap())
	}
	return nil
}
