package gitinit

// StepLogger receives structured gitinit timeline steps (init → scratch
// remove → re-conformance → semantic). Implementations must not log secrets
// or full host home paths.
//
// Production generate may wire report/event sinks; tests use testutil or a
// recording logger. Nil loggers are safe no-ops.
type StepLogger interface {
	// GitStep records one step. outcome is "ok", "fail", "skip", or "info".
	// detail must never include env values — hash and key names only.
	GitStep(step, outcome, detail string)
}

// nopLogger is used when Options.Logger is nil.
type nopLogger struct{}

func (nopLogger) GitStep(step, outcome, detail string) {}

func loggerOrNop(l StepLogger) StepLogger {
	if l == nil {
		return nopLogger{}
	}
	return l
}
