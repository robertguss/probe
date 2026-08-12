package generate_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestCommitDominatesReportingTable covers success/conflict/fail × report ok/fail
// (Section 31.9 / FND-012 / REQ-158). Report stream failure never changes exit.
func TestCommitDominatesReportingTable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("commit_dominates_matrix")

	type row struct {
		name     string
		outcome  generate.CommitOutcome
		reportOK bool
		wantExit int
	}
	rows := []row{
		{"committed_report_ok", generate.OutcomeCommitted, true, 0},
		{"committed_report_fail", generate.OutcomeCommitted, false, 0},
		{"conflicted_report_ok", generate.OutcomeConflicted, true, 2},
		{"conflicted_report_fail", generate.OutcomeConflicted, false, 2},
		{"failed_report_ok", generate.OutcomeFailedPreserved, true, 1},
		{"failed_report_fail", generate.OutcomeFailedPreserved, false, 1},
		{"ambiguous_report_ok", generate.OutcomeAmbiguous, true, 1},
		{"ambiguous_report_fail", generate.OutcomeAmbiguous, false, 1},
		{"cancelled_report_ok", generate.OutcomeCancelled, true, 130},
		{"cancelled_report_fail", generate.OutcomeCancelled, false, 130},
		{"not_started_report_ok", generate.OutcomeNotStarted, true, 1},
		{"not_started_report_fail", generate.OutcomeNotStarted, false, 1},
	}

	for _, r := range rows {
		r := r
		t.Run(r.name, func(t *testing.T) {
			tl := testutil.New(t)
			tl.Phase(r.name)
			got := generate.DominatedExit(r.outcome, r.reportOK)
			tl.Inputs(map[string]string{
				"outcome":   string(r.outcome),
				"report_ok": itoa(boolInt(r.reportOK)),
			})
			tl.Assert("exit", got == r.wantExit, r.wantExit, got)
			// reportOK must not affect DominatedExit (pair rows share exit).
			other := generate.DominatedExit(r.outcome, !r.reportOK)
			tl.Assert("report_irrelevant", other == got, got, other)
			tl.PhaseEnd(r.name, testutil.OutcomeOK)
		})
	}

	log.PhaseEnd("commit_dominates_matrix", testutil.OutcomeOK)
}

// TestClassifyStreamFailurePrePost covers pre-commit vs post-commit policy.
func TestClassifyStreamFailurePrePost(t *testing.T) {
	log := testutil.New(t)
	log.Phase("classify_stream")

	// Pre-commit, no stage: exit 1, block, no preserve.
	sf := generate.ClassifyStreamFailure(
		generate.StreamPreCommit, false, "", generate.StreamStdout, "EPIPE", syscall.EPIPE,
	)
	log.Assert("pre_exit_1", sf.Exit == 1, 1, sf.Exit)
	log.Assert("pre_block", sf.BlockPlacement, true, sf.BlockPlacement)
	log.Assert("pre_no_preserve", !sf.Preserve, false, sf.Preserve)
	log.Assert("pre_not_absorb", !sf.Absorb, false, sf.Absorb)
	log.Assert("pre_errno", sf.Errno == "EPIPE", "EPIPE", sf.Errno)
	log.Assert("pre_phase", sf.Phase == generate.StreamPreCommit, generate.StreamPreCommit, sf.Phase)

	// Pre-commit, stage exists: preserve.
	sf2 := generate.ClassifyStreamFailure(
		generate.StreamPreCommit, true, "", generate.StreamStdout, "EPIPE", nil,
	)
	log.Assert("pre_preserve", sf2.Preserve, true, sf2.Preserve)
	log.Assert("pre2_exit", sf2.Exit == 1, 1, sf2.Exit)

	// Post-commit committed: absorb, exit 0.
	sf3 := generate.ClassifyStreamFailure(
		generate.StreamPostCommit, false, generate.OutcomeCommitted, generate.StreamStdout, "EPIPE", syscall.EPIPE,
	)
	log.Assert("post_absorb", sf3.Absorb, true, sf3.Absorb)
	log.Assert("post_exit_0", sf3.Exit == 0, 0, sf3.Exit)
	log.Assert("post_no_block", !sf3.BlockPlacement, false, sf3.BlockPlacement)

	// Post-commit conflicted: absorb but exit stays 2 (commit dominates).
	sf4 := generate.ClassifyStreamFailure(
		generate.StreamPostCommit, true, generate.OutcomeConflicted, generate.StreamStderr, "EPIPE", nil,
	)
	log.Assert("post_conflict_exit_2", sf4.Exit == 2, 2, sf4.Exit)
	log.Assert("post_conflict_absorb", sf4.Absorb, true, sf4.Absorb)

	log.PhaseEnd("classify_stream", testutil.OutcomeOK)
}

// TestStreamStepLoggerPhaseStreamExit asserts step logs carry phase, stream,
// errno, and exit (acceptance criterion).
func TestStreamStepLoggerPhaseStreamExit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("stream_step_logger")

	rec := &generate.RecordingLogger{}
	cases := []generate.StreamFailure{
		generate.ClassifyStreamFailure(generate.StreamPreCommit, true, "", generate.StreamStdout, "EPIPE", syscall.EPIPE),
		generate.ClassifyStreamFailure(generate.StreamPostCommit, false, generate.OutcomeCommitted, generate.StreamStdout, "EPIPE", syscall.EPIPE),
		generate.ClassifyStreamFailure(generate.StreamPostCommit, false, generate.OutcomeConflicted, generate.StreamStderr, "EIO", errors.New("input/output error")),
	}
	for i, sf := range cases {
		sf := sf
		t.Run(fmt.Sprintf("case_%d_%s", i, sf.Phase), func(t *testing.T) {
			generate.LogStreamDecision(rec, sf)
		})
	}
	log.Assert("steps", len(rec.Steps) == 3, 3, len(rec.Steps))

	// Pre-commit step.
	s0 := rec.Steps[0]
	log.Assert("s0_phase", s0.State == string(generate.StreamPreCommit), string(generate.StreamPreCommit), s0.State)
	log.Assert("s0_stream", s0.Event == "stream:stdout", "stream:stdout", s0.Event)
	log.Assert("s0_errno", strings.Contains(s0.Result, "errno=EPIPE"), true, s0.Result)
	log.Assert("s0_exit", strings.Contains(s0.Result, "exit=1"), true, s0.Result)
	log.Step("pre_commit_log", testutil.OutcomeOK, "state="+s0.State+" event="+s0.Event+" result="+s0.Result)

	// Post-commit absorbed step.
	s1 := rec.Steps[1]
	log.Assert("s1_phase", s1.State == string(generate.StreamPostCommit), string(generate.StreamPostCommit), s1.State)
	log.Assert("s1_exit0", strings.Contains(s1.Result, "exit=0"), true, s1.Result)
	log.Assert("s1_absorbed", strings.Contains(s1.Result, "absorbed=true"), true, s1.Result)
	log.Step("post_commit_log", testutil.OutcomeOK, "state="+s1.State+" event="+s1.Event+" result="+s1.Result)

	// Conflicted post-commit keeps exit 2 in the log.
	s2 := rec.Steps[2]
	log.Assert("s2_exit2", strings.Contains(s2.Result, "exit=2"), true, s2.Result)
	log.Assert("s2_stream_err", s2.Event == "stream:stderr", "stream:stderr", s2.Event)

	log.PhaseEnd("stream_step_logger", testutil.OutcomeOK)
}

// TestMachineReportFailAbsorbedWithStreamLog wires StageReport failure and
// checks exit 0 + stream step log fields.
func TestMachineReportFailAbsorbedWithStreamLog(t *testing.T) {
	log := testutil.New(t)
	log.Phase("machine_report_stream_log")

	rec := &generate.RecordingLogger{}
	stages := defaultStages()
	stages[generate.StageReport] = generate.FailStage(&generate.StageError{
		Detail: "encoder broken pipe",
		Err:    syscall.EPIPE,
	})
	m := newMachine(t, stages, &generate.CollectingSink{}, rec)
	res := m.Run(context.Background())

	log.Assert("exit_0", res.Exit == 0, 0, res.Exit)
	log.Assert("committed", res.Outcome == generate.OutcomeCommitted, generate.OutcomeCommitted, res.Outcome)

	// Must have a stream:stdout post-commit log with exit=0.
	found := false
	for _, s := range rec.Steps {
		if s.State == string(generate.StreamPostCommit) && strings.HasPrefix(s.Event, "stream:") {
			found = true
			log.Assert("log_exit0", strings.Contains(s.Result, "exit=0"), true, s.Result)
			log.Assert("log_absorbed", strings.Contains(s.Result, "absorbed=true"), true, s.Result)
			log.Assert("log_errno", strings.Contains(s.Result, "errno="), true, s.Result)
			log.Step("stream_decision", testutil.OutcomeOK,
				"state="+s.State+" event="+s.Event+" result="+s.Result)
		}
		if s.Event == string(generate.StageReport) && s.Result == "report_fail_absorbed" {
			log.Step("report_stage", testutil.OutcomeOK, "result="+s.Result)
		}
	}
	log.Assert("stream_step_present", found, true, found)

	log.PhaseEnd("machine_report_stream_log", testutil.OutcomeOK)
}

// TestPreCommitStreamFailureBlocksPlacement models report.failed before commit:
// stage preserved, exit 1, never committed.
func TestPreCommitStreamFailureBlocksPlacement(t *testing.T) {
	log := testutil.New(t)
	log.Phase("precommit_stream_fail")

	rec := &generate.RecordingLogger{}
	// Fail at report-plan-network (stage 7, pre-stage) and at render (post-stage)
	// as stream failures.
	t.Run("pre_stage", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("pre_stage_stream")
		stages := map[generate.StageID]generate.StageFunc{
			generate.StageReportPlanNetwork: generate.FailStage(
				generate.StreamFailStageError(generate.StreamStdout, syscall.EPIPE),
			),
		}
		m := newMachine(t, stages, &generate.CollectingSink{}, rec)
		res := m.Run(context.Background())
		tl.Assert("exit_1", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
		tl.Assert("no_preserve", !res.StagePreserved, false, res.StagePreserved)
		tl.Assert("not_committed", res.Outcome != generate.OutcomeCommitted, true, res.Outcome)
		tl.Assert("failed_stage", res.FailedStage == generate.StageReportPlanNetwork,
			generate.StageReportPlanNetwork, res.FailedStage)
		// Classification log.
		sf := generate.ClassifyStreamFailure(generate.StreamPreCommit, false, "", generate.StreamStdout, "EPIPE", syscall.EPIPE)
		generate.LogStreamDecision(rec, sf)
		tl.PhaseEnd("pre_stage_stream", testutil.OutcomeOK)
	})

	t.Run("post_stage_preserve", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("post_stage_stream")
		rec2 := &generate.RecordingLogger{}
		stages := defaultStages()
		stages[generate.StageRender] = generate.FailStage(
			generate.StreamFailStageError(generate.StreamStdout, syscall.EPIPE),
		)
		// Do not run commit.
		delete(stages, generate.StageCommit)
		m := newMachine(t, stages, &generate.CollectingSink{}, rec2)
		res := m.Run(context.Background())
		tl.Assert("exit_1", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
		tl.Assert("preserve", res.StagePreserved, true, res.StagePreserved)
		tl.Assert("path", res.StagePath != "", true, res.StagePath)
		tl.Assert("failed_preserved", res.Outcome == generate.OutcomeFailedPreserved,
			generate.OutcomeFailedPreserved, res.Outcome)
		tl.Assert("not_committed", res.Outcome != generate.OutcomeCommitted, true, res.Outcome)

		sf := generate.ClassifyStreamFailure(generate.StreamPreCommit, true, "", generate.StreamStdout, "EPIPE", syscall.EPIPE)
		generate.LogStreamDecision(rec2, sf)
		found := false
		for _, s := range rec2.Steps {
			if s.State == string(generate.StreamPreCommit) && s.Event == "stream:stdout" {
				found = true
				tl.Assert("exit_in_log", strings.Contains(s.Result, "exit=1"), true, s.Result)
			}
		}
		tl.Assert("logged", found, true, found)
		tl.PhaseEnd("post_stage_stream", testutil.OutcomeOK)
	})

	log.PhaseEnd("precommit_stream_fail", testutil.OutcomeOK)
}

// TestCommitOutcomeMatrixWithReportInjection runs the machine for each commit
// outcome with a failing report stage and asserts DominatedExit holds.
func TestCommitOutcomeMatrixWithReportInjection(t *testing.T) {
	log := testutil.New(t)
	log.Phase("machine_commit_x_report")

	type row struct {
		name    string
		outcome generate.CommitOutcome
		exit    int
		err     error
	}
	rows := []row{
		{"committed", generate.OutcomeCommitted, 0, nil},
		{"conflicted", generate.OutcomeConflicted, 2, &generate.StageError{
			Exit: 2, Outcome: generate.OutcomeConflicted, Preserve: true, Detail: "EEXIST",
		}},
		{"failed", generate.OutcomeFailedPreserved, 1, &generate.StageError{
			Exit: 1, Outcome: generate.OutcomeFailedPreserved, Preserve: true, Detail: "rename failed",
		}},
		{"ambiguous", generate.OutcomeAmbiguous, 1, &generate.StageError{
			Exit: 1, Outcome: generate.OutcomeAmbiguous, Preserve: true, Detail: "ambiguous",
		}},
	}

	for _, r := range rows {
		r := r
		for _, reportOK := range []bool{true, false} {
			name := r.name + "_report_" + map[bool]string{true: "ok", false: "fail"}[reportOK]
			t.Run(name, func(t *testing.T) {
				tl := testutil.New(t)
				tl.Phase(name)
				stages := defaultStages()
				stages[generate.StageCommit] = generate.CommitStage(
					r.outcome, "demo", ".foundry-demo-deadbeef", r.err,
				)
				if !reportOK {
					stages[generate.StageReport] = generate.FailStage(&generate.StageError{
						Detail: "broken pipe",
						Err:    syscall.EPIPE,
					})
				}
				rec := &generate.RecordingLogger{}
				m := newMachine(t, stages, &generate.CollectingSink{}, rec)
				res := m.Run(context.Background())
				want := generate.DominatedExit(r.outcome, reportOK)
				tl.Assert("exit", res.Exit == want, want, res.Exit)
				tl.Assert("outcome", res.Outcome == r.outcome, r.outcome, res.Outcome)
				if r.outcome == generate.OutcomeCommitted {
					tl.Assert("no_preserve", !res.StagePreserved, false, res.StagePreserved)
				} else {
					tl.Assert("preserve", res.StagePreserved, true, res.StagePreserved)
				}
				// Report fail must not flip exit vs report ok for same outcome.
				tl.Assert("dominated", res.Exit == r.exit, r.exit, res.Exit)
				tl.PhaseEnd(name, testutil.OutcomeOK)
			})
		}
	}

	log.PhaseEnd("machine_commit_x_report", testutil.OutcomeOK)
}

// TestNeverCommittedErrorJSONLie documents FND-012 guard.
func TestNeverCommittedErrorJSONLie(t *testing.T) {
	log := testutil.New(t)
	log.Phase("fnd012_lie")
	log.Assert("lie_detected", generate.NeverCommittedErrorJSON(generate.OutcomeCommitted, false), true, true)
	log.Assert("ok_committed_fine", !generate.NeverCommittedErrorJSON(generate.OutcomeCommitted, true), false, false)
	log.Assert("fail_uncommitted_fine", !generate.NeverCommittedErrorJSON(generate.OutcomeFailedPreserved, false), false, false)
	log.PhaseEnd("fnd012_lie", testutil.OutcomeOK)
}

// TestErrnoTokenStable maps common errors to stable tokens.
func TestErrnoTokenStable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("errno_token")
	cases := []struct {
		err  error
		want string
	}{
		{nil, "none"},
		{syscall.EPIPE, "EPIPE"},
		{io.ErrClosedPipe, "EPIPE"},
		{errors.New("write: broken pipe"), "EPIPE"},
		{os.ErrClosed, "EPIPE"},
		{errors.New("something else"), "unknown"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.want, func(t *testing.T) {
			got := generate.ErrnoToken(c.err)
			log.Assert("token_"+c.want, got == c.want, c.want, got)
		})
	}
	log.Assert("is_broken", generate.IsBrokenPipe(syscall.EPIPE), true, true)
	log.PhaseEnd("errno_token", testutil.OutcomeOK)
}

// TestRealPipeWritePrePostCommit closes the reader end of a real OS pipe and
// classifies pre vs post commit (package-level real-pipe evidence for fy9;
// process-boundary SIGPIPE is in cmd/foundry).
func TestRealPipeWritePrePostCommit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("real_pipe")

	// Pre-commit: write to closed pipe → EPIPE class, exit 1, block placement.
	t.Run("pre_commit_closed_reader", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("pre_pipe")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		_ = r.Close() // break pipe before write
		_, werr := w.Write([]byte("foundry-progress\n"))
		_ = w.Close()
		tl.Assert("write_err", werr != nil, true, werr)
		tl.Assert("epipe_class", generate.IsBrokenPipe(werr), true, generate.ErrnoToken(werr))

		sf := generate.ClassifyStreamFailure(
			generate.StreamPreCommit, true, "", generate.StreamStdout, "", werr,
		)
		tl.Assert("exit_1", sf.Exit == 1, 1, sf.Exit)
		tl.Assert("block", sf.BlockPlacement, true, sf.BlockPlacement)
		tl.Assert("preserve", sf.Preserve, true, sf.Preserve)
		tl.Assert("not_absorb", !sf.Absorb, false, sf.Absorb)
		rec := &generate.RecordingLogger{}
		generate.LogStreamDecision(rec, sf)
		tl.Assert("logged", len(rec.Steps) == 1, 1, len(rec.Steps))
		tl.Step("pre_pipe_decision", testutil.OutcomeOK, rec.Steps[0].Result)
		tl.PhaseEnd("pre_pipe", testutil.OutcomeOK)
	})

	// Post-commit: same real pipe failure is absorbed → exit 0.
	t.Run("post_commit_closed_reader", func(t *testing.T) {
		tl := testutil.New(t)
		tl.Phase("post_pipe")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		_ = r.Close()
		_, werr := w.Write([]byte(`{"ok":true}` + "\n"))
		_ = w.Close()
		tl.Assert("write_err", werr != nil, true, werr)

		sf := generate.ClassifyStreamFailure(
			generate.StreamPostCommit, false, generate.OutcomeCommitted, generate.StreamStdout, "", werr,
		)
		tl.Assert("exit_0", sf.Exit == 0, 0, sf.Exit)
		tl.Assert("absorb", sf.Absorb, true, sf.Absorb)
		tl.Assert("errno", sf.Errno == "EPIPE", "EPIPE", sf.Errno)
		rec := &generate.RecordingLogger{}
		generate.LogStreamDecision(rec, sf)
		tl.Assert("log_phase", rec.Steps[0].State == string(generate.StreamPostCommit),
			string(generate.StreamPostCommit), rec.Steps[0].State)
		tl.PhaseEnd("post_pipe", testutil.OutcomeOK)
	})

	log.PhaseEnd("real_pipe", testutil.OutcomeOK)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
