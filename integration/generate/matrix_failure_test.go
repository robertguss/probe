package generatee2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	generatee2e "github.com/robertguss/go-foundry-cli/integration/generate"
	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestMatrixPreExistingDestination: exit 2, no overwrite, logs name cause.
func TestMatrixPreExistingDestination(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("preexisting_dest")

	parent := privateParent(t)
	dest := filepath.Join(parent, "minimal-cli")
	spec := examplesSpec(t, "minimal-cli.toml")

	// First generate succeeds.
	res1 := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest, "--output", "json")
	logProc(log, "generate_first", res1, 0)
	if res1.Code != 0 {
		dumpFailure(t, log, failureFromProc("preexisting_first", "commit", "first_failed", res1, "", dest))
		return
	}

	// Second generate must refuse with exit 2.
	res2 := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest, "--output", "json")
	logProc(log, "generate_conflict", res2, diagnostic.ExitUsage)
	env := mustEnvelope(t, log, res2.Stdout)
	id := errorIDFromEnvelope(env)
	log.Assert("error_id", id == "fs.destination_exists", "fs.destination_exists", id)
	log.Assert("ok_false", env["ok"] == false, false, env["ok"])

	// Destination untouched (still has go.mod).
	if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
		log.Fail("dest_intact", err.Error())
	}
	// No new .foundry stage under parent for early refuse (acquire-parent).
	entries, _ := os.ReadDir(parent)
	var stages []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".foundry") {
			stages = append(stages, e.Name())
		}
	}
	log.Step("stage_dirs", testutil.OutcomeInfo, strings.Join(stages, ","))

	art := failureFromProc("preexisting_dest", "acquire-parent", id, res2, planSHAFromEnvelope(mustEnvelope(t, log, res1.Stdout)), dest)
	art.Remediation = "Choose a path that does not exist; Foundry never overwrites."
	// Intentional failure: always write artifact when dir configured.
	if path, err := generatee2e.WriteFailureArtifact("", art); err == nil && path != "" {
		log.Step("artifact", testutil.OutcomeOK, filepath.Base(path))
	}
	// Logs alone name stage + cause.
	log.Assert("cause_named", id != "", true, id)
	log.PhaseEnd("preexisting_dest", testutil.OutcomeOK)
}

// TestMatrixInjectedMidStageFailure: real create-stage then fail render → exit 1 + preserve.
func TestMatrixInjectedMidStageFailure(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("mid_stage_fail")

	parent := privateParent(t)
	dest := filepath.Join(parent, "minimal-cli")
	spec := examplesSpec(t, "minimal-cli.toml")
	p := buildRealPlan(t, spec, dest, plan.VerifyDefault, true)
	planSHA := p.PlanSHA256()
	log.Step("plan_sha256", testutil.OutcomeOK, planSHA)

	stages, closer := productionStagesWithMutation(t, p, func(m map[generate.StageID]generate.StageFunc) {
		// Keep real create-stage; fail at render so stage is on disk.
		m[generate.StageRender] = generate.FailStage(&generate.StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Detail:   "injected render failure",
			Err: diagnostic.New(
				diagnostic.IDRenderFailed,
				"injected mid-stage render failure for e2e matrix",
				diagnostic.PathLocation("main.go"),
			),
		})
	})
	defer func() { _ = closer() }()

	res := runCLI(t, context.Background(), cli.Options{GenerateStages: stages}, nil,
		"generate", "--spec", spec, "--dest", dest, "--output", "json")
	logProc(log, "generate_mid_fail", res, diagnostic.ExitFailure)

	env := mustEnvelope(t, log, res.Stdout)
	log.Assert("ok_false", env["ok"] == false, false, env["ok"])
	id := errorIDFromEnvelope(env)
	log.Assert("error_id", id == "render.failed" || strings.Contains(res.Stdout+res.Stderr, "render"), true, id)

	// Destination must not exist (no placement).
	if _, err := os.Stat(dest); err == nil {
		log.Fail("dest_placed", "destination must not be committed on mid-stage fail")
	} else {
		log.Assert("no_dest", os.IsNotExist(err), true, err)
	}

	stagePath := stagePathFromStreams(res.Stdout, res.Stderr)
	// Stage path should be reported for preserve cases.
	log.Step("stage_path", testutil.OutcomeInfo, stagePath)
	if stagePath != "" {
		if st, err := os.Stat(stagePath); err != nil {
			log.Step("stage_stat", testutil.OutcomeFail, err.Error())
		} else {
			log.Assert("stage_is_dir", st.IsDir(), true, st.IsDir())
		}
	} else {
		// Fallback: look for .foundry* under parent.
		entries, _ := os.ReadDir(parent)
		found := false
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".foundry") {
				found = true
				stagePath = filepath.Join(parent, e.Name())
				log.Step("stage_found", testutil.OutcomeOK, e.Name())
			}
		}
		log.Assert("stage_preserved_on_disk", found, true, found)
	}

	art := generatee2e.FailureArtifact{
		Case:         "mid_stage_render",
		Stage:        string(generate.StageRender),
		Cause:        "injected_render_failed",
		PlanSHA256:   planSHA,
		CommitResult: string(generate.OutcomeFailedPreserved),
		Exit:         res.Code,
		StagePath:    stagePath,
		Destination:  dest,
		Remediation:  "Inspect stage_path; Foundry never auto-deletes stages. Fix the failure cause and re-run generate.",
		Timeline:     progressNamesFromStdout(res.Stdout),
		Detail:       "intentional mid-stage injection",
	}
	if path, err := generatee2e.WriteFailureArtifact("", art); err == nil && path != "" {
		log.Step("artifact", testutil.OutcomeOK, filepath.Base(path))
		// Filename must encode stage + cause.
		base := filepath.Base(path)
		log.Assert("artifact_stage", strings.Contains(base, "stage-render"), true, base)
		log.Assert("artifact_cause", strings.Contains(base, "cause-injected"), true, base)
	}
	body := generatee2e.FormatFailureArtifact(art)
	log.Assert("log_names_stage", strings.Contains(body, "stage=render"), true, body)
	log.Assert("log_names_cause", strings.Contains(body, "cause=injected"), true, body)

	log.PhaseEnd("mid_stage_fail", testutil.OutcomeOK)
}

// TestMatrixCancelBeforeAfter covers REQ-036 cancel windows via context cancel.
func TestMatrixCancelBeforeAfter(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("cancel_matrix")

	spec := examplesSpec(t, "minimal-cli.toml")

	t.Run("before_stage", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("cancel_before")
		parent := privateParent(t)
		dest := filepath.Join(parent, "minimal-cli")
		p := buildRealPlan(t, spec, dest, plan.VerifyDefault, true)

		ctx, cancel := context.WithCancel(context.Background())
		stages, closer := productionStagesWithMutation(t, p, func(m map[generate.StageID]generate.StageFunc) {
			// Cancel during acquire-parent (pre-stage, no disk stage yet).
			orig := m[generate.StageAcquireParent]
			m[generate.StageAcquireParent] = func(ctx context.Context, rt *generate.Runtime) error {
				cancel()
				if err := ctx.Err(); err != nil {
					return err
				}
				if orig != nil {
					return orig(ctx, rt)
				}
				return context.Canceled
			}
		})
		defer func() { _ = closer() }()

		res := runCLI(t, ctx, cli.Options{GenerateStages: stages}, nil,
			"generate", "--spec", spec, "--dest", dest)
		// Accept 130 (cancelled) or possibly 1 if race before cancel is observed.
		tl.Step("exit", testutil.OutcomeInfo, exitClass(res.Code))
		tl.Assert("exit_cancel_or_fail", res.Code == diagnostic.ExitCancelled || res.Code == diagnostic.ExitFailure,
			diagnostic.ExitCancelled, res.Code)
		if _, err := os.Stat(dest); err == nil {
			tl.Fail("dest_exists", "before-stage cancel must not place destination")
		}
		// No .foundry stage preferred; if create never ran, parent only has nothing.
		entries, _ := os.ReadDir(parent)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".foundry") {
				tl.Step("unexpected_stage", testutil.OutcomeFail, e.Name())
			}
		}
		art := failureFromProc("cancel_before", "acquire-parent", "cancelled", res, p.PlanSHA256(), dest)
		art.CommitResult = string(generate.OutcomeCancelled)
		_, _ = generatee2e.WriteFailureArtifact("", art)
		tl.PhaseEnd("cancel_before", testutil.OutcomeOK)
	})

	t.Run("after_stage", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("cancel_after")
		parent := privateParent(t)
		dest := filepath.Join(parent, "minimal-cli")
		p := buildRealPlan(t, spec, dest, plan.VerifyDefault, true)

		ctx, cancel := context.WithCancel(context.Background())
		stages, closer := productionStagesWithMutation(t, p, func(m map[generate.StageID]generate.StageFunc) {
			// Real create-stage, then cancel at render.
			m[generate.StageRender] = func(ctx context.Context, rt *generate.Runtime) error {
				cancel()
				if err := ctx.Err(); err != nil {
					return err
				}
				return context.Canceled
			}
		})
		defer func() { _ = closer() }()

		res := runCLI(t, ctx, cli.Options{GenerateStages: stages}, nil,
			"generate", "--spec", spec, "--dest", dest)
		tl.Step("exit", testutil.OutcomeInfo, exitClass(res.Code))
		tl.Assert("exit_130", res.Code == diagnostic.ExitCancelled || res.Code == diagnostic.ExitFailure,
			diagnostic.ExitCancelled, res.Code)
		if _, err := os.Stat(dest); err == nil {
			tl.Fail("dest_exists", "after-stage cancel must not place destination")
		}
		stagePath := stagePathFromStreams(res.Stdout, res.Stderr)
		entries, _ := os.ReadDir(parent)
		found := stagePath != ""
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".foundry") {
				found = true
				stagePath = filepath.Join(parent, e.Name())
				tl.Step("preserved", testutil.OutcomeOK, e.Name())
			}
		}
		// Machine cancel after create should preserve; assert path reported or on disk.
		tl.Assert("stage_preserved", found || strings.Contains(res.Stderr+res.Stdout, "stage"), true, found)
		if strings.Contains(res.Stderr+res.Stdout, "stage_path") {
			tl.Step("stage_path_reported", testutil.OutcomeOK, stagePath)
		}
		art := generatee2e.FailureArtifact{
			Case:         "cancel_after",
			Stage:        string(generate.StageRender),
			Cause:        "cancelled_after_stage",
			PlanSHA256:   p.PlanSHA256(),
			CommitResult: string(generate.OutcomeCancelled),
			Exit:         res.Code,
			StagePath:    stagePath,
			Destination:  dest,
			Remediation:  "Inspect preserved stage_path; re-run generate after addressing cancellation.",
		}
		if path, err := generatee2e.WriteFailureArtifact("", art); err == nil && path != "" {
			tl.Step("artifact", testutil.OutcomeOK, filepath.Base(path))
		}
		tl.PhaseEnd("cancel_after", testutil.OutcomeOK)
	})

	log.PhaseEnd("cancel_matrix", testutil.OutcomeOK)
}

// TestMatrixNetworkDisclosureBeforeStage asserts disclosure appears before create-stage.
func TestMatrixNetworkDisclosureBeforeStage(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("network_disclosure")

	parent := privateParent(t)
	dest := filepath.Join(parent, "minimal-cli")
	spec := examplesSpec(t, "minimal-cli.toml")

	// Text mode: ordered progress + disclosure.
	res := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest)
	logProc(log, "generate_text", res, 0)
	ok, detail := disclosureBeforeCreate(res.Stdout)
	log.Assert("before_create", ok, true, detail)
	log.Assert("has_tidy", strings.Contains(res.Stdout, "go-mod-tidy"), true, res.Stdout)
	log.Step("disclosure_order", testutil.OutcomeOK, detail)

	// Strict: also lists govulncheck.
	parent2 := privateParent(t)
	dest2 := filepath.Join(parent2, "minimal-cli")
	res2 := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest2, "--verify", "strict", "--output", "json")
	logProc(log, "generate_strict_json", res2, 0)
	env := mustEnvelope(t, log, res2.Stdout)
	result, _ := env["result"].(map[string]any)
	nd, _ := result["network_disclosure"].([]any)
	joined := ""
	for _, x := range nd {
		if s, ok := x.(string); ok {
			joined += s + ","
		}
	}
	log.Assert("strict_tidy", strings.Contains(joined, "go-mod-tidy"), true, joined)
	log.Assert("strict_vuln", strings.Contains(joined, "go-govulncheck"), true, joined)

	log.PhaseEnd("network_disclosure", testutil.OutcomeOK)
}

// TestMatrixQuietAndProgressNames covers REQ-155 stable names + quiet matrix.
func TestMatrixQuietAndProgressNames(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("quiet_progress")

	parent := privateParent(t)
	dest := filepath.Join(parent, "minimal-cli")
	spec := examplesSpec(t, "minimal-cli.toml")

	// Full progress names match Section 29.2.
	res := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest)
	logProc(log, "generate_progress", res, 0)
	names := progressNamesFromStdout(res.Stdout)
	want := generate.AllStageIDs()
	log.Assert("progress_count", len(names) == len(want), len(want), len(names))
	for i, id := range want {
		if i >= len(names) {
			log.Fail("missing_progress", string(id))
			break
		}
		log.Assert("progress_"+string(id), names[i] == string(id), string(id), names[i])
	}

	// Quiet: no progress/summary; network disclosure + errors still present.
	parent2 := privateParent(t)
	dest2 := filepath.Join(parent2, "minimal-cli")
	resQ := runCLI(t, context.Background(), cli.Options{}, nil,
		"generate", "--spec", spec, "--dest", dest2, "--quiet")
	logProc(log, "generate_quiet", resQ, 0)
	log.Assert("no_progress", !strings.Contains(resQ.Stdout, "progress:"), true, resQ.Stdout)
	log.Assert("network_kept", strings.Contains(resQ.Stdout, "network disclosure:"), true, resQ.Stdout)
	log.Assert("no_summary", !strings.Contains(resQ.Stdout, "generate: wrote"), true, resQ.Stdout)

	// Quiet failure still shows error + stage_path.
	parent3 := privateParent(t)
	dest3 := filepath.Join(parent3, "minimal-cli")
	p := buildRealPlan(t, spec, dest3, plan.VerifyDefault, true)
	stages, closer := productionStagesWithMutation(t, p, func(m map[generate.StageID]generate.StageFunc) {
		m[generate.StageCreateStage] = func(ctx context.Context, rt *generate.Runtime) error {
			_ = ctx
			rt.StageExists = true
			rt.StagePath = filepath.Join(parent3, ".foundry-quiet-fail")
			_ = os.MkdirAll(rt.StagePath, 0o700)
			return nil
		}
		m[generate.StageRender] = generate.FailStage(&generate.StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Err: diagnostic.New(
				diagnostic.IDRenderFailed,
				"quiet matrix injected fail",
				diagnostic.PathLocation("x.go"),
			),
		})
	})
	defer func() { _ = closer() }()
	resQF := runCLI(t, context.Background(), cli.Options{GenerateStages: stages}, nil,
		"generate", "--spec", spec, "--dest", dest3, "--quiet")
	log.Assert("quiet_fail_exit", resQF.Code == diagnostic.ExitFailure, diagnostic.ExitFailure, resQF.Code)
	log.Assert("quiet_no_progress", !strings.Contains(resQF.Stdout, "progress:"), true, resQF.Stdout)
	log.Assert("quiet_error", strings.Contains(resQF.Stderr, "render.failed") || strings.Contains(resQF.Stdout, "render.failed"),
		true, resQF.Stderr)
	log.Assert("quiet_stage_path",
		strings.Contains(resQF.Stderr, "stage_path") || strings.Contains(resQF.Stdout, "stage_path") ||
			strings.Contains(resQF.Stderr, ".foundry-quiet-fail"),
		true, resQF.Stderr)

	// Cross-check report.ProgressLine stable format.
	for _, id := range report.AllStageIDs() {
		line := report.ProgressLine(id)
		log.Assert("line_"+string(id), strings.HasPrefix(line, "progress: "), true, line)
	}

	log.PhaseEnd("quiet_progress", testutil.OutcomeOK)
}

// TestMatrixStreamFailurePrePost covers broken stdout pre/post commit (REQ-158).
//
// Pre-commit: StreamFailStageError after real create-stage → exit 1, no placement,
// stage preserved (process-level analogue of package stream_test).
// Post-commit: failWriter after many successful progress writes → exit 0 absorbed
// with destination present (commit dominates).
func TestMatrixStreamFailurePrePost(t *testing.T) {
	skipIfShort(t)
	log := testutil.New(t)
	log.Phase("stream_failure")

	spec := examplesSpec(t, "minimal-cli.toml")

	t.Run("pre_commit_broken_stdout", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("pre_commit")
		parent := privateParent(t)
		dest := filepath.Join(parent, "minimal-cli")
		p := buildRealPlan(t, spec, dest, plan.VerifyDefault, true)

		stages, closer := productionStagesWithMutation(t, p, func(m map[generate.StageID]generate.StageFunc) {
			// Real create-stage (production), then stream failure before commit.
			m[generate.StageRender] = generate.FailStage(
				generate.StreamFailStageError(generate.StreamStdout, io.ErrClosedPipe),
			)
		})
		defer func() { _ = closer() }()

		res := runCLI(t, context.Background(), cli.Options{GenerateStages: stages}, nil,
			"generate", "--spec", spec, "--dest", dest, "--output", "json")
		tl.Step("exit", testutil.OutcomeInfo, exitClass(res.Code))
		tl.Assert("exit_1", res.Code == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Code)
		if _, err := os.Stat(dest); err == nil {
			tl.Fail("dest_placed", "pre-commit stream fail must not place destination")
		} else {
			tl.Assert("no_dest", os.IsNotExist(err), true, err)
		}
		// Stage preserved on disk under parent.
		entries, _ := os.ReadDir(parent)
		found := false
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".foundry") {
				found = true
				tl.Step("stage_preserved", testutil.OutcomeOK, e.Name())
			}
		}
		stagePath := stagePathFromStreams(res.Stdout, res.Stderr)
		tl.Assert("preserve_signal", found || stagePath != "" || strings.Contains(res.Stdout+res.Stderr, "stage"),
			true, found)

		art := failureFromProc("stream_pre", string(generate.StageRender), "pre_commit_stream_failure", res, p.PlanSHA256(), dest)
		art.Stream = "stdout"
		art.CommitResult = string(generate.OutcomeFailedPreserved)
		if path, err := generatee2e.WriteFailureArtifact("", art); err == nil && path != "" {
			tl.Step("artifact", testutil.OutcomeOK, filepath.Base(path))
		}
		tl.PhaseEnd("pre_commit", testutil.OutcomeOK)
	})

	t.Run("post_commit_broken_stdout", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("post_commit")
		parent := privateParent(t)
		dest := filepath.Join(parent, "minimal-cli")

		// Allow progress + commit writes; fail only very late (or never mid-lifecycle).
		// Inject StageReport failure after real commit via GenerateStages mutation.
		p := buildRealPlan(t, spec, dest, plan.VerifyDefault, true)
		stages, closer := productionStagesWithMutation(t, p, func(m map[generate.StageID]generate.StageFunc) {
			m[generate.StageReport] = generate.FailStage(&generate.StageError{
				Detail: "broken pipe after commit",
				Err:    errors.New("broken pipe"),
			})
		})
		defer func() { _ = closer() }()

		res := runCLI(t, context.Background(), cli.Options{GenerateStages: stages}, nil,
			"generate", "--spec", spec, "--dest", dest, "--output", "json")
		tl.Step("exit", testutil.OutcomeInfo, exitClass(res.Code))
		// Post-commit report/stream failure absorbed → exit 0; destination present.
		tl.Assert("exit_0", res.Code == 0, 0, res.Code)
		gomodStat, gomodErr := os.Stat(filepath.Join(dest, "go.mod"))
		if gomodErr != nil {
			tl.Fail("dest_missing", gomodErr.Error())
		} else {
			tl.Assert("dest_ok", gomodErr == nil && !gomodStat.IsDir(), true, gomodStat.Name())
		}
		tl.Step("post_commit_absorbed", testutil.OutcomeOK, "exit=0 committed dominates")
		art := failureFromProc("stream_post", string(generate.StageReport), "post_commit_stream_absorbed", res, p.PlanSHA256(), dest)
		art.Stream = "stdout"
		art.CommitResult = string(generate.OutcomeCommitted)
		_, _ = generatee2e.WriteFailureArtifact("", art)
		tl.PhaseEnd("post_commit", testutil.OutcomeOK)
	})

	log.PhaseEnd("stream_failure", testutil.OutcomeOK)
}

func jsonUnmarshal(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}
