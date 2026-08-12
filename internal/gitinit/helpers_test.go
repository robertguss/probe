//go:build unix

package gitinit_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// memLog records GitStep calls for timeline assertions.
type memLog struct {
	mu      sync.Mutex
	entries []logEntry
}

type logEntry struct {
	Step, Outcome, Detail string
}

func (m *memLog) GitStep(step, outcome, detail string) {
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

func captureOrig(t *testing.T) *toolrun.OriginalCWD {
	t.Helper()
	orig, err := toolrun.CaptureOriginalCWD()
	if err != nil {
		t.Fatalf("CaptureOriginalCWD: %v", err)
	}
	t.Cleanup(func() { _ = orig.Close() })
	return orig
}

func openStageFD(t *testing.T, dir string) int {
	t.Helper()
	fd, err := toolrun.OpenDirFD(dir)
	if err != nil {
		t.Fatalf("OpenDirFD(%s): %v", dir, err)
	}
	t.Cleanup(func() { _ = toolrun.CloseFD(fd) })
	return fd
}

func newExecutor(t *testing.T, opts toolrun.BoundStarterOptions) *toolrun.Executor {
	t.Helper()
	orig := captureOrig(t)
	starter, err := toolrun.NewBoundStarter(orig, opts)
	if err != nil {
		t.Fatalf("NewBoundStarter: %v", err)
	}
	ex, err := toolrun.NewExecutor(starter)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	ex.KillGrace = 50 * time.Millisecond
	return ex
}

func lookGit(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	return p
}

func stageWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// plantGitViaFakeStart is a StartChild that plants semantic-valid .git and succeeds.
func plantGitViaFakeStart(stageDir, branch string) toolrun.BoundStarterOptions {
	return toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, binary string, args, env []string, dir string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			if dir != "" {
				return 0, nil, fmt.Errorf("pathname Dir set: %q", dir)
			}
			// Mimic git init by planting layout; extract branch from args.
			b := branch
			for _, a := range args {
				if strings.HasPrefix(a, "--initial-branch=") {
					b = strings.TrimPrefix(a, "--initial-branch=")
				}
			}
			if err := PlantMinimalGitDir(stageDir, b); err != nil {
				return 0, nil, err
			}
			_ = binary
			_ = env
			return 7001, func() ([]byte, []byte, error) {
				return []byte("Initialized empty Git repository\n"), nil, nil
			}, nil
		},
	}
}

// ensure no unused import when helpers evolve
var _ = testutil.New
