//go:build unix

package verify_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/verify"
)

// memLog records VerifyStep calls for timeline assertions.
type memLog struct {
	mu      sync.Mutex
	entries []logEntry
}

type logEntry struct {
	Step, Outcome, Detail string
}

func (m *memLog) VerifyStep(step, outcome, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, logEntry{step, outcome, detail})
}

func (m *memLog) Entries() []logEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]logEntry, len(m.entries))
	copy(out, m.entries)
	return out
}

func (m *memLog) Has(step, outcome string) bool {
	for _, e := range m.Entries() {
		if e.Step == step && e.Outcome == outcome {
			return true
		}
	}
	return false
}

func (m *memLog) DetailContains(substr string) bool {
	for _, e := range m.Entries() {
		if strings.Contains(e.Detail, substr) {
			return true
		}
	}
	return false
}

func (m *memLog) FailedStepName() string {
	for _, e := range m.Entries() {
		if e.Outcome == "fail" && e.Step != "pipeline_fail" {
			return e.Step
		}
	}
	return ""
}

// writeStage creates files under a temp stage and returns the path.
func writeStage(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	return root
}

// minimalModule is a tiny go.mod + main + test that is format-clean.
func minimalModule(module string) map[string]string {
	if module == "" {
		module = "example.com/demo"
	}
	return map[string]string{
		"go.mod": "module " + module + "\n\ngo 1.26.0\n",
		"main.go": `package main

func main() {}
`,
		"main_test.go": `package main

import "testing"

func TestOK(t *testing.T) {}
`,
	}
}

// baseOpts builds Options for a frozen-skip-tidy run (no real go needed when
// using FakeStepRunner for remaining external steps).
func baseOpts(stage string, mode verify.Mode, fake *verify.FakeStepRunner) verify.Options {
	_ = fake
	return verify.Options{
		Mode:         mode,
		GoBinary:     "/usr/bin/go", // basename used; fake never execs
		Env:          map[string]string{"PATH": "/usr/bin", "HOME": "/tmp", "TMPDIR": "/tmp"},
		StageFD:      3, // fake ignores
		StageDir:     stage,
		SkipEnvCheck: true,
		SkipTidy:     true,
	}
}
