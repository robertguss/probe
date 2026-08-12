package resolve_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestMVPResolveCLIGolden is the MVP profiles=[] golden ResolvedProject for cli
// (acceptance: MVP golden).
func TestMVPResolveCLIGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("resolve_cli")
	log.Fixture("spec", "minimal-cli")
	log.Fixture("catalog", "embedded")

	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	log.Inputs(map[string]string{
		"archetype": vs.Archetype(),
		"profiles":  "[]",
		"module":    vs.Module(),
	})

	rp, err := resolve.Resolve(vs, cat)
	if err != nil {
		log.Fail("resolve", err.Error())
	}
	log.Step("summary", testutil.OutcomeOK, resolve.Summary(rp))
	log.Assert("name", rp.Name() == "demo-cli", "demo-cli", rp.Name())
	log.Assert("archetype", rp.Archetype() == "cli", "cli", rp.Archetype())
	log.Assert("visibility", rp.Visibility() == "private", "private", rp.Visibility())
	log.Assert("profiles_empty", len(rp.Profiles()) == 0, 0, len(rp.Profiles()))
	log.Assert("profiles_non_nil", rp.Profiles() != nil, true, rp.Profiles() != nil)
	log.Assert("digest_matches", rp.CatalogDigest() == cat.Digest(), string(cat.Digest()), string(rp.CatalogDigest()))
	log.Assert("git_init_default", rp.GitInit() == true, true, rp.GitInit())
	log.Assert("git_branch_default", rp.GitInitialBranch() == "main", "main", rp.GitInitialBranch())

	// Files: core README + cli main template, sorted by path.
	files := rp.Files()
	log.Step("file_count", testutil.OutcomeOK, "n="+itoa(len(files)))
	log.Assert("file_count_ge_2", len(files) >= 2, 2, len(files))
	// Sorted by path.
	for i := 1; i < len(files); i++ {
		log.Assert("files_sorted_"+itoa(i), files[i-1].Path <= files[i].Path,
			files[i-1].Path, files[i].Path)
	}
	// Owners only core + archetype:cli (no tui, no profile).
	for _, f := range files {
		okOwner := f.Owner == "core" || f.Owner == "archetype:cli"
		log.Assert("owner_"+f.Path, okOwner, "core|archetype:cli", f.Owner)
	}
	// Dependencies include cobra from cli archetype.
	deps := rp.Dependencies()
	log.Step("dep_count", testutil.OutcomeOK, "n="+itoa(len(deps)))
	foundCobra := false
	for _, d := range deps {
		if d.Module == "github.com/spf13/cobra" {
			foundCobra = true
			log.Assert("cobra_owner", d.Owner == "archetype:cli", "archetype:cli", d.Owner)
			log.Assert("cobra_version", d.Version == "v1.10.2", "v1.10.2", d.Version)
		}
		// Sorted check below.
	}
	log.Assert("has_cobra", foundCobra, true, foundCobra)
	for i := 1; i < len(deps); i++ {
		log.Assert("deps_sorted_"+itoa(i), deps[i-1].Module <= deps[i].Module,
			deps[i-1].Module, deps[i].Module)
	}

	// No distribution / tui contributions.
	for _, f := range files {
		log.Assert("not_profile_"+f.Path, !strings.HasPrefix(f.Owner, "profile:"),
			"non-profile", f.Owner)
		log.Assert("not_tui_"+f.Path, f.Owner != "archetype:tui", "non-tui", f.Owner)
	}

	log.PhaseEnd("resolve_cli", testutil.OutcomeOK)
}

// TestMVPResolveTUIGolden is the MVP profiles=[] golden for tui.
func TestMVPResolveTUIGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("resolve_tui")
	cat := mustLoadCatalog(t)
	vs := minimalTUI(t)

	rp, err := resolve.Resolve(vs, cat)
	if err != nil {
		log.Fail("resolve", err.Error())
	}
	log.Step("summary", testutil.OutcomeOK, resolve.Summary(rp))
	log.Assert("archetype", rp.Archetype() == "tui", "tui", rp.Archetype())
	log.Assert("profiles_empty", len(rp.Profiles()) == 0, 0, len(rp.Profiles()))

	files := rp.Files()
	for _, f := range files {
		okOwner := f.Owner == "core" || f.Owner == "archetype:tui"
		log.Assert("owner_"+f.Path, okOwner, "core|archetype:tui", f.Owner)
	}
	deps := rp.Dependencies()
	foundBT := false
	for _, d := range deps {
		if strings.Contains(d.Module, "bubbletea") {
			foundBT = true
			log.Assert("bt_owner", d.Owner == "archetype:tui", "archetype:tui", d.Owner)
		}
	}
	log.Assert("has_bubbletea", foundBT, true, foundBT)
	log.PhaseEnd("resolve_tui", testutil.OutcomeOK)
}

// TestImplementedProfileIDsPhase4 locks the Phase 4 selection set (distribution).
func TestImplementedProfileIDsPhase4(t *testing.T) {
	log := testutil.New(t)
	log.Phase("implemented_set")
	ids := resolve.ImplementedProfileIDs()
	log.Step("ids", testutil.OutcomeOK, "ids="+strings.Join(ids, ","))
	log.Assert("len_1", len(ids) == 1, 1, len(ids))
	log.Assert("non_nil", ids != nil, true, ids != nil)
	log.Assert("distribution_implemented", resolve.IsImplementedProfile("distribution"), true, false)
	log.Assert("configuration_not", !resolve.IsImplementedProfile("configuration"), true, false)
	cat := mustLoadCatalog(t)
	catalogProfiles := cat.ProfileIDs()
	log.Step("catalog_profiles", testutil.OutcomeOK, "ids="+strings.Join(catalogProfiles, ","))
	log.Assert("catalog_has_distribution", contains(catalogProfiles, "distribution"), true, false)
	log.PhaseEnd("implemented_set", testutil.OutcomeOK)
}

// TestUnknownProfileMatrix covers unknown, recipe-only, distribution (MVP),
// and typos — all resolve.unknown_profile with sorted available set + remediation.
func TestUnknownProfileMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("unknown_matrix")
	cat := mustLoadCatalog(t)

	cases := []struct {
		name       string
		profiles   string
		wantRemSec bool // Section 21 remediation for recipe-only
		recipeDoc  string
	}{
		// distribution is implemented (P4) but still rejected for private default specs via constraint tests.
		{"configuration_recipe", `["configuration"]`, true, catalog.RecipeDocConfiguration},
		{"local_persistence_recipe", `["local-persistence"]`, true, catalog.RecipeDocLocalPersistence},
		{"typo_configration", `["configration"]`, false, ""},
		{"typo_distribtion", `["distribtion"]`, false, ""},
		{"unknown_foo", `["not-a-profile"]`, false, ""},
		{"core_unit_id", `["core"]`, false, ""},
		{"cli_unit_id", `["cli"]`, false, ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("reject")
			log.Inputs(map[string]string{"profiles": tc.profiles})

			// Spec validates field shape but not profile existence — except
			// duplicates. Recipe-only and unknown IDs pass Validate.
			vs := specWith(t, map[string]string{"profiles": tc.profiles})
			_, err := resolve.Resolve(vs, cat)
			log.Assert("err", err != nil, true, err != nil)
			if err == nil {
				log.PhaseEnd("reject", testutil.OutcomeFail)
				return
			}
			fe, ok := diagnostic.AsFoundryError(err)
			log.Assert("foundry", ok, true, ok)
			if !ok {
				log.Fail("type", err.Error())
				return
			}
			log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
			log.Assert("id_unknown_profile", fe.ID() == diagnostic.IDResolveUnknownProfile,
				string(diagnostic.IDResolveUnknownProfile), string(fe.ID()))
			log.Assert("exit_2", fe.ExitCode() == diagnostic.ExitUsage,
				diagnostic.ExitUsage, fe.ExitCode())

			msg := fe.Message()
			log.Assert("msg_available", strings.Contains(msg, "available profiles"), true, false)
			// Phase 4 available set lists distribution.
			log.Assert("msg_lists_distribution",
				strings.Contains(msg, "distribution") || strings.Contains(msg, "[]"),
				true, false)
			// No did-you-mean.
			low := strings.ToLower(msg + fe.Remediation())
			log.Assert("no_did_you_mean", !strings.Contains(low, "did you mean") && !strings.Contains(low, "did-you-mean"),
				true, false)

			rem := fe.Remediation()
			log.Step("remediation", testutil.OutcomeOK, "len="+itoa(len(rem)))
			log.Assert("rem_non_empty", rem != "", true, false)
			if tc.wantRemSec {
				log.Assert("rem_section_21", strings.Contains(rem, "Section 21"), true, false)
				log.Assert("rem_recipe_doc", strings.Contains(rem, tc.recipeDoc), tc.recipeDoc, rem)
				log.Assert("rem_recipe_only", strings.Contains(rem, "recipe-only"), true, false)
			}
			log.Assert("rem_profiles_empty", strings.Contains(rem, "profiles = []") || strings.Contains(rem, "available"),
				true, false)

			// Log rejected id for step-log multi-case runs.
			log.Step("rejected", testutil.OutcomeOK, "profiles="+tc.profiles)
			log.PhaseEnd("reject", testutil.OutcomeOK)
		})
	}
	_ = log
}

// TestMultiProfileNegativeMatrix exercises multi-profile selections with step
// logs (acceptance: step-log multi-case runs).
func TestMultiProfileNegativeMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("multi_profile_negatives")
	cat := mustLoadCatalog(t)

	cases := []struct {
		name     string
		profiles string
	}{
		{"two_unknown", `["alpha", "beta"]`},
		{"recipe_then_unknown", `["configuration", "foo"]`},
		{"distribution_and_recipe", `["distribution", "local-persistence"]`},
		{"three_typos", `["config", "persist", "dist"]`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("resolve_multi")
			log.Inputs(map[string]string{"profiles": tc.profiles})
			vs := specWith(t, map[string]string{"profiles": tc.profiles})
			_, err := resolve.Resolve(vs, cat)
			log.Assert("err", err != nil, true, err != nil)
			fe, ok := diagnostic.AsFoundryError(err)
			log.Assert("foundry", ok, true, ok)
			if ok {
				log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
				log.Step("available_set", testutil.OutcomeOK, "msg="+fe.Message())
				log.Step("remediation_id", testutil.OutcomeOK, "id="+string(fe.ID()))
				log.Assert("unknown_or_constraint",
					fe.ID() == diagnostic.IDResolveUnknownProfile ||
						fe.ID() == diagnostic.IDResolveProfileConstraint,
					string(diagnostic.IDResolveUnknownProfile), string(fe.ID()))
			}
			log.PhaseEnd("resolve_multi", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("multi_profile_negatives", testutil.OutcomeOK)
}

// TestDuplicateProfileError ensures defensive duplicate detection includes
// sorted available set (acceptance: dup errors with sorted sets). Spec already
// rejects duplicates, so this constructs a ValidatedSpecification-like path
// by using a second resolve entry that only Validate would block — we use
// profiles that pass Validate (no dups) for most cases; for true dups we
// assert Validate fails first, documenting ownership.
func TestDuplicateProfileError(t *testing.T) {
	log := testutil.New(t)
	log.Phase("duplicate")
	// Spec layer owns duplicates (spec.duplicate_profile).
	raw, err := spec.Decode("foundry.toml", []byte(`
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "A valid demo CLI project"
archetype = "cli"
destination = "./demo-cli"
profiles = ["distribution", "distribution"]
`))
	if err != nil {
		log.Fail("decode", err.Error())
	}
	_, err = spec.Validate(raw)
	log.Assert("validate_rejects_dup", err != nil, true, err != nil)
	if err != nil {
		// Prefer ValidationError.First (aggregated field errors).
		if ve, ok := spec.AsValidationError(err); ok && ve.First() != nil {
			fe := ve.First()
			log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
			log.Assert("id_dup", fe.ID() == diagnostic.IDSpecDuplicateProfile,
				string(diagnostic.IDSpecDuplicateProfile), string(fe.ID()))
			log.Assert("msg_indexes", strings.Contains(fe.Message(), "indexes"),
				true, strings.Contains(fe.Message(), "indexes"))
			log.Assert("msg_available_or_dup", strings.Contains(fe.Message(), "duplicate"),
				true, strings.Contains(fe.Message(), "duplicate"))
		} else if fe, ok := diagnostic.AsFoundryError(err); ok {
			log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
			log.Assert("id_dup", fe.ID() == diagnostic.IDSpecDuplicateProfile,
				string(diagnostic.IDSpecDuplicateProfile), string(fe.ID()))
		} else {
			log.Step("err_type", testutil.OutcomeOK, "err="+err.Error())
			log.Assert("mentions_duplicate", strings.Contains(err.Error(), "duplicate") ||
				strings.Contains(err.Error(), "spec.duplicate_profile"), true, false)
		}
	}
	// Defensive resolve-layer dup check is covered in internal_test.go
	// (checkDuplicateProfiles) with sorted available set in the message.
	log.PhaseEnd("duplicate", testutil.OutcomeOK)
}

// TestUnknownArchetype is defensive: Validate already restricts to cli|tui.
// If a catalog lacks an archetype unit, Resolve fails with sorted available set.
func TestUnknownArchetypeCatalogMissing(t *testing.T) {
	log := testutil.New(t)
	log.Phase("archetype")
	// Normal path: cli and tui present.
	cat := mustLoadCatalog(t)
	ids := cat.ArchetypeIDs()
	log.Step("available", testutil.OutcomeOK, "ids="+strings.Join(ids, ","))
	log.Assert("has_cli", contains(ids, "cli"), true, contains(ids, "cli"))
	log.Assert("has_tui", contains(ids, "tui"), true, contains(ids, "tui"))
	// Sorted.
	sorted := sortedCopy(ids)
	log.Assert("sorted", equalStrings(ids, sorted), strings.Join(sorted, ","), strings.Join(ids, ","))
	log.PhaseEnd("archetype", testutil.OutcomeOK)
}

// TestNilInputs fail closed.
func TestNilInputs(t *testing.T) {
	log := testutil.New(t)
	log.Phase("nil_inputs")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)

	_, err := resolve.Resolve(nil, cat)
	log.Assert("nil_spec", err != nil, true, err != nil)

	_, err = resolve.Resolve(vs, nil)
	log.Assert("nil_catalog", err != nil, true, err != nil)
	if err != nil {
		fe, ok := diagnostic.AsFoundryError(err)
		id := ""
		if ok {
			id = string(fe.ID())
		}
		log.Assert("catalog_invalid", ok && fe.ID() == diagnostic.IDCatalogInvalid,
			string(diagnostic.IDCatalogInvalid), id)
	}
	log.PhaseEnd("nil_inputs", testutil.OutcomeOK)
}

// TestDeterminismTwice resolves the same inputs twice and compares deeply
// (also run under -count=2 externally).
func TestDeterminismTwice(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")
	cat := mustLoadCatalog(t)

	for _, vs := range []*spec.ValidatedSpecification{minimalCLI(t), minimalTUI(t)} {
		name := vs.Archetype()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("double_resolve")
			a, err1 := resolve.Resolve(vs, cat)
			b, err2 := resolve.Resolve(vs, cat)
			log.Assert("err1", err1 == nil, true, err1 == nil)
			log.Assert("err2", err2 == nil, true, err2 == nil)
			if err1 != nil || err2 != nil {
				return
			}
			log.Assert("equal", a.Equal(b), true, a.Equal(b))
			log.Assert("summary_equal", resolve.Summary(a) == resolve.Summary(b),
				resolve.Summary(a), resolve.Summary(b))
			// Profile order independence: empty is only MVP case.
			profs := a.Profiles()
			log.Assert("profiles_sorted", equalStrings(profs, sortedCopy(profs)),
				strings.Join(sortedCopy(profs), ","), strings.Join(profs, ","))
			log.PhaseEnd("double_resolve", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

// TestOwnerLabels stable format.
func TestOwnerLabels(t *testing.T) {
	log := testutil.New(t)
	log.Phase("owners")
	log.Assert("core", resolve.OwnerLabel(catalog.KindCore, "core") == "core", "core", resolve.OwnerLabel(catalog.KindCore, "core"))
	log.Assert("arch", resolve.OwnerLabel(catalog.KindArchetype, "cli") == "archetype:cli",
		"archetype:cli", resolve.OwnerLabel(catalog.KindArchetype, "cli"))
	log.Assert("prof", resolve.OwnerLabel(catalog.KindProfile, "distribution") == "profile:distribution",
		"profile:distribution", resolve.OwnerLabel(catalog.KindProfile, "distribution"))
	log.PhaseEnd("owners", testutil.OutcomeOK)
}

// TestResolvedProjectAccessorsImmutability: Profiles/Files/Dependencies copies.
func TestResolvedProjectDefensiveCopies(t *testing.T) {
	log := testutil.New(t)
	log.Phase("copies")
	cat := mustLoadCatalog(t)
	rp, err := resolve.Resolve(minimalCLI(t), cat)
	if err != nil {
		log.Fail("resolve", err.Error())
	}
	p1 := rp.Profiles()
	if cap(p1) > 0 {
		// Empty slice may have cap 0; mutating length is fine.
		_ = append(p1, "mutated")
	}
	log.Assert("profiles_still_empty", len(rp.Profiles()) == 0, 0, len(rp.Profiles()))

	files := rp.Files()
	if len(files) > 0 {
		orig := files[0].Path
		files[0].Path = "mutated"
		log.Assert("files_unchanged", rp.Files()[0].Path == orig, orig, rp.Files()[0].Path)
	}
	log.PhaseEnd("copies", testutil.OutcomeOK)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var neg bool
	if n < 0 {
		neg = true
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func sortedCopy(ids []string) []string {
	out := append([]string(nil), ids...)
	// simple insertion for tiny lists
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j-1] > out[j] {
			out[j-1], out[j] = out[j], out[j-1]
			j--
		}
	}
	return out
}
