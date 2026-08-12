package engine

// ExitCode is the process status paired with Envelope.Meta.ExitCode.
type ExitCode int

const (
	ExitSuccess     ExitCode = 0
	ExitTransport   ExitCode = 1
	ExitUsage       ExitCode = 2
	ExitHTTP4xx     ExitCode = 3
	ExitHTTP5xx     ExitCode = 4
	ExitRateLimited ExitCode = 5
)

// Result is the only value command handlers return.
// Invariant: Envelope.Meta.ExitCode == ExitCode.
type Result struct {
	Envelope Envelope
	ExitCode ExitCode
}

// Envelope is the machine-readable command outcome.
type Envelope struct {
	OK      bool       `json:"ok"`
	Command string     `json:"command"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error"` // null on success
	Meta    Meta       `json:"meta"`
}

// ErrorBody carries actionable failure detail for agents.
type ErrorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Hint    string   `json:"hint,omitempty"`
	Next    []string `json:"next,omitempty"`
}

// Meta is envelope metadata shared by every command.
type Meta struct {
	ExitCode  ExitCode `json:"exitCode"`
	Version   string   `json:"version"`
	RequestID string   `json:"requestId,omitempty"`
}

func (e *Engine) ok(command string, data any) Result {
	return Result{
		ExitCode: ExitSuccess,
		Envelope: Envelope{
			OK:      true,
			Command: command,
			Data:    data,
			Error:   nil,
			Meta: Meta{
				ExitCode: ExitSuccess,
				Version:  Version,
			},
		},
	}
}

func (e *Engine) fail(command string, code ExitCode, errCode, msg, hint string, next []string) Result {
	if code == ExitRateLimited && errCode != "rate_limited" {
		panic("ExitRateLimited requires error.code rate_limited")
	}
	if errCode == "rate_limited" && code != ExitRateLimited {
		panic("error.code rate_limited requires ExitRateLimited")
	}
	return Result{
		ExitCode: code,
		Envelope: Envelope{
			OK:      false,
			Command: command,
			Error: &ErrorBody{
				Code:    errCode,
				Message: msg,
				Hint:    hint,
				Next:    next,
			},
			Meta: Meta{
				ExitCode: code,
				Version:  Version,
			},
		},
	}
}

// usageError builds exit 2 with a copy-pasteable example invocation.
func (e *Engine) usageError(command, msg, example string) Result {
	return e.fail(command, ExitUsage, "usage", msg, example, []string{example})
}
