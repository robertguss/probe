package generatee2e_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// writeGitBranchSpec writes a minimal CLI spec with a non-default
// git.initial_branch (bead go-foundry-cli-wet.3.4).
func writeGitBranchSpec(t *testing.T, dir, name, branch string) string {
	t.Helper()
	body := fmt.Sprintf(`schema = 1
name = %q
module = "github.com/example/%s"
description = "generate e2e fixture custom initial branch"
archetype = "cli"
destination = "./%s"
profiles = []
[git]
init = true
initial_branch = %q
`, name, name, name, branch)
	path := filepath.Join(dir, "foundry.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// TestMatrixCustomInitialBranch generates with git.initial_branch != "main"
// and asserts the real generated repository's HEAD matches (bead
// go-foundry-cli-wet.3.4 / Generate Integration Matrix Expansion).
func TestMatrixCustomInitialBranch(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("custom_branch")

	parent := privateParent(t)
	work := t.TempDir()
	const branch = "develop"
	specPath := writeGitBranchSpec(t, work, "branch-cli", branch)
	dest := filepath.Join(parent, "branch-cli")
	log.Fixture("spec", filepath.Base(specPath))
	log.NotePath(dest)

	res := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", specPath, "--dest", dest, "--output", "json")
	logProc(log, "generate_branch", res, 0)
	if res.Code != 0 {
		dumpFailure(t, log, failureFromProc("custom_branch", "commit", "generate_failed", res, "", dest))
		log.PhaseEnd("custom_branch", testutil.OutcomeFail)
		return
	}
	env := mustEnvelope(t, log, res.Stdout)
	log.Assert("ok", env["ok"] == true, true, env["ok"])
	log.Assert("commit_outcome", commitOutcomeFromEnvelope(env) == "committed",
		"committed", commitOutcomeFromEnvelope(env))

	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	cmd := exec.Command(gitBin, "symbolic-ref", "--short", "HEAD")
	cmd.Dir = dest
	out, err := cmd.Output()
	if err != nil {
		log.Fail("symbolic_ref", err.Error())
	}
	got := strings.TrimSpace(string(out))
	log.Assert("head_branch", got == branch, branch, got)
	log.PhaseEnd("custom_branch", testutil.OutcomeOK)
}

// TestMatrixCustomGitTemplateDir_HostHooksAbsent proves generate's isolated
// git init ignores a hostile ambient git template configuration — both
// GIT_TEMPLATE_DIR and a poisoned global gitconfig's init.templateDir — and
// never executes or copies its hooks/content into the generated repository
// (bead go-foundry-cli-wet.3.4). This is the full-CLI companion to the
// toolrun/ConstructGitEnv-level coverage in
// integration/hostile/sentinel (REQ-214): Foundry's git env is fully closed
// (GIT_CONFIG_GLOBAL=/dev/null, GIT_CONFIG_NOSYSTEM=1, its own owned empty
// scratch GIT_TEMPLATE_DIR), so ambient poisoning must have no effect.
func TestMatrixCustomGitTemplateDir_HostHooksAbsent(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("hostile_template")

	hostTemplate := t.TempDir()
	marker := filepath.Join(hostTemplate, "marker.log")
	hooksDir := filepath.Join(hostTemplate, "hooks")
	if err := os.MkdirAll(hooksDir, 0o700); err != nil {
		t.Fatal(err)
	}
	hook := "#!/bin/sh\necho host-template-hook-ran >> \"" + marker + "\"\n"
	for _, name := range []string{"post-checkout", "pre-commit"} {
		if err := os.WriteFile(filepath.Join(hooksDir, name), []byte(hook), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sentinelFile := filepath.Join(hostTemplate, "HOSTILE-TEMPLATE-SENTINEL.txt")
	if err := os.WriteFile(sentinelFile, []byte("do-not-copy\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Poison ambient git configuration two ways.
	t.Setenv("GIT_TEMPLATE_DIR", hostTemplate)
	fakeGlobalConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(fakeGlobalConfig,
		[]byte("[init]\n\ttemplateDir = "+hostTemplate+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", fakeGlobalConfig)

	parent := privateParent(t)
	specPath := examplesSpec(t, "minimal-cli.toml")
	dest := filepath.Join(parent, "minimal-cli")
	log.Fixture("spec", filepath.Base(specPath))
	log.NotePath(dest)

	res := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", specPath, "--dest", dest, "--output", "json")
	logProc(log, "generate_hostile_template", res, 0)
	if res.Code != 0 {
		dumpFailure(t, log, failureFromProc("hostile_template", "commit", "generate_failed", res, "", dest))
		log.PhaseEnd("hostile_template", testutil.OutcomeFail)
		return
	}
	env := mustEnvelope(t, log, res.Stdout)
	log.Assert("ok", env["ok"] == true, true, env["ok"])

	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		log.Fail("git_present", err.Error())
	}

	for _, name := range []string{"post-checkout", "pre-commit"} {
		p := filepath.Join(dest, ".git", "hooks", name)
		data, err := os.ReadFile(p)
		switch {
		case err == nil:
			log.Assert("hook_not_hostile_"+name,
				!strings.Contains(string(data), "host-template-hook-ran"), true, string(data))
		case os.IsNotExist(err):
			log.Step("hook_absent_"+name, testutil.OutcomeOK, "not present (Foundry owned empty template)")
		default:
			log.Fail("hook_read_"+name, err.Error())
		}
	}

	if _, err := os.Stat(marker); err == nil {
		log.Fail("hostile_hook_executed", "marker file exists: hostile host template hook ran")
	} else {
		log.Step("marker_untouched", testutil.OutcomeOK, "hostile hooks never ran")
	}

	if _, err := os.Stat(filepath.Join(dest, "HOSTILE-TEMPLATE-SENTINEL.txt")); err == nil {
		log.Fail("hostile_template_leak", "sentinel file copied into generated tree")
	} else {
		log.Step("no_template_leak", testutil.OutcomeOK, "hostile template content absent from generated tree")
	}
	log.PhaseEnd("hostile_template", testutil.OutcomeOK)
}
