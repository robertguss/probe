package testutil_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestLoggerResidualHelpers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("residual")
	log.Skip("skip-reason")
	log.Assertf(true, "ok_%s", "x")
	log.Subprocess("go-test", []string{"go", "test"}, "hash", 0, 10, 0, "first", "last", false)
	log.Subprocess("go-fail", []string{"go", "test"}, "hash", 1, 0, 20, "e1", "e2", true)
	log.NotePath("/tmp/x")
	log.NoteID("id-1")
	steps := log.Steps()
	if len(steps) == 0 {
		t.Fatal("expected steps")
	}
	log.DumpLast()
	// plandiff edges
	diff := testutil.UnifiedDiff("a", "line1\nline2\n", "b", "line1\nlineX\n")
	if !strings.Contains(diff, "line") {
		t.Fatalf("diff: %q", diff)
	}
	red := testutil.RedactPlanJSONForDiff([]byte(`{"destination":{"path":"/home/u/proj","parent":"/home/u","basename":"proj"}}`))
	if strings.Contains(red, "/home/u/proj") && !strings.Contains(red, "proj") {
		t.Fatalf("redact failed: %s", red)
	}
	log.PhaseEnd("residual", testutil.OutcomeOK)
}
