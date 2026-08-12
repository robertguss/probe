package toolrun

import (
	"fmt"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// ObservedStep is one actually-started subprocess recorded by a runner
// (REQ-214 process-tree audit). Env is the full child environment as constructed;
// failure dumps use key names only via FormatTreeDiffs / IsolationDump.
type ObservedStep struct {
	ID     string
	Binary string
	Argv   []string
	Env    map[string]string
	// Shell is true if the step was launched via a shell — always a tree failure.
	Shell bool
}

// TreeDiff describes a process-tree vs plan mismatch (REQ-120 / REQ-214).
type TreeDiff struct {
	Kind   string // count | missing | extra | argv | env | shell | binary
	StepID string
	// Detail never includes secret-bearing env values — key names and argv only
	// (public plan argv / binary basenames are allowed).
	Detail string
}

// CompareProcessTree checks observed steps against the plan's external_steps.
//
// Contract (REQ-120 / REQ-214 / FND-005):
//
//   - same length and order (plan is the exact execution list)
//   - each id, argv, binary match
//   - each observed env is exactly the planned allowlist (key set + values)
//   - no step was launched via a shell
//
// Env mismatch details name keys only; values are never emitted (redaction).
func CompareProcessTree(planned []plan.ExternalStep, observed []ObservedStep) []TreeDiff {
	var diffs []TreeDiff
	if len(planned) != len(observed) {
		diffs = append(diffs, TreeDiff{
			Kind:   "count",
			Detail: fmt.Sprintf("planned=%d observed=%d", len(planned), len(observed)),
		})
	}
	n := len(planned)
	if len(observed) < n {
		n = len(observed)
	}
	for i := 0; i < n; i++ {
		p, o := planned[i], observed[i]
		if o.Shell {
			diffs = append(diffs, TreeDiff{Kind: "shell", StepID: p.ID, Detail: "step launched via shell"})
		}
		if p.ID != o.ID {
			diffs = append(diffs, TreeDiff{
				Kind: "missing", StepID: p.ID,
				Detail: fmt.Sprintf("order mismatch: planned id %q observed id %q at index %d", p.ID, o.ID, i),
			})
			continue
		}
		if p.Binary != o.Binary && p.Binary != "" && o.Binary != "" {
			if p.Binary != o.Binary {
				diffs = append(diffs, TreeDiff{
					Kind: "binary", StepID: p.ID,
					Detail: fmt.Sprintf("binary planned=%q observed=%q", p.Binary, o.Binary),
				})
			}
		}
		if !argvEqual(p.Argv, o.Argv) {
			diffs = append(diffs, TreeDiff{
				Kind: "argv", StepID: p.ID,
				Detail: fmt.Sprintf("argv planned=%q observed=%q", p.Argv, o.Argv),
			})
		}
		// Env: planned is authority. Observed must match exactly for all planned keys
		// and must not contain keys outside the planned allowlist.
		// Diff details: key names only — never values (REQ-214 redaction).
		for k, want := range p.Env {
			got, ok := o.Env[k]
			if !ok {
				diffs = append(diffs, TreeDiff{
					Kind: "env", StepID: p.ID,
					Detail: fmt.Sprintf("missing env key %s", k),
				})
				continue
			}
			if got != want {
				diffs = append(diffs, TreeDiff{
					Kind: "env", StepID: p.ID,
					Detail: fmt.Sprintf("env key %s value mismatch", k),
				})
			}
		}
		for k := range o.Env {
			if _, ok := p.Env[k]; !ok {
				diffs = append(diffs, TreeDiff{
					Kind: "env", StepID: p.ID,
					Detail: fmt.Sprintf("extra env key %s (not in plan allowlist)", k),
				})
			}
		}
	}
	if len(observed) > len(planned) {
		for i := len(planned); i < len(observed); i++ {
			if observed[i].Shell {
				diffs = append(diffs, TreeDiff{
					Kind: "shell", StepID: observed[i].ID,
					Detail: "unplanned step launched via shell",
				})
			}
			diffs = append(diffs, TreeDiff{
				Kind: "extra", StepID: observed[i].ID,
				Detail: fmt.Sprintf("unplanned step at index %d argv=%q", i, observed[i].Argv),
			})
		}
	}
	return diffs
}

// FormatTreeDiffs returns a stable multi-line summary for logs (no env values).
func FormatTreeDiffs(diffs []TreeDiff) string {
	if len(diffs) == 0 {
		return "process-tree equals plan external_steps"
	}
	parts := make([]string, 0, len(diffs))
	for _, d := range diffs {
		parts = append(parts, fmt.Sprintf("%s step=%s %s", d.Kind, d.StepID, d.Detail))
	}
	return strings.Join(parts, "; ")
}

// PlannedArgvSummary is a one-line dump of plan external_steps for isolation logs.
func PlannedArgvSummary(steps []plan.ExternalStep) string {
	parts := make([]string, 0, len(steps))
	for _, s := range steps {
		parts = append(parts, fmt.Sprintf("%s:%s", s.ID, strings.Join(s.Argv, " ")))
	}
	return strings.Join(parts, " | ")
}

// FakePlanRunner records planned executions without starting processes.
// Used for process-tree equality unit tests (REQ-214 fake-runner).
type FakePlanRunner struct {
	Observed []ObservedStep
	// InjectExtra injects unplanned steps after RunPlan (negative tests).
	InjectExtra []ObservedStep
	// MutateEnv mutates env of each step before recording (negative).
	MutateEnv func(id string, env map[string]string)
}

// RunPlan "executes" each step by recording binary/argv/env for tree audit.
func (r *FakePlanRunner) RunPlan(steps []plan.ExternalStep) []ObservedStep {
	r.Observed = r.Observed[:0]
	for _, s := range steps {
		env := copyStringMap(s.Env)
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

// EnvKeySetEqual reports whether observed KEY=value slice has exactly the
// same key set as wantKeys (order-independent). Values are not compared.
func EnvKeySetEqual(env []string, wantKeys []string) bool {
	m := envSliceToMap(env)
	if len(m) != len(wantKeys) {
		return false
	}
	allowed := make(map[string]struct{}, len(wantKeys))
	for _, k := range wantKeys {
		allowed[k] = struct{}{}
	}
	for k := range m {
		if _, ok := allowed[k]; !ok {
			return false
		}
	}
	return true
}

// EnvKeySetMatchesAllowlist reports keys outside allowlist and required keys
// that are missing. Optional host keys (e.g. TMPDIR) listed in optional may
// be absent without counting as missing. Returns sorted extra and missing.
func EnvKeySetMatchesAllowlist(env map[string]string, allowlist []string, optional map[string]struct{}) (extra, missing []string) {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, k := range allowlist {
		allowed[k] = struct{}{}
	}
	for k := range env {
		if _, ok := allowed[k]; !ok {
			extra = append(extra, k)
		}
	}
	for _, k := range allowlist {
		if optional != nil {
			if _, opt := optional[k]; opt {
				continue
			}
		}
		if _, ok := env[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	return extra, missing
}

func argvEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
