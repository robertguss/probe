//go:build unix

package fsx_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// memLog records FSXStep calls for assertions.
type memLog struct {
	mu      sync.Mutex
	entries []logEntry
}

type logEntry struct {
	Probe, Step, Outcome, Detail string
}

func (m *memLog) FSXStep(probe, step, outcome, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, logEntry{probe, step, outcome, detail})
}

func (m *memLog) Entries() []logEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]logEntry, len(m.entries))
	copy(out, m.entries)
	return out
}

func (m *memLog) Has(probe, step, outcome string) bool {
	for _, e := range m.Entries() {
		if e.Probe == probe && e.Step == step && e.Outcome == outcome {
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

// privateParent creates a 0700 directory under root for custody-safe tests.
func privateParent(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustFoundryID(t *testing.T, err error) diagnostic.Identifier {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	fe, ok := diagnostic.AsFoundryError(err)
	if !ok {
		t.Fatalf("expected *diagnostic.FoundryError, got %T: %v", err, err)
	}
	return fe.ID()
}

func requireID(t *testing.T, err error, want diagnostic.Identifier) {
	t.Helper()
	got := mustFoundryID(t, err)
	if got != want {
		t.Fatalf("error id=%s want=%s err=%v", got, want, err)
	}
	// Remediation must be non-empty for agent actionability.
	fe, _ := diagnostic.AsFoundryError(err)
	if fe.Remediation() == "" {
		t.Fatalf("remediation empty for %s", want)
	}
}

// bridgeLog forwards fsx steps into the testutil step logger.
type bridgeLog struct {
	l *testutil.Logger
}

func (b bridgeLog) FSXStep(probe, step, outcome, detail string) {
	o := testutil.OutcomeOK
	if outcome == "fail" {
		o = testutil.OutcomeFail
	} else if outcome == "info" {
		o = testutil.OutcomeInfo
	}
	b.l.Step(probe+"."+step, o, detail)
}

func newBridge(t *testing.T) (fsx.StepLogger, *testutil.Logger, *memLog) {
	t.Helper()
	tl := testutil.New(t)
	ml := &memLog{}
	return multiLog{tl: bridgeLog{tl}, mem: ml}, tl, ml
}

type multiLog struct {
	tl  bridgeLog
	mem *memLog
}

func (m multiLog) FSXStep(probe, step, outcome, detail string) {
	m.tl.FSXStep(probe, step, outcome, detail)
	m.mem.FSXStep(probe, step, outcome, detail)
}

func fmtMode(mode os.FileMode) string {
	return fmt.Sprintf("%04o", mode.Perm())
}
