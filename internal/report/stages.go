package report

// StageID is a stable progress name for generate reporting (Section 36.2).
// Names are fixed strings so logs are diffable across runs and hosts.
//
// The inventory exhausts Section 29.2 ordered stages (1–19). Lifecycle
// state names from Section 29.1 are also listed (LifecycleStates) for
// state-machine event encoding; generate emits StageIDs for progress lines.
type StageID string

// Section 29.2 ordered stage identifiers (stable progress names).
const (
	StageReadParseSpec      StageID = "read-parse-spec"      // 1
	StageValidateSpec       StageID = "validate-spec"        // 2
	StageLoadCatalog        StageID = "load-catalog"         // 3
	StageResolve            StageID = "resolve"              // 4
	StageBuildPlan          StageID = "build-plan"           // 5
	StageToolPreflight      StageID = "tool-preflight"       // 6
	StageReportPlanNetwork  StageID = "report-plan-network"  // 7
	StageAcquireParent      StageID = "acquire-parent"       // 8
	StageCreateStage        StageID = "create-stage"         // 9
	StageRender             StageID = "render"               // 10
	StageGoModTidy          StageID = "go-mod-tidy"          // 11
	StageFreezeTree         StageID = "freeze-tree"          // 12
	StageVerifyTools        StageID = "verify-tools"         // 13
	StageFinalConformance   StageID = "final-conformance"    // 14
	StageGitInit            StageID = "git-init"             // 15
	StageGitTemplateCleanup StageID = "git-template-cleanup" // 16
	StageParentReobserve    StageID = "parent-reobserve"     // 17
	StageCommit             StageID = "commit"               // 18
	StageReport             StageID = "report"               // 19
)

// AllStageIDs is the complete Section 29.2 ordered stage list (indices 0..18
// map to stages 1..19). Tests assert this exhausts the table.
func AllStageIDs() []StageID {
	return []StageID{
		StageReadParseSpec,
		StageValidateSpec,
		StageLoadCatalog,
		StageResolve,
		StageBuildPlan,
		StageToolPreflight,
		StageReportPlanNetwork,
		StageAcquireParent,
		StageCreateStage,
		StageRender,
		StageGoModTidy,
		StageFreezeTree,
		StageVerifyTools,
		StageFinalConformance,
		StageGitInit,
		StageGitTemplateCleanup,
		StageParentReobserve,
		StageCommit,
		StageReport,
	}
}

// LifecycleState is a Section 29.1 state-machine state name. Progress lines
// prefer StageID (29.2); lifecycle names appear in GenerationEvent encoding.
type LifecycleState string

// Section 29.1 state names (stable).
const (
	LifePlanned         LifecycleState = "planned"
	LifeParentAcquired  LifecycleState = "parent-acquired"
	LifeStageCreated    LifecycleState = "stage-created"
	LifeRendered        LifecycleState = "rendered"
	LifeNormalized      LifecycleState = "normalized"
	LifeFrozen          LifecycleState = "frozen"
	LifeVerified        LifecycleState = "verified"
	LifeConformed       LifecycleState = "conformed"
	LifeGitInitialized  LifecycleState = "git-initialized"
	LifeCommitting      LifecycleState = "committing"
	LifeCommitted       LifecycleState = "committed"
	LifeConflicted      LifecycleState = "conflicted"
	LifeFailedPreserved LifecycleState = "failed-preserved"
	LifeAmbiguous       LifecycleState = "ambiguous"
)

// LifecycleStates returns every Section 29.1 state name in table order.
func LifecycleStates() []LifecycleState {
	return []LifecycleState{
		LifePlanned,
		LifeParentAcquired,
		LifeStageCreated,
		LifeRendered,
		LifeNormalized,
		LifeFrozen,
		LifeVerified,
		LifeConformed,
		LifeGitInitialized,
		LifeCommitting,
		LifeCommitted,
		LifeConflicted,
		LifeFailedPreserved,
		LifeAmbiguous,
	}
}

// ValidStageID reports whether id is a known Section 29.2 stage.
func ValidStageID(id StageID) bool {
	for _, s := range AllStageIDs() {
		if s == id {
			return true
		}
	}
	return false
}

// ProgressLine formats a stable text progress line for stage (Section 36.2).
// Format is fixed: "progress: <stage-id>" so agents and goldens can match.
func ProgressLine(id StageID) string {
	return "progress: " + string(id)
}
