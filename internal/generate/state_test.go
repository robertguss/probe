package generate_test

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestLegalTransitionGraphCompleteness(t *testing.T) {
	log := testutil.New(t)
	log.Phase("transition_graph")

	// Section 29.1 success spine (no terminals).
	spine := []generate.LifecycleState{
		generate.LifePlanned,
		generate.LifeParentAcquired,
		generate.LifeStageCreated,
		generate.LifeRendered,
		generate.LifeNormalized,
		generate.LifeFrozen,
		generate.LifeVerified,
		generate.LifeConformed,
		generate.LifeGitInitialized,
		generate.LifeCommitting,
	}
	for i := 0; i < len(spine)-1; i++ {
		from, to := spine[i], spine[i+1]
		ok := generate.LegalTransition(from, to)
		log.Assert("edge_"+string(from)+"_"+string(to), ok, true, ok)
	}

	// Commit terminals from committing (Section 31.9).
	for _, term := range []generate.LifecycleState{
		generate.LifeCommitted,
		generate.LifeConflicted,
		generate.LifeFailedPreserved,
		generate.LifeAmbiguous,
	} {
		ok := generate.LegalTransition(generate.LifeCommitting, term)
		log.Assert("commit_"+string(term), ok, true, ok)
		log.Assert("terminal_"+string(term), generate.IsTerminal(term), true, false)
	}

	// Cancel edges from every non-terminal success state except committing
	// (cancel during commit is classified by commit result, not LifeCancelled).
	for _, from := range spine {
		if from == generate.LifeCommitting {
			ok := generate.LegalTransition(from, generate.LifeCancelled)
			log.Assert("no_cancel_from_committing", !ok, false, ok)
			continue
		}
		ok := generate.LegalTransition(from, generate.LifeCancelled)
		log.Assert("cancel_from_"+string(from), ok, true, ok)
	}

	// Failed-preserved from every non-terminal success state.
	for _, from := range spine {
		if from == generate.LifeCommitting {
			// committing → failed-preserved is legal (commit matrix).
			ok := generate.LegalTransition(from, generate.LifeFailedPreserved)
			log.Assert("fail_from_committing", ok, true, ok)
			continue
		}
		ok := generate.LegalTransition(from, generate.LifeFailedPreserved)
		log.Assert("fail_from_"+string(from), ok, true, ok)
	}

	// Optional git path: conformed may go directly to committing.
	log.Assert("conformed_to_committing",
		generate.LegalTransition(generate.LifeConformed, generate.LifeCommitting),
		true, false)

	// Terminals have no outgoing edges.
	for _, term := range generate.TerminalStates() {
		for _, to := range generate.LifecycleStates() {
			if generate.LegalTransition(term, to) {
				log.Fail("terminal_outgoing", string(term)+"→"+string(to))
			}
		}
	}

	edges := generate.AllLegalEdges()
	log.Assert("edge_count_positive", len(edges) > 20, ">20", len(edges))
	log.Inputs(map[string]string{"legal_edges": string(rune(len(edges) + '0'))})
	log.Step("edges", testutil.OutcomeOK, "count="+itoa(len(edges)))

	log.PhaseEnd("transition_graph", testutil.OutcomeOK)
}

func TestRejectSkipReorder(t *testing.T) {
	log := testutil.New(t)
	log.Phase("reject_skip_reorder")

	// Skip: planned → rendered
	err := generate.IllegalTransitionProbe(generate.LifePlanned, generate.LifeRendered)
	log.Assert("skip_planned_rendered", err != nil, true, err)
	if err != nil {
		log.Step("skip_err", testutil.OutcomeOK, err.Error())
	}

	// Reorder: stage-created → planned
	err = generate.IllegalTransitionProbe(generate.LifeStageCreated, generate.LifePlanned)
	log.Assert("reorder_backwards", err != nil, true, err)

	// Skip commit intermediates: verified → committed
	err = generate.IllegalTransitionProbe(generate.LifeVerified, generate.LifeCommitted)
	log.Assert("skip_to_committed", err != nil, true, err)

	// From terminal
	err = generate.IllegalTransitionProbe(generate.LifeCommitted, generate.LifePlanned)
	log.Assert("from_terminal", err != nil, true, err)

	// Legal still works
	err = generate.IllegalTransitionProbe(generate.LifePlanned, generate.LifeParentAcquired)
	log.Assert("legal_ok", err == nil, true, err)

	// Machine.Transition rejects the same way
	m := generate.New(generate.Config{InitialState: generate.LifeFrozen})
	err = m.Transition(generate.LifeRendered)
	log.Assert("machine_reject_reorder", err != nil, true, err)
	log.Assert("state_unchanged", m.State() == generate.LifeFrozen, generate.LifeFrozen, m.State())

	err = m.Transition(generate.LifeVerified)
	log.Assert("machine_legal", err == nil, true, err)
	log.Assert("state_advanced", m.State() == generate.LifeVerified, generate.LifeVerified, m.State())

	log.PhaseEnd("reject_skip_reorder", testutil.OutcomeOK)
}

func TestStageTableMatchesSection292(t *testing.T) {
	log := testutil.New(t)
	log.Phase("stage_table_parity")

	// Hard-coded Section 29.2 spellings (must match report.StageID without
	// importing report — layer_direction forbids generate → report even in tests).
	wantStages := []string{
		"read-parse-spec",
		"validate-spec",
		"load-catalog",
		"resolve",
		"build-plan",
		"tool-preflight",
		"report-plan-network",
		"acquire-parent",
		"create-stage",
		"render",
		"go-mod-tidy",
		"freeze-tree",
		"verify-tools",
		"final-conformance",
		"git-init",
		"git-template-cleanup",
		"parent-reobserve",
		"commit",
		"report",
	}
	gStages := generate.AllStageIDs()
	log.Assert("count_19", len(gStages) == 19, 19, len(gStages))
	log.Assert("count_want", len(gStages) == len(wantStages), len(wantStages), len(gStages))
	for i := range wantStages {
		log.Assert("stage_"+itoa(i+1), string(gStages[i]) == wantStages[i], wantStages[i], gStages[i])
	}

	wantLife := []string{
		"planned",
		"parent-acquired",
		"stage-created",
		"rendered",
		"normalized",
		"frozen",
		"verified",
		"conformed",
		"git-initialized",
		"committing",
		"committed",
		"conflicted",
		"failed-preserved",
		"ambiguous",
	}
	gLife := generate.LifecycleStates()
	log.Assert("life_count", len(gLife) == len(wantLife), len(wantLife), len(gLife))
	for i := range wantLife {
		log.Assert("life_"+wantLife[i], string(gLife[i]) == wantLife[i], wantLife[i], gLife[i])
	}

	// Stage predicates
	log.Assert("pre_1", generate.StageIsPreStage(generate.StageReadParseSpec), true, false)
	log.Assert("pre_8", generate.StageIsPreStage(generate.StageAcquireParent), true, false)
	log.Assert("pre_9_false", !generate.StageIsPreStage(generate.StageCreateStage), false, true)
	log.Assert("post_9", generate.StageIsPostStagePreCommit(generate.StageCreateStage), true, false)
	log.Assert("post_17", generate.StageIsPostStagePreCommit(generate.StageParentReobserve), true, false)
	log.Assert("commit_18", generate.StageIsCommit(generate.StageCommit), true, false)
	log.Assert("report_19", generate.StageIsReport(generate.StageReport), true, false)

	log.PhaseEnd("stage_table_parity", testutil.OutcomeOK)
}

func TestClassifyCancel(t *testing.T) {
	log := testutil.New(t)
	log.Phase("classify_cancel")

	cases := []struct {
		name        string
		stageExists bool
		inCommit    bool
		want        generate.CancelClass
	}{
		{"before_stage", false, false, generate.CancelBeforeStage},
		{"after_stage", true, false, generate.CancelAfterStage},
		{"racing_commit", true, true, generate.CancelRacingCommit},
		{"racing_commit_no_stage_flag", false, true, generate.CancelRacingCommit},
	}
	for _, tc := range cases {
		got := generate.ClassifyCancel(tc.stageExists, tc.inCommit)
		log.Assert(tc.name, got == tc.want, tc.want, got)
	}

	before := generate.ApplyCancel(generate.CancelBeforeStage, "", generate.StageToolPreflight, nil)
	log.Assert("before_exit", before.Exit == 130, 130, before.Exit)
	log.Assert("before_preserve", !before.StagePreserved, false, before.StagePreserved)
	log.Assert("before_outcome", before.Outcome == generate.OutcomeCancelled, generate.OutcomeCancelled, before.Outcome)

	after := generate.ApplyCancel(generate.CancelAfterStage, ".foundry-x", generate.StageRender, nil)
	log.Assert("after_exit", after.Exit == 130, 130, after.Exit)
	log.Assert("after_preserve", after.StagePreserved, true, after.StagePreserved)
	log.Assert("after_path", after.StagePath == ".foundry-x", ".foundry-x", after.StagePath)

	log.PhaseEnd("classify_cancel", testutil.OutcomeOK)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
