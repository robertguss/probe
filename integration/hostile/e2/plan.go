package e2

import (
	"fmt"
	"strings"
)

// ExternalStep is one planned subprocess step (Section 28.2 / Appendix C
// external_steps subset used by the E2 process-tree audit).
type ExternalStep struct {
	ID             string            `json:"id"`
	Binary         string            `json:"binary"` // absolute path, resolved at startup
	Argv           []string          `json:"argv"`
	Cwd            string            `json:"cwd"` // "stage-descriptor" for transactional steps
	Mutates        []string          `json:"mutates,omitempty"`
	Network        string            `json:"network"` // "no" | "may"
	TimeoutS       int               `json:"timeout_s"`
	OutputCapBytes int               `json:"output_cap_bytes"`
	Env            map[string]string `json:"env"` // exact allowlist map
}

// ObservedStep is one actually-started subprocess recorded by a runner.
type ObservedStep struct {
	ID     string
	Binary string
	Argv   []string
	Env    map[string]string // full child env as constructed
	// Shell is true if the step was launched via a shell — always a failure.
	Shell bool
}

// DefaultExternalSteps returns the Section 34.1 closed-world step list for
// default verification (no git). Binary paths are placeholders filled by
// ResolveBinaries. Env maps use the fixed Section 34.2 values; host-captured
// keys are filled when a HostCapture is applied via ApplyHostToSteps.
func DefaultExternalSteps() []ExternalStep {
	goEnv := fixedGoEnvMap()
	return []ExternalStep{
		{
			ID: "go-mod-tidy", Binary: "go",
			Argv: []string{"go", "mod", "tidy"},
			Cwd:  "stage-descriptor", Mutates: []string{"go.mod", "go.sum"},
			Network: "may", TimeoutS: 600, OutputCapBytes: 4 << 20, Env: copyMap(goEnv),
		},
		{
			ID: "go-mod-verify", Binary: "go",
			Argv: []string{"go", "mod", "verify"},
			Cwd:  "stage-descriptor", Mutates: nil,
			Network: "no", TimeoutS: 120, OutputCapBytes: 4 << 20, Env: copyMap(goEnv),
		},
		{
			ID: "go-test", Binary: "go",
			Argv: []string{"go", "test", "-count=1", "-buildvcs=false", "-mod=readonly", "./..."},
			Cwd:  "stage-descriptor", Mutates: nil,
			Network: "no", TimeoutS: 300, OutputCapBytes: 4 << 20, Env: copyMap(goEnv),
		},
		{
			ID: "go-vet", Binary: "go",
			Argv: []string{"go", "vet", "-buildvcs=false", "-mod=readonly", "./..."},
			Cwd:  "stage-descriptor", Mutates: nil,
			Network: "no", TimeoutS: 300, OutputCapBytes: 4 << 20, Env: copyMap(goEnv),
		},
	}
}

// StrictExternalSteps adds staticcheck + govulncheck (Section 34.1 strict).
func StrictExternalSteps() []ExternalStep {
	goEnv := fixedGoEnvMap()
	return append(DefaultExternalSteps(),
		ExternalStep{
			ID: "go-staticcheck", Binary: "go",
			Argv: []string{"go", "tool", "staticcheck", "./..."},
			Cwd:  "stage-descriptor", Network: "no", TimeoutS: 300,
			OutputCapBytes: 4 << 20, Env: copyMap(goEnv),
		},
		ExternalStep{
			ID: "go-govulncheck", Binary: "go",
			Argv: []string{"go", "tool", "govulncheck", "./..."},
			Cwd:  "stage-descriptor", Network: "may", TimeoutS: 600,
			OutputCapBytes: 4 << 20, Env: copyMap(goEnv),
		},
	)
}

// GitInitStep returns the optional isolated git init step (Section 34.2).
// templateDir must be the Foundry-owned empty scratch path.
func GitInitStep(gitBinary, branch, templateDir, path string) ExternalStep {
	return ExternalStep{
		ID:      "git-init",
		Binary:  gitBinary,
		Argv:    []string{"git", "init", "--initial-branch=" + branch, "--template=" + templateDir, "."},
		Cwd:     "stage-descriptor",
		Network: "no", TimeoutS: 60, OutputCapBytes: 4 << 20,
		Env: EnvMap(ConstructGitEnv(path, templateDir)),
	}
}

// ApplyHostToSteps fills PATH/HOME/TMPDIR/cache/proxy keys into each go step's env.
func ApplyHostToSteps(steps []ExternalStep, h HostCapture) []ExternalStep {
	out := make([]ExternalStep, len(steps))
	for i, s := range steps {
		s.Env = copyMap(s.Env)
		if s.ID == "git-init" {
			// git env is already complete except PATH may be refreshed
			s.Env["PATH"] = h.PATH
			out[i] = s
			continue
		}
		// go steps
		constructed := EnvMap(ConstructGoEnv(h))
		s.Env = constructed
		if s.Binary == "go" || s.Binary == "" {
			// Binary resolved later.
		}
		out[i] = s
	}
	return out
}

// ResolveBinaries sets Binary to absolute paths via lookPath.
func ResolveBinaries(steps []ExternalStep, lookPath func(string) (string, error)) ([]ExternalStep, error) {
	out := make([]ExternalStep, len(steps))
	for i, s := range steps {
		name := s.Binary
		if name == "" && len(s.Argv) > 0 {
			name = s.Argv[0]
		}
		abs, err := lookPath(name)
		if err != nil {
			return nil, fmt.Errorf("resolve binary for step %s (%s): %w", s.ID, name, err)
		}
		s.Binary = abs
		out[i] = s
	}
	return out, nil
}

// TreeDiff describes a process-tree vs plan mismatch.
type TreeDiff struct {
	Kind   string // count | missing | extra | argv | env | shell | binary
	StepID string
	Detail string
}

// CompareProcessTree checks observed steps against the plan's external_steps.
// Contract (REQ-120 / REQ-214 / FND-005):
//
//   - same length and order (plan is the exact execution list)
//   - each id, argv, binary match
//   - each observed env is exactly the planned allowlist (key set + values for fixed keys)
//   - no step was launched via a shell
func CompareProcessTree(planned []ExternalStep, observed []ObservedStep) []TreeDiff {
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
			// Allow planned placeholder "go"/"git" only if observed absolute ends with it —
			// after ResolveBinaries both should be absolute and equal.
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
		// Env: planned is the authority. Observed must match exactly for all planned keys
		// and must not contain keys outside the planned allowlist.
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
				// Redact values in detail — key name + mismatch flag only for secrets safety;
				// for non-secret diagnostics we still show both (normative values are public).
				diffs = append(diffs, TreeDiff{
					Kind: "env", StepID: p.ID,
					Detail: fmt.Sprintf("env key %s: planned=%q observed=%q", k, want, got),
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

// FormatTreeDiffs returns a stable multi-line summary for logs.
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

// EnvAllowlistHash returns a stable sorted "KEY=val" join for logging (public values only).
func EnvAllowlistHash(env map[string]string) string {
	keys := EnvKeys(env)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+env[k])
	}
	return strings.Join(parts, "\n")
}

func fixedGoEnvMap() map[string]string {
	m := make(map[string]string, len(FixedGoEnv)+8)
	for k, v := range FixedGoEnv {
		m[k] = v
	}
	// placeholders for host-captured; filled by ApplyHostToSteps
	m["PATH"] = ""
	m["HOME"] = ""
	m["GOMODCACHE"] = ""
	m["GOCACHE"] = ""
	m["GOPATH"] = ""
	m["GOPROXY"] = ""
	m["GOSUMDB"] = ""
	return m
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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

// SortedIDs returns step ids in plan order (not alphabetically sorted).
func SortedIDs(steps []ExternalStep) []string {
	ids := make([]string, len(steps))
	for i, s := range steps {
		ids[i] = s.ID
	}
	return ids
}

// PlannedArgvSummary is a one-line dump of the plan for evidence logs.
func PlannedArgvSummary(steps []ExternalStep) string {
	parts := make([]string, 0, len(steps))
	for _, s := range steps {
		parts = append(parts, fmt.Sprintf("%s:%s", s.ID, strings.Join(s.Argv, " ")))
	}
	return strings.Join(parts, " | ")
}
