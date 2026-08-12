package verify_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/verify"
)

func TestParseMode_Admitted(t *testing.T) {
	log := testutil.New(t)
	log.Phase("parse_admitted")
	cases := []struct {
		in   string
		want verify.Mode
	}{
		{"", verify.ModeDefault},
		{"default", verify.ModeDefault},
		{"DEFAULT", verify.ModeDefault},
		{" strict ", verify.ModeStrict},
		{"strict", verify.ModeStrict},
	}
	for _, tc := range cases {
		got, err := verify.ParseMode(tc.in)
		if err != nil {
			log.Fail("parse_"+tc.in, err.Error())
			continue
		}
		log.Assert("mode_"+tc.in, got == tc.want, tc.want, got)
	}
	log.PhaseEnd("parse_admitted", testutil.OutcomeOK)
}

func TestParseMode_Forbidden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("parse_forbidden")
	for _, bad := range verify.ForbiddenModeTokens {
		_, err := verify.ParseMode(bad)
		if err == nil {
			log.Fail("accepted_"+bad, "expected error")
			continue
		}
		fe, ok := diagnostic.AsFoundryError(err)
		if !ok {
			log.Fail("type_"+bad, err.Error())
			continue
		}
		log.Assert("id_"+bad, fe.ID() == diagnostic.IDVerifyFailed,
			diagnostic.IDVerifyFailed, fe.ID())
		log.Step("reject_"+bad, testutil.OutcomeOK, fe.Error())
	}
	// Arbitrary garbage.
	_, err := verify.ParseMode("maybe")
	if err == nil {
		log.Fail("accepted_maybe", "expected error")
	}
	log.PhaseEnd("parse_forbidden", testutil.OutcomeOK)
}

func TestMode_PlanInterop(t *testing.T) {
	log := testutil.New(t)
	log.Assert("default_plan", verify.ModeDefault.PlanMode() == plan.VerifyDefault,
		plan.VerifyDefault, verify.ModeDefault.PlanMode())
	log.Assert("strict_plan", verify.ModeStrict.PlanMode() == plan.VerifyStrict,
		plan.VerifyStrict, verify.ModeStrict.PlanMode())
	log.Assert("from_default", verify.FromPlan(plan.VerifyDefault) == verify.ModeDefault,
		verify.ModeDefault, verify.FromPlan(plan.VerifyDefault))
	log.Assert("from_strict", verify.FromPlan(plan.VerifyStrict) == verify.ModeStrict,
		verify.ModeStrict, verify.FromPlan(plan.VerifyStrict))
	log.Assert("from_empty", verify.FromPlan("") == verify.ModeDefault,
		verify.ModeDefault, verify.FromPlan(""))
}

func TestAssertNoEnvWeaken(t *testing.T) {
	log := testutil.New(t)
	log.Phase("env_weaken")
	// Clean slate for keys we control.
	for _, k := range verify.EnvWeakenKeys {
		t.Setenv(k, "")
		_ = osUnset(t, k)
	}
	if err := verify.AssertNoEnvWeaken(); err != nil {
		log.Fail("clean", err.Error())
	} else {
		log.Step("clean", testutil.OutcomeOK, "no weaken keys")
	}

	t.Setenv("FOUNDRY_SKIP_VERIFY", "1")
	err := verify.AssertNoEnvWeaken()
	if err == nil {
		log.Fail("skip_verify_1", "expected error")
	} else {
		fe, _ := diagnostic.AsFoundryError(err)
		log.Assert("id", fe != nil && fe.ID() == diagnostic.IDVerifyFailed,
			diagnostic.IDVerifyFailed, idOf(fe))
		log.Assert("names_key", strings.Contains(err.Error(), "FOUNDRY_SKIP_VERIFY"),
			true, err.Error())
		log.Step("reject_weaken", testutil.OutcomeOK, err.Error())
	}
	// Explicit false is allowed (not weaken).
	t.Setenv("FOUNDRY_SKIP_VERIFY", "false")
	if err := verify.AssertNoEnvWeaken(); err != nil {
		log.Fail("false_ok", err.Error())
	} else {
		log.Step("false_ok", testutil.OutcomeOK, "false ignored")
	}
	log.PhaseEnd("env_weaken", testutil.OutcomeOK)
}

func idOf(fe *diagnostic.FoundryError) diagnostic.Identifier {
	if fe == nil {
		return ""
	}
	return fe.ID()
}

// osUnset clears env for keys that t.Setenv("") still leaves present on some
// platforms via LookupEnv. t.Setenv with empty still LookupEnv ok=true.
func osUnset(t *testing.T, key string) error {
	t.Helper()
	// testing.T.Setenv cannot truly unset; empty value with our Assert logic
	// treats empty as non-weaken. Returning nil.
	t.Setenv(key, "")
	return nil
}
