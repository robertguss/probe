package verify_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/verify"
)

func TestSteps_DefaultVsStrict_Table(t *testing.T) {
	log := testutil.New(t)
	log.Phase("step_lists")

	def := verify.DefaultSteps()
	str := verify.StrictSteps()
	defIDs := stepIDs(def)
	strIDs := stepIDs(str)

	// Exact default order (execution order including tidy).
	wantDefault := []string{
		verify.StepGoModTidy,
		verify.CheckModuleMutation,
		verify.CheckGofmt,
		verify.CheckGoModVerify,
		verify.CheckGoTest,
		verify.CheckGoVet,
		verify.CheckFinalConformance,
	}
	log.Assert("default_len", len(defIDs) == len(wantDefault), len(wantDefault), len(defIDs))
	for i, id := range wantDefault {
		if i >= len(defIDs) {
			break
		}
		log.Assert("default_"+id, defIDs[i] == id, id, defIDs[i])
	}

	// Strict = default with staticcheck + govulncheck before final-conformance.
	wantStrict := []string{
		verify.StepGoModTidy,
		verify.CheckModuleMutation,
		verify.CheckGofmt,
		verify.CheckGoModVerify,
		verify.CheckGoTest,
		verify.CheckGoVet,
		verify.CheckGoStaticcheck,
		verify.CheckGoGovulncheck,
		verify.CheckFinalConformance,
	}
	log.Assert("strict_len", len(strIDs) == len(wantStrict), len(wantStrict), len(strIDs))
	for i, id := range wantStrict {
		if i >= len(strIDs) {
			break
		}
		log.Assert("strict_"+id, strIDs[i] == id, id, strIDs[i])
	}

	// CheckNames match plan.verification.checks (no tidy).
	wantChecksDef := []string{
		verify.CheckGofmt, verify.CheckModuleMutation, verify.CheckGoModVerify,
		verify.CheckGoTest, verify.CheckGoVet, verify.CheckFinalConformance,
	}
	gotChecks := verify.CheckNames(verify.ModeDefault)
	log.Assert("checks_default", strings.Join(gotChecks, ",") == strings.Join(wantChecksDef, ","),
		strings.Join(wantChecksDef, ","), strings.Join(gotChecks, ","))

	wantChecksStrict := []string{
		verify.CheckGofmt, verify.CheckModuleMutation, verify.CheckGoModVerify,
		verify.CheckGoTest, verify.CheckGoVet,
		verify.CheckGoStaticcheck, verify.CheckGoGovulncheck,
		verify.CheckFinalConformance,
	}
	gotStrictChecks := verify.CheckNames(verify.ModeStrict)
	log.Assert("checks_strict", strings.Join(gotStrictChecks, ",") == strings.Join(wantChecksStrict, ","),
		strings.Join(wantChecksStrict, ","), strings.Join(gotStrictChecks, ","))

	// External argv shapes (Appendix E).
	cases := map[string][]string{
		verify.StepGoModTidy:    {"go", "mod", "tidy"},
		verify.CheckGoModVerify: {"go", "mod", "verify"},
		verify.CheckGoTest: {
			"go", "test", "-count=1", "-buildvcs=false", "-mod=readonly", "./...",
		},
		verify.CheckGoVet: {
			"go", "vet", "-buildvcs=false", "-mod=readonly", "./...",
		},
		verify.CheckGoStaticcheck: {"go", "tool", "staticcheck", "./..."},
		verify.CheckGoGovulncheck: {"go", "tool", "govulncheck", "./..."},
	}
	for id, want := range cases {
		got := verify.ExternalArgv(id)
		log.Assert("argv_"+id, strings.Join(got, " ") == strings.Join(want, " "),
			strings.Join(want, " "), strings.Join(got, " "))
	}

	// After-tool conform flags on post-freeze externals only.
	for _, s := range str {
		if s.Kind != verify.KindExternal {
			continue
		}
		if s.ID == verify.StepGoModTidy {
			log.Assert("tidy_no_after_conform", !s.AfterToolConform, false, s.AfterToolConform)
			continue
		}
		log.Assert("after_conform_"+s.ID, s.AfterToolConform, true, s.AfterToolConform)
	}

	log.PhaseEnd("step_lists", testutil.OutcomeOK)
}

func TestSteps_RaceAbsent_StaticAndRuntime(t *testing.T) {
	log := testutil.New(t)
	log.Phase("race_absent")

	for _, mode := range []verify.Mode{verify.ModeDefault, verify.ModeStrict} {
		steps := verify.StepsFor(mode)
		if verify.ContainsRace(steps) {
			log.Fail("contains_race_"+string(mode), "generation gate must not include race")
		} else {
			log.Step("no_race_"+string(mode), testutil.OutcomeOK, "steps="+strings.Join(stepIDs(steps), ","))
		}
		// Static scan of argv text.
		for _, s := range steps {
			joined := strings.ToLower(strings.Join(s.Argv, " ") + " " + s.Description)
			if strings.Contains(joined, "-race") {
				log.Fail("argv_race_"+s.ID, joined)
			}
			if s.IsRace {
				log.Fail("is_race_flag_"+s.ID, "IsRace true")
			}
		}
	}

	// Synthetic list with race must be detected.
	bad := []verify.Step{
		{ID: "go-test-race", Argv: []string{"go", "test", "-race"}, IsRace: false},
	}
	log.Assert("detect_race_id", verify.ContainsRace(bad), true, verify.ContainsRace(bad))
	bad2 := []verify.Step{
		{ID: "x", Argv: []string{"go", "test", "-race", "./..."}},
	}
	log.Assert("detect_race_argv", verify.ContainsRace(bad2), true, verify.ContainsRace(bad2))

	// -count=1 present on go-test.
	log.Assert("count_one_default", verify.HasCountOne(verify.DefaultSteps()), true, false)
	log.Assert("count_one_strict", verify.HasCountOne(verify.StrictSteps()), true, false)

	log.PhaseEnd("race_absent", testutil.OutcomeOK)
}

func TestPlanExternalSteps_MatchesAppendixE(t *testing.T) {
	log := testutil.New(t)
	steps := verify.PlanExternalSteps(verify.ModeStrict)
	ids := make([]string, 0, len(steps))
	for _, s := range steps {
		ids = append(ids, s.ID)
		log.Assert("cwd_"+s.ID, s.Cwd == "stage-descriptor", "stage-descriptor", s.Cwd)
		if s.ID == verify.StepGoModTidy {
			log.Assert("tidy_mutates", len(s.Mutates) == 2, 2, len(s.Mutates))
		}
	}
	log.Step("external_ids", testutil.OutcomeOK, strings.Join(ids, ","))
	// No race, no git in verify plan helpers (git is separate).
	for _, id := range ids {
		if strings.Contains(id, "race") || id == "git-init" {
			log.Fail("unexpected_id", id)
		}
	}
}

func stepIDs(steps []verify.Step) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.ID
	}
	return out
}
