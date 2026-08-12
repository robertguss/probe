package generate

// StageID is a stable Section 29.2 progress name. Spellings match
// report.StageID so progress lines and goldens stay host/diff stable.
type StageID string

// DefaultStagePath is the fallback stage directory name when a transaction
// does not report an explicit StagePath (Section 31.6).
const DefaultStagePath = ".foundry-stage"

// Section 29.2 ordered stage identifiers (1–19).
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

// AllStageIDs returns the complete Section 29.2 ordered stage list
// (indices 0..18 map to stages 1..19).
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

// StageNumber returns the 1-based Section 29.2 stage number, or 0 if unknown.
func StageNumber(id StageID) int {
	for i, s := range AllStageIDs() {
		if s == id {
			return i + 1
		}
	}
	return 0
}

// ValidStageID reports whether id is a known Section 29.2 stage.
func ValidStageID(id StageID) bool {
	return StageNumber(id) != 0
}

// StageCreatesDisk reports whether successful completion of this stage means
// a Foundry stage directory exists on disk (stage 9+). Stages 1–8 leave
// nothing on disk (REQ-036 pre-stage cancel window).
func StageCreatesDisk(id StageID) bool {
	n := StageNumber(id)
	return n >= 9 && n <= 18
}

// StageIsPreStage reports stages 1–8 (cancel → 130, nothing on disk).
func StageIsPreStage(id StageID) bool {
	n := StageNumber(id)
	return n >= 1 && n <= 8
}

// StageIsPostStagePreCommit reports stages 9–17 (cancel → preserve + 130).
func StageIsPostStagePreCommit(id StageID) bool {
	n := StageNumber(id)
	return n >= 9 && n <= 17
}

// StageIsCommit is stage 18 (cancel concurrent with commit → commit dominates).
func StageIsCommit(id StageID) bool {
	return id == StageCommit
}

// StageIsReport is stage 19 (post-commit best-effort; never flips exit 0).
func StageIsReport(id StageID) bool {
	return id == StageReport
}

// FailurePreservesStage reports whether a failure at this stage leaves a
// created stage preserved (Section 29.2 failure column). Stages 1–8 never
// leave a stage; 9–18 preserve when a stage exists.
func FailurePreservesStage(id StageID) bool {
	n := StageNumber(id)
	return n >= 9 && n <= 18
}

// DefaultFailureExit returns the Section 29.2 default exit code class for a
// stage failure when the error does not carry a more specific exit.
// Stages 1–2 and 4/8 commonly use exit 2 (usage/selection/path); others
// default to exit 1. Callers may override via Result from stage funcs.
func DefaultFailureExit(id StageID) int {
	switch id {
	case StageReadParseSpec, StageValidateSpec, StageResolve, StageAcquireParent:
		return 2
	default:
		return 1
	}
}
