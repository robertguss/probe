package fixtures_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestSmokeTUIPlanGolden locks plan JSON for foundry-smoke-tui profiles=[]
// (bead go-foundry-cli-afh).
//
// Update: UPDATE_GOLDEN=1 go test ./integration/fixtures -run TestSmokeTUIPlanGolden
func TestSmokeTUIPlanGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	log.Fixture("spec", "foundry-smoke-tui/foundry.toml")
	log.Fixture("golden", "foundry_smoke_tui_plan.golden")
	log.Inputs(map[string]string{
		"fixture":   "foundry-smoke-tui",
		"archetype": "tui",
		"profiles":  "[]",
		"verify":    "default",
	})
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	p, _ := mustSmokeTUIPlan(t)
	log.Step("file_count", testutil.OutcomeOK, "n="+itoa(p.FileCount()))
	log.Step("external_step_count", testutil.OutcomeOK, "n="+itoa(p.ExternalStepCount()))
	log.Step("plan_sha256", testutil.OutcomeOK, p.PlanSHA256())
	log.NoteID(p.PlanSHA256())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	golden := testutil.GoldenPath(filepath.Join("testdata"), "foundry_smoke_tui_plan")
	testutil.CompareGolden(t, golden, p.JSON())
	if !t.Failed() {
		log.Step("golden_compare", testutil.OutcomeOK, "path=foundry_smoke_tui_plan.golden")
	} else {
		log.Step("golden_compare", testutil.OutcomeFail, "mismatch")
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestSmokeTUITreeGolden locks path/mode/render/content_sha256 inventory for TUI.
func TestSmokeTUITreeGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	log.Fixture("golden", "foundry_smoke_tui_tree.golden")
	log.Fixture("meta", "foundry_smoke_tui_meta.golden")
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	p, _ := mustSmokeTUIPlan(t)
	tree := treeGoldenLines(p)
	meta := metaGolden(p)
	log.Step("plan_sha256", testutil.OutcomeOK, p.PlanSHA256())
	log.Step("content_digest_aggregate", testutil.OutcomeOK, aggregateContentDigest(p))
	log.Step("file_count", testutil.OutcomeOK, "n="+itoa(p.FileCount()))
	log.NoteID(p.PlanSHA256())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	treePath := testutil.GoldenPath(filepath.Join("testdata"), "foundry_smoke_tui_tree")
	metaPath := testutil.GoldenPath(filepath.Join("testdata"), "foundry_smoke_tui_meta")
	testutil.CompareGolden(t, treePath, tree)
	testutil.CompareGolden(t, metaPath, meta)
	if t.Failed() {
		var paths []string
		for _, f := range p.Files() {
			paths = append(paths, f.Path)
		}
		dumpPathList(t, "plan_files", paths)
		log.Step("golden_compare", testutil.OutcomeFail, "tree_or_meta_mismatch")
	} else {
		log.Step("golden_compare", testutil.OutcomeOK, "tree+meta")
	}
	// Core 11 + TUI 11 + go.mod = 23.
	log.Assert("file_count_23", p.FileCount() == 23, 23, p.FileCount())
	// Section 18.2 TUI-owned paths present.
	wantPaths := []string{
		"cmd/foundry-smoke-tui/main.go",
		"docs/ui-architecture.md",
		"internal/tui/app.go",
		"internal/tui/state.go",
		"internal/tui/update.go",
		"internal/tui/view.go",
	}
	got := map[string]bool{}
	for _, f := range p.Files() {
		got[f.Path] = true
	}
	for _, w := range wantPaths {
		log.Assert("has_"+sanitize(w), got[w], true, got[w])
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestSmokeTUIAbsenceChecks ensures no demos, no multi-owner signals, no Foundry dep.
func TestSmokeTUIAbsenceChecks(t *testing.T) {
	log := testutil.New(t)
	log.Phase("absence")
	p, cat := mustSmokeTUIPlan(t)

	for _, f := range p.Files() {
		low := strings.ToLower(f.Path)
		for _, bad := range []string{"greet", "demo", "effects.go", "messages.go"} {
			if strings.Contains(low, bad) || strings.HasSuffix(low, bad) {
				log.Fail("forbidden_path", f.Path)
			}
		}
	}

	jobs, err := generate.JobsFromPlan(p, cat)
	if err != nil {
		log.Fail("jobs", err.Error())
	}
	mw := render.NewMemoryWriter()
	if _, err := render.RenderAll(cat, jobs, mw); err != nil {
		log.Fail("render", err.Error())
	}
	gomod, _, ok := mw.Get("go.mod")
	if !ok {
		log.Fail("go.mod_missing", "go.mod not rendered")
	}
	log.Assert("no_foundry_module",
		!strings.Contains(string(gomod), "github.com/robertguss/go-foundry-cli"),
		true, !strings.Contains(string(gomod), "github.com/robertguss/go-foundry-cli"))

	var body strings.Builder
	for _, path := range mw.Paths() {
		b, _, _ := mw.Get(path)
		body.Write(b)
		body.WriteByte('\n')
	}
	content := body.String()
	// Single lifecycle owner: NotifyContext only in main (not a second owner).
	mainBody, _, _ := mw.Get("cmd/foundry-smoke-tui/main.go")
	appBody, _, _ := mw.Get("internal/tui/app.go")
	mainHas := strings.Contains(string(mainBody), "signal.NotifyContext")
	// app.go may document the main-owned contract in comments; forbid a call site.
	appCallsNotify := strings.Contains(string(appBody), "signal.NotifyContext(")
	appWithCtx := strings.Contains(string(appBody), "tea.WithContext")
	appNoSig := strings.Contains(string(appBody), "tea.WithoutSignalHandler")
	log.Assert("main_has_notify", mainHas, true, mainHas)
	log.Assert("app_no_notify_call", !appCallsNotify, true, !appCallsNotify)
	log.Assert("app_with_context", appWithCtx, true, appWithCtx)
	log.Assert("app_without_signal_handler", appNoSig, true, appNoSig)
	for _, tok := range []string{"package greet", "package demo", "WithoutCatchPanics("} {
		present := strings.Contains(content, tok)
		if present {
			log.Fail("forbidden_content", tok)
		}
		log.Assert("absent_"+sanitize(tok), !present, true, !present)
	}
	// REQ-133: no hostpaths/usernames/timestamps in rendered tree (rough scan).
	low := strings.ToLower(content)
	for _, bad := range []string{"/home/", "/users/", "c:\\users\\"} {
		if strings.Contains(low, bad) {
			log.Fail("hostpath_leak", bad)
		}
	}
	log.Step("paths_scanned", testutil.OutcomeOK, "n="+itoa(mw.Len()))
	log.Step("plan_sha256", testutil.OutcomeOK, p.PlanSHA256())
	log.PhaseEnd("absence", testutil.OutcomeOK)
}

// TestDoubleGenerationTUIByteEquality pure plan+render double pass (REQ-010).
func TestDoubleGenerationTUIByteEquality(t *testing.T) {
	log := testutil.New(t)
	log.Phase("double_generation_pure_tui")

	p1, cat1 := mustSmokeTUIPlan(t)
	p2, cat2 := mustSmokeTUIPlan(t)

	log.Step("plan_sha256_1", testutil.OutcomeOK, p1.PlanSHA256())
	log.Step("plan_sha256_2", testutil.OutcomeOK, p2.PlanSHA256())
	log.Assert("plan_sha256_equal", p1.PlanSHA256() == p2.PlanSHA256(),
		p1.PlanSHA256(), p2.PlanSHA256())

	agg1 := aggregateContentDigest(p1)
	agg2 := aggregateContentDigest(p2)
	log.Assert("content_digest_equal", agg1 == agg2, agg1, agg2)

	jobs1, err := generate.JobsFromPlan(p1, cat1)
	if err != nil {
		log.Fail("jobs1", err.Error())
	}
	jobs2, err := generate.JobsFromPlan(p2, cat2)
	if err != nil {
		log.Fail("jobs2", err.Error())
	}
	w1 := render.NewMemoryWriter()
	w2 := render.NewMemoryWriter()
	if _, err := render.RenderAll(cat1, jobs1, w1); err != nil {
		log.Fail("render1", err.Error())
	}
	if _, err := render.RenderAll(cat2, jobs2, w2); err != nil {
		log.Fail("render2", err.Error())
	}
	log.Assert("path_count", w1.Len() == w2.Len(), w1.Len(), w2.Len())
	for i, path := range w1.Paths() {
		if path != w2.Paths()[i] {
			log.Fail("path_order", path)
		}
		b1, _, _ := w1.Get(path)
		b2, _, _ := w2.Get(path)
		if string(b1) != string(b2) {
			log.Fail("bytes_"+path, "mismatch")
		}
	}
	log.Step("rendered_files", testutil.OutcomeOK, "n="+itoa(w1.Len()))
	log.PhaseEnd("double_generation_pure_tui", testutil.OutcomeOK)
}

// TestDoubleGenerationTUIRealGenerate two full generate transactions (REQ-010).
func TestDoubleGenerationTUIRealGenerate(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		log.Skip("pinned go1.26.5 toolchain not found on host")
		return
	}
	log.Phase("double_generation_real_tui")
	parent := privateParent(t)
	dest := filepath.Join(parent, "foundry-smoke-tui")

	sha1 := runGenerateSmokeTUI(t, goBin, dest)
	d1, agg1, paths1 := nonGitTreeDigest(t, dest)
	log.Step("plan_sha256_1", testutil.OutcomeOK, sha1)
	log.Step("tree_digest_1", testutil.OutcomeOK, agg1)

	if err := os.RemoveAll(dest); err != nil {
		t.Fatalf("remove dest: %v", err)
	}

	sha2 := runGenerateSmokeTUI(t, goBin, dest)
	d2, agg2, paths2 := nonGitTreeDigest(t, dest)
	log.Step("plan_sha256_2", testutil.OutcomeOK, sha2)
	log.Step("tree_digest_2", testutil.OutcomeOK, agg2)

	log.Assert("plan_sha256_equal", sha1 == sha2, sha1, sha2)
	log.Assert("tree_aggregate_equal", agg1 == agg2, agg1, agg2)
	log.Assert("path_count_equal", len(paths1) == len(paths2), len(paths1), len(paths2))
	for _, p := range paths1 {
		if d1[p] != d2[p] {
			log.Fail("digest_"+p, d1[p]+" vs "+d2[p])
		}
	}
	gomod, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		log.Fail("read_gomod", err.Error())
	}
	log.Assert("no_foundry_dep", !strings.Contains(string(gomod), "go-foundry-cli"),
		true, !strings.Contains(string(gomod), "go-foundry-cli"))
	log.PhaseEnd("double_generation_real_tui", testutil.OutcomeOK)
}

// TestGenerateSmokeTUICompileTest generates once and runs go test.
func TestGenerateSmokeTUICompileTest(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		log.Skip("pinned go1.26.5 toolchain not found on host")
		return
	}
	log.Phase("generate_compile_tui")
	parent := privateParent(t)
	dest := filepath.Join(parent, "foundry-smoke-tui")
	sha := runGenerateSmokeTUI(t, goBin, dest)
	log.Step("plan_sha256", testutil.OutcomeOK, sha)
	log.NotePath(dest)
	log.NoteID(sha)
	goTestClean(t, dest, goBin)
	log.Step("go_test", testutil.OutcomeOK, "ok")
	log.PhaseEnd("generate_compile_tui", testutil.OutcomeOK)
}
