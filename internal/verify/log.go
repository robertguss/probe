package verify

// StepLogger receives structured verify timeline steps. Implementations must
// not log secrets, env values, or full host home paths.
//
// Production generate may wire report/event sinks; tests use a recording
// logger. Nil loggers are safe no-ops.
type StepLogger interface {
	// VerifyStep records one step. outcome is "ok", "fail", "skip", or "info".
	// detail must never include env values — hash and key names only.
	VerifyStep(step, outcome, detail string)
}

// nopLogger is used when Options.Logger is nil.
type nopLogger struct{}

func (nopLogger) VerifyStep(step, outcome, detail string) {}

func loggerOrNop(l StepLogger) StepLogger {
	if l == nil {
		return nopLogger{}
	}
	return l
}
