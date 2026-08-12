package dogfood_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// productCell is one product E2E generate+dogfood cell (go-foundry-cli-79a.4/5).
type productCell struct {
	name       string // test name
	specRel    string // path under repo root, or empty if writeSpec used
	writeSpec  func(t *testing.T, dir string) (specPath, destBase string)
	destBase   string // destination basename (== project name)
	archetype  string // cli | tui
	distFiles  bool   // assert distribution scaffolding
	runCLIHelp bool   // run binary --help (and version for CLI)
}

// TestDogfoodProductMatrix generates each product-surface cell into a custody-safe
// parent, asserts distribution artifacts when requested, then full dogfood:
// go test ./..., go build, and for CLI run --help + version.
//
// Covers plan inventory C1/C3/C4/C5 and G1–G5 generate paths (bead
// go-foundry-cli-79a.5 / 79a.4). TUI interactive PTY remains under
// integration/tui with -tags=tui_pty (79a.6).
func TestDogfoodProductMatrix(t *testing.T) {
	skipIfShort(t)
	goBin := pinnedGoBinary(t)
	if goBin == "" {
		testutil.New(t).Skip("pinned go1.26.x toolchain not found on host")
		return
	}

	root := repoRoot(t)
	cells := []productCell{
		{
			name:       "examples_minimal_cli",
			specRel:    filepath.Join("examples", "minimal-cli.toml"),
			destBase:   "minimal-cli",
			archetype:  "cli",
			runCLIHelp: true,
		},
		{
			name:       "examples_minimal_tui",
			specRel:    filepath.Join("examples", "minimal-tui.toml"),
			destBase:   "minimal-tui",
			archetype:  "tui",
			runCLIHelp: false,
		},
		{
			name:       "examples_appendix_b_private_cli",
			specRel:    filepath.Join("examples", "appendix-b-private-cli.toml"),
			destBase:   "repo-map",
			archetype:  "cli",
			runCLIHelp: true,
		},
		{
			name:       "examples_appendix_b_private_tui",
			specRel:    filepath.Join("examples", "appendix-b-private-tui.toml"),
			destBase:   "worktree-status",
			archetype:  "tui",
			runCLIHelp: false,
		},
		{
			name:       "examples_appendix_b_public_cli_distribution",
			specRel:    filepath.Join("examples", "appendix-b-public-cli.toml"),
			destBase:   "repo-map",
			archetype:  "cli",
			distFiles:  true,
			runCLIHelp: true,
		},
		{
			name: "synthetic_tui_distribution",
			writeSpec: func(t *testing.T, dir string) (string, string) {
				t.Helper()
				name := "dist-tui"
				body := fmt.Sprintf(`schema = 1
name = %q
module = "github.com/example/%s"
description = "product e2e tui distribution"
archetype = "tui"
destination = "./%s"
visibility = "public"
profiles = ["distribution"]
`, name, name, name)
				path := filepath.Join(dir, "foundry.toml")
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatalf("write spec: %v", err)
				}
				return path, name
			},
			destBase:   "dist-tui",
			archetype:  "tui",
			distFiles:  true,
			runCLIHelp: false,
		},
	}

	for _, c := range cells {
		c := c
		t.Run(c.name, func(t *testing.T) {
			log := testutil.New(t)
			log.Phase("arrange")
			log.Inputs(map[string]string{
				"cell":      c.name,
				"archetype": c.archetype,
				"dist":      fmt.Sprintf("%v", c.distFiles),
				"go":        goBin,
			})
			opts := dogfoodOpts(t, goBin)
			parent := privateParent(t)
			work := t.TempDir()

			var specPath string
			destBase := c.destBase
			if c.writeSpec != nil {
				specPath, destBase = c.writeSpec(t, work)
			} else {
				specPath = filepath.Join(root, c.specRel)
				if _, err := os.Stat(specPath); err != nil {
					log.Fail("spec_missing", err.Error())
				}
			}
			dest := filepath.Join(parent, destBase)
			log.Fixture("spec", filepath.Base(specPath))
			log.NotePath(dest)
			log.PhaseEnd("arrange", testutil.OutcomeOK)

			// validate + plan (write-free product path)
			log.Phase("writefree")
			resV := runCLI(t, t.Context(), opts, "validate", "--spec", specPath, "--dest", dest)
			logProc(log, "validate", resV, 0)
			resP := runCLI(t, t.Context(), opts, "plan", "--spec", specPath, "--dest", dest)
			logProc(log, "plan", resP, 0)
			log.PhaseEnd("writefree", testutil.OutcomeOK)

			log.Phase("generate")
			resG := runCLI(t, t.Context(), opts,
				"generate", "--spec", specPath, "--dest", dest, "--verify", "default")
			logProc(log, "generate", resG, 0)
			if resG.Code != 0 {
				log.PhaseEnd("generate", testutil.OutcomeFail)
				return
			}
			if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
				log.Fail("go_mod", err.Error())
			}
			log.PhaseEnd("generate", testutil.OutcomeOK)

			if c.distFiles {
				log.Phase("distribution_artifacts")
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
						log.Step("artifact_"+filepath.ToSlash(rel), testutil.OutcomeOK, "present")
					}
				}
				log.PhaseEnd("distribution_artifacts", testutil.OutcomeOK)
			}

			log.Phase("dogfood")
			goTestClean(t, dest, goBin)
			log.Step("go_test", testutil.OutcomeOK, "green")

			bin := filepath.Join(dest, "bin-"+destBase)
			pkg := "./cmd/" + destBase
			goBuild(t, dest, goBin, pkg, bin)
			log.Step("go_build", testutil.OutcomeOK, pkg)

			if c.runCLIHelp {
				helpOut, _, helpCode := runBin(t, bin, "--help")
				log.Assert("help_exit", helpCode == 0, 0, helpCode)
				log.Assert("help_has_version", strings.Contains(helpOut, "version"),
					true, strings.Contains(helpOut, "version"))

				verOut, _, verCode := runBin(t, bin, "version")
				log.Assert("version_exit", verCode == 0, 0, verCode)
				log.Assert("version_has_go", strings.Contains(verOut, "go"),
					true, strings.Contains(verOut, "go"))
			} else {
				// TUI binary: process starts under no-TTY may exit non-zero; only
				// require that the binary is executable and non-empty.
				st, err := os.Stat(bin)
				log.Assert("tui_bin_exists", err == nil && st.Size() > 0, true, err == nil)
				log.Step("tui_bin", testutil.OutcomeOK, fmt.Sprintf("bytes=%d", st.Size()))
			}
			log.PhaseEnd("dogfood", testutil.OutcomeOK)
		})
	}
}

// TestDogfoodExamplesInvalidMatrix re-checks product invalid examples via the
// real CLI (JSON envelope) so product-e2e does not rely only on testscript
// (go-foundry-cli-79a.4). Full invalid catalog remains writefree validate_invalid.
func TestDogfoodExamplesInvalidMatrix(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	// Pure validate path — no generate tools required.
	var opts cli.Options

	root := repoRoot(t)
	cases := []struct {
		file string
		id   string
	}{
		{"invalid-unknown-field.toml", "spec.unknown_field"},
		{"invalid-bad-name.toml", "spec.invalid_field"},
		{"invalid-bad-profile.toml", "resolve.unknown_profile"},
	}
	log.Phase("invalid_examples")
	for _, c := range cases {
		c := c
		t.Run(c.file, func(t *testing.T) {
			tl := testutil.New(t)
			spec := filepath.Join(root, "examples", c.file)
			res := runCLI(t, t.Context(), opts, "validate", "--spec", spec, "--output", "json")
			logProc(tl, "validate_"+c.file, res, 2)
			tl.Assert("error_id", strings.Contains(res.Stdout, c.id), c.id, res.Stdout)
		})
	}
	log.PhaseEnd("invalid_examples", testutil.OutcomeOK)
}
