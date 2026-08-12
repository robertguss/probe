// Package testutil provides test-only helpers for Foundry unit and integration
// tests: structured step logging, golden comparison with UPDATE_GOLDEN guards,
// and secret redaction in failure dumps.
//
// Production code MUST NOT import this package (enforced by internal/archtest).
//
// Convention (REQ-210–219, docs/dev/testing.md):
//   - Multi-step tests MUST use the step logger.
//   - Trivial table-driven rows may use plain t.Run without a logger.
package testutil

import (
	"fmt"
	"path"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Level is a structured log level for a step.
type Level string

const (
	LevelStep    Level = "STEP"
	LevelAssert  Level = "ASSERT"
	LevelFixture Level = "FIXTURE"
	LevelSkip    Level = "SKIP"
	LevelFail    Level = "FAIL"
	LevelPhase   Level = "PHASE"
)

// Outcome is the result of a logged step.
type Outcome string

const (
	OutcomeOK    Outcome = "ok"
	OutcomeFail  Outcome = "fail"
	OutcomeSkip  Outcome = "skip"
	OutcomeInfo  Outcome = "info"
	OutcomeStart Outcome = "start"
	OutcomeEnd   Outcome = "end"
)

// DefaultDumpLast is how many recent steps are dumped on failure.
const DefaultDumpLast = 20

// DefaultTruncate is the max rune length for expected/actual in assert logs.
const DefaultTruncate = 256

// Step is one recorded log line.
type Step struct {
	Index    int
	Level    Level
	Name     string
	Outcome  Outcome
	Elapsed  time.Duration
	Detail   string
	Expected string
	Actual   string
}

// Logger records structured steps for a single test.
//
// Every emitted line includes: package, test name, step index, elapsed ms,
// level, name, outcome, and optional detail. On failure, DumpLast dumps the
// recent ring buffer plus any attached context (paths/IDs/exit codes) with
// secret-like keys redacted.
type Logger struct {
	t        *testing.T
	pkg      string
	testName string
	start    time.Time
	mu       sync.Mutex
	steps    []Step
	index    int
	// Context fields attached to failure dumps (never secret values).
	paths     []string
	ids       []string
	exitCodes []int
	// LastN controls dump depth (default DefaultDumpLast).
	LastN int
}

// New constructs a step logger bound to t. Call once per test (or subtest).
func New(t *testing.T) *Logger {
	t.Helper()
	pkg := callerPackage()
	l := &Logger{
		t:        t,
		pkg:      pkg,
		testName: t.Name(),
		start:    time.Now(),
		steps:    make([]Step, 0, 32),
		LastN:    DefaultDumpLast,
	}
	t.Cleanup(func() {
		if t.Failed() {
			l.DumpLast()
		}
	})
	return l
}

// Phase logs the start of a named phase (phase start/end are required for
// multi-step Foundry tests).
func (l *Logger) Phase(name string) {
	l.t.Helper()
	l.record(LevelPhase, name, OutcomeStart, "", "", "")
}

// PhaseEnd logs the end of a named phase.
func (l *Logger) PhaseEnd(name string, outcome Outcome) {
	l.t.Helper()
	if outcome == "" {
		outcome = OutcomeOK
	}
	l.record(LevelPhase, name, outcome, "end", "", "")
}

// Step logs a general work step.
func (l *Logger) Step(name string, outcome Outcome, detail string) {
	l.t.Helper()
	if outcome == "" {
		outcome = OutcomeOK
	}
	l.record(LevelStep, name, outcome, detail, "", "")
}

// Fixture logs fixture setup/teardown.
func (l *Logger) Fixture(name string, detail string) {
	l.t.Helper()
	l.record(LevelFixture, name, OutcomeOK, detail, "", "")
}

// Assert logs an assertion. On failure it marks the test failed (via t.Errorf)
// and records expected vs actual (truncated).
func (l *Logger) Assert(name string, ok bool, expected, actual any) {
	l.t.Helper()
	exp := truncate(fmt.Sprint(expected), DefaultTruncate)
	act := truncate(fmt.Sprint(actual), DefaultTruncate)
	if ok {
		l.record(LevelAssert, name, OutcomeOK, "", exp, act)
		return
	}
	detail := fmt.Sprintf("expected=%q actual=%q", exp, act)
	l.record(LevelAssert, name, OutcomeFail, detail, exp, act)
	l.t.Errorf("ASSERT fail name=%s expected=%q actual=%q", name, exp, act)
}

// Assertf is Assert with a formatted name/detail prefix.
func (l *Logger) Assertf(ok bool, nameFmt string, args ...any) {
	l.t.Helper()
	name := fmt.Sprintf(nameFmt, args...)
	if ok {
		l.record(LevelAssert, name, OutcomeOK, "", "", "")
		return
	}
	l.record(LevelAssert, name, OutcomeFail, "", "", "")
	l.t.Errorf("ASSERT fail name=%s", name)
}

// Skip logs a skip and calls t.Skip.
func (l *Logger) Skip(reason string) {
	l.t.Helper()
	l.record(LevelSkip, "skip", OutcomeSkip, reason, "", "")
	l.t.Skip(reason)
}

// Fail logs a hard failure and calls t.Fatal.
func (l *Logger) Fail(name, detail string) {
	l.t.Helper()
	l.record(LevelFail, name, OutcomeFail, detail, "", "")
	l.DumpLast()
	l.t.Fatalf("FAIL name=%s detail=%s", name, detail)
}

// Inputs summarizes non-sensitive test inputs (spec basename, archetype,
// verify mode). Absolute host homes must not appear in golden streams — pass
// basenames only.
func (l *Logger) Inputs(fields map[string]string) {
	l.t.Helper()
	parts := make([]string, 0, len(fields))
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s=%s", k, sanitizePath(v)))
	}
	l.record(LevelStep, "inputs", OutcomeInfo, strings.Join(parts, " "), "", "")
}

// Subprocess records argv summary, env-allowlist hash, exit code, and stream
// sizes. On failure (non-zero or caller-marked), includes first/last lines of
// combined output (already should be free of secrets).
func (l *Logger) Subprocess(name string, argv []string, envAllowHash string, exitCode int, stdoutN, stderrN int, firstLine, lastLine string, failed bool) {
	l.t.Helper()
	detail := fmt.Sprintf("argv=%q env_hash=%s exit=%d stdout_n=%d stderr_n=%d",
		argv, envAllowHash, exitCode, stdoutN, stderrN)
	outcome := OutcomeOK
	if failed {
		outcome = OutcomeFail
		detail += fmt.Sprintf(" first=%q last=%q", truncate(firstLine, 120), truncate(lastLine, 120))
	}
	l.record(LevelStep, name, outcome, detail, "", "")
	l.exitCodes = append(l.exitCodes, exitCode)
}

// NotePath attaches a path for failure dumps (sanitized).
func (l *Logger) NotePath(p string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = append(l.paths, sanitizePath(p))
}

// NoteID attaches a correlation ID for failure dumps.
func (l *Logger) NoteID(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ids = append(l.ids, id)
}

// Steps returns a copy of recorded steps (for tests of the logger itself).
func (l *Logger) Steps() []Step {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Step, len(l.steps))
	copy(out, l.steps)
	return out
}

// DumpLast prints the last N steps and attached context to the test log.
// Secret-like keys in detail strings are redacted.
func (l *Logger) DumpLast() {
	l.t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.LastN
	if n <= 0 {
		n = DefaultDumpLast
	}
	start := 0
	if len(l.steps) > n {
		start = len(l.steps) - n
	}
	l.t.Logf("STEPLOG_DUMP test=%s package=%s total_steps=%d showing=%d..%d",
		l.testName, l.pkg, len(l.steps), start+1, len(l.steps))
	for _, s := range l.steps[start:] {
		l.t.Logf("  [%d] %s name=%s outcome=%s elapsed_ms=%d detail=%s",
			s.Index, s.Level, s.Name, s.Outcome, s.Elapsed.Milliseconds(), RedactSecrets(s.Detail))
	}
	if len(l.paths) > 0 {
		l.t.Logf("  paths=%v", l.paths)
	}
	if len(l.ids) > 0 {
		l.t.Logf("  ids=%v", l.ids)
	}
	if len(l.exitCodes) > 0 {
		l.t.Logf("  exit_codes=%v", l.exitCodes)
	}
}

func (l *Logger) record(level Level, name string, outcome Outcome, detail, expected, actual string) {
	l.t.Helper()
	l.mu.Lock()
	l.index++
	idx := l.index
	elapsed := time.Since(l.start)
	s := Step{
		Index:    idx,
		Level:    level,
		Name:     name,
		Outcome:  outcome,
		Elapsed:  elapsed,
		Detail:   detail,
		Expected: expected,
		Actual:   actual,
	}
	l.steps = append(l.steps, s)
	l.mu.Unlock()

	// Stable, machine-parseable line (no host homes in detail by convention).
	line := fmt.Sprintf("STEPLOG package=%s test=%s idx=%d elapsed_ms=%d level=%s name=%s outcome=%s",
		l.pkg, l.testName, idx, elapsed.Milliseconds(), level, name, outcome)
	if detail != "" {
		line += " detail=" + RedactSecrets(detail)
	}
	if expected != "" || actual != "" {
		line += fmt.Sprintf(" expected=%q actual=%q", expected, actual)
	}
	l.t.Log(line)
}

func callerPackage() string {
	// Skip New + callerPackage frames.
	pc, file, _, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	fn := runtime.FuncForPC(pc)
	if fn != nil {
		// github.com/robertguss/go-foundry-cli/internal/foo.TestBar → internal/foo
		name := fn.Name()
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[:i]
		}
		const mod = "github.com/robertguss/go-foundry-cli/"
		if strings.HasPrefix(name, mod) {
			return strings.TrimPrefix(name, mod)
		}
		return name
	}
	return path.Dir(file)
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// sanitizePath replaces absolute home-like prefixes with a stable token so
// golden streams and logs are host-independent.
func sanitizePath(p string) string {
	if p == "" {
		return p
	}
	// Never leak full home directories into logs/goldens.
	if strings.HasPrefix(p, "/home/") || strings.HasPrefix(p, "/Users/") {
		parts := strings.SplitN(p, "/", 4)
		if len(parts) >= 4 {
			return "$HOME/" + parts[3]
		}
		return "$HOME"
	}
	return p
}
