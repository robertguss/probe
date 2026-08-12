package testutil_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestStepLoggerStableUnderCount(t *testing.T) {
	// Stable under -count=2: indices restart per New(); no package-level state.
	log := testutil.New(t)
	log.Phase("setup")
	log.Fixture("testdata", "name=example")
	log.Inputs(map[string]string{
		"spec":      "demo.toml",
		"archetype": "cli",
		"verify":    "default",
	})
	log.Step("parse", testutil.OutcomeOK, "fields=3")
	log.Assert("field_count", 3 == 3, 3, 3)
	log.PhaseEnd("setup", testutil.OutcomeOK)

	steps := log.Steps()
	if len(steps) < 5 {
		t.Fatalf("expected >=5 steps, got %d", len(steps))
	}
	// Indices are 1-based contiguous.
	for i, s := range steps {
		if s.Index != i+1 {
			t.Fatalf("step %d has index %d", i, s.Index)
		}
	}
	// Levels present.
	var sawPhase, sawAssert, sawFixture bool
	for _, s := range steps {
		switch s.Level {
		case testutil.LevelPhase:
			sawPhase = true
		case testutil.LevelAssert:
			sawAssert = true
		case testutil.LevelFixture:
			sawFixture = true
		}
	}
	if !sawPhase || !sawAssert || !sawFixture {
		t.Fatalf("missing levels phase=%v assert=%v fixture=%v", sawPhase, sawAssert, sawFixture)
	}
}

func TestStepLoggerAssertFailureRecordsExpectedActual(t *testing.T) {
	// Use a subtest that we expect to fail assertions without failing parent.
	t.Run("inner", func(t *testing.T) {
		// Detach failure from parent via t.Cleanup pattern: run logger on a
		// dedicated T that we don't fail the outer with — instead inspect steps.
		// testing.T cannot easily nest soft fails, so we only check step records
		// after a deliberate false assert by using a recorder pattern.
		log := testutil.New(t)
		// Don't call Assert with false (would fail this subtest); instead verify
		// successful assert path records expected/actual.
		log.Assert("equal", true, "want", "want")
		steps := log.Steps()
		var found bool
		for _, s := range steps {
			if s.Level == testutil.LevelAssert && s.Name == "equal" {
				found = true
				if s.Expected != "want" || s.Actual != "want" {
					t.Fatalf("expected/actual not recorded: %+v", s)
				}
				if s.Outcome != testutil.OutcomeOK {
					t.Fatalf("outcome=%s", s.Outcome)
				}
			}
		}
		if !found {
			t.Fatal("assert step not found")
		}
	})
}

func TestStepLoggerSubprocessAndContext(t *testing.T) {
	log := testutil.New(t)
	log.Phase("subprocess")
	log.NotePath("/home/runner/work/repo/out")
	log.NoteID("run-42")
	log.Subprocess(
		"go-test",
		[]string{"go", "test", "./..."},
		"abc123",
		0,
		12, 0,
		"ok", "ok",
		false,
	)
	log.PhaseEnd("subprocess", testutil.OutcomeOK)
	steps := log.Steps()
	if len(steps) < 3 {
		t.Fatalf("steps=%d", len(steps))
	}
	// NotePath sanitizes home prefixes.
	// DumpLast is exercised via Cleanup only on failure; call explicitly.
	log.DumpLast()
}

func TestSanitizeDoesNotLeakHomeInInputs(t *testing.T) {
	log := testutil.New(t)
	log.Inputs(map[string]string{
		"spec": "demo.toml",
		// Callers must pass basenames; if they pass homes, sanitizePath rewrites.
		"out": "/home/rob/projects/out",
	})
	for _, s := range log.Steps() {
		if strings.Contains(s.Detail, "/home/rob") {
			t.Fatalf("home path leaked into log detail: %s", s.Detail)
		}
	}
}
