package plan_test

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestPlanDeterministicProperty verifies that the canonical plan JSON is
// identical for two separate Pipeline invocations with the same valid spec
// (REQ-211 / Section 28). Includes bare and distribution profile fixtures.
func TestPlanDeterministicProperty(t *testing.T) {
	log := testutil.New(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}

	cfg := &quick.Config{
		MaxCount: 40,
		Values: func(args []reflect.Value, r *rand.Rand) {
			args[0] = reflect.ValueOf(testutil.GenValidProfileFixture(r))
		},
	}

	prop := func(f testutil.SpecFixture) bool {
		vs, err := f.Validate(testutil.FieldOrderA)
		if err != nil {
			// Discard rare invalid draws (generator is tuned for valid inputs).
			return true
		}
		dest := fixedDestination(vs.Destination(), plan.ObservationAbsent)
		opts := defaultPipelineOpts(dest)
		p1, err := plan.Pipeline(vs, cat, opts)
		if err != nil {
			t.Logf("Pipeline failed for %s: %v", f, err)
			return false
		}
		p2, err := plan.Pipeline(vs, cat, opts)
		if err != nil {
			t.Logf("Pipeline failed on second run for %s: %v", f, err)
			return false
		}
		if !p1.Equal(p2) {
			t.Logf("plan equality failed for %s", f)
			return false
		}
		return true
	}

	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("property failed: %v", err)
	}
	log.Step("plan_deterministic_property", testutil.OutcomeOK, "ok")
}

// TestPlanFieldOrderIndependentProperty verifies that permuting the order of
// top-level TOML fields in the spec does not change the canonical plan JSON,
// confirming that Validate normalizes input shape before Pipeline consumes it
// (REQ-211). Covers bare and distribution.
func TestPlanFieldOrderIndependentProperty(t *testing.T) {
	log := testutil.New(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}

	cfg := &quick.Config{
		MaxCount: 40,
		Values: func(args []reflect.Value, r *rand.Rand) {
			args[0] = reflect.ValueOf(testutil.GenValidProfileFixture(r))
		},
	}

	prop := func(f testutil.SpecFixture) bool {
		vsA, err := f.Validate(testutil.FieldOrderA)
		if err != nil {
			return true
		}
		vsB, err := f.Validate(testutil.FieldOrderB)
		if err != nil {
			t.Logf("order B validate failed for %s: %v", f, err)
			return false
		}
		dest := fixedDestination(vsA.Destination(), plan.ObservationAbsent)
		opts := defaultPipelineOpts(dest)
		pA, err := plan.Pipeline(vsA, cat, opts)
		if err != nil {
			t.Logf("Pipeline failed for order A %s: %v", f, err)
			return false
		}
		pB, err := plan.Pipeline(vsB, cat, opts)
		if err != nil {
			t.Logf("Pipeline failed for order B %s: %v", f, err)
			return false
		}
		if !pA.Equal(pB) {
			t.Logf("plan equality failed across field orders for %s", f)
			return false
		}
		return true
	}

	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("property failed: %v", err)
	}
	log.Step("plan_field_order_independent_property", testutil.OutcomeOK, "ok")
}

// TestPlanDistributionDeterministicProperty is an explicit distribution-only
// property so CI -run Property always exercises the profile surface even if
// random draws under-sample it.
func TestPlanDistributionDeterministicProperty(t *testing.T) {
	log := testutil.New(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}

	cfg := &quick.Config{
		MaxCount: 20,
		Values: func(args []reflect.Value, r *rand.Rand) {
			args[0] = reflect.ValueOf(testutil.GenDistributionFixture(r))
		},
	}

	prop := func(f testutil.SpecFixture) bool {
		vs, err := f.Validate(testutil.FieldOrderA)
		if err != nil {
			t.Logf("distribution validate failed for %s: %v", f, err)
			return false
		}
		dest := fixedDestination(vs.Destination(), plan.ObservationAbsent)
		opts := defaultPipelineOpts(dest)
		p1, err := plan.Pipeline(vs, cat, opts)
		if err != nil {
			t.Logf("Pipeline failed for %s: %v", f, err)
			return false
		}
		p2, err := plan.Pipeline(vs, cat, opts)
		if err != nil {
			return false
		}
		if !p1.Equal(p2) {
			return false
		}
		// Distribution contributes profile-owned files.
		if len(p1.Files()) == 0 {
			t.Logf("expected files in distribution plan for %s", f)
			return false
		}
		return true
	}

	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("distribution property failed: %v", err)
	}
	log.Step("plan_distribution_deterministic_property", testutil.OutcomeOK, "ok")
}

// TestPlanInvalidProfileRejectedProperty ensures unknown profiles and
// private+distribution never produce a plan (fail closed at resolve/plan).
func TestPlanInvalidProfileRejectedProperty(t *testing.T) {
	log := testutil.New(t)
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}

	cfg := &quick.Config{
		MaxCount: 30,
		Values: func(args []reflect.Value, r *rand.Rand) {
			// Force only invalid cases.
			f := testutil.GenBareSpecFixture(r)
			if r.Intn(2) == 0 {
				f.Profiles = []string{"not-a-real-profile"}
			} else {
				f.Visibility = "private"
				f.Archetype = "cli"
				f.Profiles = []string{"distribution"}
			}
			args[0] = reflect.ValueOf(f)
		},
	}

	prop := func(f testutil.SpecFixture) bool {
		vs, err := f.Validate(testutil.FieldOrderA)
		if err != nil {
			// Spec-layer rejection is also fine (fail closed).
			return true
		}
		dest := fixedDestination(vs.Destination(), plan.ObservationAbsent)
		opts := defaultPipelineOpts(dest)
		_, err = plan.Pipeline(vs, cat, opts)
		if err == nil {
			t.Logf("expected plan failure for invalid profile fixture %s", f)
			return false
		}
		return true
	}

	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("invalid profile property failed: %v", err)
	}
	log.Step("plan_invalid_profile_rejected_property", testutil.OutcomeOK, "ok")
}
