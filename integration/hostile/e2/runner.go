package e2

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// FakeRunner records planned executions without starting processes.
// Used for process-tree equality unit tests (REQ-214 fake-runner unit tests).
type FakeRunner struct {
	mu       sync.Mutex
	Observed []ObservedStep
	// Extra, if set, injects unplanned steps after RunPlan (negative tests).
	InjectExtra []ObservedStep
	// MutateEnv, if set, mutates env of each step before recording (negative).
	MutateEnv func(id string, env map[string]string)
}

// RunPlan "executes" each step by recording binary/argv/env.
func (r *FakeRunner) RunPlan(steps []ExternalStep) []ObservedStep {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Observed = r.Observed[:0]
	for _, s := range steps {
		env := copyMap(s.Env)
		if r.MutateEnv != nil {
			r.MutateEnv(s.ID, env)
		}
		r.Observed = append(r.Observed, ObservedStep{
			ID: s.ID, Binary: s.Binary, Argv: append([]string(nil), s.Argv...), Env: env,
		})
	}
	for _, extra := range r.InjectExtra {
		r.Observed = append(r.Observed, extra)
	}
	return append([]ObservedStep(nil), r.Observed...)
}

// RealRunner executes steps with the constructed env (no Dir pathname —
// caller sets process cwd if needed). Records observations for tree audit.
// Used for live go env / git init isolation proofs.
type RealRunner struct {
	mu       sync.Mutex
	Observed []ObservedStep
}

// Run records and executes one step. stdout/stderr are captured (capped by
// caller if needed). Returns combined output and error.
func (r *RealRunner) Run(ctx context.Context, step ExternalStep, workDir string) (stdout, stderr []byte, err error) {
	envSlice := mapToEnv(step.Env)
	obs := ObservedStep{
		ID:     step.ID,
		Binary: step.Binary,
		Argv:   append([]string(nil), step.Argv...),
		Env:    copyMap(step.Env),
		Shell:  false,
	}
	r.mu.Lock()
	r.Observed = append(r.Observed, obs)
	r.mu.Unlock()

	if len(step.Argv) == 0 {
		return nil, nil, fmt.Errorf("step %s: empty argv", step.ID)
	}
	// Argv[0] is the conventional program name; Binary is the absolute path.
	cmd := exec.CommandContext(ctx, step.Binary, step.Argv[1:]...)
	cmd.Env = envSlice
	if workDir != "" {
		cmd.Dir = workDir
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

// Snapshot returns a copy of observed steps.
func (r *RealRunner) Snapshot() []ObservedStep {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ObservedStep(nil), r.Observed...)
}

// RunGoEnv runs `go env KEY…` under ConstructGoEnv and returns KEY→value.
func RunGoEnv(ctx context.Context, goBinary string, h HostCapture, keys ...string) (map[string]string, []string, error) {
	if goBinary == "" {
		var err error
		goBinary, err = exec.LookPath("go")
		if err != nil {
			return nil, nil, err
		}
	}
	env := ConstructGoEnv(h)
	args := append([]string{"env"}, keys...)
	cmd := exec.CommandContext(ctx, goBinary, args...)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return nil, env, fmt.Errorf("go env: %w", err)
	}
	lines := splitLines(string(out))
	if len(lines) != len(keys) {
		return nil, env, fmt.Errorf("go env: expected %d lines, got %d: %q", len(keys), len(lines), string(out))
	}
	m := make(map[string]string, len(keys))
	for i, k := range keys {
		m[k] = lines[i]
	}
	return m, env, nil
}

// WithTimeout returns a context with the step's timeout.
func WithTimeout(parent context.Context, timeoutS int) (context.Context, context.CancelFunc) {
	if timeoutS <= 0 {
		timeoutS = 60
	}
	return context.WithTimeout(parent, time.Duration(timeoutS)*time.Second)
}

// splitLines splits go env multi-value output into one value per requested key.
// Empty values are preserved (e.g. GOFLAGS=). A single trailing newline from
// the tool is dropped so N keys produce N lines; internal blank lines stay.
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	// Drop exactly one trailing \n (or \r\n) from the process output block.
	if strings.HasSuffix(s, "\r\n") {
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
