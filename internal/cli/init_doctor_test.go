package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func TestInitWritesValidSpec(t *testing.T) {
	log := testutil.New(t)
	log.Phase("init_write")
	dir := t.TempDir()
	out := filepath.Join(dir, "my-cli.toml")
	res := runCLI(t,
		"init",
		"--out", out,
		"--name", "my-cli",
		"--module", "github.com/example/my-cli",
		"--description", "Example CLI",
	)
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	log.Assert("mentions_wrote", strings.Contains(res.Stdout, "init: wrote"), true, res.Stdout)
	body, err := os.ReadFile(out)
	if err != nil {
		log.Fail("read_out", err.Error())
	}
	text := string(body)
	for _, want := range []string{
		`schema = 1`,
		`name = "my-cli"`,
		`module = "github.com/example/my-cli"`,
		`archetype = "cli"`,
		`profiles = []`,
	} {
		log.Assert("body_"+want, strings.Contains(text, want), true, text)
	}
	// validate accepts the written spec
	vres := runCLI(t, "validate", "--spec", out)
	log.Assert("validate_0", vres.Code == 0, 0, vres.Code)
	log.PhaseEnd("init_write", testutil.OutcomeOK)
}

func TestInitRefusesOverwrite(t *testing.T) {
	log := testutil.New(t)
	log.Phase("init_no_overwrite")
	dir := t.TempDir()
	out := filepath.Join(dir, "exists.toml")
	if err := os.WriteFile(out, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := runCLI(t,
		"init",
		"--out", out,
		"--name", "my-cli",
		"--module", "github.com/example/my-cli",
	)
	log.Assert("exit_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	log.Assert("mentions_exists", strings.Contains(res.Stderr, "already exists"), true, res.Stderr)
	log.PhaseEnd("init_no_overwrite", testutil.OutcomeOK)
}

func TestInitRejectsBadArchetype(t *testing.T) {
	log := testutil.New(t)
	log.Phase("init_bad_archetype")
	out := filepath.Join(t.TempDir(), "x.toml")
	res := runCLI(t,
		"init",
		"--out", out,
		"--name", "my-cli",
		"--module", "github.com/example/my-cli",
		"--archetype", "web",
	)
	log.Assert("exit_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		log.Fail("wrote_anyway", out)
	}
	log.PhaseEnd("init_bad_archetype", testutil.OutcomeOK)
}

func TestDoctorReportsPin(t *testing.T) {
	log := testutil.New(t)
	log.Phase("doctor")
	res := runCLI(t, "doctor")
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	log.Assert("has_pin", strings.Contains(res.Stdout, "catalog_go_pin: "+toolrun.DefaultPinnedGoTag), true, res.Stdout)
	log.Assert("has_guidance", strings.Contains(res.Stdout, "guidance:"), true, res.Stdout)
	log.Assert("has_foundry_go_bin", strings.Contains(res.Stdout, "FOUNDRY_GO_BIN:"), true, res.Stdout)

	resJSON := runCLI(t, "doctor", "--output", "json")
	log.Assert("json_0", resJSON.Code == 0, 0, resJSON.Code)
	log.Assert("json_pin", strings.Contains(resJSON.Stdout, `"catalog_go_pin"`), true, resJSON.Stdout)
	log.PhaseEnd("doctor", testutil.OutcomeOK)
}

func TestPlanTextIsRicher(t *testing.T) {
	log := testutil.New(t)
	log.Phase("plan_text")
	spec := examplesPath(t, "minimal-cli.toml")
	res := runCLI(t, "plan", "--spec", spec)
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	for _, want := range []string{
		"plan: project=",
		"files:",
		"dependencies:",
		"external_steps:",
		"verify:",
		"network:",
		"git:",
		"go_pin:",
		"FOUNDRY_GO_BIN",
	} {
		log.Assert("has_"+want, strings.Contains(res.Stdout, want), true, res.Stdout)
	}
	vres := runCLI(t, "plan", "--spec", spec, "--verbose")
	log.Assert("verbose_0", vres.Code == 0, 0, vres.Code)
	log.Assert("verbose_tools", strings.Contains(vres.Stdout, "tools:"), true, vres.Stdout)
	log.Assert("verbose_content_sha", strings.Contains(vres.Stdout, "content_sha256="), true, vres.Stdout)
	log.PhaseEnd("plan_text", testutil.OutcomeOK)
}

func TestGenerateSummaryWording(t *testing.T) {
	log := testutil.New(t)
	log.Phase("generate_summary_wording")
	out := cli.FormatGenerateSuccessForTest(report.GenerateResult{
		Destination: "demo",
		PlanSHA256:  "abc",
	})
	log.Assert("wrote", strings.Contains(out, "generate: wrote destination=demo"), true, out)
	log.Assert("not_committed_word", !strings.Contains(out, "generate: committed"), true, out)
	log.Assert("git_note", strings.Contains(out, "git initialized (no commits)"), true, out)
	log.PhaseEnd("generate_summary_wording", testutil.OutcomeOK)
}
