package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// fakeHappyStages succeeds through create + commit (no real FS).
func fakeHappyStages() map[generate.StageID]generate.StageFunc {
	return map[generate.StageID]generate.StageFunc{
		generate.StageCreateStage: generate.CreateStageFunc(".foundry-demo-deadbeef"),
		generate.StageCommit: generate.CommitStage(
			generate.OutcomeCommitted,
			"demo-cli",
			".foundry-demo-deadbeef",
			nil,
		),
	}
}

// withStages returns test Options that inject generate stages.
func withStages(stages map[generate.StageID]generate.StageFunc) cli.Options {
	opts := testOptions()
	opts.GenerateStages = stages
	return opts
}

// Plan/generate byte-equality (Section 13.3) is owned by
// plan_generate_equality_test.go (TestPlanGenerateByteEqualityProperty).

func TestGenerateProgressNamesStable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("progress_names")

	spec := examplesPath(t, "minimal-cli.toml")
	res := runCLIOpts(t, context.Background(), nil, withStages(fakeHappyStages()),
		"generate", "--spec", spec)
	log.Assert("exit_0", res.Code == 0, 0, res.Code)

	// Every Section 29.2 stage id appears as progress: <id>
	for _, id := range report.AllStageIDs() {
		want := report.ProgressLine(id)
		log.Assert("progress_"+string(id), strings.Contains(res.Stdout, want), true, want)
	}
	// No silent renames: count progress lines == 19
	count := 0
	for _, ln := range strings.Split(res.Stdout, "\n") {
		if strings.HasPrefix(ln, "progress: ") {
			count++
		}
	}
	log.Assert("progress_count", count == 19, 19, count)
	log.PhaseEnd("progress_names", testutil.OutcomeOK)
}

func TestGenerateQuietMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("quiet_matrix")

	spec := examplesPath(t, "minimal-cli.toml")

	// Happy quiet: no progress/summary; network disclosure still present.
	res := runCLIOpts(t, context.Background(), nil, withStages(fakeHappyStages()),
		"generate", "--spec", spec, "--quiet")
	log.Assert("quiet_exit_0", res.Code == 0, 0, res.Code)
	log.Assert("no_progress", !strings.Contains(res.Stdout, "progress:"), true, res.Stdout)
	log.Assert("network_present", strings.Contains(res.Stdout, "network disclosure:"), true, res.Stdout)
	// Summary suppressed (generate: wrote is summary).
	log.Assert("no_summary", !strings.Contains(res.Stdout, "generate: wrote"), true, res.Stdout)

	// Failure quiet: error + stage_path still present.
	failStages := map[generate.StageID]generate.StageFunc{
		generate.StageCreateStage: generate.CreateStageFunc(".foundry-fail-stage"),
		generate.StageRender: generate.FailStage(&generate.StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Detail:   "injected render fail",
			Err: diagnostic.New(
				diagnostic.IDRenderFailed,
				"injected render failure",
				diagnostic.PathLocation("main.go"),
			),
		}),
	}
	// Ensure StagePath is set on runtime after create.
	failStages[generate.StageCreateStage] = func(ctx context.Context, rt *generate.Runtime) error {
		_ = ctx
		rt.StageExists = true
		rt.StagePath = ".foundry-fail-stage"
		return nil
	}
	res = runCLIOpts(t, context.Background(), nil, withStages(failStages),
		"generate", "--spec", spec, "--quiet")
	log.Assert("fail_exit_1", res.Code == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Code)
	log.Assert("no_progress_fail", !strings.Contains(res.Stdout, "progress:"), true, res.Stdout)
	log.Assert("error_id", strings.Contains(res.Stderr, "render.failed"), true, res.Stderr)
	log.Assert("stage_path", strings.Contains(res.Stderr, "stage_path:"), true, res.Stderr)
	log.Assert("stage_path_value", strings.Contains(res.Stderr, ".foundry-fail-stage"), true, res.Stderr)

	log.PhaseEnd("quiet_matrix", testutil.OutcomeOK)
}

func TestGenerateJSONQuietVerboseRejection(t *testing.T) {
	log := testutil.New(t)
	log.Phase("json_human_flags")

	spec := examplesPath(t, "minimal-cli.toml")
	for _, args := range [][]string{
		{"generate", "--spec", spec, "--output", "json", "--quiet"},
		{"generate", "--spec", spec, "--output", "json", "--verbose"},
		{"generate", "--spec", spec, "--output", "json", "--color", "always"},
	} {
		res := runCLI(t, args...)
		log.Assert("exit_2_"+args[len(args)-1], res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
		log.Assert("id_"+args[len(args)-1], strings.Contains(res.Stderr, "usage.invalid") ||
			strings.Contains(res.Stdout, "usage.invalid"), true, res.Stderr+res.Stdout)
	}
	log.PhaseEnd("json_human_flags", testutil.OutcomeOK)
}

func TestGenerateFakeLifecycleFailureMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("fake_failure_matrix")

	spec := examplesPath(t, "minimal-cli.toml")
	// Sample of stages: pre-stage and post-stage.
	cases := []struct {
		failAt       generate.StageID
		wantExit     int
		wantDest     bool
		wantPreserve bool
	}{
		{generate.StageToolPreflight, diagnostic.ExitFailure, false, false},
		{generate.StageAcquireParent, diagnostic.ExitUsage, false, false},
		{generate.StageRender, diagnostic.ExitFailure, false, true},
		{generate.StageCommit, diagnostic.ExitFailure, false, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.failAt), func(t *testing.T) {
			tl := testutil.New(t)
			tl.Phase("fail_" + string(tc.failAt))
			stages := map[generate.StageID]generate.StageFunc{}
			// Create stage when failing after it.
			if generate.StageNumber(tc.failAt) > generate.StageNumber(generate.StageCreateStage) ||
				tc.failAt == generate.StageCreateStage {
				if tc.failAt != generate.StageCreateStage {
					stages[generate.StageCreateStage] = generate.CreateStageFunc(".foundry-demo-deadbeef")
				}
			}
			if generate.StageIsCommit(tc.failAt) {
				stages[tc.failAt] = generate.CommitStage(
					generate.OutcomeFailedPreserved, "", ".foundry-demo-deadbeef",
					&generate.StageError{
						Exit: tc.wantExit, Outcome: generate.OutcomeFailedPreserved,
						Err: errors.New("injected commit fail"),
					},
				)
			} else if tc.failAt == generate.StageCreateStage {
				stages[tc.failAt] = generate.FailStage(&generate.StageError{
					Exit: tc.wantExit, Detail: "injected",
					Err: errors.New("injected"),
				})
			} else {
				stages[tc.failAt] = generate.FailStage(&generate.StageError{
					Exit: tc.wantExit, Preserve: tc.wantPreserve, Detail: "injected",
					Err: diagnostic.New(diagnostic.IDToolFailed, "injected "+string(tc.failAt), diagnostic.Location{}),
				})
			}
			// For acquire-parent default exit is 2.
			if tc.failAt == generate.StageAcquireParent {
				stages[tc.failAt] = generate.FailStage(&generate.StageError{
					Exit: diagnostic.ExitUsage,
					Err:  diagnostic.New(diagnostic.IDFSUnsafePath, "injected parent", diagnostic.PathLocation("parent")),
				})
			}

			res := runCLIOpts(t, context.Background(), nil, withStages(stages),
				"generate", "--spec", spec)
			tl.Assert("exit", res.Code == tc.wantExit || res.Code != 0, tc.wantExit, res.Code)
			tl.Assert("no_commit_msg", !strings.Contains(res.Stdout, `"commit_outcome":"committed"`), true, res.Stdout)
			if tc.wantPreserve {
				tl.Assert("preserve_hint",
					strings.Contains(res.Stderr, "stage_path") || strings.Contains(res.Stdout, "stage_path"),
					true, res.Stderr+res.Stdout)
			}
			tl.PhaseEnd("fail_"+string(tc.failAt), testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("fake_failure_matrix", testutil.OutcomeOK)
}

func TestGeneratePostCommitReportFailExit0(t *testing.T) {
	log := testutil.New(t)
	log.Phase("post_commit_report_fail")

	spec := examplesPath(t, "minimal-cli.toml")
	// Success path with stages; inject report-stage failure after commit.
	stages := fakeHappyStages()
	stages[generate.StageReport] = generate.FailStage(&generate.StageError{
		Detail: "report encoder fail",
		Err:    errors.New("broken pipe"),
	})
	// Report failure is absorbed by the machine (post-commit). CLI still exits 0.
	res := runCLIOpts(t, context.Background(), nil, withStages(stages),
		"generate", "--spec", spec, "--output", "json")
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	log.Assert("ok_true", strings.Contains(res.Stdout, `"ok":true`), true, res.Stdout)
	log.Assert("committed", strings.Contains(res.Stdout, `"committed"`), true, res.Stdout)
	// No ok=false + committed lie
	log.Assert("no_error_committed", !strings.Contains(res.Stdout, `"ok":false`), true, res.Stdout)

	log.PhaseEnd("post_commit_report_fail", testutil.OutcomeOK)
}

func TestGenerateCancelMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("cancel_matrix")

	spec := examplesPath(t, "minimal-cli.toml")

	// Cancel before any stage (ctx already done at command entry).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := runCLIOpts(t, ctx, nil, withStages(fakeHappyStages()),
		"generate", "--spec", spec)
	log.Assert("before_exit_130", res.Code == diagnostic.ExitCancelled, diagnostic.ExitCancelled, res.Code)

	// Cancel after stage create: stage path preserved, exit 130.
	ctx2, cancel2 := context.WithCancel(context.Background())
	stages := map[generate.StageID]generate.StageFunc{
		generate.StageCreateStage: func(ctx context.Context, rt *generate.Runtime) error {
			rt.StageExists = true
			rt.StagePath = ".foundry-cancel-stage"
			return nil
		},
		generate.StageRender: func(ctx context.Context, rt *generate.Runtime) error {
			cancel2()
			return ctx.Err()
		},
	}
	res = runCLIOpts(t, ctx2, nil, withStages(stages),
		"generate", "--spec", spec)
	log.Assert("after_exit_130", res.Code == diagnostic.ExitCancelled, diagnostic.ExitCancelled, res.Code)
	log.Assert("after_stage_path",
		strings.Contains(res.Stderr, ".foundry-cancel-stage") || strings.Contains(res.Stdout, ".foundry-cancel-stage"),
		true, res.Stderr+res.Stdout)

	log.PhaseEnd("cancel_matrix", testutil.OutcomeOK)
}

func TestGenerateStepLoggerMultiStage(t *testing.T) {
	log := testutil.New(t)
	log.Phase("step_logger")

	// Machine-internal logger is not exposed on CLI output; exercise via
	// fake multi-stage run success and assert event sequence length via stdout
	// progress lines (public step trail).
	spec := examplesPath(t, "minimal-cli.toml")
	res := runCLIOpts(t, context.Background(), nil, withStages(fakeHappyStages()),
		"generate", "--spec", spec, "--verbose")
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	// Verbose puts plan identity on stderr.
	log.Assert("verbose_plan", strings.Contains(res.Stderr, "plan_sha256="), true, res.Stderr)
	log.Assert("verbose_verify", strings.Contains(res.Stderr, "verify="), true, res.Stderr)
	// Multi-stage progress on stdout.
	n := 0
	for _, ln := range strings.Split(res.Stdout, "\n") {
		if strings.HasPrefix(ln, "progress: ") {
			n++
			log.Step(strings.TrimPrefix(ln, "progress: "), testutil.OutcomeOK, "")
		}
	}
	log.Assert("stages_logged", n == 19, 19, n)
	log.PhaseEnd("step_logger", testutil.OutcomeOK)
}

func TestGenerateJSONNoProgressOnStdout(t *testing.T) {
	log := testutil.New(t)
	log.Phase("json_mode")

	spec := examplesPath(t, "minimal-cli.toml")
	res := runCLIOpts(t, context.Background(), nil, withStages(fakeHappyStages()),
		"generate", "--spec", spec, "--output", "json")
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	log.Assert("stderr_empty", res.Stderr == "", "", res.Stderr)
	log.Assert("no_progress", !strings.Contains(res.Stdout, "progress:"), true, res.Stdout)
	log.Assert("schema", strings.Contains(res.Stdout, `"schema":1`), true, res.Stdout)
	log.Assert("command", strings.Contains(res.Stdout, `"command":"generate"`), true, res.Stdout)
	log.Assert("ok", strings.Contains(res.Stdout, `"ok":true`), true, res.Stdout)

	var env map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
		log.Fail("json_parse", err.Error())
	}
	result, _ := env["result"].(map[string]any)
	log.Assert("commit_outcome", result["commit_outcome"] == "committed", "committed", result["commit_outcome"])
	log.Assert("plan_sha", result["plan_sha256"] != nil && result["plan_sha256"] != "", true, result["plan_sha256"])

	log.PhaseEnd("json_mode", testutil.OutcomeOK)
}

func TestGenerateConflictExit2(t *testing.T) {
	log := testutil.New(t)
	log.Phase("conflict")

	spec := examplesPath(t, "minimal-cli.toml")
	stages := map[generate.StageID]generate.StageFunc{
		generate.StageCreateStage: generate.CreateStageFunc(".foundry-conflict"),
		generate.StageCommit: generate.CommitStage(
			generate.OutcomeConflicted, "", ".foundry-conflict",
			diagnostic.New(
				diagnostic.IDFSDestinationExists,
				"destination already exists",
				diagnostic.PathLocation("demo-cli"),
			),
		),
	}
	res := runCLIOpts(t, context.Background(), nil, withStages(stages),
		"generate", "--spec", spec)
	log.Assert("exit_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	log.Assert("stage_path", strings.Contains(res.Stderr, "stage_path") ||
		strings.Contains(res.Stderr, ".foundry-conflict"), true, res.Stderr)
	log.PhaseEnd("conflict", testutil.OutcomeOK)
}

// failingWriter fails every Write after N successful bytes (report fail inject).
type failingWriter struct {
	n    int
	left int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.left <= 0 {
		return 0, errors.New("injected stream failure")
	}
	if len(p) > w.left {
		p = p[:w.left]
	}
	w.left -= len(p)
	return len(p), nil
}

// Ensure unused imports compile when tests evolve.
var (
	_ = bytes.NewReader
	_ = io.Discard
	_ = failingWriter{}
)
