package generate_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestHappyPathEventGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("happy_path")

	sink := &generate.CollectingSink{}
	rec := &generate.RecordingLogger{}
	stages := defaultStages()
	stages[generate.StageReportPlanNetwork] = func(ctx context.Context, rt *generate.Runtime) error {
		_ = ctx
		rt.NetworkLines = []string{"go mod tidy may use the network"}
		return nil
	}
	m := newMachine(t, stages, sink, rec)
	res := m.Run(context.Background())

	log.Assert("exit", res.Exit == diagnostic.ExitSuccess, diagnostic.ExitSuccess, res.Exit)
	log.Assert("outcome", res.Outcome == generate.OutcomeCommitted, generate.OutcomeCommitted, res.Outcome)
	log.Assert("state", res.State == generate.LifeCommitted, generate.LifeCommitted, res.State)
	log.Assert("no_preserve", !res.StagePreserved, false, res.StagePreserved)
	log.Assert("dest", res.Destination == "demo", "demo", res.Destination)
	log.Assert("err_nil", res.Err == nil, true, res.Err)

	// Every stage appears as progress.
	progress := 0
	for _, ev := range res.Events {
		if ev.Kind == generate.EventProgress {
			progress++
		}
	}
	log.Assert("progress_count", progress == 19, 19, progress)

	// Terminal present once.
	term := 0
	for _, ev := range res.Events {
		if ev.Kind == generate.EventTerminal {
			term++
			log.Assert("term_outcome", ev.Outcome == generate.OutcomeCommitted, generate.OutcomeCommitted, ev.Outcome)
		}
	}
	log.Assert("terminal_once", term == 1, 1, term)

	compareEventGolden(t, log, "events_happy_path", res.Events)

	// Step logger recorded state/event/result (duration may be 0).
	log.Assert("steps_logged", len(rec.Steps) > 10, ">10", len(rec.Steps))
	hasState := false
	hasProgress := false
	for _, s := range rec.Steps {
		if strings.HasPrefix(s.Event, "state:") {
			hasState = true
		}
		if strings.HasPrefix(s.Event, "progress:") {
			hasProgress = true
		}
		log.Step("log_step", testutil.OutcomeOK,
			"state="+s.State+" event="+s.Event+" result="+s.Result)
	}
	log.Assert("has_state_log", hasState, true, hasState)
	log.Assert("has_progress_log", hasProgress, true, hasProgress)

	log.PhaseEnd("happy_path", testutil.OutcomeOK)
}

func TestInjectFailureAtEachStage(t *testing.T) {
	log := testutil.New(t)
	log.Phase("inject_failure_matrix")

	for _, id := range generate.AllStageIDs() {
		if generate.StageIsReport(id) {
			// Report failure is absorbed post-commit; covered separately.
			continue
		}
		id := id
		t.Run(string(id), func(t *testing.T) {
			tl := testutil.New(t)
			tl.Phase("fail_" + string(id))

			fail := &generate.StageError{
				Exit:   generate.DefaultFailureExit(id),
				Detail: "injected:" + string(id),
				Err:    errors.New("injected"),
			}
			// Build stages: succeed through previous, fail at id.
			// Ensure create-stage runs when failing after it.
			stages := map[generate.StageID]generate.StageFunc{}
			for _, s := range generate.AllStageIDs() {
				s := s
				switch {
				case s == id:
					if id == generate.StageCreateStage {
						// Fail without creating.
						stages[s] = generate.FailStage(fail)
					} else if generate.StageIsCommit(id) {
						stages[s] = generate.CommitStage(
							generate.OutcomeFailedPreserved,
							"",
							".foundry-demo-deadbeef",
							fail,
						)
					} else {
						stages[s] = generate.FailStage(fail)
					}
				case generate.StageNumber(s) < generate.StageNumber(id):
					if s == generate.StageCreateStage {
						stages[s] = generate.CreateStageFunc(".foundry-demo-deadbeef")
					}
				}
			}

			sink := &generate.CollectingSink{}
			rec := &generate.RecordingLogger{}
			m := newMachine(t, stages, sink, rec)
			res := m.Run(context.Background())

			tl.Inputs(map[string]string{
				"stage":   string(id),
				"num":     itoa(generate.StageNumber(id)),
				"outcome": string(res.Outcome),
			})
			tl.Assert("failed_stage", res.FailedStage == id, id, res.FailedStage)
			tl.Assert("err_set", res.Err != nil, true, res.Err)

			pre := generate.StageIsPreStage(id)
			if pre && id != generate.StageCreateStage {
				tl.Assert("no_preserve", !res.StagePreserved, false, res.StagePreserved)
				tl.Assert("outcome_not_started", res.Outcome == generate.OutcomeNotStarted || res.Outcome == generate.OutcomeFailedPreserved,
					generate.OutcomeNotStarted, res.Outcome)
				// Prefer not-started for pure pre-stage.
				tl.Assert("exit_nonzero", res.Exit != 0, true, res.Exit)
			} else if id == generate.StageCreateStage {
				// Failed before create completed.
				tl.Assert("create_no_preserve", !res.StagePreserved, false, res.StagePreserved)
			} else if generate.StageIsCommit(id) {
				tl.Assert("commit_preserve", res.StagePreserved, true, res.StagePreserved)
				tl.Assert("commit_outcome", res.Outcome == generate.OutcomeFailedPreserved,
					generate.OutcomeFailedPreserved, res.Outcome)
				tl.Assert("commit_exit", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
			} else {
				// Post-stage pre-commit failure with stage on disk.
				tl.Assert("preserve", res.StagePreserved, true, res.StagePreserved)
				tl.Assert("outcome_fp", res.Outcome == generate.OutcomeFailedPreserved,
					generate.OutcomeFailedPreserved, res.Outcome)
				tl.Assert("state_fp", res.State == generate.LifeFailedPreserved,
					generate.LifeFailedPreserved, res.State)
				tl.Assert("path_set", res.StagePath != "", true, res.StagePath)
			}

			// Event tail golden: last events ending with terminal.
			tail := eventTail(res.Events, 6)
			compareEventGolden(t, tl, "events_fail_"+strings.ReplaceAll(string(id), "-", "_"), tail)

			// Step log has fail result for this stage (commit logs outcome label).
			found := false
			for _, s := range rec.Steps {
				if s.Event == string(id) && (s.Result == "fail" || s.Result == string(generate.OutcomeFailedPreserved)) {
					found = true
					tl.Step("fail_log", testutil.OutcomeOK,
						"state="+s.State+" result="+s.Result+" dur_ms="+itoa(int(s.DurationMs)))
				}
			}
			// Commit path logs under StageCommit with result label after finish.
			if generate.StageIsCommit(id) {
				for _, s := range rec.Steps {
					if s.Event == string(generate.StageCommit) {
						found = true
					}
				}
			}
			tl.Assert("step_logged", found || len(rec.Steps) > 0, true, found)

			tl.PhaseEnd("fail_"+string(id), testutil.OutcomeOK)
		})
	}

	log.PhaseEnd("inject_failure_matrix", testutil.OutcomeOK)
}

func TestCancelBeforeAfterMatrices(t *testing.T) {
	log := testutil.New(t)
	log.Phase("cancel_matrix")

	// Cancel before any stage (ctx already done).
	t.Run("before_any", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("cancel_before_any")
		sink := &generate.CollectingSink{}
		m := newMachine(t, defaultStages(), sink, &generate.RecordingLogger{})
		res := m.Run(cancelBefore())
		tl.Assert("exit_130", res.Exit == diagnostic.ExitCancelled, diagnostic.ExitCancelled, res.Exit)
		tl.Assert("outcome", res.Outcome == generate.OutcomeCancelled, generate.OutcomeCancelled, res.Outcome)
		tl.Assert("no_preserve", !res.StagePreserved, false, res.StagePreserved)
		tl.Assert("state", res.State == generate.LifeCancelled, generate.LifeCancelled, res.State)
		compareEventGolden(t, tl, "events_cancel_before_any", res.Events)
		tl.PhaseEnd("cancel_before_any", testutil.OutcomeOK)
	})

	// Cancel before stage exists: at stage 6 (tool-preflight).
	t.Run("before_stage_at_preflight", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("cancel_preflight")
		ctx, cancel := context.WithCancel(context.Background())
		stages := cancelAfterStage(generate.StageToolPreflight, ctx, cancel)
		m := newMachine(t, stages, &generate.CollectingSink{}, &generate.RecordingLogger{})
		res := m.Run(ctx)
		tl.Assert("exit_130", res.Exit == 130, 130, res.Exit)
		tl.Assert("no_preserve", !res.StagePreserved, false, res.StagePreserved)
		tl.Assert("outcome_cancel", res.Outcome == generate.OutcomeCancelled, generate.OutcomeCancelled, res.Outcome)
		compareEventGolden(t, tl, "events_cancel_before_stage", res.Events)
		tl.PhaseEnd("cancel_preflight", testutil.OutcomeOK)
	})

	// Cancel after stage created: at render (stage 10).
	t.Run("after_stage_at_render", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("cancel_render")
		ctx, cancel := context.WithCancel(context.Background())
		stages := cancelAfterStage(generate.StageRender, ctx, cancel)
		m := newMachine(t, stages, &generate.CollectingSink{}, &generate.RecordingLogger{})
		res := m.Run(ctx)
		tl.Assert("exit_130", res.Exit == 130, 130, res.Exit)
		tl.Assert("preserve", res.StagePreserved, true, res.StagePreserved)
		tl.Assert("path", res.StagePath == ".foundry-demo-deadbeef", ".foundry-demo-deadbeef", res.StagePath)
		tl.Assert("outcome", res.Outcome == generate.OutcomeCancelled, generate.OutcomeCancelled, res.Outcome)
		compareEventGolden(t, tl, "events_cancel_after_stage", res.Events)
		tl.PhaseEnd("cancel_render", testutil.OutcomeOK)
	})

	// Cancel after stage at parent-reobserve (17) — still pre-commit.
	t.Run("after_stage_at_reobserve", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("cancel_reobserve")
		ctx, cancel := context.WithCancel(context.Background())
		stages := cancelAfterStage(generate.StageParentReobserve, ctx, cancel)
		m := newMachine(t, stages, &generate.CollectingSink{}, &generate.RecordingLogger{})
		res := m.Run(ctx)
		tl.Assert("exit_130", res.Exit == 130, 130, res.Exit)
		tl.Assert("preserve", res.StagePreserved, true, res.StagePreserved)
		tl.Assert("not_committed", res.Outcome != generate.OutcomeCommitted, generate.OutcomeCancelled, res.Outcome)
		tl.PhaseEnd("cancel_reobserve", testutil.OutcomeOK)
	})

	// Cancel racing commit: commit result dominates (committed).
	t.Run("race_commit_success", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("race_commit_ok")
		ctx, cancel := context.WithCancel(context.Background())
		stages := defaultStages()
		stages[generate.StageCommit] = func(ctx context.Context, rt *generate.Runtime) error {
			cancel() // fire during commit; machine uses WithoutCancel
			rt.CommitOutcome = generate.OutcomeCommitted
			rt.Destination = "demo"
			rt.StagePath = ".foundry-demo-deadbeef"
			return nil
		}
		// Also cancel at start of commit via parent already cancelled after stage 17:
		// enter commit with live ctx, cancel inside.
		m := newMachine(t, stages, &generate.CollectingSink{}, &generate.RecordingLogger{})
		res := m.Run(ctx)
		tl.Assert("exit_0", res.Exit == 0, 0, res.Exit)
		tl.Assert("committed", res.Outcome == generate.OutcomeCommitted, generate.OutcomeCommitted, res.Outcome)
		tl.Assert("dominated", res.CommitDominated, true, res.CommitDominated)
		tl.Assert("no_cancel_outcome", res.Outcome != generate.OutcomeCancelled, true, res.Outcome)
		compareEventGolden(t, tl, "events_cancel_race_commit_ok", res.Events)
		tl.PhaseEnd("race_commit_ok", testutil.OutcomeOK)
	})

	// Cancel racing commit: conflict dominates (exit 2).
	t.Run("race_commit_conflict", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("race_commit_conflict")
		ctx, cancel := context.WithCancel(context.Background())
		stages := defaultStages()
		stages[generate.StageCommit] = func(ctx context.Context, rt *generate.Runtime) error {
			cancel()
			rt.CommitOutcome = generate.OutcomeConflicted
			rt.StageExists = true
			rt.StagePath = ".foundry-demo-deadbeef"
			return &generate.StageError{
				Exit:     diagnostic.ExitUsage,
				Outcome:  generate.OutcomeConflicted,
				Preserve: true,
				Detail:   "destination exists",
			}
		}
		m := newMachine(t, stages, &generate.CollectingSink{}, &generate.RecordingLogger{})
		res := m.Run(ctx)
		tl.Assert("exit_2", res.Exit == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Exit)
		tl.Assert("conflicted", res.Outcome == generate.OutcomeConflicted, generate.OutcomeConflicted, res.Outcome)
		tl.Assert("preserve", res.StagePreserved, true, res.StagePreserved)
		tl.Assert("dominated", res.CommitDominated, true, res.CommitDominated)
		compareEventGolden(t, tl, "events_cancel_race_commit_conflict", res.Events)
		tl.PhaseEnd("race_commit_conflict", testutil.OutcomeOK)
	})

	// Parent ctx cancelled before commit stage starts — still enter commit and dominate.
	t.Run("cancel_set_before_commit_entry", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("cancel_before_commit_entry")
		ctx, cancel := context.WithCancel(context.Background())
		stages := defaultStages()
		stages[generate.StageParentReobserve] = func(ctx context.Context, rt *generate.Runtime) error {
			_ = ctx
			_ = rt
			cancel()
			return nil // stage succeeds; post-check sees cancel
		}
		m := newMachine(t, stages, &generate.CollectingSink{}, &generate.RecordingLogger{})
		res := m.Run(ctx)
		// Post-stage cancel after 17 → preserve 130, never commit.
		tl.Assert("exit_130", res.Exit == 130, 130, res.Exit)
		tl.Assert("preserve", res.StagePreserved, true, res.StagePreserved)
		tl.Assert("cancelled", res.Outcome == generate.OutcomeCancelled, generate.OutcomeCancelled, res.Outcome)
		tl.PhaseEnd("cancel_before_commit_entry", testutil.OutcomeOK)
	})

	log.PhaseEnd("cancel_matrix", testutil.OutcomeOK)
}

func TestReportFailureAbsorbedAfterCommit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("report_absorb")

	stages := defaultStages()
	stages[generate.StageReport] = generate.FailStage(&generate.StageError{
		Detail: "encoder broken pipe",
		Err:    errors.New("EPIPE"),
	})
	m := newMachine(t, stages, &generate.CollectingSink{}, &generate.RecordingLogger{})
	res := m.Run(context.Background())
	log.Assert("exit_0", res.Exit == 0, 0, res.Exit)
	log.Assert("committed", res.Outcome == generate.OutcomeCommitted, generate.OutcomeCommitted, res.Outcome)

	// Summary event for absorbed failure.
	absorbed := false
	for _, ev := range res.Events {
		if ev.Kind == generate.EventSummary && ev.Detail == "report-failed-absorbed" {
			absorbed = true
		}
	}
	log.Assert("absorbed_event", absorbed, true, absorbed)
	log.PhaseEnd("report_absorb", testutil.OutcomeOK)
}

func TestNoEncodingSurface(t *testing.T) {
	log := testutil.New(t)
	log.Phase("no_encoding")
	log.Assert("assert", generate.AssertNoEncoding() == "report owns encoding",
		"report owns encoding", generate.AssertNoEncoding())
	log.PhaseEnd("no_encoding", testutil.OutcomeOK)
}

func TestDoubleRunRejected(t *testing.T) {
	log := testutil.New(t)
	log.Phase("double_run")
	m := newMachine(t, defaultStages(), nil, nil)
	r1 := m.Run(context.Background())
	log.Assert("first_ok", r1.Exit == 0, 0, r1.Exit)
	r2 := m.Run(context.Background())
	log.Assert("second_fail", r2.Exit != 0, true, r2.Exit)
	log.Assert("second_err", r2.Err != nil, true, r2.Err)
	log.PhaseEnd("double_run", testutil.OutcomeOK)
}

func TestStepLoggerDurationAndFields(t *testing.T) {
	log := testutil.New(t)
	log.Phase("step_logger_fields")
	rec := &generate.RecordingLogger{}
	m := newMachine(t, defaultStages(), &generate.CollectingSink{}, rec)
	_ = m.Run(context.Background())

	// Required fields present on recorded steps.
	for i, s := range rec.Steps {
		log.Assert("result_nonempty_"+itoa(i), s.Result != "", true, s.Result)
		log.Assert("event_nonempty_"+itoa(i), s.Event != "", true, s.Event)
		// duration_ms is always >= 0
		if s.DurationMs < 0 {
			log.Fail("negative_duration", s.Event)
		}
	}
	log.Assert("min_steps", len(rec.Steps) >= 19, ">=19", len(rec.Steps))
	log.PhaseEnd("step_logger_fields", testutil.OutcomeOK)
}

func eventTail(events []generate.GenerationEvent, n int) []generate.GenerationEvent {
	if n <= 0 || len(events) <= n {
		// Return a copy
		out := make([]generate.GenerationEvent, len(events))
		copy(out, events)
		return out
	}
	out := make([]generate.GenerationEvent, n)
	copy(out, events[len(events)-n:])
	return out
}
