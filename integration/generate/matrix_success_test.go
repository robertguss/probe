package generatee2e_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	generatee2e "github.com/robertguss/go-foundry-cli/integration/generate"
	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// TestMatrixSuccessVerifyGit covers success cells:
//
//	archetype=cli × verify={default,strict} × git={true,false}
//
// Asserts commit, plan_sha256, process-tree ids, digests, go test, REQ-133 scan.
func TestMatrixSuccessVerifyGit(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("matrix_success")

	type cell struct {
		name    string
		verify  string
		gitInit bool
		spec    func(t *testing.T, work string) (specPath, destBase string)
	}
	cells := []cell{
		{
			name: "default_git_true", verify: "default", gitInit: true,
			spec: func(t *testing.T, _ string) (string, string) {
				return examplesSpec(t, "minimal-cli.toml"), "minimal-cli"
			},
		},
		{
			name: "default_git_false", verify: "default", gitInit: false,
			spec: func(t *testing.T, work string) (string, string) {
				return writeGitFalseSpec(t, work, "nogit-cli"), "nogit-cli"
			},
		},
		{
			name: "strict_git_true", verify: "strict", gitInit: true,
			spec: func(t *testing.T, _ string) (string, string) {
				return examplesSpec(t, "minimal-cli.toml"), "minimal-cli"
			},
		},
		{
			name: "strict_git_false", verify: "strict", gitInit: false,
			spec: func(t *testing.T, work string) (string, string) {
				return writeGitFalseSpec(t, work, "nogit-cli"), "nogit-cli"
			},
		},
	}

	for _, c := range cells {
		c := c
		t.Run(c.name, func(t *testing.T) {
			tl := testutil.New(t)
			tl.Phase(c.name)
			tl.Inputs(map[string]string{
				"archetype": "cli",
				"verify":    c.verify,
				"git_init":  boolStr(c.gitInit),
			})

			parent := privateParent(t)
			work := t.TempDir()
			specPath, destBase := c.spec(t, work)
			dest := filepath.Join(parent, destBase)
			tl.NotePath(dest)
			tl.Fixture("spec", filepath.Base(specPath))

			ids, planSHA := planExternalStepIDs(t, tl, specPath, dest, c.verify)
			want := wantToolIDs(c.verify, c.gitInit)
			tl.Assert("process_tree_ids", generate.StepIDsMatchPlan(want, ids),
				strings.Join(want, ","), strings.Join(ids, ","))
			tl.Step("plan_sha256", testutil.OutcomeOK, planSHA)
			tl.NoteID(planSHA)

			args := []string{"generate", "--spec", specPath, "--dest", dest, "--output", "json"}
			if c.verify == "strict" {
				args = append(args, "--verify", "strict")
			}
			res := runCLI(t, context.Background(), cli.Options{}, nil, args...)
			logProc(tl, "generate_"+c.name, res, 0)
			if res.Code != 0 {
				dumpFailure(t, tl, failureFromProc(c.name, "commit", "generate_failed", res, planSHA, dest))
				tl.PhaseEnd(c.name, testutil.OutcomeFail)
				return
			}
			env := mustEnvelope(t, tl, res.Stdout)
			tl.Assert("ok", env["ok"] == true, true, env["ok"])
			gotSHA := planSHAFromEnvelope(env)
			tl.Assert("plan_sha256_eq", gotSHA == planSHA, planSHA, gotSHA)
			tl.Assert("commit_outcome", commitOutcomeFromEnvelope(env) == "committed",
				"committed", commitOutcomeFromEnvelope(env))

			if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
				tl.Fail("go.mod", err.Error())
			}
			_, gitErr := os.Stat(filepath.Join(dest, ".git"))
			if c.gitInit {
				tl.Assert("git_present", gitErr == nil, true, gitErr == nil)
			} else {
				tl.Assert("git_absent", os.IsNotExist(gitErr), true, gitErr)
			}

			_, agg, paths := nonGitTreeDigest(t, dest)
			tl.Step("tree_digest", testutil.OutcomeOK, agg)
			tl.Step("path_count", testutil.OutcomeOK, fmt.Sprintf("%d", len(paths)))
			scanContentREQ133(t, tl, dest)
			goTestGenerated(t, tl, dest)

			result, _ := env["result"].(map[string]any)
			nd, _ := result["network_disclosure"].([]any)
			tl.Assert("network_tidy", len(nd) >= 1, true, len(nd))
			if c.verify == "strict" {
				joined := ""
				for _, x := range nd {
					if s, ok := x.(string); ok {
						joined += s + ","
					}
				}
				tl.Assert("network_govuln", strings.Contains(joined, "go-govulncheck"), true, joined)
			}

			tl.PhaseEnd(c.name, testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("matrix_success", testutil.OutcomeOK)
}

// TestMatrixSmokeCLI is the dogfood-adjacent fixture cell (soft-related to vu8).
func TestMatrixSmokeCLI(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("smoke_cli")
	parent := privateParent(t)
	dest := filepath.Join(parent, "foundry-smoke-cli")
	spec := smokeSpec(t)
	log.Fixture("spec", "foundry-smoke-cli")

	ids, planSHA := planExternalStepIDs(t, log, spec, dest, "default")
	// smoke-cli has no [git] table → product default init=true.
	want := wantToolIDs("default", true)
	if !generate.StepIDsMatchPlan(want, ids) {
		wantFalse := wantToolIDs("default", false)
		log.Assert("process_tree_ids", generate.StepIDsMatchPlan(wantFalse, ids),
			strings.Join(want, ","), strings.Join(ids, ","))
	} else {
		log.Assert("process_tree_ids", true, strings.Join(want, ","), strings.Join(ids, ","))
	}
	log.Step("plan_sha256", testutil.OutcomeOK, planSHA)

	res := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest, "--output", "json")
	logProc(log, "generate_smoke", res, 0)
	if res.Code != 0 {
		dumpFailure(t, log, failureFromProc("smoke_cli", "commit", "generate_failed", res, planSHA, dest))
		return
	}
	env := mustEnvelope(t, log, res.Stdout)
	log.Assert("plan_eq", planSHAFromEnvelope(env) == planSHA, planSHA, planSHAFromEnvelope(env))
	goTestGenerated(t, log, dest)
	scanContentREQ133(t, log, dest)
	log.PhaseEnd("smoke_cli", testutil.OutcomeOK)
}

// TestMatrixDoubleGeneration proves two successive generates → equal digests + plan_sha256 (REQ-010).
func TestMatrixDoubleGeneration(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("double_generation")

	work := t.TempDir()
	spec := writeGitFalseSpec(t, work, "dbl-cli")
	parent := privateParent(t)
	dest := filepath.Join(parent, "dbl-cli")

	runOne := func(label string) (sha, agg string) {
		res := runCLI(t, context.Background(), cli.Options{}, nil,
			"generate", "--spec", spec, "--dest", dest, "--output", "json")
		logProc(log, "generate_"+label, res, 0)
		if res.Code != 0 {
			dumpFailure(t, log, failureFromProc("double_gen_"+label, "commit", "generate_failed", res, "", dest))
			log.Fail("generate_"+label, capBody(res.Stderr+res.Stdout, 400))
		}
		env := mustEnvelope(t, log, res.Stdout)
		sha = planSHAFromEnvelope(env)
		_, agg, _ = nonGitTreeDigest(t, dest)
		log.Step(label+"_plan_sha256", testutil.OutcomeOK, sha)
		log.Step(label+"_tree_digest", testutil.OutcomeOK, agg)
		return sha, agg
	}

	sha1, agg1 := runOne("a")
	if err := os.RemoveAll(dest); err != nil {
		log.Fail("remove", err.Error())
	}
	sha2, agg2 := runOne("b")

	log.Assert("plan_sha256_equal", sha1 == sha2, sha1, sha2)
	log.Assert("tree_digest_equal", agg1 == agg2, agg1, agg2)
	log.PhaseEnd("double_generation", testutil.OutcomeOK)
}

// TestMatrixProcessTreeFakeRunner locks FakePlanRunner observations to plan external_steps.
func TestMatrixProcessTreeFakeRunner(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("process_tree_fake")

	for _, tc := range []struct {
		name    string
		verify  plan.VerifyMode
		gitInit bool
	}{
		{"default_no_git", plan.VerifyDefault, false},
		{"default_git", plan.VerifyDefault, true},
		{"strict_no_git", plan.VerifyStrict, false},
		{"strict_git", plan.VerifyStrict, true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tl := testutil.New(t)
			ids := generate.PlannedToolStepIDs(tc.verify, tc.gitInit)
			steps := make([]plan.ExternalStep, len(ids))
			for i, id := range ids {
				steps[i] = plan.ExternalStep{
					ID:     id,
					Binary: "go",
					Argv:   []string{"go", "version"},
					Env:    map[string]string{"PATH": "/usr/bin"},
				}
				if id == generate.PlanStepGitInit {
					steps[i].Binary = "git"
					steps[i].Argv = []string{"git", "init"}
				}
			}
			fake := &toolrun.FakePlanRunner{}
			obs := fake.RunPlan(steps)
			diffs := toolrun.CompareProcessTree(steps, obs)
			tl.Assert("no_diffs", len(diffs) == 0, 0, len(diffs))
			fake2 := &toolrun.FakePlanRunner{InjectExtra: []toolrun.ObservedStep{{
				ID: "extra-shell", Binary: "sh", Argv: []string{"sh", "-c", "true"}, Shell: true,
			}}}
			obs2 := fake2.RunPlan(steps)
			diffs2 := toolrun.CompareProcessTree(steps, obs2)
			tl.Assert("extra_detected", len(diffs2) > 0, true, len(diffs2))
			tl.Step("planned_ids", testutil.OutcomeOK, strings.Join(ids, ","))
		})
	}
	log.PhaseEnd("process_tree_fake", testutil.OutcomeOK)
}

// TestMatrixPlanGenerateEqualitySmoke re-checks plan_sha256 equality (full property: j8h.4).
func TestMatrixPlanGenerateEqualitySmoke(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("plan_generate_eq")
	parent := privateParent(t)
	dest := filepath.Join(parent, "minimal-cli")
	spec := examplesSpec(t, "minimal-cli.toml")

	_, planSHA := planExternalStepIDs(t, log, spec, dest, "default")
	res := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest, "--output", "json")
	logProc(log, "generate", res, 0)
	env := mustEnvelope(t, log, res.Stdout)
	log.Assert("sha_eq", planSHAFromEnvelope(env) == planSHA, planSHA, planSHAFromEnvelope(env))
	log.PhaseEnd("plan_generate_eq", testutil.OutcomeOK)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func failureFromProc(caseName, stage, cause string, res proc, planSHA, dest string) generatee2e.FailureArtifact {
	env, _ := tryEnvelope(res.Stdout)
	a := generatee2e.FailureArtifact{
		Case:         caseName,
		Stage:        stage,
		Cause:        cause,
		PlanSHA256:   planSHA,
		CommitResult: commitOutcomeFromEnvelope(env),
		Exit:         res.Code,
		Destination:  dest,
		StagePath:    stagePathFromStreams(res.Stdout, res.Stderr),
		Detail:       capBody(res.Stderr+res.Stdout, 400),
		Timeline:     progressNamesFromStdout(res.Stdout),
	}
	if id := errorIDFromEnvelope(env); id != "" {
		a.Cause = id
	}
	if a.CommitResult == "" {
		a.CommitResult = exitClass(res.Code)
	}
	return a
}

func tryEnvelope(stdout string) (map[string]any, error) {
	var env map[string]any
	err := jsonUnmarshal(stdout, &env)
	return env, err
}
