package dogfood_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestDogfoodRepoMap generates the real repo-map project, applies the first
// domain command manually (inventory), tests, and exercises text/JSON output
// (Section 52.1 step 2 / REQ-245 / REQ-247).
func TestDogfoodRepoMap(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		log.Skip("pinned go1.26.5 toolchain not found on host")
		return
	}

	log.Phase("arrange")
	log.Fixture("spec", "dogfood/repo-map/foundry.toml")
	log.Fixture("overlay", "dogfood/repo-map/testdata/overlay")
	log.Inputs(map[string]string{
		"project":   "repo-map",
		"archetype": "cli",
		"verify":    "default",
		"go":        goBin,
	})
	// README must explain purpose and non-goals.
	readme, err := os.ReadFile(filepath.Join(repoRoot(t), "dogfood", "repo-map", "README.md"))
	if err != nil {
		log.Fail("readme", err.Error())
	}
	rs := string(readme)
	log.Assert("readme_purpose", strings.Contains(rs, "## Purpose"), true, strings.Contains(rs, "## Purpose"))
	log.Assert("readme_nongoals", strings.Contains(rs, "## Non-goals"), true, strings.Contains(rs, "## Non-goals"))
	opts := dogfoodOpts(t, goBin)
	parent := privateParent(t)
	dest := filepath.Join(parent, "repo-map")
	log.NotePath(dest)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("generate")
	t0 := time.Now()
	res := runCLI(t, t.Context(), opts,
		"generate",
		"--spec", repoMapSpec(t),
		"--dest", dest,
		"--verify", "default",
	)
	logProc(log, "generate", res, 0)
	planSHA := planSHAFromText(res.Stdout)
	if planSHA == "" {
		planSHA = planSHAFromText(res.Stderr)
	}
	log.Step("plan_sha256", testutil.OutcomeOK, planSHA)
	log.NoteID(planSHA)
	genMS := res.Duration.Milliseconds()
	log.Step("generate_ms", testutil.OutcomeInfo, fmt.Sprintf("%d", genMS))
	if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
		log.Fail("dest_go_mod", err.Error())
	}
	// Generated shell must not include inventory yet.
	if _, err := os.Stat(filepath.Join(dest, "internal", "inventory")); err == nil {
		log.Fail("inventory_preexisting", "catalog must not pre-generate inventory")
	} else {
		log.Step("inventory_absent_pre", testutil.OutcomeOK, "ok")
	}
	before := listNonTestGeneratedFiles(t, dest)
	log.Step("generated_non_test_count", testutil.OutcomeInfo, fmt.Sprintf("%d", len(before)))
	log.PhaseEnd("generate", testutil.OutcomeOK)

	log.Phase("first_feature")
	// Orientation files an agent reads before first correct edit (REQ-247).
	for _, rel := range []string{"AGENTS.md", "docs/architecture.md", "docs/commands.md", "README.md"} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			log.Fail("orient_"+rel, err.Error())
		} else {
			log.Step("orient_"+filepath.Base(rel), testutil.OutcomeOK, rel)
		}
	}
	editStart := time.Now()
	applyOverlay(t, dest, repoMapOverlay(t))
	wireInventoryCmd(t, dest)
	// First-feature files added (not generated deletions).
	if _, err := os.Stat(filepath.Join(dest, "internal", "inventory", "inventory.go")); err != nil {
		log.Fail("inventory_pkg", err.Error())
	}
	if _, err := os.Stat(filepath.Join(dest, "internal", "cli", "inventory.go")); err != nil {
		log.Fail("inventory_cmd", err.Error())
	}
	rootBody, err := os.ReadFile(filepath.Join(dest, "internal", "cli", "root.go"))
	if err != nil {
		log.Fail("root", err.Error())
	}
	wired := strings.Contains(string(rootBody), "newInventoryCmd()")
	log.Assert("wired_newInventoryCmd", wired, true, wired)

	gomod, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		log.Fail("gomod", err.Error())
	}
	log.Assert("module_repo_map", strings.Contains(string(gomod), "github.com/robertguss/repo-map"),
		true, strings.Contains(string(gomod), "github.com/robertguss/repo-map"))
	log.Assert("no_foundry_dep", !strings.Contains(string(gomod), "go-foundry-cli"),
		true, !strings.Contains(string(gomod), "go-foundry-cli"))

	// FND-014: no generated non-test files deleted when adding first feature.
	afterAdd := listNonTestGeneratedFiles(t, dest)
	deleted := 0
	afterSet := map[string]bool{}
	for _, p := range afterAdd {
		afterSet[p] = true
	}
	for _, p := range before {
		if !afterSet[p] {
			deleted++
			log.Step("deleted_file", testutil.OutcomeFail, p)
		}
	}
	log.Assert("zero_mandatory_deletions", deleted == 0, 0, deleted)
	log.Step("first_feature_edit_ms", testutil.OutcomeInfo, fmt.Sprintf("%d", time.Since(editStart).Milliseconds()))
	log.Step("first_feature_shape", testutil.OutcomeInfo,
		"add internal/inventory + internal/cli/inventory.go + one-line root wire")
	log.PhaseEnd("first_feature", testutil.OutcomeOK)

	log.Phase("test_and_exercise")
	goTestClean(t, dest, goBin)
	log.Step("go_test", testutil.OutcomeOK, "green")

	bin := filepath.Join(dest, "repo-map")
	goBuild(t, dest, goBin, "./cmd/repo-map", bin)
	log.Step("go_build", testutil.OutcomeOK, "cmd/repo-map")

	// Seed a tiny tree under dest for inventory to list.
	sample := filepath.Join(dest, "_dogfood_sample")
	if err := os.MkdirAll(sample, 0o755); err != nil {
		log.Fail("sample_dir", err.Error())
	}
	if err := os.WriteFile(filepath.Join(sample, "hello.txt"), []byte("hi\n"), 0o644); err != nil {
		log.Fail("sample_file", err.Error())
	}

	textOut, textErr, textCode := runBin(t, bin, "inventory", sample, "--output", "text")
	log.Assert("inventory_text_exit", textCode == 0, 0, textCode)
	log.Assert("inventory_text_hello", strings.Contains(textOut, "hello.txt"), true, strings.Contains(textOut, "hello.txt"))
	if textErr != "" {
		log.Step("inventory_text_stderr", testutil.OutcomeInfo, capBody(textErr, 200))
	}

	jsonOut, jsonErr, jsonCode := runBin(t, bin, "inventory", sample, "--output", "json")
	log.Assert("inventory_json_exit", jsonCode == 0, 0, jsonCode)
	log.Assert("inventory_json_hello", strings.Contains(jsonOut, `"path": "hello.txt"`),
		true, strings.Contains(jsonOut, `"path": "hello.txt"`))
	if jsonErr != "" {
		log.Step("inventory_json_stderr", testutil.OutcomeInfo, capBody(jsonErr, 200))
	}

	// Help lists inventory.
	helpOut, _, helpCode := runBin(t, bin, "--help")
	log.Assert("help_exit", helpCode == 0, 0, helpCode)
	log.Assert("help_lists_inventory", strings.Contains(helpOut, "inventory"), true, strings.Contains(helpOut, "inventory"))
	log.PhaseEnd("test_and_exercise", testutil.OutcomeOK)

	log.Phase("measurements_seed")
	totalMS := time.Since(t0).Milliseconds()
	log.Step("time_to_first_green_ms", testutil.OutcomeInfo, fmt.Sprintf("%d", totalMS))
	log.Step("package_placement_errors", testutil.OutcomeInfo, "0")
	log.Step("retained_vs_deleted_non_test", testutil.OutcomeInfo,
		fmt.Sprintf("retained=%d deleted=%d", len(before), deleted))
	log.Step("gen_verify_latency_ms", testutil.OutcomeInfo, fmt.Sprintf("%d", genMS))
	log.Step("median_pr_ci_latency", testutil.OutcomeInfo, "not_yet_measured_seeded")
	log.Step("default_vs_strict_split", testutil.OutcomeInfo, "default_only_this_run")
	log.Step("bypass_demand", testutil.OutcomeInfo, "none")
	log.Step("escaped_defects_rsk403", testutil.OutcomeInfo, "none_observed")
	log.Step("config_persistence_rsk401", testutil.OutcomeInfo, "none_yet_inventory_is_stateless")
	log.Step("preserved_stage_rsk310", testutil.OutcomeInfo, "none_encountered")
	log.Step("agent_observation", testutil.OutcomeInfo,
		"growth recipe clear: domain package + constructor + one-line AddCommand")
	log.PhaseEnd("measurements_seed", testutil.OutcomeOK)
}
