package report

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// SchemaVersion is the top-level JSON document schema integer (Section 37).
// Additive evolution only within a major; plan schema and envelope share
// the major-version discipline.
const SchemaVersion = 1

// Envelope is the stable top-level JSON object for every command
// (Section 37): schema, command, ok, result, error, warnings.
//
// Field order is structural (Go struct order) for deterministic encoding.
// Warnings is never null — empty slice when none.
type Envelope struct {
	Schema   int          `json:"schema"`
	Command  string       `json:"command"`
	OK       bool         `json:"ok"`
	Result   any          `json:"result"`
	Error    *ErrorObject `json:"error"`
	Warnings []string     `json:"warnings"`
}

// ErrorObject is the agent-legible failure projection (bead 41p / REQ-155–159).
//
// Required on failure: error_id (domain.reason when registered), message,
// remediation, exit_code. Optional: path, line, col (from Location),
// stage_path (preserved stage), location (combined string form).
//
// Stack dumps are never the primary message. Secret-like content is redacted
// via diagnostic before fields are set.
type ErrorObject struct {
	ErrorID     string `json:"error_id,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
	Path        string `json:"path,omitempty"`
	Line        int    `json:"line,omitempty"`
	Col         int    `json:"col,omitempty"`
	StagePath   string `json:"stage_path,omitempty"`
	Location    string `json:"location,omitempty"`
	ExitCode    int    `json:"exit_code"`
}

// SuccessEnvelope builds a success document. result may be nil (encoded as
// null). warnings is normalized to a non-nil empty slice.
func SuccessEnvelope(command string, result any, warnings []string) Envelope {
	if warnings == nil {
		warnings = []string{}
	}
	return Envelope{
		Schema:   SchemaVersion,
		Command:  command,
		OK:       true,
		Result:   result,
		Error:    nil,
		Warnings: warnings,
	}
}

// FailureEnvelope builds a failure document from err and optional stagePath.
// result is always null on failure. JSON never reports ok=false together with
// a committed repository result (FND-012) — callers must not pass a committed
// GenerateResult into FailureEnvelope.
func FailureEnvelope(command string, err error, stagePath string, warnings []string) Envelope {
	if warnings == nil {
		warnings = []string{}
	}
	return Envelope{
		Schema:   SchemaVersion,
		Command:  command,
		OK:       false,
		Result:   nil,
		Error:    ErrorFrom(err, stagePath),
		Warnings: warnings,
	}
}

// ErrorFrom projects err into the stable ErrorObject. stagePath is set when
// a preserved stage exists (Section 31.6 / 36.2). All string fields are
// redacted.
func ErrorFrom(err error, stagePath string) *ErrorObject {
	stagePath = diagnostic.Redact(stagePath)
	if err == nil {
		return &ErrorObject{
			ErrorID:     string(diagnostic.IDInternalBug),
			Message:     "internal error: nil failure encoded",
			Remediation: diagnostic.RemediationFor(diagnostic.IDInternalBug),
			StagePath:   stagePath,
			ExitCode:    diagnostic.ExitFailure,
		}
	}

	if diagnostic.IsCancelled(err) {
		obj := &ErrorObject{
			// Cancellation is not an Appendix D domain.reason (Section 38.1).
			// Stable sentinel so agents branch without scraping prose.
			ErrorID:     "cancelled",
			Message:     diagnostic.Redact(err.Error()),
			Remediation: "Re-run the command when ready. If a stage was preserved, inspect stage_path; Foundry never auto-deletes stages.",
			StagePath:   stagePath,
			ExitCode:    diagnostic.ExitCancelled,
		}
		if obj.Message == "" {
			obj.Message = "cancelled before commit"
		}
		return obj
	}

	if fe, ok := diagnostic.AsFoundryError(err); ok {
		loc := fe.Location()
		obj := &ErrorObject{
			ErrorID:     string(fe.ID()),
			Message:     fe.Message(),
			Remediation: fe.Remediation(),
			StagePath:   stagePath,
			Location:    loc.String(),
			ExitCode:    fe.ExitCode(),
		}
		// Prefer filesystem Path; fall back to File for spec locations.
		if loc.Path != "" {
			obj.Path = loc.Path
		} else if loc.File != "" {
			obj.Path = loc.File
		}
		if loc.Line > 0 {
			obj.Line = loc.Line
		}
		if loc.Column > 0 {
			obj.Col = loc.Column
		}
		if obj.Remediation == "" {
			obj.Remediation = diagnostic.RemediationFor(fe.ID())
		}
		// Ensure remediation is never empty for registered ids.
		if obj.Remediation == "" {
			obj.Remediation = "See the Foundry error identifier " + obj.ErrorID + " in the specification Appendix D."
		}
		return obj
	}

	// Unknown error — fail closed with internal.bug (no stack as primary text).
	msg := diagnostic.Redact(err.Error())
	if msg == "" {
		msg = "unexpected error"
	}
	return &ErrorObject{
		ErrorID:     string(diagnostic.IDInternalBug),
		Message:     msg,
		Remediation: diagnostic.RemediationFor(diagnostic.IDInternalBug),
		StagePath:   stagePath,
		ExitCode:    diagnostic.ExitFailure,
	}
}

// WriteJSON encodes one deterministic JSON document plus a trailing newline.
// Uses encoding/json with HTML escaping disabled; map keys retain Go's
// sorted order when maps appear in result (prefer structs for stability).
func WriteJSON(w io.Writer, v any) error {
	if w == nil {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Encoder adds exactly one trailing newline.
	_, err := w.Write(buf.Bytes())
	return err
}
