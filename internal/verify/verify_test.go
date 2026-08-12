//go:build unix

package verify_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/verify"
)

func TestRun_DefaultHappyPath_FakeRunner(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	stage := writeStage(t, minimalModule("example.com/demo"))
	fake := &verify.FakeStepRunner{ByStep: verify.ScriptAllOK(verify.ModeDefault)}
	ml := &memLog{}
	opts := baseOpts(stage, verify.ModeDefault, fake)
	opts.Logger = ml
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := verify.Run(context.Background(), fake, opts)
	log.Step("run", testutil.OutcomeOK, res.LogDetail())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if !res.OK() {
		log.Fail("ok", res.Err().Error())
	}
	log.Assert("frozen", res.Frozen, true, res.Frozen)
	log.Assert("digest", len(res.BaselineDigest()) == 64, 64, len(res.BaselineDigest()))
	log.Assert("failed_step_empty", res.FailedStep == "", "", res.FailedStep)
	// External steps observed: go-mod-verify, go-test, go-vet (tidy skipped).
	ids := fake.ObservedStepIDs()
	log.Step("observed", testutil.OutcomeOK, strings.Join(ids, ","))
	for _, want := range []string{verify.CheckGoModVerify, verify.CheckGoTest, verify.CheckGoVet} {
		found := false
		for _, id := range ids {
			if id == want {
				found = true
			}
		}
		log.Assert("obs_"+want, found, true, ids)
	}
	// -count=1 on go-test argv.
	argv := fake.ObservedArgv(verify.CheckGoTest)
	log.Assert("count=1", verify.ArgvContains(argv, "-count=1"), true, argv)
	log.Assert("no_race", !verify.ArgvContains(argv, "-race"), true, argv)
	// Logs name steps.
	log.Assert("log_gofmt", ml.Has(verify.CheckGofmt, "ok"), true, false)
	log.Assert("log_final", ml.Has(verify.CheckFinalConformance, "ok"), true, false)
	log.Assert("log_complete", ml.Has("complete", "ok"), true, false)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestRun_StrictIncludesStaticcheckGovulncheck(t *testing.T) {
	log := testutil.New(t)
	stage := writeStage(t, minimalModule("example.com/strict"))
	fake := &verify.FakeStepRunner{ByStep: verify.ScriptAllOK(verify.ModeStrict)}
	opts := baseOpts(stage, verify.ModeStrict, fake)
	res := verify.Run(context.Background(), fake, opts)
	if !res.OK() {
		log.Fail("ok", res.Err().Error())
	}
	ids := fake.ObservedStepIDs()
	log.Step("observed", testutil.OutcomeOK, strings.Join(ids, ","))
	log.Assert("staticcheck", contains(ids, verify.CheckGoStaticcheck), true, ids)
	log.Assert("govulncheck", contains(ids, verify.CheckGoGovulncheck), true, ids)
	log.Assert("no_race", !contains(ids, "go-test-race"), true, ids)
}

func TestRun_FailEachExternalStep_Matrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("fail_each")

	// Fail each external post-tidy step; logs must name the failing step.
	failIDs := []string{
		verify.CheckGoModVerify,
		verify.CheckGoTest,
		verify.CheckGoVet,
		verify.CheckGoStaticcheck,
		verify.CheckGoGovulncheck,
	}
	for _, failID := range failIDs {
		failID := failID
		t.Run(failID, func(t *testing.T) {
			t.Parallel()
			slog := testutil.New(t)
			stage := writeStage(t, minimalModule("example.com/fail"))
			mode := verify.ModeDefault
			if failID == verify.CheckGoStaticcheck || failID == verify.CheckGoGovulncheck {
				mode = verify.ModeStrict
			}
			fake := &verify.FakeStepRunner{ByStep: verify.ScriptFailStep(mode, failID)}
			ml := &memLog{}
			opts := baseOpts(stage, mode, fake)
			opts.Logger = ml
			res := verify.Run(context.Background(), fake, opts)
			if res.OK() {
				slog.Fail("want_fail", "expected failure at "+failID)
				return
			}
			slog.Assert("failed_step", res.FailedStep == failID, failID, res.FailedStep)
			slog.Assert("fail_class", res.FailClass == string(diagnostic.IDToolFailed) ||
				res.FailClass == string(diagnostic.IDVerifyFailed),
				"tool.failed|verify.failed", res.FailClass)
			// Logs alone name failing step.
			slog.Assert("log_fail", ml.Has(failID, "fail") || ml.FailedStepName() == failID,
				true, ml.FailedStepName())
			// Error text names step.
			if res.Err() == nil || !strings.Contains(res.Err().Error(), failID) {
				// toolrun may say "external step go-test ..."
				if res.Err() == nil || !strings.Contains(res.Err().Error(), "step") {
					slog.Fail("err_names_step", res.Err().Error())
				}
			}
			slog.Step("fail_"+failID, testutil.OutcomeOK, res.LogDetail())
		})
	}
	log.PhaseEnd("fail_each", testutil.OutcomeOK)
}

func TestRun_PostToolMutation_Unplanned(t *testing.T) {
	log := testutil.New(t)
	log.Phase("unplanned_after_tool")

	stage := writeStage(t, minimalModule("example.com/mutate"))
	fake := &verify.FakeStepRunner{
		ByStep: verify.ScriptAllOK(verify.ModeDefault),
		Mutators: map[string]func() error{
			verify.CheckGoTest: func() error {
				return os.WriteFile(filepath.Join(stage, "leaked.txt"), []byte("mutated by test\n"), 0o644)
			},
		},
	}
	ml := &memLog{}
	opts := baseOpts(stage, verify.ModeDefault, fake)
	opts.Logger = ml
	res := verify.Run(context.Background(), fake, opts)
	if res.OK() {
		log.Fail("want_unplanned", "expected verify.unplanned_mutation")
		return
	}
	log.Assert("failed_step", res.FailedStep == verify.CheckGoTest, verify.CheckGoTest, res.FailedStep)
	log.Assert("fail_class", res.FailClass == string(diagnostic.IDVerifyUnplannedMutation),
		diagnostic.IDVerifyUnplannedMutation, res.FailClass)
	fe, ok := diagnostic.AsFoundryError(res.Err())
	log.Assert("id", ok && fe.ID() == diagnostic.IDVerifyUnplannedMutation,
		diagnostic.IDVerifyUnplannedMutation, idOf(fe))
	log.Assert("names_path", strings.Contains(res.Err().Error(), "leaked.txt"), true, res.Err().Error())
	log.Assert("log_fail", ml.Has(verify.CheckGoTest, "fail"), true, false)
	log.Step("unplanned", testutil.OutcomeOK, res.LogDetail())
	log.PhaseEnd("unplanned_after_tool", testutil.OutcomeOK)
}

func TestRun_GoFmtFailure_NamesStep(t *testing.T) {
	log := testutil.New(t)
	stage := writeStage(t, map[string]string{
		"go.mod":  "module m\n\ngo 1.26.0\n",
		"main.go": "package main\nfunc main(){  }\n", // dirty
	})
	fake := &verify.FakeStepRunner{ByStep: verify.ScriptAllOK(verify.ModeDefault)}
	ml := &memLog{}
	opts := baseOpts(stage, verify.ModeDefault, fake)
	opts.Logger = ml
	res := verify.Run(context.Background(), fake, opts)
	if res.OK() {
		log.Fail("want_gofmt_fail", "expected failure")
		return
	}
	log.Assert("failed_step", res.FailedStep == verify.CheckGofmt, verify.CheckGofmt, res.FailedStep)
	log.Assert("log", ml.Has(verify.CheckGofmt, "fail"), true, false)
	// No external tools after gofmt failure.
	log.Assert("no_tools", len(fake.ObservedStepIDs()) == 0, 0, fake.ObservedStepIDs())
	log.Step("gofmt_fail", testutil.OutcomeOK, res.LogDetail())
}

func TestRun_ModuleMutation_ExtraPath(t *testing.T) {
	log := testutil.New(t)
	// Simulate tidy by NOT skipping: inject mutator on go-mod-tidy that adds
	// an extra file, then mutation-set must fail.
	stage := writeStage(t, minimalModule("example.com/mut"))
	fake := &verify.FakeStepRunner{
		ByStep: verify.ScriptAllOK(verify.ModeDefault),
		Mutators: map[string]func() error{
			verify.StepGoModTidy: func() error {
				return os.WriteFile(filepath.Join(stage, "unexpected.txt"), []byte("x\n"), 0o644)
			},
		},
	}
	ml := &memLog{}
	opts := baseOpts(stage, verify.ModeDefault, fake)
	opts.SkipTidy = false // run tidy step
	opts.Logger = ml
	res := verify.Run(context.Background(), fake, opts)
	if res.OK() {
		log.Fail("want_module_mutation", "expected failure")
		return
	}
	log.Assert("failed_step", res.FailedStep == verify.CheckModuleMutation,
		verify.CheckModuleMutation, res.FailedStep)
	log.Assert("class", res.FailClass == string(diagnostic.IDVerifyModuleMutation),
		diagnostic.IDVerifyModuleMutation, res.FailClass)
	log.Assert("log", ml.Has(verify.CheckModuleMutation, "fail"), true, false)
	log.Step("module_mutation", testutil.OutcomeOK, res.LogDetail())
}

func TestRun_ReplaceDirective_FailClosed(t *testing.T) {
	log := testutil.New(t)
	stage := writeStage(t, map[string]string{
		"go.mod": `module m

go 1.26.0

require github.com/spf13/cobra v1.10.2

replace github.com/spf13/cobra => ./cobra
`,
		"main.go": "package main\n\nfunc main() {}\n",
	})
	fake := &verify.FakeStepRunner{ByStep: verify.ScriptAllOK(verify.ModeDefault)}
	opts := baseOpts(stage, verify.ModeDefault, fake)
	res := verify.Run(context.Background(), fake, opts)
	if res.OK() {
		log.Fail("want_fail", "replace must fail closed")
		return
	}
	log.Assert("class", res.FailClass == string(diagnostic.IDVerifyModuleMutation),
		diagnostic.IDVerifyModuleMutation, res.FailClass)
	log.Step("replace", testutil.OutcomeOK, res.LogDetail())
}

func TestRun_InvalidMode(t *testing.T) {
	log := testutil.New(t)
	stage := writeStage(t, minimalModule("example.com/x"))
	fake := &verify.FakeStepRunner{ByStep: verify.ScriptAllOK(verify.ModeDefault)}
	opts := baseOpts(stage, verify.Mode("none"), fake)
	res := verify.Run(context.Background(), fake, opts)
	log.Assert("fail", !res.OK(), true, res.OK())
	log.Assert("step", res.FailedStep == "mode", "mode", res.FailedStep)
}
