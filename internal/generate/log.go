package generate

// StepLogger receives structured lifecycle transition steps. Implementations
// must not log secrets or absolute host homes in golden-bound streams.
//
// A nil logger is treated as a no-op.
type StepLogger interface {
	// GenerateStep records one machine step.
	// state is the lifecycle state after the step (or before for starts);
	// event names the GenerationEvent kind or stage id; result is ok/fail/
	// cancel/skip; durationMs is wall time for that step (0 when N/A).
	GenerateStep(state, event, result string, durationMs int64)
}

// NopLogger discards all steps.
type NopLogger struct{}

// GenerateStep implements StepLogger.
func (NopLogger) GenerateStep(state, event, result string, durationMs int64) {}

func logStep(log StepLogger, state, event, result string, durationMs int64) {
	if log == nil {
		return
	}
	log.GenerateStep(state, event, result, durationMs)
}

// RecordingLogger captures steps for tests (package-visible via tests).
type RecordingLogger struct {
	Steps []LogStep
}

// LogStep is one recorded generate step.
type LogStep struct {
	State      string
	Event      string
	Result     string
	DurationMs int64
}

// GenerateStep implements StepLogger.
func (r *RecordingLogger) GenerateStep(state, event, result string, durationMs int64) {
	if r == nil {
		return
	}
	r.Steps = append(r.Steps, LogStep{
		State:      state,
		Event:      event,
		Result:     result,
		DurationMs: durationMs,
	})
}
