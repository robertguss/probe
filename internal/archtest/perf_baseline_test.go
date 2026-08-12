package archtest_test

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestPerfBaselineSchema locks the Section 49 / REQ-165 capture format.
// Format stability only — never asserts absolute millisecond gates.
func TestPerfBaselineSchema(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)

	log.Phase("arrange")
	log.Fixture("perf_dir", archtest.DocPerfDir)
	log.Fixture("latest", archtest.DocPerfLatestJSON)
	log.Fixture("schema", archtest.DocPerfSchemaJSON)
	log.Fixture("summary", archtest.DocPerfSummaryMD)
	log.Fixture("p28_citation", archtest.DocPerfP2ExitLinkMD)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("check")
	findings := archtest.CheckPerfBaselineSchema(root)
	log.Inputs(map[string]string{
		"findings": itoa(len(findings)),
	})
	for _, f := range findings {
		t.Logf("finding path=%s msg=%s", f.Path, f.Message)
	}
	log.Assert("schema_clean", len(findings) == 0, 0, len(findings))
	log.PhaseEnd("check", testutil.OutcomeOK)
}
