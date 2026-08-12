package resolve

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestResolveFlatWithImplementedDistribution exercises predicates end-to-end
// when distribution is treated as implemented (future P4 path; synthetic
// allowlist only — production ImplementedProfileIDs stays empty).
func TestResolveFlatWithImplementedDistribution(t *testing.T) {
	log := testutil.New(t)
	log.Phase("p4_preview")

	cat, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}

	// Happy: public + GH module + distribution selected.
	vs := mustValidateInternal(t, `
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "public cli with distribution"
archetype = "cli"
destination = "./demo-cli"
visibility = "public"
profiles = ["distribution"]
`)
	rp, err := resolveFlat(vs, cat, []string{"distribution"})
	if err != nil {
		log.Fail("resolve_ok", err.Error())
	}
	log.Step("summary", testutil.OutcomeOK, Summary(rp))
	log.Assert("profiles", len(rp.Profiles()) == 1 && rp.Profiles()[0] == "distribution",
		"distribution", strings.Join(rp.Profiles(), ","))
	// Files include distribution contribution.
	foundRel := false
	for _, f := range rp.Files() {
		if f.Owner == "profile:distribution" {
			foundRel = true
			log.Step("dist_file", testutil.OutcomeOK, "path="+f.Path)
		}
	}
	log.Assert("has_dist_file", foundRel, true, foundRel)

	// Constraint: private visibility fails.
	vsPriv := mustValidateInternal(t, `
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "private with distribution"
archetype = "cli"
destination = "./demo-cli"
visibility = "private"
profiles = ["distribution"]
`)
	_, err = resolveFlat(vsPriv, cat, []string{"distribution"})
	log.Assert("private_fails", err != nil, true, err != nil)
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
		log.Assert("constraint", fe.ID() == diagnostic.IDResolveProfileConstraint,
			string(diagnostic.IDResolveProfileConstraint), string(fe.ID()))
		log.Assert("names_vis", strings.Contains(fe.Message(), "visibility") ||
			strings.Contains(fe.Message(), PredicateRequiresVisibility), true, false)
	}

	// Constraint: non-GH module fails.
	vsGL := mustValidateInternal(t, `
schema = 1
name = "demo-cli"
module = "gitlab.com/example/demo-cli"
description = "gitlab module"
archetype = "cli"
destination = "./demo-cli"
visibility = "public"
profiles = ["distribution"]
`)
	// Wait — module final segment must equal name; gitlab.com/example/demo-cli is fine for Validate.
	_, err = resolveFlat(vsGL, cat, []string{"distribution"})
	log.Assert("gitlab_fails", err != nil, true, err != nil)
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
		log.Assert("module_host", fe.ID() == diagnostic.IDResolveProfileConstraint,
			string(diagnostic.IDResolveProfileConstraint), string(fe.ID()))
		log.Assert("predicate", strings.Contains(fe.Message(), PredicateModuleHost) ||
			strings.Contains(fe.Message(), "GitHub"), true, false)
	}

	// Unknown still fails even when another profile is implemented.
	vsUnk := mustValidateInternal(t, `
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "unknown profile"
archetype = "cli"
destination = "./demo-cli"
profiles = ["not-real"]
`)
	_, err = resolveFlat(vsUnk, cat, []string{"distribution"})
	log.Assert("unknown", err != nil, true, err != nil)
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		log.Assert("id", fe.ID() == diagnostic.IDResolveUnknownProfile,
			string(diagnostic.IDResolveUnknownProfile), string(fe.ID()))
		// Available set is sorted and includes distribution only.
		log.Assert("lists_distribution", strings.Contains(fe.Message(), "distribution"), true, false)
		log.Assert("sorted_brackets", strings.Contains(fe.Message(), "available profiles"), true, false)
	}

	// Duplicate defensive path inside resolveFlat.
	// Construct a ValidatedSpecification with dups is hard (unexported fields).
	// checkDuplicateProfiles unit:
	err = checkDuplicateProfiles([]string{"distribution", "distribution"}, []string{"distribution"})
	log.Assert("dup_err", err != nil, true, err != nil)
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		log.Assert("dup_id", fe.ID() == diagnostic.IDSpecDuplicateProfile,
			string(diagnostic.IDSpecDuplicateProfile), string(fe.ID()))
		log.Assert("dup_available", strings.Contains(fe.Message(), "available profiles"), true, false)
		log.Assert("dup_indexes", strings.Contains(fe.Message(), "indexes"), true, false)
	}

	log.PhaseEnd("p4_preview", testutil.OutcomeOK)
}

// TestResolveFlatSortedAvailableSet asserts multi-id available sets sort.
func TestResolveFlatSortedAvailableSet(t *testing.T) {
	log := testutil.New(t)
	log.Phase("sorted_available")
	cat, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	vs := mustValidateInternal(t, `
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "x"
archetype = "cli"
destination = "./demo-cli"
profiles = ["zzz"]
`)
	// Unsorted implemented input → error available set sorted.
	_, err = resolveFlat(vs, cat, []string{"zeta", "alpha", "mu"})
	log.Assert("err", err != nil, true, err != nil)
	fe, ok := diagnostic.AsFoundryError(err)
	log.Assert("foundry", ok, true, ok)
	if ok {
		msg := fe.Message()
		log.Step("msg", testutil.OutcomeOK, msg)
		// alpha before mu before zeta
		ai := strings.Index(msg, "alpha")
		mi := strings.Index(msg, "mu")
		zi := strings.Index(msg, "zeta")
		log.Assert("order", ai >= 0 && mi > ai && zi > mi, true, false)
	}
	log.PhaseEnd("sorted_available", testutil.OutcomeOK)
}

func mustValidateInternal(t *testing.T, toml string) *spec.ValidatedSpecification {
	t.Helper()
	raw, err := spec.Decode("foundry.toml", []byte(toml))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return vs
}
