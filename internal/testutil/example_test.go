package testutil_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestExampleStepLog is the scaffold example multi-step test (P1.0 acceptance).
// Later packages copy this shape: Phase → Inputs → Fixtures → Steps → Asserts.
func TestExampleStepLog(t *testing.T) {
	log := testutil.New(t)

	log.Phase("arrange")
	log.Inputs(map[string]string{
		"spec":      "example.toml",
		"archetype": "cli",
		"verify":    "default",
	})
	dir := t.TempDir()
	log.NotePath(dir)
	golden := testutil.GoldenPath(filepath.Join("testdata"), "example_plan")
	log.Fixture("golden_path", golden)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	// Scaffold "plan" is a fixed string until internal/plan lands.
	planJSON := []byte(`{"schema":1,"command":"plan","ok":true,"result":{"files":[]}}` + "\n")
	log.Step("build_plan", testutil.OutcomeOK, "files=0")
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	// Seed golden on first run if missing (local only; CI has the file checked in).
	if _, err := filepath.Glob(golden); err == nil {
		// CompareGolden creates when UPDATE_GOLDEN=1; for green CI we write once
		// via testdata checked in. Ensure file exists for compare.
	}
	// Always compare against checked-in golden under this package.
	testutil.CompareGolden(t, golden, planJSON)
	log.Assert("plan_ok_field", bytes.Contains(planJSON, []byte(`"ok":true`)), true, string(planJSON))
	log.Assert("plan_schema", bytes.Contains(planJSON, []byte(`"schema":1`)), true, string(planJSON))
	log.PhaseEnd("assert", testutil.OutcomeOK)
}
