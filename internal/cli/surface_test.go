package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/version"
)

func TestPublicCommandsRegistered(t *testing.T) {
	log := testutil.New(t)
	log.Phase("public_commands")
	root := cli.NewRoot(cli.Options{SkipCatalogLoad: true})

	// Public surface: init, validate, plan, generate, catalog, doctor, version;
	// catalog contributes list+show.
	wantTop := map[string]bool{
		"init":     false,
		"validate": false,
		"plan":     false,
		"generate": false,
		"catalog":  false,
		"doctor":   false,
		"version":  false,
	}
	for _, c := range root.Commands() {
		if c.Hidden || c.Name() == "help" {
			continue
		}
		if _, ok := wantTop[c.Name()]; ok {
			wantTop[c.Name()] = true
		} else {
			log.Fail("extra_command", c.Name())
		}
	}
	for name, ok := range wantTop {
		log.Assert("cmd_"+name, ok, true, ok)
	}

	// catalog list + catalog show
	cat, _, err := root.Find([]string{"catalog"})
	if err != nil {
		log.Fail("find_catalog", err.Error())
	}
	subWant := map[string]bool{"list": false, "show": false}
	for _, c := range cat.Commands() {
		if c.Hidden {
			continue
		}
		if _, ok := subWant[c.Name()]; ok {
			subWant[c.Name()] = true
		} else {
			log.Fail("extra_catalog_cmd", c.Name())
		}
	}
	for name, ok := range subWant {
		log.Assert("catalog_"+name, ok, true, ok)
	}

	names := cli.PublicCommandNames(root)
	log.Step("public_names", testutil.OutcomeInfo, strings.Join(names, ","))
	log.PhaseEnd("public_commands", testutil.OutcomeOK)
}

func TestHelpTeachesPlanIsDryRun(t *testing.T) {
	log := testutil.New(t)
	log.Phase("plan_dry_run_help")

	// Root help
	res := runCLI(t, "--help")
	log.Assert("root_exit_0", res.Code == 0, 0, res.Code)
	rootHelp := res.Stdout
	log.Assert("root_teaches_dry_run",
		strings.Contains(rootHelp, "authoritative dry run"),
		true, rootHelp)
	log.Assert("root_no_separate",
		strings.Contains(rootHelp, "no separate dry run"),
		true, rootHelp)

	// Plan help
	res = runCLI(t, "plan", "--help")
	log.Assert("plan_exit_0", res.Code == 0, 0, res.Code)
	planHelp := res.Stdout
	log.Assert("plan_teaches",
		strings.Contains(planHelp, "authoritative dry run"),
		true, planHelp)

	// Banned surface tokens must not appear in help.
	for _, tok := range bannedSurfaceTokens() {
		name := strings.TrimPrefix(tok, "--")
		log.Assert("root_no_"+name, !strings.Contains(rootHelp, tok), false, strings.Contains(rootHelp, tok))
		log.Assert("plan_no_"+name, !strings.Contains(planHelp, tok), false, strings.Contains(planHelp, tok))
	}
	log.PhaseEnd("plan_dry_run_help", testutil.OutcomeOK)
}

func TestHelpGoldens(t *testing.T) {
	log := testutil.New(t)
	log.Phase("help_goldens")

	// Capture help with fixed options; strip host-dependent nothing (help is stable).
	cases := []struct {
		name string
		args []string
	}{
		{name: "root", args: []string{"--help"}},
		{name: "init", args: []string{"init", "--help"}},
		{name: "plan", args: []string{"plan", "--help"}},
		{name: "validate", args: []string{"validate", "--help"}},
		{name: "generate", args: []string{"generate", "--help"}},
		{name: "doctor", args: []string{"doctor", "--help"}},
		{name: "version", args: []string{"version", "--help"}},
		{name: "catalog", args: []string{"catalog", "--help"}},
	}
	dir := filepath.Join("testdata")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := testutil.New(t)
			res := runCLI(t, tc.args...)
			sub.Assert("exit_0", res.Code == 0, 0, res.Code)
			got := normalizeHelp(res.Stdout)
			// Help must not contain absolute host homes.
			sub.Assert("no_home", !strings.Contains(got, "/home/"), false, strings.Contains(got, "/home/"))
			path := testutil.GoldenPath(dir, "help_"+tc.name)
			testutil.CompareGolden(t, path, []byte(got))
			sub.Step("golden", testutil.OutcomeOK, path)
		})
	}
	log.PhaseEnd("help_goldens", testutil.OutcomeOK)
}

func TestSpecStdinAcceptedAtFlagLayer(t *testing.T) {
	log := testutil.New(t)
	log.Phase("spec_stdin")
	// Full pipeline: --spec - with fixture body on stdin (P1.7.b).
	body := readExample(t, "minimal-cli.toml")
	for _, cmd := range []string{"validate", "plan"} {
		stdin := bytes.NewReader(body)
		res := runCLICtx(t, context.Background(), stdin, cmd, "--spec", "-")
		log.Step(cmd, testutil.OutcomeOK, "exit="+itoa(res.Code))
		log.Assert(cmd+"_exit_0", res.Code == 0, 0, res.Code)
		if cmd == "validate" {
			log.Assert(cmd+"_mentions_stdin",
				strings.Contains(res.Stdout, "stdin") || strings.Contains(res.Stdout, `"spec":"-"`),
				true, res.Stdout)
		} else {
			log.Assert(cmd+"_schema",
				strings.Contains(res.Stdout, `"schema"`) || strings.Contains(res.Stdout, "plan:"),
				true, res.Stdout)
		}
	}
	// generate accepts --spec - (body may still fail decode on empty stdin).
	// Flag parse must not reject "-".
	res := runCLI(t, "generate", "--spec", "-")
	// Empty stdin → decode/validation failure (exit 2), not unknown-flag.
	log.Assert("generate_stdin_not_unknown_flag", res.Code != 0, true, res.Code)
	log.Assert("generate_not_flag_parse",
		!strings.Contains(res.Stderr, "unknown flag"),
		true, res.Stderr)
	log.PhaseEnd("spec_stdin", testutil.OutcomeOK)
}

func TestGenerateWiredNotHardStop(t *testing.T) {
	log := testutil.New(t)
	log.Phase("generate_wired")
	// Missing path → usage/read error, not Phase-1 hard-stop prose.
	res := runCLI(t, "generate", "--spec", "foundry-does-not-exist.toml")
	log.Assert("exit_nonzero", res.Code != 0, true, res.Code)
	log.Assert("no_phase1_hardstop",
		!strings.Contains(res.Stderr, "not available until Phase 2") &&
			!strings.Contains(res.Stdout, "not available until Phase 2"),
		true, res.Stderr+res.Stdout)
	// Valid fixture with injected happy stages succeeds.
	spec := examplesPath(t, "minimal-cli.toml")
	opts := testOptions()
	opts.GenerateStages = map[generate.StageID]generate.StageFunc{
		generate.StageCreateStage: generate.CreateStageFunc(".foundry-demo-deadbeef"),
		generate.StageCommit: generate.CommitStage(
			generate.OutcomeCommitted, "demo", ".foundry-demo-deadbeef", nil,
		),
	}
	res = runCLIOpts(t, context.Background(), nil, opts, "generate", "--spec", spec)
	log.Assert("wired_exit_0", res.Code == 0, 0, res.Code)
	log.Assert("progress_present", strings.Contains(res.Stdout, "progress:"), true, res.Stdout)
	log.PhaseEnd("generate_wired", testutil.OutcomeOK)
}

func TestVersionTextAndJSON(t *testing.T) {
	log := testutil.New(t)
	log.Phase("version")

	res := runCLI(t, "version")
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	for _, field := range []string{"foundry ", "commit:", "go:", "catalog_digest:", "catalog_go_pin:", "FOUNDRY_GO_BIN:"} {
		log.Assert("text_"+field, strings.Contains(res.Stdout, strings.TrimSpace(field)), true, res.Stdout)
	}

	res = runCLI(t, "version", "--output", "json")
	log.Assert("json_exit", res.Code == 0, 0, res.Code)
	log.Assert("json_stderr_empty", res.Stderr == "", "", res.Stderr)

	var env map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
		log.Fail("json_parse", err.Error())
	}
	log.Assert("ok", env["ok"] == true, true, env["ok"])
	log.Assert("command", env["command"] == "version", "version", env["command"])
	result, _ := env["result"].(map[string]any)
	if result == nil {
		log.Fail("result", "missing result object")
	}
	for _, k := range []string{"version", "commit", "go", "catalog_digest"} {
		_, ok := result[k]
		log.Assert("field_"+k, ok, true, ok)
	}
	// Catalog digest should be non-empty hex from embed.
	dig, _ := result["catalog_digest"].(string)
	log.Assert("digest_len", len(dig) == 64, 64, len(dig))
	log.PhaseEnd("version", testutil.OutcomeOK)
}

func TestExitCodeTable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("exit_codes")

	// 0 — success paths
	log.Assert("version_0", runCLI(t, "version").Code == 0, 0, runCLI(t, "version").Code)
	log.Assert("help_0", runCLI(t, "--help").Code == 0, 0, runCLI(t, "--help").Code)
	spec := examplesPath(t, "minimal-cli.toml")
	log.Assert("validate_0", runCLI(t, "validate", "--spec", spec).Code == 0, 0, 0)
	log.Assert("plan_0", runCLI(t, "plan", "--spec", spec).Code == 0, 0, 0)
	log.Assert("catalog_list_0", runCLI(t, "catalog", "list").Code == 0, 0, 0)

	// 2 — usage
	log.Assert("missing_spec_2", runCLI(t, "validate").Code == diagnostic.ExitUsage, diagnostic.ExitUsage, runCLI(t, "validate").Code)
	log.Assert("unknown_cmd_2", runCLI(t, "not-a-real-command").Code == diagnostic.ExitUsage, diagnostic.ExitUsage, runCLI(t, "not-a-real-command").Code)
	log.Assert("doctor_0", runCLI(t, "doctor").Code == 0, 0, runCLI(t, "doctor").Code)
	log.Assert("quiet_verbose_2", runCLI(t, "version", "--quiet", "--verbose").Code == diagnostic.ExitUsage, diagnostic.ExitUsage, 2)
	// Missing/invalid generate spec still exits 2 (usage/read).
	log.Assert("generate_missing_spec_2", runCLI(t, "generate", "--spec", "x-missing-spec.toml").Code == diagnostic.ExitUsage, diagnostic.ExitUsage, 2)
	// bad verify fails at flag validation before pipeline body (exit 2).
	log.Assert("bad_verify_2", runCLI(t, "plan", "--spec", spec, "--verify", "none").Code == diagnostic.ExitUsage, diagnostic.ExitUsage, 2)

	// 1 — catalog.invalid (unknown unit)
	res := runCLI(t, "catalog", "show", "does-not-exist-unit")
	log.Assert("catalog_show_unknown_1", res.Code == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Code)
	log.Assert("catalog_id", strings.Contains(res.Stderr, "catalog.invalid"), true, res.Stderr)

	// 130 — cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res = runCLICtx(t, ctx, nil, "version")
	log.Assert("cancelled_130", res.Code == diagnostic.ExitCancelled, diagnostic.ExitCancelled, res.Code)

	log.PhaseEnd("exit_codes", testutil.OutcomeOK)
}

func TestRootNoArgsHelpExit0(t *testing.T) {
	log := testutil.New(t)
	res := runCLI(t)
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	log.Assert("has_usage", strings.Contains(res.Stdout, "Usage:") || strings.Contains(res.Stdout, "foundry"), true, res.Stdout)
}

func TestVersionPackageRead(t *testing.T) {
	log := testutil.New(t)
	info := version.Read()
	log.Assert("version_nonempty", info.Version != "", true, info.Version)
	log.Assert("go_nonempty", info.Go != "", true, info.Go)
	// Default when unstamped
	if info.Version == "" {
		log.Fail("version", "empty")
	}
	with := info.WithCatalog("deadbeef")
	log.Assert("with_catalog", with.CatalogDigest == "deadbeef", "deadbeef", with.CatalogDigest)
	log.Assert("base_unchanged", info.CatalogDigest == "", "", info.CatalogDigest)
}

func TestSpecRequiredExplicit(t *testing.T) {
	log := testutil.New(t)
	// No implicit discovery: bare validate fails.
	res := runCLI(t, "validate")
	log.Assert("exit_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
}

func TestQuietSuppressesText(t *testing.T) {
	log := testutil.New(t)
	res := runCLI(t, "version", "--quiet")
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	log.Assert("stdout_empty", strings.TrimSpace(res.Stdout) == "", "", res.Stdout)
}
