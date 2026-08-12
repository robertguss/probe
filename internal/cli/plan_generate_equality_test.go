package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// equalityFixture is one golden/write-free plan fixture for Section 13.3.
type equalityFixture struct {
	name string // short name for subtests / logs
	// absSpec resolves the Project Specification path.
	absSpec func(t *testing.T) string
	// projectName is the required destination basename.
	projectName string
}

func equalityFixtures() []equalityFixture {
	return []equalityFixture{
		{
			name:        "mvp_minimal_cli",
			projectName: "minimal-cli",
			absSpec:     func(t *testing.T) string { return examplesPath(t, "minimal-cli.toml") },
		},
		{
			name:        "mvp_minimal_tui",
			projectName: "minimal-tui",
			absSpec:     func(t *testing.T) string { return examplesPath(t, "minimal-tui.toml") },
		},
		{
			name:        "appendix_b_private_cli",
			projectName: "private-cli",
			absSpec:     func(t *testing.T) string { return examplesPath(t, "appendix-b-private-cli.toml") },
		},
		{
			name:        "appendix_b_private_tui",
			projectName: "private-tui",
			absSpec:     func(t *testing.T) string { return examplesPath(t, "appendix-b-private-tui.toml") },
		},
		{
			name:        "foundry_smoke_cli",
			projectName: "foundry-smoke-cli",
			absSpec: func(t *testing.T) string {
				return filepath.Join(repoRoot(t), "integration", "fixtures", "foundry-smoke-cli", "foundry.toml")
			},
		},
	}
}

// TestPlanGenerateByteEqualityProperty is the Section 13.3 defect-class suite:
// plan command construction is byte-equal to generate's pure plan path for
// every golden fixture × verify mode, including --dest overrides.
//
// Uses fake generate stages so CI stays write-free and host-independent.
// Process-level e2e lives under cmd/foundry (TestPlanGenerateEquality).
func TestPlanGenerateByteEqualityProperty(t *testing.T) {
	log := testutil.New(t)
	log.Phase("section_13_3_property")

	opts := planPackageOpts()
	opts.GenerateStages = fakeHappyStages()
	verifies := []string{"default", "strict"}

	for _, fx := range equalityFixtures() {
		for _, verify := range verifies {
			fx, verify := fx, verify
			name := fx.name + "_" + verify
			t.Run(name, func(t *testing.T) {
				sub := testutil.New(t)
				sub.Phase("arrange")
				specPath := fx.absSpec(t)
				sub.Fixture("spec", filepath.Base(specPath))
				sub.Inputs(map[string]string{
					"fixture": fx.name,
					"verify":  verify,
					"spec":    filepath.Base(specPath),
				})
				sub.PhaseEnd("arrange", testutil.OutcomeOK)

				sub.Phase("act")
				// Pure package dual-construction (plan builder ≡ generate builder).
				pPlan, pGen := dualPipelinePlans(t, specPath, plan.VerifyMode(verify), "")
				sub.Step("package_plan_sha256", testutil.OutcomeOK, pPlan.PlanSHA256())
				sub.Step("package_gen_sha256", testutil.OutcomeOK, pGen.PlanSHA256())
				sub.PhaseEnd("act", testutil.OutcomeOK)

				sub.Phase("assert_package")
				if !bytes.Equal(pPlan.JSON(), pGen.JSON()) {
					testutil.AssertPlanJSONEqual(t, name+"/package", pPlan.JSON(), pGen.JSON())
					sub.Fail("package_byte_equal", "canonical plan JSON differs")
				}
				sub.Assert("package_sha_equal", pPlan.PlanSHA256() == pGen.PlanSHA256(),
					pPlan.PlanSHA256(), pGen.PlanSHA256())
				// -count=2 stability within the table row.
				pPlan2, _ := dualPipelinePlans(t, specPath, plan.VerifyMode(verify), "")
				sub.Assert("package_stable", pPlan.PlanSHA256() == pPlan2.PlanSHA256(),
					pPlan.PlanSHA256(), pPlan2.PlanSHA256())
				if !bytes.Equal(pPlan.JSON(), pPlan2.JSON()) {
					testutil.AssertPlanJSONEqual(t, name+"/stable", pPlan.JSON(), pPlan2.JSON())
					sub.Fail("package_stable_bytes", "second construction diverged")
				}
				sub.PhaseEnd("assert_package", testutil.OutcomeOK)

				sub.Phase("assert_cli")
				planRes := runCLIOpts(t, context.Background(), nil, opts,
					"plan", "--spec", specPath, "--verify", verify, "--output", "json")
				genRes := runCLIOpts(t, context.Background(), nil, opts,
					"generate", "--spec", specPath, "--verify", verify, "--output", "json")
				sub.Assert("plan_exit_0", planRes.Code == 0, 0, planRes.Code)
				sub.Assert("gen_exit_0", genRes.Code == 0, 0, genRes.Code)
				sub.Assert("plan_stderr_empty", planRes.Stderr == "", "", planRes.Stderr)
				sub.Assert("gen_stderr_empty", genRes.Stderr == "", "", genRes.Stderr)

				planEnv := parseEnvelope(t, planRes.Stdout)
				genEnv := parseEnvelope(t, genRes.Stdout)
				planResult, _ := planEnv["result"].(map[string]any)
				genResult, _ := genEnv["result"].(map[string]any)
				if planResult == nil {
					sub.Fail("plan_result", "missing result")
				}
				planSHA, _ := planResult["plan_sha256"].(string)
				genSHA, _ := genResult["plan_sha256"].(string)
				sub.Step("cli_plan_sha256", testutil.OutcomeOK, planSHA)
				sub.Step("cli_gen_sha256", testutil.OutcomeOK, genSHA)
				sub.Assert("cli_sha_equal", planSHA == genSHA, planSHA, genSHA)
				sub.Assert("sha_len", len(planSHA) == 64, 64, len(planSHA))

				// CLI plan result must match package dual-construction under
				// planPackageOpts (same pure path identity).
				cliPlanBody, err := json.Marshal(planResult)
				if err != nil {
					sub.Fail("marshal_cli_plan", err.Error())
				}
				// Re-seal via package plan for fair compare: CLI SpecPath is abs.
				pkgWithAbs, _ := dualPipelinePlans(t, specPath, plan.VerifyMode(verify), specPath)
				// Compare digests first; dump redacted JSON on mismatch.
				if planSHA != pkgWithAbs.PlanSHA256() {
					testutil.AssertPlanJSONEqual(t, name+"/cli_vs_package",
						pkgWithAbs.JSON(), append(cliPlanBody, '\n'))
					sub.Fail("cli_package_sha", planSHA+" vs "+pkgWithAbs.PlanSHA256())
				}
				sub.Assert("cli_matches_package", planSHA == pkgWithAbs.PlanSHA256(),
					pkgWithAbs.PlanSHA256(), planSHA)

				// Step list / verification mode recorded on plan.
				ver, _ := planResult["verification"].(map[string]any)
				sub.Assert("verify_mode", ver["mode"] == verify, verify, ver["mode"])
				steps := stepIDsFromPlan(planResult)
				if verify == "strict" {
					sub.Assert("strict_staticcheck", containsStr(steps, "go-staticcheck"), true, steps)
					sub.Assert("strict_govulncheck", containsStr(steps, "go-govulncheck"), true, steps)
				} else {
					sub.Assert("default_no_staticcheck", !containsStr(steps, "go-staticcheck"), false, steps)
				}
				sub.PhaseEnd("assert_cli", testutil.OutcomeOK)
			})
		}
	}

	// Cross-mode: default vs strict diverge identically for plan and generate.
	log.Phase("verify_matrix_cross")
	spec := examplesPath(t, "minimal-cli.toml")
	planDef := extractSHA(t, runCLIOpts(t, context.Background(), nil, opts,
		"plan", "--spec", spec, "--verify", "default", "--output", "json"))
	planStrict := extractSHA(t, runCLIOpts(t, context.Background(), nil, opts,
		"plan", "--spec", spec, "--verify", "strict", "--output", "json"))
	genDef := extractSHA(t, runCLIOpts(t, context.Background(), nil, opts,
		"generate", "--spec", spec, "--verify", "default", "--output", "json"))
	genStrict := extractSHA(t, runCLIOpts(t, context.Background(), nil, opts,
		"generate", "--spec", spec, "--verify", "strict", "--output", "json"))
	log.Assert("default_plan_gen", planDef == genDef, planDef, genDef)
	log.Assert("strict_plan_gen", planStrict == genStrict, planStrict, genStrict)
	log.Assert("modes_diverge", planDef != planStrict, true, planDef == planStrict)
	log.Step("default_sha", testutil.OutcomeOK, planDef)
	log.Step("strict_sha", testutil.OutcomeOK, planStrict)
	log.PhaseEnd("verify_matrix_cross", testutil.OutcomeOK)

	log.PhaseEnd("section_13_3_property", testutil.OutcomeOK)
}

// TestPlanGenerateDestOverrideEquality proves --dest is recorded identically
// by plan and generate (Section 13.3 / REQ-033).
func TestPlanGenerateDestOverrideEquality(t *testing.T) {
	log := testutil.New(t)
	log.Phase("dest_override_equality")

	opts := planPackageOpts()
	opts.GenerateStages = fakeHappyStages()
	specPath := examplesPath(t, "minimal-cli.toml")
	dest := "/tmp/foundry-plan-gen-eq/minimal-cli"

	log.Inputs(map[string]string{
		"spec": "minimal-cli.toml",
		"dest": "minimal-cli", // basename only in logs
	})

	planRes := runCLIOpts(t, context.Background(), nil, opts,
		"plan", "--spec", specPath, "--dest", dest, "--output", "json")
	genRes := runCLIOpts(t, context.Background(), nil, opts,
		"generate", "--spec", specPath, "--dest", dest, "--output", "json")
	log.Assert("plan_0", planRes.Code == 0, 0, planRes.Code)
	log.Assert("gen_0", genRes.Code == 0, 0, genRes.Code)

	planEnv := parseEnvelope(t, planRes.Stdout)
	genEnv := parseEnvelope(t, genRes.Stdout)
	planResult := planEnv["result"].(map[string]any)
	genResult := genEnv["result"].(map[string]any)
	planSHA, _ := planResult["plan_sha256"].(string)
	genSHA, _ := genResult["plan_sha256"].(string)
	log.Assert("sha_equal", planSHA == genSHA, planSHA, genSHA)

	d, _ := planResult["destination"].(map[string]any)
	log.Assert("path", d["path"] == dest, dest, d["path"])
	log.Assert("basename", d["basename"] == "minimal-cli", "minimal-cli", d["basename"])
	log.Assert("observation", d["observation"] == "absent", "absent", d["observation"])

	// Package dual construction with dest override.
	p1, p2 := dualPipelinePlansWithDest(t, specPath, plan.VerifyDefault, dest)
	log.Assert("package_sha", p1.PlanSHA256() == p2.PlanSHA256(), p1.PlanSHA256(), p2.PlanSHA256())
	if !bytes.Equal(p1.JSON(), p2.JSON()) {
		testutil.AssertPlanJSONEqual(t, "dest_package", p1.JSON(), p2.JSON())
		log.Fail("package_bytes", "dest dual pipeline diverged")
	}
	// CLI plan sha matches package under same absolute dest + SpecPath.
	pkg, _ := dualPipelinePlansWithDest(t, specPath, plan.VerifyDefault, dest)
	// dual returns pair; rebuild with SpecPath = abs for CLI match.
	pCLI := packagePlanWithOpts(t, specPath, plan.VerifyDefault, dest, specPath)
	log.Assert("cli_matches_package_dest", planSHA == pCLI.PlanSHA256(), pCLI.PlanSHA256(), planSHA)
	log.Step("plan_sha256", testutil.OutcomeOK, planSHA)
	_ = pkg
	log.PhaseEnd("dest_override_equality", testutil.OutcomeOK)
}

// TestPlanGenerateEqualityCount2Stability locks -count=2-style double-run
// stability for the property suite entry fixture.
func TestPlanGenerateEqualityCount2Stability(t *testing.T) {
	log := testutil.New(t)
	log.Phase("count2")
	opts := planPackageOpts()
	opts.GenerateStages = fakeHappyStages()
	specPath := examplesPath(t, "minimal-cli.toml")

	var planSHAs, genSHAs []string
	var planBodies []string
	for i := 0; i < 2; i++ {
		pr := runCLIOpts(t, context.Background(), nil, opts,
			"plan", "--spec", specPath, "--output", "json")
		gr := runCLIOpts(t, context.Background(), nil, opts,
			"generate", "--spec", specPath, "--output", "json")
		log.Assert("plan_0_"+itoa(i), pr.Code == 0, 0, pr.Code)
		log.Assert("gen_0_"+itoa(i), gr.Code == 0, 0, gr.Code)
		ps := extractSHA(t, pr)
		gs := extractSHA(t, gr)
		planSHAs = append(planSHAs, ps)
		genSHAs = append(genSHAs, gs)
		planBodies = append(planBodies, pr.Stdout)
		log.Step("run_"+itoa(i), testutil.OutcomeOK, "plan="+ps+" gen="+gs)
		log.Assert("pair_"+itoa(i), ps == gs, ps, gs)
	}
	log.Assert("plan_stable", planSHAs[0] == planSHAs[1], planSHAs[0], planSHAs[1])
	log.Assert("gen_stable", genSHAs[0] == genSHAs[1], genSHAs[0], genSHAs[1])
	log.Assert("stdout_byte_stable", planBodies[0] == planBodies[1], true, planBodies[0] == planBodies[1])
	log.PhaseEnd("count2", testutil.OutcomeOK)
}

// dualPipelinePlans builds the same plan twice through plan.Pipeline —
// modeling plan-command construction vs generate-command construction
// (both share plan.Pipeline; double-build proves path identity + determinism).
// When specPathForPlan is non-empty it is used as PipelineOptions.SpecPath
// (CLI path source uses the authored --spec string).
func dualPipelinePlans(t *testing.T, absSpec string, verify plan.VerifyMode, specPathForPlan string) (planSide, genSide *plan.Plan) {
	t.Helper()
	sp := specPathForPlan
	if sp == "" {
		sp = filepath.Base(absSpec)
	}
	p1 := packagePlanWithOpts(t, absSpec, verify, "", sp)
	p2 := packagePlanWithOpts(t, absSpec, verify, "", sp)
	return p1, p2
}

func dualPipelinePlansWithDest(t *testing.T, absSpec string, verify plan.VerifyMode, destOverride string) (*plan.Plan, *plan.Plan) {
	t.Helper()
	sp := filepath.Base(absSpec)
	p1 := packagePlanWithOpts(t, absSpec, verify, destOverride, sp)
	p2 := packagePlanWithOpts(t, absSpec, verify, destOverride, sp)
	return p1, p2
}

func packagePlanWithOpts(t *testing.T, absSpec string, verify plan.VerifyMode, destOverride, specPath string) *plan.Plan {
	t.Helper()
	data, err := os.ReadFile(absSpec)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	raw, err := spec.Decode(filepath.Base(absSpec), data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	authored := destOverride
	if authored == "" {
		authored = vs.Destination()
	}
	dest, err := fixedObserve(authored)
	if err != nil {
		t.Fatal(err)
	}
	host := fixedPipelineHost()
	p, err := plan.Pipeline(vs, cat, plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  "testcommit000000000000000000000000000001",
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       specPath,
		Destination:    dest,
		Verify:         verify,
		GoBinary:       host.GoBinary,
		GitBinary:      host.GitBinary,
		GitTemplateDir: host.GitTemplateDir,
		Host:           host.Host,
	})
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	return p
}

func extractSHA(t *testing.T, res runResult) string {
	t.Helper()
	if res.Code != 0 {
		t.Fatalf("command failed code=%d stderr=%s stdout=%s", res.Code, res.Stderr, truncate(res.Stdout, 300))
	}
	env := parseEnvelope(t, res.Stdout)
	result, _ := env["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing result: %s", truncate(res.Stdout, 300))
	}
	sha, _ := result["plan_sha256"].(string)
	if len(sha) != 64 {
		t.Fatalf("bad plan_sha256 %q", sha)
	}
	return sha
}

// Ensure Options still compiles with GenerateStages field used above.
var _ = cli.Options{}
