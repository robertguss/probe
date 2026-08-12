package fixtures_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestPlanGeneratePackageByteEquality is the cross-package Section 13.3 property:
// pure plan.Pipeline construction (shared by plan and generate) is byte-stable
// for smoke-cli + write-free plan goldens under default and strict verify.
//
// Complements internal/cli TestPlanGenerateByteEqualityProperty (CLI path +
// plan_sha256 vs generate success field) and cmd/foundry e2e testscript.
func TestPlanGeneratePackageByteEquality(t *testing.T) {
	log := testutil.New(t)
	log.Phase("package_plan_generate_equality")

	type row struct {
		name   string
		load   func(t *testing.T) (*spec.ValidatedSpecification, plan.PipelineOptions)
		golden string // optional plan golden under testdata/ (smoke only)
		verify plan.VerifyMode
	}

	cases := []row{
		{
			name:   "smoke_cli_default",
			verify: plan.VerifyDefault,
			golden: "foundry_smoke_cli_plan",
			load: func(t *testing.T) (*spec.ValidatedSpecification, plan.PipelineOptions) {
				vs := mustValidateSmoke(t)
				return vs, smokePipelineOpts(t)
			},
		},
		{
			name:   "smoke_cli_strict",
			verify: plan.VerifyStrict,
			load: func(t *testing.T) (*spec.ValidatedSpecification, plan.PipelineOptions) {
				vs := mustValidateSmoke(t)
				opts := smokePipelineOpts(t)
				opts.Verify = plan.VerifyStrict
				return vs, opts
			},
		},
		{
			name:   "mvp_minimal_cli_default",
			verify: plan.VerifyDefault,
			load: func(t *testing.T) (*spec.ValidatedSpecification, plan.PipelineOptions) {
				return loadExamplePipeline(t, "minimal-cli.toml", plan.VerifyDefault)
			},
		},
		{
			name:   "mvp_minimal_cli_strict",
			verify: plan.VerifyStrict,
			load: func(t *testing.T) (*spec.ValidatedSpecification, plan.PipelineOptions) {
				return loadExamplePipeline(t, "minimal-cli.toml", plan.VerifyStrict)
			},
		},
		{
			name:   "mvp_minimal_tui_default",
			verify: plan.VerifyDefault,
			load: func(t *testing.T) (*spec.ValidatedSpecification, plan.PipelineOptions) {
				return loadExamplePipeline(t, "minimal-tui.toml", plan.VerifyDefault)
			},
		},
		{
			name:   "appendix_b_private_cli_default",
			verify: plan.VerifyDefault,
			load: func(t *testing.T) (*spec.ValidatedSpecification, plan.PipelineOptions) {
				return loadExamplePipeline(t, "appendix-b-private-cli.toml", plan.VerifyDefault)
			},
		},
		{
			name:   "appendix_b_private_tui_default",
			verify: plan.VerifyDefault,
			load: func(t *testing.T) (*spec.ValidatedSpecification, plan.PipelineOptions) {
				return loadExamplePipeline(t, "appendix-b-private-tui.toml", plan.VerifyDefault)
			},
		},
	}

	cat := mustLoadCatalog(t)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sub := testutil.New(t)
			sub.Phase("arrange")
			sub.Inputs(map[string]string{
				"fixture": tc.name,
				"verify":  string(tc.verify),
			})
			sub.PhaseEnd("arrange", testutil.OutcomeOK)

			sub.Phase("act")
			vs, opts := tc.load(t)
			// plan path construction
			pPlan, err := plan.Pipeline(vs, cat, opts)
			if err != nil {
				sub.Fail("plan_pipeline", err.Error())
			}
			// generate path construction (identical opts — shared pure entry)
			pGen, err := plan.Pipeline(vs, cat, opts)
			if err != nil {
				sub.Fail("gen_pipeline", err.Error())
			}
			sub.Step("file_count", testutil.OutcomeOK, "n="+itoa(pPlan.FileCount()))
			sub.Step("external_step_count", testutil.OutcomeOK, "n="+itoa(pPlan.ExternalStepCount()))
			sub.Step("plan_sha256", testutil.OutcomeOK, pPlan.PlanSHA256())
			sub.Step("gen_sha256", testutil.OutcomeOK, pGen.PlanSHA256())
			sub.PhaseEnd("act", testutil.OutcomeOK)

			sub.Phase("assert")
			sub.Assert("sha_equal", pPlan.PlanSHA256() == pGen.PlanSHA256(),
				pPlan.PlanSHA256(), pGen.PlanSHA256())
			if !bytes.Equal(pPlan.JSON(), pGen.JSON()) {
				testutil.AssertPlanJSONEqual(t, tc.name, pPlan.JSON(), pGen.JSON())
				sub.Fail("byte_equal", "plan vs generate construction diverged")
			}
			// Determinism: third construction still matches.
			p3, err := plan.Pipeline(vs, cat, opts)
			if err != nil {
				sub.Fail("third_pipeline", err.Error())
			}
			sub.Assert("count2_sha", pPlan.PlanSHA256() == p3.PlanSHA256(),
				pPlan.PlanSHA256(), p3.PlanSHA256())
			if !bytes.Equal(pPlan.JSON(), p3.JSON()) {
				testutil.AssertPlanJSONEqual(t, tc.name+"/count2", pPlan.JSON(), p3.JSON())
				sub.Fail("count2_bytes", "third construction diverged")
			}

			if tc.golden != "" {
				golden := testutil.GoldenPath(filepath.Join("testdata"), tc.golden)
				testutil.CompareGolden(t, golden, pPlan.JSON())
				if t.Failed() {
					sub.Step("golden", testutil.OutcomeFail, tc.golden+".golden")
				} else {
					sub.Step("golden", testutil.OutcomeOK, tc.golden+".golden")
				}
			}

			// Verify-mode step list shape.
			steps := pPlan.ExternalSteps()
			ids := make([]string, 0, len(steps))
			for _, s := range steps {
				ids = append(ids, s.ID)
			}
			if tc.verify == plan.VerifyStrict {
				sub.Assert("has_staticcheck", containsID(ids, "go-staticcheck"), true, ids)
			} else {
				sub.Assert("no_staticcheck", !containsID(ids, "go-staticcheck"), false, ids)
			}
			sub.PhaseEnd("assert", testutil.OutcomeOK)
		})
	}

	// Cross-mode: default and strict diverge for smoke-cli.
	log.Phase("smoke_verify_modes")
	vs := mustValidateSmoke(t)
	optsDef := smokePipelineOpts(t)
	optsStrict := smokePipelineOpts(t)
	optsStrict.Verify = plan.VerifyStrict
	pDef, err := plan.Pipeline(vs, cat, optsDef)
	if err != nil {
		log.Fail("def", err.Error())
	}
	pStrict, err := plan.Pipeline(vs, cat, optsStrict)
	if err != nil {
		log.Fail("strict", err.Error())
	}
	log.Assert("modes_diverge", pDef.PlanSHA256() != pStrict.PlanSHA256(),
		true, pDef.PlanSHA256() == pStrict.PlanSHA256())
	log.Step("default_sha", testutil.OutcomeOK, pDef.PlanSHA256())
	log.Step("strict_sha", testutil.OutcomeOK, pStrict.PlanSHA256())
	log.PhaseEnd("smoke_verify_modes", testutil.OutcomeOK)

	log.PhaseEnd("package_plan_generate_equality", testutil.OutcomeOK)
}

func loadExamplePipeline(t *testing.T, exampleName string, verify plan.VerifyMode) (*spec.ValidatedSpecification, plan.PipelineOptions) {
	t.Helper()
	path := filepath.Join(repoRoot(t), "examples", exampleName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", exampleName, err)
	}
	raw, err := spec.Decode(exampleName, data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	dest := vs.Destination()
	p := dest
	if len(p) >= 2 && p[0] == '.' && p[1] == '/' {
		p = "/home/foundry/projects/" + p[2:]
	}
	parent, base := splitPath(p)
	opts := plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  fixedCommit,
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       exampleName,
		Destination: plan.DestinationInfo{
			Path:        p,
			Parent:      parent,
			Basename:    base,
			Observation: plan.ObservationAbsent,
		},
		Verify:         verify,
		GoBinary:       fixedGoBinary,
		GitBinary:      fixedGitBinary,
		GitTemplateDir: fixedGitTmpl,
		Host:           fixedHostEnv(),
	}
	return vs, opts
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
