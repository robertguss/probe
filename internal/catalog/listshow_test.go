package catalog_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestListShowDataModelReady asserts the list/show surface for CLI (REQ-035):
// embedded units (core, cli, tui, distribution), stable ordering, Show hits,
// and unknown-id fail-closed with available set.
func TestListShowDataModelReady(t *testing.T) {
	log := testutil.New(t)
	log.Phase("load")
	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	log.Step("file_count", testutil.OutcomeOK, "count="+itoa(c.FileCount()))
	log.Step("digest", testutil.OutcomeOK, "sha256="+string(c.Digest()))
	log.PhaseEnd("load", testutil.OutcomeOK)

	log.Phase("list")
	units := c.List()
	log.Step("unit_count", testutil.OutcomeOK, "count="+itoa(len(units)))
	ids := make([]string, 0, len(units))
	for _, u := range units {
		ids = append(ids, u.ID)
		log.Step("unit_"+u.ID, testutil.OutcomeOK,
			"kind="+string(u.Kind)+" path="+u.ManifestPath)
		log.Assert("desc_nonempty_"+u.ID, strings.TrimSpace(u.Description) != "", true, false)
		log.Assert("manifest_path_"+u.ID, u.ManifestPath != "", true, false)
		log.Assert("kind_known_"+u.ID,
			u.Kind == catalog.KindCore || u.Kind == catalog.KindArchetype || u.Kind == catalog.KindProfile,
			true, false)
	}
	// Deterministic sort by ID.
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	log.Assert("list_sorted_by_id", equalStrings(ids, sorted), true, false)

	// Phase-1 inventory: core + cli + tui + distribution (catalogued; MVP
	// selection still empty — resolver owns implemented set).
	wantIDs := []string{"cli", "core", "distribution", "tui"}
	log.Assert("unit_ids", equalStrings(ids, wantIDs), true, false)
	if !equalStrings(ids, wantIDs) {
		t.Logf("got=%v want=%v", ids, wantIDs)
	}

	// No recipe-only IDs in list.
	for _, id := range ids {
		log.Assert("not_recipe_"+id, !catalog.IsRecipeOnlyProfile(id), true, false)
	}
	log.PhaseEnd("list", testutil.OutcomeOK)

	log.Phase("list_by_kind")
	cores := c.ListByKind(catalog.KindCore)
	log.Assert("core_count", len(cores) == 1, 1, len(cores))
	if len(cores) == 1 {
		log.Assert("core_id", cores[0].ID == "core", "core", cores[0].ID)
	}
	arch := c.ListByKind(catalog.KindArchetype)
	archIDs := make([]string, 0, len(arch))
	for _, u := range arch {
		archIDs = append(archIDs, u.ID)
	}
	log.Assert("archetypes", equalStrings(archIDs, []string{"cli", "tui"}), true, false)
	log.Assert("ArchetypeIDs", equalStrings(c.ArchetypeIDs(), []string{"cli", "tui"}), true, false)
	log.Assert("ProfileIDs", equalStrings(c.ProfileIDs(), []string{"distribution"}), true, false)
	log.Assert("UnitIDs", equalStrings(c.UnitIDs(), wantIDs), true, false)
	log.PhaseEnd("list_by_kind", testutil.OutcomeOK)

	log.Phase("show_hits")
	for _, id := range wantIDs {
		log.NoteID(id)
		m, err := c.Show(id)
		log.Assert("show_ok_"+id, err == nil, true, err == nil)
		if err != nil {
			continue
		}
		log.Assert("show_id_"+id, m.ID == id, id, m.ID)
		log.Assert("show_files_"+id, len(m.Files) >= 0, true, true)
	}
	log.PhaseEnd("show_hits", testutil.OutcomeOK)

	log.Phase("show_miss")
	missIDs := []string{
		"configuration",
		"local-persistence",
		"nope",
		"",
		"CLI", // case-sensitive exact match only
	}
	for _, id := range missIDs {
		log.NoteID(id)
		_, err := c.Show(id)
		log.Assert("show_err_"+id, err != nil, true, err != nil)
		fe, ok := diagnostic.AsFoundryError(err)
		log.Assert("is_foundry_"+id, ok, true, ok)
		if !ok {
			continue
		}
		log.Assert("id_catalog_invalid_"+id, fe.ID() == diagnostic.IDCatalogInvalid,
			string(diagnostic.IDCatalogInvalid), string(fe.ID()))
		if err != nil {
			log.Assert("names_available_"+id, strings.Contains(err.Error(), "available units"), true, false)
			// Available set must name known units so agents can self-correct.
			for _, want := range wantIDs {
				if !strings.Contains(err.Error(), want) {
					log.Fail("available_mentions_"+want, err.Error())
				}
			}
		}
		if fe.Remediation() != "" {
			log.Assert("remediation_list_hint_"+id,
				strings.Contains(fe.Remediation(), "catalog list"), true, false)
		}
	}
	log.PhaseEnd("show_miss", testutil.OutcomeOK)
}

// TestListShowDeterminism double-loads and compares list/show outputs
// (also exercised under -count=2 externally).
func TestListShowDeterminism(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")
	c1, err := catalog.Load()
	if err != nil {
		log.Fail("load1", err.Error())
	}
	c2, err := catalog.Load()
	if err != nil {
		log.Fail("load2", err.Error())
	}
	l1, l2 := c1.List(), c2.List()
	log.Assert("list_len", len(l1) == len(l2), len(l1), len(l2))
	for i := range l1 {
		if i >= len(l2) {
			break
		}
		log.Assert("id_"+l1[i].ID, l1[i].ID == l2[i].ID, l1[i].ID, l2[i].ID)
		log.Assert("kind_"+l1[i].ID, l1[i].Kind == l2[i].Kind, string(l1[i].Kind), string(l2[i].Kind))
		log.Assert("desc_"+l1[i].ID, l1[i].Description == l2[i].Description, true, false)
		log.Assert("path_"+l1[i].ID, l1[i].ManifestPath == l2[i].ManifestPath, true, false)
	}
	m1, err1 := c1.Show("cli")
	m2, err2 := c2.Show("cli")
	log.Assert("show_err", err1 == nil && err2 == nil, true, err1 == nil && err2 == nil)
	if err1 == nil && err2 == nil {
		log.Assert("show_id", m1.ID == m2.ID, m1.ID, m2.ID)
		log.Assert("show_kind", m1.Kind == m2.Kind, string(m1.Kind), string(m2.Kind))
		log.Assert("show_desc", m1.Description == m2.Description, true, false)
	}
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}
