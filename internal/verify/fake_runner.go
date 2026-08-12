package verify

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// FakeStepRunner is a test double for StepRunner. It records every Run call
// and returns scripted outcomes by step id.
//
// Production code must not use FakeStepRunner (test files and test helpers
// only by convention; the type is exported for sibling package tests of
// generate later).
type FakeStepRunner struct {
	mu sync.Mutex

	// ByStep maps step id → scripted result. Missing entries fail closed.
	ByStep map[string]FakeStepScript

	// Observed holds every request in call order.
	Observed []toolrun.StepRequest

	// Mutators, when set for a step id, run after a successful scripted step
	// to mutate the stage directory (for unplanned-mutation fixtures).
	// The absolute StageDir must be closed over by the mutator.
	Mutators map[string]func() error
}

// FakeStepScript is one scripted external step outcome.
type FakeStepScript struct {
	// ExitCode is the process exit code (0 = success).
	ExitCode int
	// Stdout / Stderr are captured streams.
	Stdout string
	Stderr string
	// Fail forces a tool.failed diagnostic even when ExitCode is 0.
	Fail bool
	// Timeout marks the step as timed out.
	Timeout bool
	// Err, when set, becomes StepResult.err (and FailClass tool.failed unless Timeout).
	Err error
	// Sleep delays the Run return (tests of cancellation/duration).
	Sleep time.Duration
}

// Run implements StepRunner.
func (f *FakeStepRunner) Run(_ context.Context, req toolrun.StepRequest) toolrun.StepResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Observed = append(f.Observed, cloneReq(req))

	start := time.Now()
	res := toolrun.StepResult{
		StepID:     req.StepID,
		BinaryBase: filepath.Base(req.Binary),
		Argv:       append([]string{filepath.Base(req.Binary)}, req.Args...),
		ExitCode:   0,
	}
	if req.Env != nil {
		res.EnvHash = toolrun.AllowlistHash(req.Env)
		res.EnvKeys = toolrun.EnvKeys(req.Env)
	}

	script, ok := f.ByStep[req.StepID]
	if !ok {
		res.ExitCode = 1
		res.FailClass = toolrun.FailClassFailed
		res.Stderr = []byte("fake: no script for step " + req.StepID)
		res.StderrBytes = int64(len(res.Stderr))
		// Use unexported err via diagnostic on a local copy — StepResult.err is
		// unexported. Return a result that !OK() via FailClass + ExitCode; Run
		// in verify also checks tr.Err(). Provide Err through a wrapper by
		// setting fields that OK() checks.
		//
		// toolrun.StepResult.err is unexported, so we cannot set it from another
		// package. verify.runExternal treats !tr.OK() and nil Err as tool.failed.
		_ = start
		return res
	}

	if script.Sleep > 0 {
		time.Sleep(script.Sleep)
	}

	res.Stdout = []byte(script.Stdout)
	res.Stderr = []byte(script.Stderr)
	res.StdoutBytes = int64(len(res.Stdout))
	res.StderrBytes = int64(len(res.Stderr))
	res.ExitCode = script.ExitCode
	res.Duration = time.Since(start)

	if script.Timeout {
		res.TimedOut = true
		res.FailClass = toolrun.FailClassTimeout
		res.ExitCode = -1
		return res
	}
	if script.Fail || script.ExitCode != 0 || script.Err != nil {
		res.FailClass = toolrun.FailClassFailed
		if res.ExitCode == 0 {
			res.ExitCode = 1
		}
		return res
	}

	// Success path: optional stage mutation (hostile fixture).
	if f.Mutators != nil {
		if mut, ok := f.Mutators[req.StepID]; ok && mut != nil {
			if err := mut(); err != nil {
				res.FailClass = toolrun.FailClassFailed
				res.ExitCode = 1
				res.Stderr = []byte(fmt.Sprintf("mutator error: %v", err))
				res.StderrBytes = int64(len(res.Stderr))
				return res
			}
		}
	}
	return res
}

// ObservedArgv returns full argv (basename + args) for step id, or nil.
func (f *FakeStepRunner) ObservedArgv(stepID string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.Observed {
		if r.StepID == stepID {
			return append([]string{filepath.Base(r.Binary)}, r.Args...)
		}
	}
	return nil
}

// ObservedStepIDs returns step ids in call order.
func (f *FakeStepRunner) ObservedStepIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.Observed))
	for _, r := range f.Observed {
		out = append(out, r.StepID)
	}
	return out
}

// ScriptAllOK returns a ByStep map that succeeds every external step in
// DefaultSteps/StrictSteps.
func ScriptAllOK(mode Mode) map[string]FakeStepScript {
	m := make(map[string]FakeStepScript)
	for _, s := range StepsFor(mode) {
		if s.Kind == KindExternal {
			m[s.ID] = FakeStepScript{ExitCode: 0}
		}
	}
	return m
}

// ScriptFailStep returns ScriptAllOK with one step failing.
func ScriptFailStep(mode Mode, failID string) map[string]FakeStepScript {
	m := ScriptAllOK(mode)
	m[failID] = FakeStepScript{ExitCode: 1, Stderr: "injected failure for " + failID}
	return m
}

func cloneReq(r toolrun.StepRequest) toolrun.StepRequest {
	cp := r
	if r.Args != nil {
		cp.Args = append([]string(nil), r.Args...)
	}
	if r.Env != nil {
		cp.Env = make(map[string]string, len(r.Env))
		for k, v := range r.Env {
			cp.Env[k] = v
		}
	}
	return cp
}

// Ensure FakeStepRunner failure surfaces as a diagnostic when ExitCode != 0.
// verify.runExternal maps !OK() with nil Err to tool.failed — good.

// ArgvContains reports whether observed argv for stepID contains token.
func ArgvContains(argv []string, token string) bool {
	for _, a := range argv {
		if a == token || strings.Contains(a, token) {
			return true
		}
	}
	return false
}

// NewToolFailed is a helper for tests expecting diagnostic tool.failed.
func NewToolFailed(stepID, msg string) error {
	return diagnostic.Newf(
		diagnostic.IDToolFailed,
		diagnostic.StepLocation(stepID),
		"%s", msg,
	)
}
