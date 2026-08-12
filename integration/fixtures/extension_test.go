package fixtures_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestExtensionPathSubcommand proves Foundry-owned extension-path overlay
// compiles and tests when applied to a Foundry-generated smoke-cli tree
// (REQ-066 / P2.6.c acceptance: extension path proven).
func TestExtensionPathSubcommand(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		log.Skip("pinned go1.26.5 toolchain not found on host")
		return
	}

	log.Phase("arrange")
	log.Fixture("spec", "foundry-smoke-cli/foundry.toml")
	log.Fixture("overlay", "extension-cli-subcommand/testdata/overlay")
	log.Fixture("readme", "extension-cli-subcommand/README.md")
	// Fixture README must explain purpose and non-goals.
	readme, err := os.ReadFile(fixtureDir(t, "extension-cli-subcommand/README.md"))
	if err != nil {
		log.Fail("readme", err.Error())
	}
	rs := string(readme)
	hasPurpose := strings.Contains(rs, "## Purpose")
	hasNonGoals := strings.Contains(rs, "## Non-goals")
	hasNever := strings.Contains(strings.ToLower(rs), "never") || strings.Contains(rs, "not generated")
	log.Assert("readme_purpose", hasPurpose, true, hasPurpose)
	log.Assert("readme_nongoals", hasNonGoals, true, hasNonGoals)
	log.Assert("readme_not_generated", hasNever, true, hasNever)
	// Top-level fixtures README.
	top, err := os.ReadFile(fixtureDir(t, "README.md"))
	if err != nil {
		log.Fail("top_readme", err.Error())
	}
	topPurpose := strings.Contains(string(top), "## Purpose")
	topNonGoals := strings.Contains(string(top), "## Non-goals")
	log.Assert("top_readme_purpose", topPurpose, true, topPurpose)
	log.Assert("top_readme_nongoals", topNonGoals, true, topNonGoals)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("generate")
	dest, sha := runGenerateOnce(t, goBin)
	log.Step("plan_sha256", testutil.OutcomeOK, sha)
	log.NoteID(sha)
	log.NotePath(dest)
	log.PhaseEnd("generate", testutil.OutcomeOK)

	log.Phase("extend")
	applyExtensionOverlay(t, dest)
	// Overlay must introduce domain package + command without Foundry dep.
	if _, err := os.Stat(filepath.Join(dest, "internal", "ping", "ping.go")); err != nil {
		log.Fail("ping_pkg", err.Error())
	}
	if _, err := os.Stat(filepath.Join(dest, "internal", "cli", "ping.go")); err != nil {
		log.Fail("ping_cmd", err.Error())
	}
	rootBody, err := os.ReadFile(filepath.Join(dest, "internal", "cli", "root.go"))
	if err != nil {
		log.Fail("root", err.Error())
	}
	wired := strings.Contains(string(rootBody), "newPingCmd()")
	log.Assert("wired_newPingCmd", wired, true, wired)
	gomod, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		log.Fail("gomod", err.Error())
	}
	log.Assert("no_foundry_dep", !strings.Contains(string(gomod), "go-foundry-cli"),
		true, !strings.Contains(string(gomod), "go-foundry-cli"))
	// Still no greet/demo packages.
	low := strings.ToLower(string(rootBody))
	log.Assert("no_greet", !strings.Contains(low, "greet"), true, !strings.Contains(low, "greet"))
	log.PhaseEnd("extend", testutil.OutcomeOK)

	log.Phase("test")
	goTestClean(t, dest, goBin)
	log.Step("go_test", testutil.OutcomeOK, "extension_green")
	log.PhaseEnd("test", testutil.OutcomeOK)
}

// TestFixtureREADMEsPresent is a cheap always-on check that fixture docs exist.
func TestFixtureREADMEsPresent(t *testing.T) {
	log := testutil.New(t)
	for _, rel := range []string{
		"README.md",
		"foundry-smoke-cli/foundry.toml",
		"extension-cli-subcommand/README.md",
		"extension-cli-subcommand/testdata/overlay/internal/ping/ping.go",
		"extension-cli-subcommand/testdata/overlay/internal/cli/ping.go",
	} {
		p := fixtureDir(t, rel)
		if _, err := os.Stat(p); err != nil {
			log.Fail("missing_"+rel, err.Error())
		} else {
			log.Step("present_"+filepath.Base(rel), testutil.OutcomeOK, rel)
		}
	}
}
