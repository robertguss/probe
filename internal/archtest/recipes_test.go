package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestSection21RecipeDocsExist ensures both Section 21 recipes are on disk with
// required structure (go-foundry-cli-6hp / REQ-073–075 content review).
func TestSection21RecipeDocsExist(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)

	log.Phase("paths_match_catalog")
	log.Assert("configuration_path",
		archtest.DocRecipeConfigurationPath == catalog.RecipeDocConfiguration,
		catalog.RecipeDocConfiguration, archtest.DocRecipeConfigurationPath)
	log.Assert("local_persistence_path",
		archtest.DocRecipeLocalPersistencePath == catalog.RecipeDocLocalPersistence,
		catalog.RecipeDocLocalPersistence, archtest.DocRecipeLocalPersistencePath)
	log.Assert("docs_dir", archtest.DocRecipesDir == catalog.RecipeDocsDir,
		catalog.RecipeDocsDir, archtest.DocRecipesDir)
	log.PhaseEnd("paths_match_catalog", testutil.OutcomeOK)

	cases := []struct {
		name     string
		rel      string
		headings []string
		needles  []string
	}{
		{
			name:     "configuration",
			rel:      archtest.DocRecipeConfigurationPath,
			headings: archtest.RequiredRecipeConfigurationHeadings,
			needles:  append(append([]string{}, archtest.RequiredRecipeSharedNeedles...), archtest.RequiredRecipeConfigurationNeedles...),
		},
		{
			name:     "local-persistence",
			rel:      archtest.DocRecipeLocalPersistencePath,
			headings: archtest.RequiredRecipeLocalPersistenceHeadings,
			needles:  append(append([]string{}, archtest.RequiredRecipeSharedNeedles...), archtest.RequiredRecipeLocalPersistenceNeedles...),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := testutil.New(t)
			log.Phase("load")
			log.Fixture("recipe", tc.rel)
			path := filepath.Join(root, tc.rel)
			data, err := os.ReadFile(path)
			if err != nil {
				log.Fail("read_recipe", err.Error())
			}
			body := string(data)
			log.Inputs(map[string]string{
				"bytes":    itoa(len(data)),
				"headings": itoa(len(tc.headings)),
				"needles":  itoa(len(tc.needles)),
			})
			log.PhaseEnd("load", testutil.OutcomeOK)

			log.Phase("assert_headings")
			var missingH []string
			for _, h := range tc.headings {
				ok := linePresent(body, h)
				log.Assert("heading:"+sanitize(h), ok, true, ok)
				if !ok {
					missingH = append(missingH, h)
				}
			}
			if len(missingH) > 0 {
				t.Fatalf("%s missing required headings:\n  - %s", tc.rel, strings.Join(missingH, "\n  - "))
			}
			log.PhaseEnd("assert_headings", testutil.OutcomeOK)

			log.Phase("assert_needles")
			var missingN []string
			for _, n := range tc.needles {
				ok := strings.Contains(body, n)
				log.Assert("needle:"+sanitize(n), ok, true, ok)
				if !ok {
					missingN = append(missingN, n)
				}
			}
			if len(missingN) > 0 {
				t.Fatalf("%s missing required content needles:\n  - %s", tc.rel, strings.Join(missingN, "\n  - "))
			}
			// Spec authority link.
			log.Assert("spec_link",
				strings.Contains(body, "02-definitive-foundry-specification-revised-fable-5.md"),
				true, false)
			// Admission bar is explicit (Section 19.4 + two real projects).
			log.Assert("admission_section_19_4", strings.Contains(body, "Section 19.4"), true, false)
			log.Assert("admission_two_real", strings.Contains(strings.ToLower(body), "two real"), true, false)
			log.PhaseEnd("assert_needles", testutil.OutcomeOK)
		})
	}
}

// TestSection21RecipesReferencedFromRemediation locks the w4n → 6hp pointer:
// resolve.unknown_profile remediation and registry text name the recipe paths.
func TestSection21RecipesReferencedFromRemediation(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)

	log.Phase("registry")
	reg := diagnostic.RemediationFor(diagnostic.IDResolveUnknownProfile)
	log.Step("remediation_id", testutil.OutcomeOK, "id="+string(diagnostic.IDResolveUnknownProfile))
	log.Assert("reg_section_21", strings.Contains(reg, "Section 21"), true, false)
	log.Assert("reg_configuration_doc", strings.Contains(reg, catalog.RecipeDocConfiguration), true, false)
	log.Assert("reg_local_persistence_doc", strings.Contains(reg, catalog.RecipeDocLocalPersistence), true, false)
	log.PhaseEnd("registry", testutil.OutcomeOK)

	log.Phase("unknown_profile_errors")
	for _, id := range catalog.RecipeOnlyProfileIDs {
		fe := catalog.UnknownProfileError(id, nil, diagnostic.Location{})
		rem := fe.Remediation()
		doc := catalog.RecipeDocPath(id)
		log.Step("rejected_id", testutil.OutcomeOK, "id="+id)
		log.Assert("rem_doc_"+id, strings.Contains(rem, doc), doc, rem)
		log.Assert("rem_section_"+id, strings.Contains(rem, "Section 21"), true, false)
		// On-disk file exists for every remediation path.
		_, err := os.Stat(filepath.Join(root, doc))
		log.Assert("file_exists_"+id, err == nil, true, err == nil)
	}
	log.PhaseEnd("unknown_profile_errors", testutil.OutcomeOK)

	log.Phase("profile_admission_crosslink")
	adm, err := os.ReadFile(filepath.Join(root, archtest.DocProfileAdmissionPath))
	if err != nil {
		log.Fail("read_admission", err.Error())
	}
	body := string(adm)
	// Admission process must point at written recipes once 6hp lands.
	log.Assert("adm_link_configuration",
		strings.Contains(body, "recipes/configuration.md") || strings.Contains(body, catalog.RecipeDocConfiguration),
		true, false)
	log.Assert("adm_link_local_persistence",
		strings.Contains(body, "recipes/local-persistence.md") || strings.Contains(body, catalog.RecipeDocLocalPersistence),
		true, false)
	log.PhaseEnd("profile_admission_crosslink", testutil.OutcomeOK)
}

// TestSection21RecipesNotCatalogProfiles is the negative check: recipe IDs
// never appear as catalog profile units or forbidden layout trees.
func TestSection21RecipesNotCatalogProfiles(t *testing.T) {
	log := testutil.New(t)
	log.Phase("catalog_exclusion")

	c, err := catalog.Load()
	if err != nil {
		log.Fail("load_catalog", err.Error())
	}
	ids := c.ProfileIDs()
	log.Step("profile_ids", testutil.OutcomeOK, "ids="+strings.Join(ids, ","))
	for _, id := range catalog.RecipeOnlyProfileIDs {
		log.Assert("absent_profile_"+id, !containsString(ids, id), false, containsString(ids, id))
		_, ok := c.Manifest(id)
		log.Assert("absent_manifest_"+id, !ok, false, ok)
	}
	for _, p := range c.Paths() {
		if strings.HasPrefix(p, "profiles/configuration") || strings.HasPrefix(p, "profiles/local-persistence") {
			log.Fail("forbidden_path", p)
		}
	}
	log.PhaseEnd("catalog_exclusion", testutil.OutcomeOK)
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
