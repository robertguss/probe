package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func TestExitErrorStringerAndAsExit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("exit_error")

	nilStr := cli.ExitErrorString(0, true)
	log.Assert("nil_recv", nilStr == "exit", "exit", nilStr)

	s0 := cli.ExitErrorString(0, false)
	log.Assert("code0", s0 == "exit 0", "exit 0", s0)
	s2 := cli.ExitErrorString(2, false)
	log.Assert("code2", s2 == "exit 2", "exit 2", s2)
	s130 := cli.ExitErrorString(130, false)
	log.Assert("code130", s130 == "exit 130", "exit 130", s130)

	code, ok := cli.AsExitErrorForTest(nil)
	log.Assert("nil_err_ok", !ok && code == 0, false, ok)

	code, ok = cli.AsExitErrorForTest(errors.New("plain"))
	log.Assert("plain_ok", !ok && code == 0, false, ok)

	err := cli.ExitWithForTest(1)
	code, ok = cli.AsExitErrorForTest(err)
	log.Assert("exit_with", ok && code == 1, 1, code)
	log.Assert("err_string", err.Error() == "exit 1", "exit 1", err.Error())

	log.Step("exit_error", testutil.OutcomeOK, "stringer+asExit covered")
	log.PhaseEnd("exit_error", testutil.OutcomeOK)
}

func TestIsTerminalFalsePaths(t *testing.T) {
	log := testutil.New(t)
	log.Phase("is_terminal")

	log.Assert("nil_file", !cli.IsTerminalForTest(nil), false, cli.IsTerminalForTest(nil))

	// Regular file is never a char device.
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-tty")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	log.Assert("regular_file", !cli.IsTerminalForTest(f), false, cli.IsTerminalForTest(f))

	// Closed file: Stat fails → false.
	closed, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	log.Assert("closed_file", !cli.IsTerminalForTest(closed), false, cli.IsTerminalForTest(closed))

	log.Step("is_terminal", testutil.OutcomeOK, "nil+regular+closed=false")
	log.PhaseEnd("is_terminal", testutil.OutcomeOK)
}

func TestEnvelopeEncoderEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("envelope")

	var stdout, stderr bytes.Buffer

	// nil globalFlags → text defaults.
	enc := cli.NewEncoderForTest(&stdout, &stderr, nil)
	log.Assert("nil_flags", enc != nil, true, enc != nil)

	// JSON mode forces ColorNever and clears quiet/verbose.
	gJSON := cli.NewGlobalFlagsForTest(cli.OutputJSON, cli.ColorAlways, true, true)
	enc = cli.NewEncoderForTest(&stdout, &stderr, gJSON)
	log.Assert("json_enc", enc != nil, true, enc != nil)

	// Text + ColorAlways / ColorNever / ColorAuto.
	for _, color := range []string{cli.ColorAlways, cli.ColorNever, cli.ColorAuto} {
		g := cli.NewGlobalFlagsForTest(cli.OutputText, color, false, false)
		enc = cli.NewEncoderForTest(&stdout, &stderr, g)
		log.Assert("text_"+color, enc != nil, true, enc != nil)
	}

	// stderr as *os.File exercises the type-assert + isTerminal branch.
	f, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	g := cli.NewGlobalFlagsForTest(cli.OutputText, cli.ColorAuto, false, true)
	enc = cli.NewEncoderForTest(&stdout, f, g)
	log.Assert("file_stderr", enc != nil, true, enc != nil)

	// NO_COLOR env path.
	t.Setenv("NO_COLOR", "1")
	enc = cli.NewEncoderForTest(&stdout, &stderr, cli.NewGlobalFlagsForTest(cli.OutputText, cli.ColorAuto, false, false))
	log.Assert("nocolor", enc != nil, true, enc != nil)
	t.Setenv("NO_COLOR", "")

	log.Step("envelope", testutil.OutcomeOK, "json/text/color/file/nocolor")
	log.PhaseEnd("envelope", testutil.OutcomeOK)
}

func TestGeneratePipelineHostPinPreference(t *testing.T) {
	log := testutil.New(t)
	log.Phase("generate_pipeline_host")

	// 1) Explicit injected GoBinary wins — pin discovery must not replace it.
	injected := filepath.Join(t.TempDir(), "injected-go")
	if err := os.WriteFile(injected, []byte("#!/bin/sh\necho go version go9.9.9\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(toolrun.EnvFoundryGoBin, "")
	h := cli.GeneratePipelineHostForTest(cli.Options{
		PipelineHost: cli.PipelineHost{GoBinary: injected},
	}, false)
	log.Assert("injected_kept", h.GoBinary == injected, injected, h.GoBinary)
	log.Step("injected", testutil.OutcomeOK, "go="+h.GoBinary)

	// 2) FOUNDRY_GO_BIN wins over pin preference (envSet short-circuit).
	envBin := filepath.Join(t.TempDir(), "env-go")
	if err := os.WriteFile(envBin, []byte("#!/bin/sh\necho go version go8.8.8\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(toolrun.EnvFoundryGoBin, envBin)
	h = cli.GeneratePipelineHostForTest(cli.Options{}, false)
	log.Assert("env_kept", h.GoBinary == envBin, envBin, h.GoBinary)
	log.Step("env", testutil.OutcomeOK, "go="+h.GoBinary)

	// 3) Neither inject nor env: pin preference may replace LookPath hit.
	// When FindPinnedGoBinary finds nothing, GoBinary stays at LookPath (or empty).
	t.Setenv(toolrun.EnvFoundryGoBin, "")
	// Poison PATH so LookPath("go") fails; pin finder also fails without candidates.
	t.Setenv("PATH", t.TempDir())
	h = cli.GeneratePipelineHostForTest(cli.Options{}, false)
	// Empty is acceptable when no go is discoverable.
	log.Assert("no_go_empty_or_pin", h.GoBinary == "" || filepath.IsAbs(h.GoBinary), true, h.GoBinary)
	log.Step("no_path", testutil.OutcomeOK, fmt.Sprintf("go=%q", h.GoBinary))

	// 4) gitInit true without GitBinary attempts LookPath("git") — still empty PATH.
	h = cli.GeneratePipelineHostForTest(cli.Options{}, true)
	log.Assert("git_empty_path", h.GitBinary == "" || filepath.IsAbs(h.GitBinary), true, h.GitBinary)

	// 5) Partial Host fill: empty Host fields filled from ambient.
	t.Setenv("PATH", "/unit-test-path")
	t.Setenv("HOME", "/unit-test-home")
	t.Setenv("TMPDIR", "/unit-test-tmp")
	h = cli.PipelineHostForTest(cli.Options{
		PipelineHost: cli.PipelineHost{GoBinary: injected},
	}, false)
	// Zero Host triggers captureHostEnv entirely.
	log.Assert("host_path_set", h.Host.PATH != "", true, h.Host.PATH)

	// Partial: HOME preserved; empty PATH filled from ambient.
	partial := cli.PipelineHost{GoBinary: injected}
	partial.Host.HOME = "/kept-home"
	h = cli.PipelineHostForTest(cli.Options{PipelineHost: partial}, false)
	log.Assert("partial_home_kept", h.Host.HOME == "/kept-home", "/kept-home", h.Host.HOME)
	log.Assert("partial_path_filled", h.Host.PATH != "", true, h.Host.PATH)
	log.Step("partial_host", testutil.OutcomeOK, "home="+h.Host.HOME+" path="+h.Host.PATH)

	log.PhaseEnd("generate_pipeline_host", testutil.OutcomeOK)
}

func TestRedactAndNilGenerateHelpers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("generate_helpers")

	log.Assert("redact_empty", cli.RedactDestForLogForTest("") == "", "", cli.RedactDestForLogForTest(""))
	got := cli.RedactDestForLogForTest("/home/runner/work/proj/stage-abc")
	log.Assert("redact_base", got == "stage-abc", "stage-abc", got)

	log.Assert("net_nil", cli.NetworkStepIDsNilForTest() == nil, true, cli.NetworkStepIDsNilForTest() == nil)
	log.Assert("next_nil", cli.GenerateNextStepsNilForTest() == nil, true, cli.GenerateNextStepsNilForTest() == nil)

	log.Step("helpers", testutil.OutcomeOK, "redact+nil branches")
	log.PhaseEnd("generate_helpers", testutil.OutcomeOK)
}

func TestGeneratePipelineHostPrefersPinWhenLookPathOnly(t *testing.T) {
	log := testutil.New(t)
	log.Phase("pin_over_lookpath")

	// Ensure FOUNDRY_GO_BIN unset so generatePipelineHost may prefer pin.
	t.Setenv(toolrun.EnvFoundryGoBin, "")

	// Options with empty GoBinary → pipelineHost uses LookPath; generate may upgrade.
	hPure := cli.PipelineHostForTest(cli.Options{}, false)
	hGen := cli.GeneratePipelineHostForTest(cli.Options{}, false)

	// Preference rule: if pin found, gen uses it; otherwise both share LookPath result.
	pinned := toolrun.FindPinnedGoBinary(toolrun.DefaultPinnedGoTag)
	if pinned != "" {
		log.Assert("gen_uses_pin", hGen.GoBinary == pinned, pinned, hGen.GoBinary)
		log.Step("pin_found", testutil.OutcomeOK, "pin="+pinned+" pure="+hPure.GoBinary)
	} else {
		// No pin available in this environment — gen must equal pure (LookPath or empty).
		log.Assert("gen_eq_pure", hGen.GoBinary == hPure.GoBinary, hPure.GoBinary, hGen.GoBinary)
		log.Step("pin_absent", testutil.OutcomeOK, "go="+hGen.GoBinary)
	}
	// Either way the generate path executed the preference branch.
	log.Assert("gen_abs_or_empty", hGen.GoBinary == "" || strings.Contains(hGen.GoBinary, string(os.PathSeparator)) || filepath.IsAbs(hGen.GoBinary),
		true, hGen.GoBinary)

	log.PhaseEnd("pin_over_lookpath", testutil.OutcomeOK)
}

func TestCLIHelpersResidualBranches(t *testing.T) {
	log := testutil.New(t)
	log.Phase("cli_helpers")

	// workingDir: injected vs ambient.
	wd, wdErr := cli.WorkingDirForTest(cli.Options{WorkingDir: "/injected/wd"})
	if wdErr != nil {
		t.Fatalf("WorkingDirForTest injected: %v", wdErr)
	}
	log.Assert("wd_injected", wd == "/injected/wd", "/injected/wd", wd)
	wd, wdErr = cli.WorkingDirForTest(cli.Options{})
	if wdErr != nil {
		t.Fatalf("WorkingDirForTest ambient: %v", wdErr)
	}
	log.Assert("wd_ambient", wd != "", true, wd)

	// readFile inject + real.
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := cli.ReadFileForTest(cli.Options{}, p)
	if err != nil {
		log.Fail("read_real", err.Error())
	}
	log.Assert("read_real", string(b) == "hello", "hello", string(b))
	b, err = cli.ReadFileForTest(cli.Options{ReadFile: func(string) ([]byte, error) {
		return []byte("inject"), nil
	}}, p)
	if err != nil {
		log.Fail("read_inject", err.Error())
	}
	log.Assert("read_inject", string(b) == "inject", "inject", string(b))

	// observeDestinationAt: empty, relative with wd, abs exists/absent, parent missing.
	_, err = cli.ObserveDestinationAtForTest("", dir)
	if err == nil {
		log.Fail("obs_empty", "expected error")
	}
	info, err := cli.ObserveDestinationAtForTest("child-proj", dir)
	if err != nil {
		log.Fail("obs_rel", err.Error())
	}
	log.Assert("obs_absent", info.Observation == plan.ObservationAbsent, plan.ObservationAbsent, info.Observation)
	// Existing path.
	exist := filepath.Join(dir, "exists")
	if err := os.Mkdir(exist, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err = cli.ObserveDestinationAtForTest(exist, "")
	if err != nil {
		log.Fail("obs_abs", err.Error())
	}
	log.Assert("obs_exists", info.Observation == plan.ObservationExists, plan.ObservationExists, info.Observation)
	// Parent missing.
	info, err = cli.ObserveDestinationAtForTest(filepath.Join(dir, "no-parent", "x"), "")
	if err != nil {
		log.Fail("obs_parent", err.Error())
	}
	log.Assert("obs_parent_missing", info.Observation == plan.ObservationParentMissing, plan.ObservationParentMissing, info.Observation)
	// Parent is a file not a dir.
	fileParent := filepath.Join(dir, "fileparent")
	if err := os.WriteFile(fileParent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err = cli.ObserveDestinationAtForTest(filepath.Join(fileParent, "child"), "")
	if err != nil {
		log.Fail("obs_file_parent", err.Error())
	}
	log.Assert("obs_file_parent", info.Observation == plan.ObservationParentMissing, plan.ObservationParentMissing, info.Observation)

	// destLexicalBase edges.
	base, msg := cli.DestLexicalBaseForTest("")
	log.Assert("lex_empty", msg != "" && base == "", true, msg)
	base, msg = cli.DestLexicalBaseForTest("ok-name")
	log.Assert("lex_ok", msg == "" && base == "ok-name", "ok-name", base)
	_, msg = cli.DestLexicalBaseForTest("../escape")
	log.Assert("lex_dotdot", msg != "", true, msg)
	_, msg = cli.DestLexicalBaseForTest("~/home")
	log.Assert("lex_tilde", msg != "", true, msg)
	_, msg = cli.DestLexicalBaseForTest("$HOME/x")
	log.Assert("lex_dollar", msg != "", true, msg)

	// flagChanged nil + real cmd.
	log.Assert("flag_nil", !cli.FlagChangedForTest(nil, "output"), false, false)
	root := cli.NewRoot(testOptions())
	log.Assert("flag_unset", !cli.FlagChangedForTest(root, "output"), false, false)

	// normalizeCLIError branches.
	log.Assert("norm_nil", cli.NormalizeCLIErrorForTest(nil) == nil, true, true)
	// plain error → usage
	nerr := cli.NormalizeCLIErrorForTest(errors.New("unknown flag: --nope"))
	if nerr == nil {
		log.Fail("norm_plain", "expected error")
	}
	// empty message
	nerr = cli.NormalizeCLIErrorForTest(errors.New("   "))
	if nerr == nil {
		log.Fail("norm_blank", "expected error")
	}
	// context.Canceled
	nerr = cli.NormalizeCLIErrorForTest(context.Canceled)
	log.Assert("norm_cancel", nerr != nil, true, nerr != nil)

	// checkCancelled
	log.Assert("cancel_nil_ctx", cli.CheckCancelledForTest(nil) == nil, true, true)
	log.Assert("cancel_bg", cli.CheckCancelledForTest(context.Background()) == nil, true, true)
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	cerr := cli.CheckCancelledForTest(cctx)
	log.Assert("cancel_done", cerr != nil, true, cerr != nil)

	// requireSpec
	if cli.RequireSpecForTest("") == nil {
		log.Fail("spec_empty", "expected error")
	}
	if cli.RequireSpecForTest("  ") == nil {
		log.Fail("spec_ws", "expected error")
	}
	if err := cli.RequireSpecForTest("-"); err != nil {
		log.Fail("spec_stdin", err.Error())
	}

	// loadCatalog SkipCatalogLoad
	err = cli.LoadCatalogForTest(cli.Options{SkipCatalogLoad: true})
	if err == nil {
		log.Fail("skip_catalog", "expected error")
	}

	// versionInfo fills from Read when empty.
	ver, goV, _ := cli.VersionInfoForTest(cli.Options{})
	log.Assert("ver_filled", ver != "" || goV != "", true, ver+"/"+goV)

	// emptyDash
	log.Assert("dash_empty", cli.EmptyDashForTest("") == "-", "-", cli.EmptyDashForTest(""))
	log.Assert("dash_keep", cli.EmptyDashForTest("x") == "x", "x", cli.EmptyDashForTest("x"))

	// formatGenerateSuccess branches
	s := cli.FormatGenerateSuccessForTest(report.GenerateResult{Destination: "d"})
	log.Assert("fmt_dest", strings.Contains(s, "destination=d"), true, s)
	s = cli.FormatGenerateSuccessForTest(report.GenerateResult{
		Destination: "d", PlanSHA256: "abc", StagePath: "stage-x",
	})
	log.Assert("fmt_full", strings.Contains(s, "plan_sha256=abc") && strings.Contains(s, "stage_path=stage-x"), true, s)

	// reportSink.OnEvent nil paths
	var buf bytes.Buffer
	enc := cli.NewEncoderForTest(&buf, &buf, cli.NewGlobalFlagsForTest(cli.OutputText, cli.ColorNever, false, false))
	cli.ReportSinkOnEventForTest(enc, generate.GenerationEvent{Kind: generate.EventProgress, Detail: "x"})

	// CollectFlagNames / PublicCommandNames nil root
	log.Assert("collect_nil", len(cli.CollectFlagNames(nil)) == 0, 0, len(cli.CollectFlagNames(nil)))
	log.Assert("public_nil", len(cli.PublicCommandNames(nil)) == 0, 0, len(cli.PublicCommandNames(nil)))

	log.Step("cli_helpers", testutil.OutcomeOK, "workingDir+observe+normalize+cancel")
	log.PhaseEnd("cli_helpers", testutil.OutcomeOK)
}
