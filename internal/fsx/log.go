package fsx

// StepLogger receives structured fsx probe steps (Section 31 walk/custody/
// destination preflight). Implementations must not log secrets.
//
// Outcome is typically "pass", "fail", or "info". Detail uses basenames,
// modes (octal), error identifiers, and refusal classes — not host homes.
//
// A nil logger is treated as a no-op (production default until generate
// wires a reporter).
type StepLogger interface {
	// FSXStep records one probe step. probe names a subsystem
	// (parent_walk, custody, destination, parent_reobserve); step is a
	// finer-grained label within that probe.
	FSXStep(probe, step, outcome, detail string)
}

// NopLogger discards all steps.
type NopLogger struct{}

// FSXStep implements StepLogger.
func (NopLogger) FSXStep(probe, step, outcome, detail string) {}

func logStep(log StepLogger, probe, step, outcome, detail string) {
	if log == nil {
		return
	}
	log.FSXStep(probe, step, outcome, detail)
}
