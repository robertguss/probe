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

// TestSmokeCLIPlanGolden locks plan JSON for foundry-smoke-cli profiles=[]
// (REQ-066 / P2.6.c acceptance: plan golden).
//
// Update: UPDATE_GOLDEN=1 go test ./integration/fixtures -run TestSmokeCLIPlanGolden
func TestSmokeCLIPlanGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	log.Fixture("spec", "foundry-smoke-cli/foundry.toml")
	log.Fixture("golden", "foundry_smoke_cli_plan.golden")
	log.Inputs(map[string]string{
		"fixture":   "foundry-smoke-cli",
		"archetype": "cli",
		"profiles":  "[]",
		"verify":    "default",
	})
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	p, _ := mustSmokePlan(t)
	log.Step("file_count", testutil.OutcomeOK, "n="+itoa(p.FileCount()))
	log.Step("external_step_count", testutil.OutcomeOK, "n="+itoa(p.ExternalStepCount()))
	log.Step("plan_sha256", testutil.OutcomeOK, p.PlanSHA256())
	log.NoteID(p.PlanSHA256())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	golden := testutil.GoldenPath(filepath.Join("testdata"), "foundry_smoke_cli_plan")
	testutil.CompareGolden(t, golden, p.JSON())
	if !t.Failed() {
		log.Step("golden_compare", testutil.OutcomeOK, "path=foundry_smoke_cli_plan.golden")
	} else {
		log.Step("golden_compare", testutil.OutcomeFail, "mismatch")
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestSmokeCLITreeGolden locks path/mode/render/content_sha256 inventory
// (REQ-066 / P2.6.c acceptance: tree golden + digests in meta).
//
// Update: UPDATE_GOLDEN=1 go test ./integration/fixtures -run TestSmokeCLITreeGolden
func TestSmokeCLITreeGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	log.Fixture("golden", "foundry_smoke_cli_tree.golden")
	log.Fixture("meta", "foundry_smoke_cli_meta.golden")
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	p, _ := mustSmokePlan(t)
	tree := treeGoldenLines(p)
	meta := metaGolden(p)
	log.Step("plan_sha256", testutil.OutcomeOK, p.PlanSHA256())
	log.Step("content_digest_aggregate", testutil.OutcomeOK, aggregateContentDigest(p))
	log.Step("file_count", testutil.OutcomeOK, "n="+itoa(p.FileCount()))
	log.NoteID(p.PlanSHA256())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	treePath := testutil.GoldenPath(filepath.Join("testdata"), "foundry_smoke_cli_tree")
	metaPath := testutil.GoldenPath(filepath.Join("testdata"), "foundry_smoke_cli_meta")
	testutil.CompareGolden(t, treePath, tree)
	testutil.CompareGolden(t, metaPath, meta)
	if t.Failed() {
		// On mismatch dump path list for agents.
		var paths []string
		for _, f := range p.Files() {
			paths = append(paths, f.Path)
		}
		dumpPathList(t, "plan_files", paths)
		log.Step("golden_compare", testutil.OutcomeFail, "tree_or_meta_mismatch")
	} else {
		log.Step("golden_compare", testutil.OutcomeOK, "tree+meta")
	}
	// Core + CLI inventory size: 11 core + 9 cli + go.mod = 21.
	log.Assert("file_count_21", p.FileCount() == 21, 21, p.FileCount())
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestSmokeCLIAbsenceChecks ensures no greet/demo and no Foundry runtime dep
// in planned go.mod content (REQ-006 / FND-014 / P2.6.c).
func TestSmokeCLIAbsenceChecks(t *testing.T) {
	log := testutil.New(t)
	log.Phase("absence")
	p, cat := mustSmokePlan(t)

	// Path absence.
	for _, f := range p.Files() {
		low := strings.ToLower(f.Path)
		for _, bad := range []string{"greet", "demo", "viper"} {
			if strings.Contains(low, bad) {
				log.Fail("forbidden_path", f.Path)
			}
		}
	}

	// Content digests: render and scan bytes.
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
	log.Assert("no_foundry_runtime_import",
		!strings.Contains(string(gomod), "go-foundry-cli"),
		true, !strings.Contains(string(gomod), "go-foundry-cli"))

	var body strings.Builder
	for _, path := range mw.Paths() {
		b, _, _ := mw.Get(path)
		body.Write(b)
		body.WriteByte('\n')
	}
	content := strings.ToLower(body.String())
	for _, tok := range []string{
		"package greet",
		"package demo",
		"internal/greet",
		"func newgreet",
		"github.com/spf13/viper",
	} {
		present := strings.Contains(content, tok)
		if present {
			log.Fail("forbidden_content", tok)
		}
		log.Assert("absent_"+sanitize(tok), !present, true, !present)
	}
	log.Step("paths_scanned", testutil.OutcomeOK, "n="+itoa(mw.Len()))
	log.Step("plan_sha256", testutil.OutcomeOK, p.PlanSHA256())
	log.PhaseEnd("absence", testutil.OutcomeOK)
}

// TestDoubleGenerationByteEquality proves two pure plan+render passes from
// identical inputs yield identical plan_sha256 and content digests (REQ-010 /
// Section 32.1 envelope excluding .git / mtimes / stage names).
func TestDoubleGenerationByteEquality(t *testing.T) {
	log := testutil.New(t)
	log.Phase("double_generation_pure")

	p1, cat1 := mustSmokePlan(t)
	p2, cat2 := mustSmokePlan(t)

	log.Step("plan_sha256_1", testutil.OutcomeOK, p1.PlanSHA256())
	log.Step("plan_sha256_2", testutil.OutcomeOK, p2.PlanSHA256())
	log.Assert("plan_sha256_equal", p1.PlanSHA256() == p2.PlanSHA256(),
		p1.PlanSHA256(), p2.PlanSHA256())
	log.Assert("plan_json_equal", string(p1.JSON()) == string(p2.JSON()), true,
		string(p1.JSON()) == string(p2.JSON()))

	// Content digests from plan files.
	agg1 := aggregateContentDigest(p1)
	agg2 := aggregateContentDigest(p2)
	log.Step("content_digest_1", testutil.OutcomeOK, agg1)
	log.Step("content_digest_2", testutil.OutcomeOK, agg2)
	log.Assert("content_digest_equal", agg1 == agg2, agg1, agg2)

	// Render twice → identical MemoryWriter bytes.
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
	paths1, paths2 := w1.Paths(), w2.Paths()
	if len(paths1) != len(paths2) {
		dumpPathList(t, "render1", paths1)
		dumpPathList(t, "render2", paths2)
		log.Fail("path_set", "length mismatch")
	}
	for i := range paths1 {
		if paths1[i] != paths2[i] {
			dumpPathList(t, "render1", paths1)
			dumpPathList(t, "render2", paths2)
			log.Fail("path_order", paths1[i]+" vs "+paths2[i])
		}
		b1, _, _ := w1.Get(paths1[i])
		b2, _, _ := w2.Get(paths2[i])
		if string(b1) != string(b2) {
			log.Fail("bytes_"+paths1[i], "content mismatch")
		}
	}
	log.Step("rendered_files", testutil.OutcomeOK, "n="+itoa(w1.Len()))
	log.PhaseEnd("double_generation_pure", testutil.OutcomeOK)
}

// TestDoubleGenerationRealGenerate runs two full generate transactions to the
// same destination path (remove between) and asserts non-.git content digests
// + plan_sha256 equality within the Section 32.1 envelope (REQ-010).
// Skips when pinned Go toolchain is unavailable on the host.
func TestDoubleGenerationRealGenerate(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		log.Skip("pinned go1.26.5 toolchain not found on host")
		return
	}
	log.Phase("double_generation_real")
	log.Inputs(map[string]string{
		"go_binary": filepath.Base(goBin),
		"fixture":   "foundry-smoke-cli",
	})
	log.Step("go_binary", testutil.OutcomeOK, goBin)

	// Same absolute destination for both runs so plan_sha256 is comparable
	// (destination path is a plan input). Remove committed tree between runs.
	parent := privateParent(t)
	dest := filepath.Join(parent, "foundry-smoke-cli")

	sha1 := runGenerateTo(t, goBin, dest)
	d1, agg1, paths1 := nonGitTreeDigest(t, dest)
	log.Step("plan_sha256_1", testutil.OutcomeOK, sha1)
	log.Step("tree_digest_1", testutil.OutcomeOK, agg1)

	if err := os.RemoveAll(dest); err != nil {
		t.Fatalf("remove dest between generates: %v", err)
	}

	sha2 := runGenerateTo(t, goBin, dest)
	d2, agg2, paths2 := nonGitTreeDigest(t, dest)
	log.Step("plan_sha256_2", testutil.OutcomeOK, sha2)
	log.Step("tree_digest_2", testutil.OutcomeOK, agg2)

	log.Assert("plan_sha256_equal", sha1 == sha2, sha1, sha2)
	log.Assert("tree_aggregate_equal", agg1 == agg2, agg1, agg2)
	log.Assert("path_count_equal", len(paths1) == len(paths2), len(paths1), len(paths2))

	if agg1 != agg2 || len(paths1) != len(paths2) {
		dumpPathList(t, "gen1", paths1)
		dumpPathList(t, "gen2", paths2)
	}
	for _, p := range paths1 {
		if d1[p] != d2[p] {
			log.Fail("digest_"+p, d1[p]+" vs "+d2[p])
		}
	}
	// go.mod must not depend on Foundry.
	gomod, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		log.Fail("read_gomod", err.Error())
	}
	log.Assert("no_foundry_dep", !strings.Contains(string(gomod), "go-foundry-cli"),
		true, !strings.Contains(string(gomod), "go-foundry-cli"))
	log.PhaseEnd("double_generation_real", testutil.OutcomeOK)
}

// TestGenerateSmokeCLICompileTest generates once and runs go test on the result.
func TestGenerateSmokeCLICompileTest(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		log.Skip("pinned go1.26.5 toolchain not found on host")
		return
	}
	log.Phase("generate_compile")
	dest, sha := runGenerateOnce(t, goBin)
	log.Step("plan_sha256", testutil.OutcomeOK, sha)
	log.NotePath(dest)
	log.NoteID(sha)
	goTestClean(t, dest, goBin)
	log.Step("go_test", testutil.OutcomeOK, "ok")
	log.PhaseEnd("generate_compile", testutil.OutcomeOK)
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}
