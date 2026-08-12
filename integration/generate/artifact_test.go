package generatee2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	generatee2e "github.com/robertguss/go-foundry-cli/integration/generate"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestFailureArtifactNamesStageAndCause(t *testing.T) {
	log := testutil.New(t)
	log.Phase("artifact_format")

	a := generatee2e.FailureArtifact{
		Case:         "TestMidStage/render",
		Stage:        "render",
		Cause:        "injected_fail",
		PlanSHA256:   strings.Repeat("a", 64),
		CommitResult: "failed-preserved",
		Exit:         1,
		StagePath:    "/tmp/foundry-fx/parent/.foundry-demo-deadbeef",
		Destination:  "foundry-smoke-cli",
		Remediation:  "Inspect stage_path; Foundry never auto-deletes stages.",
		ExternalSteps: []string{
			"go-mod-tidy", "go-mod-verify", "go-test", "go-vet",
		},
		Timeline: []string{
			"progress: create-stage",
			"progress: render",
		},
		Detail: "stage preserved after injected render failure",
	}
	body := generatee2e.FormatFailureArtifact(a)
	log.Assert("has_stage", strings.Contains(body, "stage=render"), true, body)
	log.Assert("has_cause", strings.Contains(body, "cause=injected_fail"), true, body)
	log.Assert("has_commit", strings.Contains(body, "commit_result=failed-preserved"), true, body)
	log.Assert("has_exit", strings.Contains(body, "exit=1"), true, body)
	log.Assert("has_plan", strings.Contains(body, "plan_sha256="), true, body)
	log.Assert("has_stage_path", strings.Contains(body, "stage_path="), true, body)
	log.Assert("has_remediation", strings.Contains(body, "remediation="), true, body)
	// Never dump secret-like env values.
	log.Assert("no_token_value", !strings.Contains(body, "SECRET="), true, body)

	dir := t.TempDir()
	path, err := generatee2e.WriteFailureArtifact(dir, a)
	if err != nil {
		log.Fail("write", err.Error())
	}
	log.Assert("path_set", path != "", true, path)
	base := filepath.Base(path)
	log.Assert("name_has_stage", strings.Contains(base, "stage-render"), true, base)
	log.Assert("name_has_cause", strings.Contains(base, "cause-injected_fail"), true, base)

	// Env var path.
	art := t.TempDir()
	t.Setenv(generatee2e.ArtifactEnvVar, art)
	path2, err := generatee2e.WriteFailureArtifact("", a)
	if err != nil {
		log.Fail("write_env", err.Error())
	}
	log.Assert("env_path", path2 != "" && strings.HasPrefix(path2, art), true, path2)
	b, err := os.ReadFile(path2)
	if err != nil {
		log.Fail("read", err.Error())
	}
	log.Assert("body_match_prefix", strings.HasPrefix(string(b), "generate_e2e_failure"), true, string(b)[:40])

	// No dir configured → no-op.
	t.Setenv(generatee2e.ArtifactEnvVar, "")
	path3, err := generatee2e.WriteFailureArtifact("", a)
	if err != nil {
		log.Fail("noop_err", err.Error())
	}
	log.Assert("noop_empty", path3 == "", true, path3)

	log.PhaseEnd("artifact_format", testutil.OutcomeOK)
}
