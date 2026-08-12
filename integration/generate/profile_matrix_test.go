package generatee2e_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// writeDistributionSpec writes a public CLI spec with a GitHub-hosted module
// path and profiles=["distribution"] (Section 20 / REQ-076/077).
func writeDistributionSpec(t *testing.T, dir, name string) string {
	t.Helper()
	body := fmt.Sprintf(`schema = 1
name = %q
module = "github.com/example/%s"
description = "generate e2e fixture distribution profile"
archetype = "cli"
destination = "./%s"
visibility = "public"
profiles = ["distribution"]
`, name, name, name)
	path := filepath.Join(dir, "foundry.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// writeRecipeOnlyProfileSpec writes a spec that selects a retired
// recipe-only profile ID (REQ-073/075).
func writeRecipeOnlyProfileSpec(t *testing.T, dir, name, profileID string) string {
	t.Helper()
	body := fmt.Sprintf(`schema = 1
name = %q
module = "github.com/example/%s"
description = "generate e2e fixture recipe-only profile rejection"
archetype = "cli"
destination = "./%s"
profiles = [%q]
`, name, name, name, profileID)
	path := filepath.Join(dir, "foundry.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// TestMatrixDistributionProfile_PublicSpec generates a public,
// GitHub-hosted-module spec with profiles=["distribution"] and asserts
// success plus the profile's contributed artifacts on disk (bead
// go-foundry-cli-wet.3.3 / Section 20).
func TestMatrixDistributionProfile_PublicSpec(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("distribution_profile")

	parent := privateParent(t)
	work := t.TempDir()
	specPath := writeDistributionSpec(t, work, "dist-cli")
	dest := filepath.Join(parent, "dist-cli")
	log.Fixture("spec", filepath.Base(specPath))
	log.NotePath(dest)

	res := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", specPath, "--dest", dest, "--output", "json")
	logProc(log, "generate_distribution", res, 0)
	if res.Code != 0 {
		dumpFailure(t, log, failureFromProc("distribution_profile", "commit", "generate_failed", res, "", dest))
		log.PhaseEnd("distribution_profile", testutil.OutcomeFail)
		return
	}
	env := mustEnvelope(t, log, res.Stdout)
	log.Assert("ok", env["ok"] == true, true, env["ok"])
	log.Assert("commit_outcome", commitOutcomeFromEnvelope(env) == "committed",
		"committed", commitOutcomeFromEnvelope(env))

	for _, rel := range []string{
		"docs/releasing.md",
		"CONTRIBUTING.md",
		"SECURITY.md",
		filepath.Join(".github", "workflows", "release.yml"),
		filepath.Join(".github", "workflows", "dependency-review.yml"),
		".goreleaser.yaml",
	} {
		p := filepath.Join(dest, rel)
		if _, err := os.Stat(p); err != nil {
			log.Fail("artifact_"+rel, err.Error())
		} else {
			log.Step("artifact_"+rel, testutil.OutcomeOK, "present")
		}
	}
	log.PhaseEnd("distribution_profile", testutil.OutcomeOK)
}

// TestMatrixRecipeOnlyProfile_Rejection selects a retired recipe-only
// profile ID and asserts the CLI fails closed with resolve.unknown_profile,
// exit usage, and no destination placement (bead go-foundry-cli-wet.3.3 /
// REQ-073/075).
func TestMatrixRecipeOnlyProfile_Rejection(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("recipe_only_rejection")

	parent := privateParent(t)
	work := t.TempDir()
	specPath := writeRecipeOnlyProfileSpec(t, work, "recipe-cli", "configuration")
	dest := filepath.Join(parent, "recipe-cli")
	log.Fixture("spec", filepath.Base(specPath))
	log.NotePath(dest)

	res := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", specPath, "--dest", dest, "--output", "json")
	logProc(log, "generate_recipe_only", res, diagnostic.ExitUsage)

	env := mustEnvelope(t, log, res.Stdout)
	log.Assert("ok_false", env["ok"] == false, false, env["ok"])
	id := errorIDFromEnvelope(env)
	log.Assert("error_id", id == string(diagnostic.IDResolveUnknownProfile),
		string(diagnostic.IDResolveUnknownProfile), id)

	if _, err := os.Stat(dest); err == nil {
		log.Fail("dest_placed", "recipe-only profile rejection must not place destination")
	} else {
		log.Assert("no_dest", os.IsNotExist(err), true, err)
	}
	log.PhaseEnd("recipe_only_rejection", testutil.OutcomeOK)
}
