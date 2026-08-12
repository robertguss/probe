package archtest_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestReqTraceabilityCompleteness is always-on CI: every Section 53 REQ
// appears exactly once in docs/evidence/req-traceability.json with beads,
// tests, and a legal status (no TBD).
func TestReqTraceabilityCompleteness(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("load")
	log.Fixture("matrix_json", archtest.DocReqTraceJSONPath)
	log.Fixture("matrix_md", archtest.DocReqTraceMDPath)
	log.Fixture("spec", archtest.SpecAuthorityPath)
	log.PhaseEnd("load", testutil.OutcomeOK)

	log.Phase("check")
	rep, err := archtest.CheckReqTraceability(root, archtest.CheckReqTraceabilityOptions{
		// Completeness only — do not require verified yet (seed is mostly planned).
		PhaseExit:   "",
		StrictPaths: false,
		WarnOrphans: true,
	})
	if err != nil {
		log.Fail("check_error", err.Error())
	}
	log.Inputs(map[string]string{
		"spec_count":   itoa(rep.SpecCount),
		"matrix_count": itoa(rep.MatrixCount),
		"missing":      itoa(len(rep.Missing)),
		"extra":        itoa(len(rep.Extra)),
		"duplicates":   itoa(len(rep.Duplicates)),
		"tbd":          itoa(len(rep.TBDCells)),
		"orphans":      itoa(len(rep.OrphanClaims)),
	})
	// Always print the full report for agent legibility (sorted lists).
	t.Log("\n" + rep.String())
	if len(rep.OrphanClaims) > 0 {
		// Planned layers may name packages not created yet — warn only.
		log.Step("orphan_test_claims_warn", testutil.OutcomeOK, "count="+itoa(len(rep.OrphanClaims)))
	}
	log.Assert("ok", rep.OK, true, rep.OK)
	log.Assert("exactly_125_spec", rep.SpecCount == 125, 125, rep.SpecCount)
	log.Assert("exactly_125_matrix", rep.MatrixCount == 125, 125, rep.MatrixCount)
	log.Assert("no_missing", len(rep.Missing) == 0, 0, len(rep.Missing))
	log.Assert("no_extra", len(rep.Extra) == 0, 0, len(rep.Extra))
	log.Assert("no_tbd", len(rep.TBDCells) == 0, 0, len(rep.TBDCells))
	if !rep.OK {
		t.Fatalf("req traceability incomplete:\n%s", rep.String())
	}
	log.PhaseEnd("check", testutil.OutcomeOK)
}

// TestReqTraceabilityMarkdownTwin ensures the human register lists every REQ id.
func TestReqTraceabilityMarkdownTwin(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("assert_md")
	data, err := os.ReadFile(filepath.Join(root, archtest.DocReqTraceMDPath))
	if err != nil {
		log.Fail("read_md", err.Error())
	}
	body := string(data)
	ids, err := archtest.LoadSection53REQIDs(root)
	if err != nil {
		log.Fail("load_spec", err.Error())
	}
	var missing []string
	for _, id := range ids {
		if !strings.Contains(body, id) {
			missing = append(missing, id)
		}
	}
	log.Assert("md_has_all_reqs", len(missing) == 0, 0, len(missing))
	if len(missing) > 0 {
		t.Fatalf("markdown matrix missing REQ ids (sorted): %s", strings.Join(missing, ", "))
	}
	// Protocol markers required by dlv.1.
	for _, needle := range []string{
		"go-foundry-cli-dlv.1",
		"go-foundry-cli-0z4",
		"go-foundry-cli-5pr",
		"go-foundry-cli-66w",
		"go-foundry-cli-7fm",
		"FOUNDRY_MATRIX_PHASE_EXIT",
		"planned",
		"implemented",
		"verified",
	} {
		ok := strings.Contains(body, needle)
		log.Assert("md_has_"+sanitize(needle), ok, true, ok)
	}
	log.PhaseEnd("assert_md", testutil.OutcomeOK)
}

// TestReqTraceabilityPhaseExit is the hard gate for exit reviews.
// Completeness always runs; verified-row requirements apply only when
// FOUNDRY_MATRIX_PHASE_EXIT is set (P1|P2|P3|P4|S6|DoD).
//
// Without the env var this test only re-checks completeness (seed-safe).
// With the env var it fails on non-verified rows in scope — used by 0z4/5pr/66w/7fm.
func TestReqTraceabilityPhaseExit(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	phase := archtest.PhaseExitFromEnv()
	strict := archtest.StrictPathsFromEnv(phase)
	log.Phase("phase_exit")
	log.Inputs(map[string]string{
		"phase_exit":   phase,
		"strict_paths": itoa(btoi(strict)),
	})

	rep, err := archtest.CheckReqTraceability(root, archtest.CheckReqTraceabilityOptions{
		PhaseExit:   phase,
		StrictPaths: strict,
		WarnOrphans: true,
	})
	if err != nil {
		log.Fail("check_error", err.Error())
	}
	t.Log("\n" + rep.String())

	// Completeness always required.
	if len(rep.Missing) > 0 || len(rep.Extra) > 0 || len(rep.Duplicates) > 0 || len(rep.TBDCells) > 0 {
		t.Fatalf("matrix completeness failed (sorted lists in log):\n%s", rep.String())
	}

	if phase == "" {
		// Seed mode: phase-exit not requested; pass on completeness.
		log.Step("phase_exit_skipped", testutil.OutcomeSkip, "set "+archtest.EnvMatrixPhaseExit+" for exit reviews")
		log.PhaseEnd("phase_exit", testutil.OutcomeOK)
		return
	}

	// Exit mode: hard-fail on unverified in scope (and TBD already failed above).
	log.Assert("no_unverified", len(rep.Unverified) == 0, 0, len(rep.Unverified))
	if strict {
		log.Assert("no_missing_paths", len(rep.MissingPaths) == 0, 0, len(rep.MissingPaths))
	}
	if !rep.OK {
		t.Fatalf("phase-exit matrix gate failed (%s):\n%s\n"+
			"Phase exits hard-block until the scoped REQs are status=verified with test paths.\n"+
			"DoD (7fm): FOUNDRY_MATRIX_PHASE_EXIT=DoD requires zero planned/TBD rows.",
			phase, rep.String())
	}
	log.PhaseEnd("phase_exit", testutil.OutcomeOK)
}

// TestReqTraceabilityDetectsGaps is a unit test of the checker using a
// temporary broken matrix (missing one REQ, extra bogus, TBD status).
func TestReqTraceabilityDetectsGaps(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("fixture_broken_matrix")

	// Work in a temp tree: copy real matrix and mutate.
	tmp := t.TempDir()
	// Minimal tree: go.mod marker + spec section + matrix.
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/robertguss/go-foundry-cli\n\ngo 1.26.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Copy real spec (needed for §53 parse) via symlink or read.
	specSrc := filepath.Join(root, archtest.SpecAuthorityPath)
	specData, err := os.ReadFile(specSrc)
	if err != nil {
		t.Fatal(err)
	}
	specDst := filepath.Join(tmp, archtest.SpecAuthorityPath)
	if err := os.MkdirAll(filepath.Dir(specDst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specDst, specData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Build a deliberately broken matrix from the real one.
	doc, err := archtest.LoadReqTraceDoc(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Requirements) < 3 {
		t.Fatal("need seeded matrix")
	}
	// Drop first REQ, corrupt second status to TBD, add extra.
	dropped := doc.Requirements[0].ID
	doc.Requirements = doc.Requirements[1:]
	doc.Requirements[0].Status = "TBD"
	tbdID := doc.Requirements[0].ID
	doc.Requirements = append(doc.Requirements, archtest.ReqTraceRow{
		ID:          "REQ-999",
		Phase:       "P1",
		OwningBeads: []string{"nobody"},
		Tests:       []string{"nope"},
		Status:      "planned",
		LastUpdate:  "2026-07-30",
	})

	raw, err := jsonMarshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	matPath := filepath.Join(tmp, archtest.DocReqTraceJSONPath)
	if err := os.MkdirAll(filepath.Dir(matPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(matPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	// Markdown twin stub.
	if err := os.WriteFile(filepath.Join(tmp, archtest.DocReqTraceMDPath), []byte("# stub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log.PhaseEnd("fixture_broken_matrix", testutil.OutcomeOK)

	log.Phase("assert_gaps")
	rep, err := archtest.CheckReqTraceability(tmp, archtest.CheckReqTraceabilityOptions{})
	if err != nil {
		log.Fail("check", err.Error())
	}
	t.Log("\n" + rep.String())
	log.Assert("not_ok", !rep.OK, true, !rep.OK)
	log.Assert("reports_missing", contains(rep.Missing, dropped), true, false)
	log.Assert("reports_extra", contains(rep.Extra, "REQ-999"), true, false)
	log.Assert("reports_tbd", len(rep.TBDCells) > 0, true, false)
	if !contains(rep.Missing, dropped) {
		t.Errorf("expected missing %s in sorted list %v", dropped, rep.Missing)
	}
	if !contains(rep.Extra, "REQ-999") {
		t.Errorf("expected extra REQ-999 in %v", rep.Extra)
	}
	if len(rep.TBDCells) == 0 {
		t.Errorf("expected TBD detection for %s", tbdID)
	}
	log.PhaseEnd("assert_gaps", testutil.OutcomeOK)
}

// TestReqTraceabilityDoDRejectsPlanned ensures DoD mode lists unverified rows
// when any planned/implemented status remains (fixture — production matrix may
// be fully verified after Stage 6 exit).
func TestReqTraceabilityDoDRejectsPlanned(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("dod_fixture")

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/robertguss/go-foundry-cli\n\ngo 1.26.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specSrc := filepath.Join(root, archtest.SpecAuthorityPath)
	specData, err := os.ReadFile(specSrc)
	if err != nil {
		t.Fatal(err)
	}
	specDst := filepath.Join(tmp, archtest.SpecAuthorityPath)
	if err := os.MkdirAll(filepath.Dir(specDst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specDst, specData, 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := archtest.LoadReqTraceDoc(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Requirements) == 0 {
		t.Fatal("need seeded matrix")
	}
	// Force one row back to planned so DoD must fail closed.
	doc.Requirements[0].Status = "planned"
	plannedID := doc.Requirements[0].ID

	raw, err := jsonMarshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	matPath := filepath.Join(tmp, archtest.DocReqTraceJSONPath)
	if err := os.MkdirAll(filepath.Dir(matPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(matPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, archtest.DocReqTraceMDPath), []byte("# stub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log.PhaseEnd("dod_fixture", testutil.OutcomeOK)

	log.Phase("dod")
	rep, err := archtest.CheckReqTraceability(tmp, archtest.CheckReqTraceabilityOptions{
		PhaseExit:   "DoD",
		StrictPaths: false,
	})
	if err != nil {
		log.Fail("check", err.Error())
	}
	t.Log("\n" + rep.String())
	log.Assert("dod_not_ok_while_planned", !rep.OK, true, !rep.OK)
	log.Assert("has_unverified", len(rep.Unverified) > 0, true, false)
	if rep.OK {
		t.Fatalf("DoD mode must fail while planned rows remain (got OK with %s planned)", plannedID)
	}
	if !contains(rep.Unverified, plannedID+":planned") {
		t.Fatalf("expected unverified %s:planned in %v", plannedID, rep.Unverified)
	}
	log.PhaseEnd("dod", testutil.OutcomeOK)
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func jsonMarshal(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
