package generate

import (
	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// Tool-invoking plan external_step ids (Section 28.2 / 29.2 / 35.2).
// Spellings match plan.buildExternalSteps — generate never invents extras
// (REQ-034, REQ-120, REQ-185 process-tree = plan).
const (
	PlanStepGoModTidy     = "go-mod-tidy"
	PlanStepGoModVerify   = "go-mod-verify"
	PlanStepGoTest        = "go-test"
	PlanStepGoVet         = "go-vet"
	PlanStepGoStaticcheck = "go-staticcheck"
	PlanStepGoGovulncheck = "go-govulncheck"
	PlanStepGitInit       = "git-init"
)

// ToolStages returns Section 29.2 stages that may start subprocesses via
// toolrun when the corresponding plan external_steps are present.
// Order matches lifecycle order (not plan list order within verify-tools).
func ToolStages() []StageID {
	return []StageID{
		StageGoModTidy,
		StageVerifyTools,
		StageGitInit,
	}
}

// IsToolStage reports whether id may invoke toolrun for plan external_steps.
func IsToolStage(id StageID) bool {
	switch id {
	case StageGoModTidy, StageVerifyTools, StageGitInit:
		return true
	default:
		return false
	}
}

// PlanStepIDsForStage returns the plan external_steps ids that stage id may
// execute under the given verify mode (and gitInit for stage 15).
// Unknown or non-tool stages return nil.
//
// The machine itself does not start processes; orchestrators feed these ids
// into toolrun and audit with toolrun.CompareProcessTree against the plan.
func PlanStepIDsForStage(id StageID, mode plan.VerifyMode, gitInit bool) []string {
	mode = mode.Normalize()
	switch id {
	case StageGoModTidy:
		return []string{PlanStepGoModTidy}
	case StageVerifyTools:
		ids := []string{
			PlanStepGoModVerify,
			PlanStepGoTest,
			PlanStepGoVet,
		}
		if mode == plan.VerifyStrict {
			ids = append(ids, PlanStepGoStaticcheck, PlanStepGoGovulncheck)
		}
		return ids
	case StageGitInit:
		if !gitInit {
			return nil
		}
		return []string{PlanStepGitInit}
	default:
		return nil
	}
}

// PlannedToolStepIDs returns the complete ordered list of plan external_step
// ids that generate may execute for mode + gitInit. Order matches
// plan.buildExternalSteps (tidy → verify suite → optional git-init).
//
// Process-tree audit: observed toolrun starts must equal this id sequence
// when the plan's external_steps are the sole authority (REQ-120 / REQ-214).
func PlannedToolStepIDs(mode plan.VerifyMode, gitInit bool) []string {
	mode = mode.Normalize()
	var out []string
	out = append(out, PlanStepIDsForStage(StageGoModTidy, mode, gitInit)...)
	out = append(out, PlanStepIDsForStage(StageVerifyTools, mode, gitInit)...)
	out = append(out, PlanStepIDsForStage(StageGitInit, mode, gitInit)...)
	return out
}

// NetworkMayStepIDs returns plan external_step ids that disclose network:may
// for the verify mode (FND-007 / Section 13.5). Subset of PlannedToolStepIDs.
func NetworkMayStepIDs(mode plan.VerifyMode) []string {
	mode = mode.Normalize()
	ids := []string{PlanStepGoModTidy}
	if mode == plan.VerifyStrict {
		ids = append(ids, PlanStepGoGovulncheck)
	}
	return ids
}

// StepIDsMatchPlan reports whether observed step ids equal planned ids in order
// (length and position). Used by package process-tree tests; production
// orchestrators prefer toolrun.CompareProcessTree for full argv/env audit.
func StepIDsMatchPlan(planned, observed []string) bool {
	if len(planned) != len(observed) {
		return false
	}
	for i := range planned {
		if planned[i] != observed[i] {
			return false
		}
	}
	return true
}

// CollectPlanStepIDs extracts ordered ids from plan external_steps.
func CollectPlanStepIDs(steps []plan.ExternalStep) []string {
	if len(steps) == 0 {
		return nil
	}
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.ID
	}
	return out
}
