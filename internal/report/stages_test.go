package report_test

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// Section 29.2 has exactly 19 ordered stages.
const wantStageCount = 19

// TestStageIDsExhaustSection292 locks the progress name table against the
// normative ordered stage list (bead 41p acceptance).
func TestStageIDsExhaustSection292(t *testing.T) {
	log := testutil.New(t)
	log.Phase("stage_table")

	ids := report.AllStageIDs()
	log.Assert("count", len(ids) == wantStageCount, wantStageCount, len(ids))

	// Exact order and spellings (diffable logs).
	want := []report.StageID{
		report.StageReadParseSpec,
		report.StageValidateSpec,
		report.StageLoadCatalog,
		report.StageResolve,
		report.StageBuildPlan,
		report.StageToolPreflight,
		report.StageReportPlanNetwork,
		report.StageAcquireParent,
		report.StageCreateStage,
		report.StageRender,
		report.StageGoModTidy,
		report.StageFreezeTree,
		report.StageVerifyTools,
		report.StageFinalConformance,
		report.StageGitInit,
		report.StageGitTemplateCleanup,
		report.StageParentReobserve,
		report.StageCommit,
		report.StageReport,
	}
	for i, w := range want {
		log.Assert("stage_"+string(w), ids[i] == w, w, ids[i])
		log.Assert("valid_"+string(w), report.ValidStageID(w), true, false)
		line := report.ProgressLine(w)
		log.Assert("progress_prefix_"+string(w),
			len(line) > 10 && line[:10] == "progress: ",
			"progress: ", line)
	}

	// Uniqueness
	seen := map[report.StageID]bool{}
	for _, id := range ids {
		if seen[id] {
			log.Fail("duplicate", string(id))
		}
		seen[id] = true
	}

	log.Assert("invalid_false", !report.ValidStageID("not-a-stage"), false, true)
	log.PhaseEnd("stage_table", testutil.OutcomeOK)
}

func TestLifecycleStatesSection291(t *testing.T) {
	log := testutil.New(t)
	log.Phase("lifecycle")
	states := report.LifecycleStates()
	log.Assert("count", len(states) == 14, 14, len(states))
	wantFirst := report.LifePlanned
	wantLast := report.LifeAmbiguous
	log.Assert("first", states[0] == wantFirst, wantFirst, states[0])
	log.Assert("last", states[len(states)-1] == wantLast, wantLast, states[len(states)-1])
	log.PhaseEnd("lifecycle", testutil.OutcomeOK)
}
