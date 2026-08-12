package dogfood_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestDogfoodSmokeCLI generates foundry-smoke-cli on the real platform, runs
// its tests, builds the binary, and exercises help/version (Section 52.1 step 1).
func TestDogfoodSmokeCLI(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		log.Skip("pinned go1.26.5 toolchain not found on host")
		return
	}

	log.Phase("arrange")
	log.Fixture("spec", "integration/fixtures/foundry-smoke-cli/foundry.toml")
	log.Inputs(map[string]string{
		"project":   "foundry-smoke-cli",
		"archetype": "cli",
		"verify":    "default",
		"go":        goBin,
	})
	opts := dogfoodOpts(t, goBin)
	parent := privateParent(t)
	dest := filepath.Join(parent, "foundry-smoke-cli")
	log.NotePath(dest)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	// --- cold generate (first placement into this parent) ---
	log.Phase("generate_cold")
	res := runCLI(t, t.Context(), opts,
		"generate",
		"--spec", smokeSpec(t),
		"--dest", dest,
		"--verify", "default",
	)
	logProc(log, "generate_cold", res, 0)
	planSHA := planSHAFromText(res.Stdout)
	if planSHA == "" {
		// try stderr too
		planSHA = planSHAFromText(res.Stderr)
	}
	log.Step("plan_sha256", testutil.OutcomeOK, planSHA)
	log.NoteID(planSHA)
	if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
		log.Fail("dest_go_mod", err.Error())
	}
	// No preserved stage expected on success.
	if strings.Contains(res.Stdout+res.Stderr, "stage_path") &&
		strings.Contains(strings.ToLower(res.Stdout+res.Stderr), "preserved") {
		log.Step("stage_preserve", testutil.OutcomeInfo, "unexpected preserve mention on success")
	} else {
		log.Step("stage_preserve", testutil.OutcomeOK, "none")
	}
	coldMS := res.Duration.Milliseconds()
	log.Step("cold_generate_ms", testutil.OutcomeInfo, fmt.Sprintf("%d", coldMS))
	log.PhaseEnd("generate_cold", testutil.OutcomeOK)

	// --- warm generate into a sibling parent (host caches warm) ---
	log.Phase("generate_warm")
	parent2 := privateParent(t)
	dest2 := filepath.Join(parent2, "foundry-smoke-cli")
	resW := runCLI(t, t.Context(), opts,
		"generate",
		"--spec", smokeSpec(t),
		"--dest", dest2,
		"--verify", "default",
	)
	logProc(log, "generate_warm", resW, 0)
	warmMS := resW.Duration.Milliseconds()
	log.Step("warm_generate_ms", testutil.OutcomeInfo, fmt.Sprintf("%d", warmMS))
	log.PhaseEnd("generate_warm", testutil.OutcomeOK)

	// --- exercise generated project ---
	log.Phase("exercise")
	before := listNonTestGeneratedFiles(t, dest)
	log.Step("non_test_file_count", testutil.OutcomeInfo, fmt.Sprintf("%d", len(before)))

	goTestClean(t, dest, goBin)
	log.Step("go_test", testutil.OutcomeOK, "green")

	bin := filepath.Join(dest, "foundry-smoke-cli")
	goBuild(t, dest, goBin, "./cmd/foundry-smoke-cli", bin)
	log.Step("go_build", testutil.OutcomeOK, "cmd/foundry-smoke-cli")

	helpOut, helpErr, helpCode := runBin(t, bin, "--help")
	log.Assert("help_exit", helpCode == 0, 0, helpCode)
	log.Assert("help_has_version", strings.Contains(helpOut, "version"), true, strings.Contains(helpOut, "version"))
	if helpErr != "" {
		log.Step("help_stderr", testutil.OutcomeInfo, capBody(helpErr, 200))
	}

	verOut, verErr, verCode := runBin(t, bin, "version")
	log.Assert("version_exit", verCode == 0, 0, verCode)
	log.Assert("version_has_go", strings.Contains(verOut, "go"), true, strings.Contains(verOut, "go"))
	if verErr != "" {
		log.Step("version_stderr", testutil.OutcomeInfo, capBody(verErr, 200))
	}

	// FND-014: zero mandatory deletions of generated non-test files for smoke.
	// Build may add a local binary; only deletions of pre-existing generated files count.
	after := listNonTestGeneratedFiles(t, dest)
	afterSet := map[string]bool{}
	for _, p := range after {
		afterSet[p] = true
	}
	deleted := 0
	for _, p := range before {
		if !afterSet[p] {
			deleted++
			log.Step("deleted_file", testutil.OutcomeFail, p)
		}
	}
	log.Assert("zero_mandatory_deletions", deleted == 0, 0, deleted)
	log.Step("deleted_non_test", testutil.OutcomeOK, fmt.Sprintf("%d", deleted))
	log.PhaseEnd("exercise", testutil.OutcomeOK)

	log.Phase("measurements_seed")
	// REQ-247 seed rows (full longitudinal tracking continues in evidence docs).
	log.Step("orientation_files", testutil.OutcomeInfo, "AGENTS.md,docs/architecture.md,docs/commands.md,README.md")
	log.Step("first_feature", testutil.OutcomeInfo, "none_for_smoke_disposable")
	log.Step("gen_verify_cold_ms", testutil.OutcomeInfo, fmt.Sprintf("%d", coldMS))
	log.Step("gen_verify_warm_ms", testutil.OutcomeInfo, fmt.Sprintf("%d", warmMS))
	log.Step("default_vs_strict", testutil.OutcomeInfo, "default_only_this_run")
	log.Step("bypass_demand", testutil.OutcomeInfo, "none")
	log.Step("preserved_stage_cleanup", testutil.OutcomeInfo, "none_encountered")
	log.PhaseEnd("measurements_seed", testutil.OutcomeOK)
}

// isNetworkRelated reports whether a failure message looks like a transient or
// absent-network failure when running govulncheck. It is intentionally broad so
// local dev machines without network skip cleanly; CI runs with network.
func isNetworkRelated(msg string) bool {
	m := strings.ToLower(msg)
	for _, tok := range []string{
		"connection refused", "connection reset", "timeout", "no such host",
		"temporary failure", "network is unreachable", "x509:", "certificate",
		"dial tcp", "i/o timeout", "no route to host",
	} {
		if strings.Contains(m, tok) {
			return true
		}
	}
	return false
}

// TestDogfoodSmokeCLI_StrictVerify exercises the strict verification path
// (staticcheck + govulncheck) on a real generate. The govulncheck step may
// require outbound network access to fetch the vulnerability DB; when that
// fails for network reasons the test skips rather than failing (REQ-247).
func TestDogfoodSmokeCLI_StrictVerify(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		log.Skip("pinned go1.26.x toolchain not found on host")
		return
	}

	// Confirm govulncheck is on PATH. Do not require network here.
	if _, err := exec.LookPath("govulncheck"); err != nil {
		log.Skip("govulncheck not on PATH: " + err.Error())
		return
	}

	log.Phase("arrange")
	log.Fixture("spec", "integration/fixtures/foundry-smoke-cli/foundry.toml")
	log.Inputs(map[string]string{
		"project":   "foundry-smoke-cli",
		"archetype": "cli",
		"verify":    "strict",
		"go":        goBin,
	})
	opts := dogfoodOpts(t, goBin)
	parent := privateParent(t)
	// The destination basename must equal the project name in the spec.
	dest := filepath.Join(parent, "foundry-smoke-cli")
	log.NotePath(dest)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("generate_strict")
	res := runCLI(t, t.Context(), opts,
		"generate",
		"--spec", smokeSpec(t),
		"--dest", dest,
		"--verify", "strict",
	)
	logProc(log, "generate_strict", res, 0)

	if res.Code != 0 {
		if isNetworkRelated(res.Stderr + res.Stdout) {
			log.Skip("strict verify skipped: govulncheck network dependency unavailable")
			return
		}
		// logProc already failed the test for the non-zero exit.
		return
	}

	log.Phase("exercise")
	if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
		log.Fail("dest_go_mod", err.Error())
	}
	goTestClean(t, dest, goBin)
	log.Step("go_test", testutil.OutcomeOK, "green")

	bin := filepath.Join(dest, "foundry-smoke-cli")
	goBuild(t, dest, goBin, "./cmd/foundry-smoke-cli", bin)
	log.Step("go_build", testutil.OutcomeOK, "cmd/foundry-smoke-cli")

	helpOut, helpErr, helpCode := runBin(t, bin, "--help")
	log.Assert("help_exit", helpCode == 0, 0, helpCode)
	log.Assert("help_has_version", strings.Contains(helpOut, "version"), true, strings.Contains(helpOut, "version"))
	if helpErr != "" {
		log.Step("help_stderr", testutil.OutcomeInfo, capBody(helpErr, 200))
	}
	log.PhaseEnd("exercise", testutil.OutcomeOK)
}
