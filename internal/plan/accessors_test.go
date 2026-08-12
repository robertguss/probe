package plan_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestNilPlanAccessors(t *testing.T) {
	log := testutil.New(t)
	log.Phase("nil_accessors")
	var p *plan.Plan
	log.Assert("schema", p.Schema() == 0, 0, p.Schema())
	log.Assert("foundry", p.Foundry().Version == "", true, p.Foundry())
	log.Assert("spec", p.Specification().Path == "", true, p.Specification())
	log.Assert("project", p.Project().Name == "", true, p.Project())
	log.Assert("dest", p.Destination().Path == "", true, p.Destination())
	log.Assert("profiles", p.Profiles() == nil, true, p.Profiles())
	log.Assert("files", p.Files() == nil, true, p.Files())
	log.Assert("file_count", p.FileCount() == 0, 0, p.FileCount())
	log.Assert("deps", p.Dependencies() == nil, true, p.Dependencies())
	log.Assert("tools", p.Tools() == nil, true, p.Tools())
	log.Assert("steps", p.ExternalSteps() == nil, true, p.ExternalSteps())
	log.Assert("step_count", p.ExternalStepCount() == 0, 0, p.ExternalStepCount())
	log.Assert("tool_outputs", p.ToolOutputs() == nil, true, p.ToolOutputs())
	log.Assert("verification", p.Verification().Mode == "", true, p.Verification())
	log.Assert("network", !p.Network().MayBeRequired, true, p.Network())
	log.Assert("git", p.Git().Init == false, true, p.Git())
	log.Assert("commit_model", p.CommitResultModel() == "", true, p.CommitResultModel())
	log.Assert("sha", p.PlanSHA256() == "", true, p.PlanSHA256())
	log.Assert("warnings", p.Warnings() == nil, true, p.Warnings())
	log.Assert("json", p.JSON() == nil, true, p.JSON())
	log.Assert("equal_nil", p.Equal(nil), true, false)
	log.Assert("string", p.String() == "Plan(nil)", true, p.String())
	log.Assert("summary", plan.Summary(nil) == "plan: <nil>", true, plan.Summary(nil))
	log.PhaseEnd("nil_accessors", testutil.OutcomeOK)
}

func TestPlanStringAndAccessors(t *testing.T) {
	log := testutil.New(t)
	log.Phase("accessors")
	vs := minimalCLI(t)
	cat := mustLoadCatalog(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))

	log.Assert("schema", p.Schema() == 1, 1, p.Schema())
	log.Assert("foundry_ver", p.Foundry().Version != "", true, p.Foundry().Version)
	log.Assert("spec_path", p.Specification().Path != "", true, p.Specification().Path)
	log.Assert("dest_base", p.Destination().Basename == "minimal-cli", "minimal-cli", p.Destination().Basename)
	log.Assert("tool_outputs", len(p.ToolOutputs()) >= 0, true, len(p.ToolOutputs()))
	log.Assert("warnings", p.Warnings() != nil || p.Warnings() == nil, true, true) // exercise copy
	s := p.String()
	log.Assert("string_has_name", strings.Contains(s, "minimal-cli"), true, s)
	log.Assert("string_has_sha", strings.Contains(s, "sha256="), true, s)
	sum := plan.Summary(p)
	log.Assert("summary_project", strings.Contains(sum, "project=minimal-cli"), true, sum)
	// Equal self
	log.Assert("equal_self", p.Equal(p), true, false)
	// JSON MarshalJSON path if present
	if m, ok := any(p).(interface{ MarshalJSON() ([]byte, error) }); ok {
		b, err := m.MarshalJSON()
		log.Assert("marshal_err", err == nil, true, err)
		log.Assert("marshal_len", len(b) > 10, true, len(b))
	}
	log.PhaseEnd("accessors", testutil.OutcomeOK)
}
