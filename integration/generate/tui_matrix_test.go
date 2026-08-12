package generatee2e_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// writeTUIGitFalseSpec writes a minimal TUI spec with [git] init=false.
func writeTUIGitFalseSpec(t *testing.T, dir, name string) string {
	t.Helper()
	body := fmt.Sprintf(`schema = 1
name = %q
module = "github.com/example/%s"
description = "generate e2e fixture tui git.init=false"
archetype = "tui"
destination = "./%s"
profiles = []
[git]
init = false
`, name, name, name)
	path := filepath.Join(dir, "foundry.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// TestMatrixSuccessTUIVerifyGit covers success cells for the TUI archetype:
//
//	archetype=tui × verify={default,strict} × git={true,false}
//
// (bead go-foundry-cli-wet.3.2). Mirrors TestMatrixSuccessVerifyGit's CLI
// coverage: process-tree ids, plan_sha256, commit outcome, go.mod/.git
// presence, generated go test, REQ-133 scan.
func TestMatrixSuccessTUIVerifyGit(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("matrix_success_tui")

	type cell struct {
		name    string
		verify  string
		gitInit bool
		spec    func(t *testing.T, work string) (specPath, destBase string)
	}
	cells := []cell{
		{
			name: "default_git_true", verify: "default", gitInit: true,
			spec: func(t *testing.T, _ string) (string, string) {
				return examplesSpec(t, "minimal-tui.toml"), "minimal-tui"
			},
		},
		{
			name: "default_git_false", verify: "default", gitInit: false,
			spec: func(t *testing.T, work string) (string, string) {
				return writeTUIGitFalseSpec(t, work, "nogit-tui"), "nogit-tui"
			},
		},
		{
			name: "strict_git_true", verify: "strict", gitInit: true,
			spec: func(t *testing.T, _ string) (string, string) {
				return examplesSpec(t, "minimal-tui.toml"), "minimal-tui"
			},
		},
		{
			name: "strict_git_false", verify: "strict", gitInit: false,
			spec: func(t *testing.T, work string) (string, string) {
				return writeTUIGitFalseSpec(t, work, "nogit-tui"), "nogit-tui"
			},
		},
	}

	for _, c := range cells {
		c := c
		t.Run(c.name, func(t *testing.T) {
			tl := testutil.New(t)
			tl.Phase(c.name)
			tl.Inputs(map[string]string{
				"archetype": "tui",
				"verify":    c.verify,
				"git_init":  boolStr(c.gitInit),
			})

			parent := privateParent(t)
			work := t.TempDir()
			specPath, destBase := c.spec(t, work)
			dest := filepath.Join(parent, destBase)
			tl.NotePath(dest)
			tl.Fixture("spec", filepath.Base(specPath))

			ids, planSHA := planExternalStepIDs(t, tl, specPath, dest, c.verify)
			want := wantToolIDs(c.verify, c.gitInit)
			tl.Assert("process_tree_ids", generate.StepIDsMatchPlan(want, ids),
				strings.Join(want, ","), strings.Join(ids, ","))
			tl.Step("plan_sha256", testutil.OutcomeOK, planSHA)
			tl.NoteID(planSHA)

			args := []string{"generate", "--spec", specPath, "--dest", dest, "--output", "json"}
			if c.verify == "strict" {
				args = append(args, "--verify", "strict")
			}
			res := runCLI(t, context.Background(), cli.Options{}, nil, args...)
			logProc(tl, "generate_"+c.name, res, 0)
			if res.Code != 0 {
				dumpFailure(t, tl, failureFromProc(c.name, "commit", "generate_failed", res, planSHA, dest))
				tl.PhaseEnd(c.name, testutil.OutcomeFail)
				return
			}
			env := mustEnvelope(t, tl, res.Stdout)
			tl.Assert("ok", env["ok"] == true, true, env["ok"])
			gotSHA := planSHAFromEnvelope(env)
			tl.Assert("plan_sha256_eq", gotSHA == planSHA, planSHA, gotSHA)
			tl.Assert("commit_outcome", commitOutcomeFromEnvelope(env) == "committed",
				"committed", commitOutcomeFromEnvelope(env))

			if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
				tl.Fail("go.mod", err.Error())
			}
			_, gitErr := os.Stat(filepath.Join(dest, ".git"))
			if c.gitInit {
				tl.Assert("git_present", gitErr == nil, true, gitErr == nil)
			} else {
				tl.Assert("git_absent", os.IsNotExist(gitErr), true, gitErr)
			}

			_, agg, paths := nonGitTreeDigest(t, dest)
			tl.Step("tree_digest", testutil.OutcomeOK, agg)
			tl.Step("path_count", testutil.OutcomeOK, fmt.Sprintf("%d", len(paths)))
			scanContentREQ133(t, tl, dest)
			goTestGenerated(t, tl, dest)

			tl.PhaseEnd(c.name, testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("matrix_success_tui", testutil.OutcomeOK)
}

// TestMatrixDoubleGenerationTUI proves two successive TUI generates produce
// equal digests + plan_sha256 (REQ-010, TUI companion to
// TestMatrixDoubleGeneration; bead go-foundry-cli-wet.3.2).
func TestMatrixDoubleGenerationTUI(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("double_generation_tui")

	work := t.TempDir()
	spec := writeTUIGitFalseSpec(t, work, "dbl-tui")
	parent := privateParent(t)
	dest := filepath.Join(parent, "dbl-tui")

	runOne := func(label string) (sha, agg string) {
		res := runCLI(t, context.Background(), cli.Options{}, nil,
			"generate", "--spec", spec, "--dest", dest, "--output", "json")
		logProc(log, "generate_"+label, res, 0)
		if res.Code != 0 {
			dumpFailure(t, log, failureFromProc("double_gen_tui_"+label, "commit", "generate_failed", res, "", dest))
			log.Fail("generate_"+label, capBody(res.Stderr+res.Stdout, 400))
		}
		env := mustEnvelope(t, log, res.Stdout)
		sha = planSHAFromEnvelope(env)
		_, agg, _ = nonGitTreeDigest(t, dest)
		log.Step(label+"_plan_sha256", testutil.OutcomeOK, sha)
		log.Step(label+"_tree_digest", testutil.OutcomeOK, agg)
		return sha, agg
	}

	sha1, agg1 := runOne("a")
	if err := os.RemoveAll(dest); err != nil {
		log.Fail("remove", err.Error())
	}
	sha2, agg2 := runOne("b")

	log.Assert("plan_sha256_equal", sha1 == sha2, sha1, sha2)
	log.Assert("tree_digest_equal", agg1 == agg2, agg1, agg2)
	log.PhaseEnd("double_generation_tui", testutil.OutcomeOK)
}
