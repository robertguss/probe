package catalog_test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestRecipeOnlyConstantsStable locks the closed recipe-only ID set and doc
// anchors (REQ-073/075). Renaming requires a deliberate spec revision.
func TestRecipeOnlyConstantsStable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("constants")

	want := []string{"configuration", "local-persistence"}
	got := append([]string(nil), catalog.RecipeOnlyProfileIDs...)
	sort.Strings(got)
	log.Assert("ids_equal", equalStrings(got, want), strings.Join(want, ","), strings.Join(got, ","))
	log.Assert("configuration_const", catalog.RecipeOnlyProfileConfiguration == "configuration",
		"configuration", catalog.RecipeOnlyProfileConfiguration)
	log.Assert("local_persistence_const", catalog.RecipeOnlyProfileLocalPersistence == "local-persistence",
		"local-persistence", catalog.RecipeOnlyProfileLocalPersistence)
	log.Assert("section_ref", catalog.RecipeSectionRef == "Section 21", "Section 21", catalog.RecipeSectionRef)
	log.Assert("docs_dir", catalog.RecipeDocsDir == "docs/recipes", "docs/recipes", catalog.RecipeDocsDir)
	log.Assert("doc_configuration", catalog.RecipeDocConfiguration == "docs/recipes/configuration.md",
		"docs/recipes/configuration.md", catalog.RecipeDocConfiguration)
	log.Assert("doc_local_persistence", catalog.RecipeDocLocalPersistence == "docs/recipes/local-persistence.md",
		"docs/recipes/local-persistence.md", catalog.RecipeDocLocalPersistence)

	for _, id := range want {
		log.Assert("is_recipe_"+id, catalog.IsRecipeOnlyProfile(id), true, false)
		doc := catalog.RecipeDocPath(id)
		log.Assert("doc_path_"+id, strings.HasPrefix(doc, "docs/recipes/") && strings.HasSuffix(doc, ".md"),
			"docs/recipes/<id>.md", doc)
	}
	log.Assert("not_recipe_distribution", !catalog.IsRecipeOnlyProfile("distribution"), true, false)
	log.Assert("not_recipe_empty", !catalog.IsRecipeOnlyProfile(""), true, false)
	log.Assert("not_recipe_typo", !catalog.IsRecipeOnlyProfile("configration"), true, false)
	log.PhaseEnd("constants", testutil.OutcomeOK)
}

// TestEmbeddedCatalogExcludesRecipeProfiles verifies the production catalog
// has no recipe-only profile units or paths (catalog exclusion).
func TestEmbeddedCatalogExcludesRecipeProfiles(t *testing.T) {
	log := testutil.New(t)
	log.Phase("load")
	log.Fixture("catalog", "embedded")

	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	log.Step("digest", testutil.OutcomeOK, "sha256="+string(c.Digest()))
	log.PhaseEnd("load", testutil.OutcomeOK)

	log.Phase("assert_exclusion")
	profileIDs := c.ProfileIDs()
	log.Step("profile_ids", testutil.OutcomeOK, "ids="+strings.Join(profileIDs, ","))
	for _, id := range catalog.RecipeOnlyProfileIDs {
		log.Assert("profile_absent_"+id, !contains(profileIDs, id), false, contains(profileIDs, id))
		_, ok := c.Manifest(id)
		log.Assert("manifest_absent_"+id, !ok, false, ok)
	}
	// distribution may be catalogued for post-MVP but is never recipe-only.
	log.Assert("distribution_not_recipe", !catalog.IsRecipeOnlyProfile("distribution"), true, false)

	for _, p := range c.Paths() {
		if strings.HasPrefix(p, "profiles/configuration") || strings.HasPrefix(p, "profiles/local-persistence") {
			log.Fail("forbidden_path", p)
		}
	}
	log.Step("paths_checked", testutil.OutcomeOK, "n="+itoa(c.FileCount()))
	log.PhaseEnd("assert_exclusion", testutil.OutcomeOK)
}

// TestRejectUnknownProfilesRecipeOnly covers selection of configuration /
// local-persistence → resolve.unknown_profile + sorted available set +
// Section 21 / docs/recipes remediation (REQ-073/075 unit matrix item 1, 3).
func TestRejectUnknownProfilesRecipeOnly(t *testing.T) {
	log := testutil.New(t)

	// Available sets exercised: empty (MVP) and a non-empty post-MVP-ish set.
	availableCases := []struct {
		name  string
		avail []string
	}{
		{"mvp_empty", nil},
		{"mvp_empty_slice", []string{}},
		{"with_distribution", []string{"distribution"}},
		{"unsorted_input", []string{"zeta", "alpha"}},
	}

	for _, av := range availableCases {
		av := av
		for _, recipeID := range catalog.RecipeOnlyProfileIDs {
			recipeID := recipeID
			name := av.name + "/" + recipeID
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				log := testutil.New(t)
				log.Phase("reject")
				log.Inputs(map[string]string{
					"requested": recipeID,
					"available": strings.Join(av.avail, ","),
				})

				loc := diagnostic.SpecLocation("foundry.toml", 12, 1)
				err := catalog.RejectUnknownProfiles([]string{recipeID}, av.avail, loc)
				log.Assert("err_non_nil", err != nil, true, err != nil)
				if err == nil {
					log.PhaseEnd("reject", testutil.OutcomeFail)
					return
				}

				fe, ok := diagnostic.AsFoundryError(err)
				log.Assert("is_foundry_error", ok, true, ok)
				if !ok {
					log.Fail("type", err.Error())
					return
				}

				log.Step("rejected_id", testutil.OutcomeOK, "id="+recipeID)
				log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
				log.Assert("id_resolve_unknown_profile", fe.ID() == diagnostic.IDResolveUnknownProfile,
					string(diagnostic.IDResolveUnknownProfile), string(fe.ID()))
				log.Assert("exit_2", fe.ExitCode() == diagnostic.ExitUsage,
					diagnostic.ExitUsage, fe.ExitCode())

				// Message names rejected id + available set.
				msg := fe.Message()
				log.Assert("msg_names_id", strings.Contains(msg, recipeID), true, false)
				log.Assert("msg_names_available", strings.Contains(msg, "available profiles"), true, false)
				wantAvail := sortedCopy(av.avail)
				for _, a := range wantAvail {
					log.Assert("msg_lists_"+a, strings.Contains(msg, a), true, false)
				}
				log.Step("available_ids", testutil.OutcomeOK, "ids="+strings.Join(wantAvail, ","))

				// Remediation: Section 21 + docs/recipes path (stable before 6hp lands).
				rem := fe.Remediation()
				log.Step("remediation_id", testutil.OutcomeOK, "id="+string(fe.ID()))
				log.Assert("rem_non_empty", rem != "", true, false)
				log.Assert("rem_section_21", strings.Contains(rem, "Section 21"), true, false)
				doc := catalog.RecipeDocPath(recipeID)
				log.Assert("rem_recipe_doc", strings.Contains(rem, doc), doc, rem)
				log.Assert("rem_recipe_only", strings.Contains(rem, "recipe-only"), true, false)
				log.Assert("rem_profiles_empty", strings.Contains(rem, "profiles = []"), true, false)
				log.Assert("loc_spec", fe.Location().File == "foundry.toml", "foundry.toml", fe.Location().File)

				log.PhaseEnd("reject", testutil.OutcomeOK)
			})
		}
	}
	_ = log
}

// TestRejectUnknownProfilesTyposFailClosed ensures near-miss spellings and
// unrelated IDs fail closed with available set and no did-you-mean (REQ-101).
func TestRejectUnknownProfilesTyposFailClosed(t *testing.T) {
	log := testutil.New(t)
	log.Phase("typos")

	typos := []string{
		"configration",      // missing u
		"config",            // truncated
		"Configuration",     // wrong case
		"local_persistence", // underscore
		"localpersistence",  // no hyphen
		"local-persistance", // misspelled
		"distribtion",       // near distribution
		"unknown-profile",
		"core", // unit id but not a profile
	}
	available := []string{"distribution"}

	for _, id := range typos {
		id := id
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("reject_typo")
			log.Inputs(map[string]string{"requested": id, "available": "distribution"})

			err := catalog.RejectUnknownProfiles([]string{id}, available, diagnostic.Location{})
			log.Assert("err", err != nil, true, err != nil)
			fe, ok := diagnostic.AsFoundryError(err)
			log.Assert("foundry", ok, true, ok)
			if !ok {
				return
			}
			log.Step("rejected_id", testutil.OutcomeOK, "id="+id)
			log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
			log.Assert("id", fe.ID() == diagnostic.IDResolveUnknownProfile,
				string(diagnostic.IDResolveUnknownProfile), string(fe.ID()))
			msg := fe.Message()
			log.Assert("msg_id", strings.Contains(msg, id), true, false)
			log.Assert("msg_available", strings.Contains(msg, "distribution"), true, false)
			// No fuzzy / did-you-mean language.
			rem := fe.Remediation()
			low := strings.ToLower(rem + msg)
			log.Assert("no_did_you_mean", !strings.Contains(low, "did you mean") && !strings.Contains(low, "did-you-mean"),
				true, false)
			// Still points at recipe boundary for agents.
			log.Assert("rem_section_or_recipes",
				strings.Contains(rem, "Section 21") || strings.Contains(rem, "docs/recipes"),
				true, false)
			log.Step("available_ids", testutil.OutcomeOK, "ids=distribution")
			log.Step("remediation_id", testutil.OutcomeOK, "id="+string(fe.ID()))
			log.PhaseEnd("reject_typo", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("typos", testutil.OutcomeOK)
}

// TestRejectUnknownProfilesAcceptsAvailable confirms implemented IDs pass and
// multi-select stops at the first unknown (stable report order).
func TestRejectUnknownProfilesAcceptsAvailable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("accept")

	err := catalog.RejectUnknownProfiles(nil, nil, diagnostic.Location{})
	log.Assert("nil_ok", err == nil, true, err == nil)

	err = catalog.RejectUnknownProfiles([]string{}, []string{"distribution"}, diagnostic.Location{})
	log.Assert("empty_ok", err == nil, true, err == nil)

	err = catalog.RejectUnknownProfiles([]string{"distribution"}, []string{"distribution"}, diagnostic.Location{})
	log.Assert("known_ok", err == nil, true, err == nil)

	// First unknown wins (configuration before local-persistence in request order).
	err = catalog.RejectUnknownProfiles(
		[]string{"configuration", "local-persistence"},
		nil,
		diagnostic.Location{},
	)
	fe, ok := diagnostic.AsFoundryError(err)
	log.Assert("multi_err", ok, true, ok)
	if ok {
		log.Assert("first_reported", strings.Contains(fe.Message(), "configuration"), true, false)
		log.Step("rejected_id", testutil.OutcomeOK, "id=configuration")
	}

	// Recipe-only still rejected even if wrongly listed as available.
	err = catalog.RejectUnknownProfiles(
		[]string{"configuration"},
		[]string{"configuration", "distribution"},
		diagnostic.Location{},
	)
	fe, ok = diagnostic.AsFoundryError(err)
	log.Assert("recipe_overrides_available", ok && fe.ID() == diagnostic.IDResolveUnknownProfile, true, false)
	log.PhaseEnd("accept", testutil.OutcomeOK)
}

// TestCatalogRejectsRecipeOnlyManifestID ensures ParseManifest / LoadFS fail
// when a unit declares a recipe-only id (catalog exclusion; item 2).
func TestCatalogRejectsRecipeOnlyManifestID(t *testing.T) {
	log := testutil.New(t)

	for _, id := range catalog.RecipeOnlyProfileIDs {
		id := id
		t.Run("parse_"+id, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("parse_manifest")
			log.Inputs(map[string]string{"id": id})

			// Path under profiles/<id>/ is also layout-forbidden; ParseManifest
			// rejects the id independently so smuggling under another path fails.
			body := recipeOnlyManifestBody(id)
			path := "profiles/" + id + "/manifest.toml"
			_, err := catalog.ParseManifest(path, []byte(body), nil)
			log.Assert("err", err != nil, true, err != nil)
			fe, ok := diagnostic.AsFoundryError(err)
			log.Assert("foundry", ok, true, ok)
			if ok {
				log.Step("rejected_id", testutil.OutcomeOK, "id="+id)
				log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
				log.Assert("catalog_invalid", fe.ID() == diagnostic.IDCatalogInvalid,
					string(diagnostic.IDCatalogInvalid), string(fe.ID()))
				log.Assert("msg_recipe", strings.Contains(fe.Message(), "recipe-only"), true, false)
				log.Assert("msg_id", strings.Contains(fe.Message(), id), true, false)
				rem := fe.Remediation()
				log.Assert("rem_section", strings.Contains(rem, "Section 21"), true, false)
				log.Assert("rem_doc", strings.Contains(rem, catalog.RecipeDocPath(id)), true, false)
				log.Step("remediation_id", testutil.OutcomeOK, "id="+string(fe.ID()))
			}
			log.PhaseEnd("parse_manifest", testutil.OutcomeOK)
		})

		t.Run("layout_"+id, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("layout_forbid")
			base := mustEmbeddedMap(t)
			path := "profiles/" + id + "/manifest.toml"
			base[path] = []byte(recipeOnlyManifestBody(id))
			err := catalog.ValidateLayout(toMapFS(base))
			log.Assert("err", err != nil, true, err != nil)
			fe, ok := diagnostic.AsFoundryError(err)
			log.Assert("foundry", ok, true, ok)
			if ok {
				log.Step("rejected_id", testutil.OutcomeOK, "id="+id)
				log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
				log.Assert("catalog_invalid", fe.ID() == diagnostic.IDCatalogInvalid,
					string(diagnostic.IDCatalogInvalid), string(fe.ID()))
				log.Assert("loc_path", strings.Contains(fe.Location().Path, path) || strings.Contains(err.Error(), path),
					path, fe.Location().Path)
				rem := fe.Remediation()
				log.Assert("rem_recipes_dir", strings.Contains(rem, catalog.RecipeDocsDir), true, false)
				log.Assert("rem_section", strings.Contains(rem, "Section 21"), true, false)
				log.Step("remediation_id", testutil.OutcomeOK, "id="+string(fe.ID()))
				log.Step("available_ids", testutil.OutcomeOK, "ids=(layout; n/a)")
			}
			// LoadFS must also fail closed.
			_, loadErr := catalog.LoadFS(toMapFS(base))
			log.Assert("loadfs_fails", loadErr != nil, true, loadErr != nil)
			log.PhaseEnd("layout_forbid", testutil.OutcomeOK)
		})
	}
	_ = log
}

// TestUnknownProfileErrorDeterminism asserts identical outputs across repeated
// calls (supports -count=2).
func TestUnknownProfileErrorDeterminism(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")

	requested := "configuration"
	available := []string{"zeta", "alpha", "alpha"} // unsorted + dup
	loc := diagnostic.SpecLocation("foundry.toml", 3, 5)

	var msgs, rems, ids []string
	for i := 0; i < 5; i++ {
		fe := catalog.UnknownProfileError(requested, available, loc)
		ids = append(ids, string(fe.ID()))
		msgs = append(msgs, fe.Message())
		rems = append(rems, fe.Remediation())
		log.Step("iter", testutil.OutcomeOK, "i="+itoa(i)+" msg_len="+itoa(len(fe.Message())))
	}
	for i := 1; i < len(msgs); i++ {
		log.Assert("msg_stable", msgs[i] == msgs[0], msgs[0], msgs[i])
		log.Assert("rem_stable", rems[i] == rems[0], rems[0], rems[i])
		log.Assert("id_stable", ids[i] == ids[0], ids[0], ids[i])
	}
	// Available set is sorted alpha, zeta in the message.
	log.Assert("sorted_alpha_before_zeta",
		strings.Index(msgs[0], "alpha") < strings.Index(msgs[0], "zeta"),
		true, false)
	// Dup alpha appears once.
	log.Assert("deduped", strings.Count(msgs[0], "alpha") == 1, 1, strings.Count(msgs[0], "alpha"))

	// RejectUnknownProfiles same stability.
	err1 := catalog.RejectUnknownProfiles([]string{"local-persistence"}, available, loc)
	err2 := catalog.RejectUnknownProfiles([]string{"local-persistence"}, available, loc)
	log.Assert("reject_equal", err1.Error() == err2.Error(), err1.Error(), err2.Error())
	log.Step("rejected_id", testutil.OutcomeOK, "id=local-persistence")
	log.Step("available_ids", testutil.OutcomeOK, "ids=alpha,zeta")
	log.Step("remediation_id", testutil.OutcomeOK, "id="+string(diagnostic.IDResolveUnknownProfile))
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

// TestRegistryRemediationMentionsRecipes ensures the default registry text for
// resolve.unknown_profile points agents at Section 21 / docs/recipes.
func TestRegistryRemediationMentionsRecipes(t *testing.T) {
	log := testutil.New(t)
	log.Phase("registry")
	rem := diagnostic.RemediationFor(diagnostic.IDResolveUnknownProfile)
	log.Step("remediation_id", testutil.OutcomeOK, "id="+string(diagnostic.IDResolveUnknownProfile))
	log.Assert("non_empty", rem != "", true, false)
	log.Assert("section_21", strings.Contains(rem, "Section 21"), true, false)
	log.Assert("docs_recipes", strings.Contains(rem, "docs/recipes"), true, false)
	log.Assert("configuration", strings.Contains(rem, "configuration"), true, false)
	log.Assert("local_persistence", strings.Contains(rem, "local-persistence"), true, false)
	log.Assert("profiles_empty", strings.Contains(rem, "profiles = []"), true, false)
	log.PhaseEnd("registry", testutil.OutcomeOK)
}

// TestRecipeOnlyErrorsUnwrapAsFoundryError is a small chain sanity check.
func TestRecipeOnlyErrorsUnwrapAsFoundryError(t *testing.T) {
	log := testutil.New(t)
	log.Phase("unwrap")
	err := catalog.RejectUnknownProfiles([]string{"configuration"}, nil, diagnostic.Location{})
	var fe *diagnostic.FoundryError
	log.Assert("errors_as", errors.As(err, &fe), true, false)
	log.Assert("id", fe != nil && fe.ID() == diagnostic.IDResolveUnknownProfile, true, false)
	log.PhaseEnd("unwrap", testutil.OutcomeOK)
}

func recipeOnlyManifestBody(id string) string {
	return strings.Join([]string{
		"schema = 1",
		`id = "` + id + `"`,
		`kind = "profile"`,
		`description = "must be rejected"`,
		`compatible_archetypes = ["cli"]`,
		"",
		"[[files]]",
		`path = "x.md"`,
		`render = "static"`,
		`source = "files/x.md"`,
		`mode = "0644"`,
		"",
	}, "\n")
}

func sortedCopy(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
