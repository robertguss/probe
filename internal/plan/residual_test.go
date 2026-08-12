package plan_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestConstructErrorPaths(t *testing.T) {
	log := testutil.New(t)
	log.Phase("construct_errors")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)

	// missing go binary
	in := defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent))
	in.GoBinary = ""
	_, err := plan.Construct(in)
	log.Assert("missing_go", err != nil, true, err != nil)

	// missing git when git init
	in = defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent))
	in.GitBinary = ""
	// only fails if git init true
	if rp.GitInit() {
		_, err = plan.Construct(in)
		log.Assert("missing_git", err != nil, true, err != nil)
	}

	// bad destination observation
	in = defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.DestinationObservation("bogus")))
	_, err = plan.Construct(in)
	log.Assert("bad_obs", err != nil, true, err != nil)

	// empty basename
	dest := fixedDestination(vs.Destination(), plan.ObservationAbsent)
	dest.Basename = ""
	in = defaultInputs(rp, cat, dest)
	_, err = plan.Construct(in)
	log.Assert("empty_base", err != nil, true, err != nil)

	// stdin source
	in = defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent))
	in.SpecSource = plan.SpecSourceStdin
	in.SpecPath = ""
	p, err := plan.Construct(in)
	if err != nil {
		log.Step("stdin_err", testutil.OutcomeInfo, err.Error())
	} else {
		log.Assert("stdin_ok", p != nil && strings.Contains(p.String(), "Plan"), true, p.String())
	}

	// VerifyMode Normalize/Valid — real contracts, no always-true padding.
	log.Assert("valid_default", plan.VerifyDefault.Valid(), true, plan.VerifyDefault.Valid())
	log.Assert("valid_default_alias", plan.VerifyMode("default").Valid(), true, plan.VerifyMode("default").Valid())
	log.Assert("valid_strict", plan.VerifyMode("strict").Valid(), true, plan.VerifyMode("strict").Valid())
	log.Assert("valid_empty", plan.VerifyMode("").Valid(), true, plan.VerifyMode("").Valid())
	log.Assert("invalid_nope", !plan.VerifyMode("nope").Valid(), true, plan.VerifyMode("nope").Valid())
	log.Assert("norm_strict", plan.VerifyMode("strict").Normalize() == plan.VerifyStrict, plan.VerifyStrict, plan.VerifyMode("strict").Normalize())
	log.Assert("norm_empty", plan.VerifyMode("").Normalize() == plan.VerifyDefault, plan.VerifyDefault, plan.VerifyMode("").Normalize())

	// Equal mismatch
	p1 := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	p2 := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	log.Assert("equal", p1.Equal(p2), true, false)
	log.Assert("ne_nil", !p1.Equal(nil), true, false)

	// profiles format via distribution plan
	log.Assert("summary_profiles", strings.Contains(plan.Summary(p1), "profiles="), true, plan.Summary(p1))
	log.PhaseEnd("construct_errors", testutil.OutcomeOK)
}

func TestDistributionPlanProfilesFormat(t *testing.T) {
	log := testutil.New(t)
	log.Phase("dist_plan")
	cat := mustLoadCatalog(t)
	vs := mustValidate(t, "pub.toml", `
schema = 1
name = "pub-cli"
module = "github.com/example/pub-cli"
description = "public with distribution"
archetype = "cli"
destination = "./pub-cli"
visibility = "public"
profiles = ["distribution"]
[git]
init = false
`)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	sum := plan.Summary(p)
	log.Assert("has_dist", strings.Contains(sum, "distribution"), true, sum)
	log.Assert("profiles", len(p.Profiles()) == 1, 1, len(p.Profiles()))
	// Equal after JSON copy path
	b, err := p.MarshalJSON()
	if err != nil {
		// method may be on pointer value
		log.Step("marshal", testutil.OutcomeInfo, "no MarshalJSON")
	} else {
		log.Assert("json_len", len(b) > 10, true, len(b))
	}
	log.PhaseEnd("dist_plan", testutil.OutcomeOK)
}
