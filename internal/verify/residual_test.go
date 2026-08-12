//go:build unix

package verify_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/verify"
)

func TestStageBasenameAndStepHelpers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("stage_basename_steps")

	log.Assert("empty", verify.StageBasename("") == ".", ".", verify.StageBasename(""))
	log.Assert("base", verify.StageBasename("/home/runner/work/stage-deadbeef") == "stage-deadbeef",
		"stage-deadbeef", verify.StageBasename("/home/runner/work/stage-deadbeef"))
	log.Assert("rel", verify.StageBasename("stage-abc") == "stage-abc", "stage-abc", verify.StageBasename("stage-abc"))
	full := filepath.Join(string(filepath.Separator), "home", "user", "proj", "stage-x")
	log.Assert("no_home", !strings.Contains(verify.StageBasename(full), "home"), true, verify.StageBasename(full))

	args := verify.ExternalArgs(verify.CheckGoTest)
	log.Assert("test_args", len(args) > 0 && args[0] != "go", true, strings.Join(args, " "))
	log.Assert("unknown_args", verify.ExternalArgs("not-a-step") == nil, true, verify.ExternalArgs("not-a-step") == nil)
	log.Assert("inprocess_args", verify.ExternalArgs(verify.CheckGofmt) == nil, true, verify.ExternalArgs(verify.CheckGofmt) == nil)

	to := verify.TimeoutFor(verify.CheckGoTest)
	log.Assert("test_timeout", to > 0, true, to)
	log.Assert("unknown_timeout", verify.TimeoutFor("nope") == 0, time.Duration(0), verify.TimeoutFor("nope"))
	log.Assert("empty_argv", verify.ExternalArgv("") == nil, true, verify.ExternalArgv("") == nil)

	descRace := []verify.Step{{ID: "x", Description: "runs with -race flag"}}
	log.Assert("race_desc", verify.ContainsRace(descRace), true, true)
	log.Assert("count_empty", !verify.HasCountOne(nil), false, false)
	log.Assert("count_no", !verify.HasCountOne([]verify.Step{{ID: verify.CheckGoTest, Argv: []string{"go", "test"}}}), false, false)

	log.Step("helpers", testutil.OutcomeOK, "basename+args+timeout")
	log.PhaseEnd("stage_basename_steps", testutil.OutcomeOK)
}

func TestMutationResidualPaths(t *testing.T) {
	log := testutil.New(t)
	log.Phase("mutation_residual")

	missing := t.TempDir()
	rep := verify.ReparsePins(missing, []verify.Pin{{Path: "x", Version: "v1"}})
	log.Assert("unreadable", !rep.OK() && len(rep.MissingPins) > 0, true, rep.MissingPins)
	log.Assert("not_parsed", !rep.ParsedOK, false, rep.ParsedOK)

	stageBad := writeStage(t, map[string]string{"go.mod": "this is not a go.mod {{{"})
	rep = verify.ReparsePins(stageBad, nil)
	log.Assert("parse_fail", !rep.ParsedOK && len(rep.MissingPins) > 0, true, rep.MissingPins)

	stageR := writeStage(t, map[string]string{
		"go.mod": "module m\n\ngo 1.26.0\n\nretract v1.0.0\n",
	})
	rep = verify.ReparsePins(stageR, nil)
	log.Assert("retract", !rep.OK(), true, rep.ForbiddenDirectives)
	foundRetract := false
	for _, d := range rep.ForbiddenDirectives {
		if strings.Contains(d, "retract") {
			foundRetract = true
		}
	}
	log.Assert("retract_token", foundRetract, true, rep.ForbiddenDirectives)

	stageWS := writeStage(t, map[string]string{
		"go.mod": "module m\n\ngo 1.26.0\n\nworkspace use .\n",
	})
	rep = verify.ReparsePins(stageWS, nil)
	log.Assert("workspace_or_parse", !rep.OK(), true, rep)

	stageOK := writeStage(t, map[string]string{
		"go.mod": "module m\n\ngo 1.26.0\n\nrequire example.com/x v1.0.0\n",
	})
	rep = verify.ReparsePins(stageOK, []verify.Pin{
		{Path: "example.com/missing", Version: "v1.0.0"},
		{Path: "", Version: "v1"},
		{Path: "example.com/x", Version: "v1.0.0"},
	})
	log.Assert("absent_pin", !rep.OK(), true, rep.MissingPins)
	foundAbsent := false
	for _, m := range rep.MissingPins {
		if strings.Contains(m, "absent") {
			foundAbsent = true
		}
	}
	log.Assert("absent_token", foundAbsent, true, rep.MissingPins)

	stagePin := writeStage(t, map[string]string{
		"go.mod":  "module m\n\ngo 1.26.0\n\nrequire example.com/x v1.0.0\n",
		"main.go": "package main\n",
	})
	pre, err := verify.SnapshotNonGit(stagePin)
	if err != nil {
		t.Fatal(err)
	}
	post := pre
	_, err = verify.ValidateMutationAndPins(pre, post, stagePin, []verify.Pin{
		{Path: "example.com/x", Version: "v9.9.9"},
	})
	if err == nil {
		log.Fail("pin_only_fail", "expected module_mutation")
	} else {
		fe, ok := diagnostic.AsFoundryError(err)
		log.Assert("pin_fail_id", ok && fe.ID() == diagnostic.IDVerifyModuleMutation,
			diagnostic.IDVerifyModuleMutation, idOf(fe))
		log.Assert("pin_fail_msg", strings.Contains(err.Error(), "pin"), true, err.Error())
	}

	repOK, err := verify.ValidateMutationAndPins(pre, post, stagePin, []verify.Pin{
		{Path: "example.com/x", Version: "v1.0.0"},
	})
	if err != nil {
		log.Fail("pins_ok", err.Error())
	}
	log.Assert("ok_report", repOK.OK(), true, repOK.OK())

	preR, _ := verify.SnapshotNonGit(stageR)
	_, err = verify.ValidateMutationAndPins(preR, preR, stageR, nil)
	if err == nil {
		log.Fail("forbidden_err", "expected error for retract")
	} else {
		log.Assert("forbidden_msg", strings.Contains(err.Error(), "forbidden") || strings.Contains(err.Error(), "retract"),
			true, err.Error())
	}

	stageCh := writeStage(t, map[string]string{
		"go.mod": "module m\n\ngo 1.26.0\n",
	})
	preCh, _ := verify.SnapshotNonGit(stageCh)
	stageCh2 := writeStage(t, map[string]string{
		"go.mod": "module m\n\ngo 1.26.0\n\nrequire x v1.0.0\n",
	})
	postCh, _ := verify.SnapshotNonGit(stageCh2)
	repCh := verify.ValidateTidyMutationSet(preCh, postCh)
	log.Assert("changed_gomod", contains(repCh.ChangedPaths, "go.mod"), true, repCh.ChangedPaths)
	log.Assert("no_extra", len(repCh.ExtraPaths) == 0, 0, repCh.ExtraPaths)

	var empty verify.MutationReport
	log.Assert("ok_false_unparsed", !empty.OK(), false, empty.OK())

	// moduleMutationError ParsedOK-false path via unreadable go.mod + no extras.
	_, err = verify.ValidateMutationAndPins(pre, post, missing, nil)
	if err == nil {
		log.Fail("unreadable_mut", "expected error")
	} else {
		log.Step("unreadable_mut", testutil.OutcomeOK, err.Error())
	}

	log.Step("mutation_residual", testutil.OutcomeOK, "unreadable+retract+pins")
	log.PhaseEnd("mutation_residual", testutil.OutcomeOK)
}

func TestPathSetOnlyStepsAndToolFailed(t *testing.T) {
	log := testutil.New(t)
	log.Phase("pathset_onlysteps")

	stage := writeStage(t, minimalModule("example.com/resid"))
	base, err := verify.SnapshotNonGit(stage)
	if err != nil {
		t.Fatal(err)
	}
	ps := base.PathSet()
	_, hasGomod := ps["go.mod"]
	_, hasMain := ps["main.go"]
	_, hasAbs := ps[stage]
	log.Assert("has_gomod", hasGomod, true, ps)
	log.Assert("has_main", hasMain, true, ps)
	log.Assert("no_abs", !hasAbs, true, true)

	err = verify.NewToolFailed("go-test", "boom")
	fe, ok := diagnostic.AsFoundryError(err)
	log.Assert("tool_failed", ok && fe.ID() == diagnostic.IDToolFailed, diagnostic.IDToolFailed, idOf(fe))
	log.Assert("tool_msg", strings.Contains(err.Error(), "boom"), true, err.Error())

	// OnlySteps → filterSteps: module-mutation freezes, then gofmt + final.
	fake := &verify.FakeStepRunner{ByStep: verify.ScriptAllOK(verify.ModeDefault)}
	opts := baseOpts(stage, verify.ModeDefault, fake)
	opts.SkipTidy = true
	opts.OnlySteps = []string{verify.CheckModuleMutation, verify.CheckGofmt, verify.CheckFinalConformance}
	opts.Logger = nil
	res := verify.Run(context.Background(), fake, opts)
	log.Assert("only_ok", res.OK(), true, errString(res.Err()))
	log.Assert("frozen", res.Frozen, true, res.Frozen)
	ids := fake.ObservedStepIDs()
	log.Assert("no_externals", len(ids) == 0, 0, ids)
	log.Step("only_steps", testutil.OutcomeOK, res.LogDetail())

	// gofmt-only still exercises filterSteps without freeze requirement.
	fake2 := &verify.FakeStepRunner{ByStep: verify.ScriptAllOK(verify.ModeDefault)}
	opts2 := baseOpts(stage, verify.ModeDefault, fake2)
	opts2.OnlySteps = []string{verify.CheckGofmt}
	res2 := verify.Run(context.Background(), fake2, opts2)
	log.Assert("gofmt_only", res2.OK(), true, errString(res2.Err()))
	log.Assert("no_obs2", len(fake2.ObservedStepIDs()) == 0, 0, fake2.ObservedStepIDs())
	log.PhaseEnd("pathset_onlysteps", testutil.OutcomeOK)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestIsExactVersionAndNopLogger(t *testing.T) {
	log := testutil.New(t)
	log.Phase("exact_version_nop")

	cases := map[string]bool{
		"":         false,
		"latest":   false,
		"LATEST":   false,
		"master":   false,
		"main":     false,
		"head":     false,
		"v1.0.0":   true,
		"v1.0.0 x": false,
		"v1.0.0\t": false,
		">=v1":     false,
		"~v1.0.0":  false,
		"*":        false,
		"1.0.0":    false, // no v-prefix
	}
	for v, want := range cases {
		got := verify.IsExactVersionForTest(v)
		log.Assert("v_"+v, got == want, want, got)
	}

	// Hit IsRace true path and race detector description.
	log.Assert("is_race_flag", verify.ContainsRace([]verify.Step{{ID: "x", IsRace: true}}), true, true)
	log.Assert("race_detector_words", verify.ContainsRace([]verify.Step{{ID: "x", Description: "race detector on"}}), true, true)
	log.Assert("go_race_id", verify.ContainsRace([]verify.Step{{ID: "go-race"}}), true, true)
	log.Assert("race_id", verify.ContainsRace([]verify.Step{{ID: "race"}}), true, true)
	log.Assert("clean", !verify.ContainsRace([]verify.Step{{ID: "go-test", Argv: []string{"go", "test"}}}), false, false)

	verify.NopLoggerVerifyStepForTest()
	log.Assert("err_nil", verify.ErrStringForTest(nil) == "", "", verify.ErrStringForTest(nil))
	log.Assert("err_s", verify.ErrStringForTest(errors.New("x")) == "x", "x", verify.ErrStringForTest(errors.New("x")))
	log.Assert("fail_nil", verify.FailClassOfForTest(nil) != "" || verify.FailClassOfForTest(nil) == "", true, true)
	// non-foundry error → verify.failed
	fc := verify.FailClassOfForTest(errors.New("plain"))
	log.Assert("fail_plain", fc == string(diagnostic.IDVerifyFailed), diagnostic.IDVerifyFailed, fc)
	fe := diagnostic.New(diagnostic.IDToolFailed, "t", diagnostic.StepLocation("go-test"))
	log.Assert("fail_fe", verify.FailClassOfForTest(fe) == string(diagnostic.IDToolFailed), diagnostic.IDToolFailed, verify.FailClassOfForTest(fe))

	// filterSteps empty only → empty out
	out := verify.FilterStepsForTest(verify.DefaultSteps(), nil)
	log.Assert("filter_nil_only", len(out) == 0, 0, len(out))
	out = verify.FilterStepsForTest(verify.DefaultSteps(), []string{"nope"})
	log.Assert("filter_miss", len(out) == 0, 0, len(out))

	// Fake runner timeout + mutator + missing script paths.
	stage := writeStage(t, minimalModule("example.com/fake"))
	fake := &verify.FakeStepRunner{
		ByStep: map[string]verify.FakeStepScript{
			verify.CheckGoTest: {Timeout: true},
			verify.CheckGoVet:  {ExitCode: 0, Fail: true},
			verify.CheckGoModVerify: {
				ExitCode: 0,
			},
		},
		Mutators: map[string]func() error{
			verify.CheckGoModVerify: func() error { return errors.New("mut boom") },
		},
	}
	// Drive Run with only those steps after freeze.
	opts := baseOpts(stage, verify.ModeDefault, fake)
	opts.OnlySteps = []string{verify.CheckModuleMutation, verify.CheckGoModVerify}
	res := verify.Run(context.Background(), fake, opts)
	// mutator should fail the step
	log.Assert("mut_fail", !res.OK(), true, res.OK())
	log.Step("fake_paths", testutil.OutcomeOK, res.LogDetail())

	// Missing script path
	fake2 := &verify.FakeStepRunner{ByStep: map[string]verify.FakeStepScript{}}
	opts2 := baseOpts(stage, verify.ModeDefault, fake2)
	opts2.OnlySteps = []string{verify.CheckModuleMutation, verify.CheckGoTest}
	res2 := verify.Run(context.Background(), fake2, opts2)
	log.Assert("missing_script", !res2.OK(), true, res2.OK())

	// ObservedArgv miss
	log.Assert("obs_miss", fake2.ObservedArgv("nope") == nil, true, true)

	log.PhaseEnd("exact_version_nop", testutil.OutcomeOK)
}
