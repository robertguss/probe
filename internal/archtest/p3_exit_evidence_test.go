package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestP3ExitEvidenceArtifacts asserts k0o/bjt/66w evidence files exist with
// required sections (bead exit gates).
func TestP3ExitEvidenceArtifacts(t *testing.T) {
	log := testutil.New(t)
	log.Phase("p3_evidence")
	root := repoRoot(t)
	checks := map[string][]string{
		"docs/evidence/three-agent-acceptance.md": {
			"Grok Build", "Codex", "Cursor", "Scenario script", "ACCEPT",
		},
		"docs/evidence/risk-register-status.md": {
			"RSK-305", "RSK-310", "RSK-403", "Dogfood", "High-impact",
		},
		"docs/evidence/dogfood-tui-smoke.md": {
			"plan_sha256", "Time-to-first-green", "REQ-247",
		},
		"docs/evidence/P3-mvp-exit-review.md": {
			"Section 50", "PASS", "Three-agent",
		},
	}
	for rel, want := range checks {
		p := filepath.Join(root, rel)
		b, err := os.ReadFile(p)
		if err != nil {
			log.Fail("missing_"+rel, err.Error())
		}
		text := string(b)
		for _, w := range want {
			ok := strings.Contains(text, w)
			log.Assert(sanitize(rel+"_"+w), ok, true, ok)
		}
		log.Step("file", testutil.OutcomeOK, rel+" bytes="+itoa(len(b)))
	}
	log.PhaseEnd("p3_evidence", testutil.OutcomeOK)
}
