package generate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// TestProcessTree_PlannedIDsMatchSection292 proves generate's tool-stage map
// equals the plan-space external_steps id order for default/strict ± git
// (REQ-120 / REQ-185 / umbrella process-tree=plan).
func TestProcessTree_PlannedIDsMatchSection292(t *testing.T) {
	log := testutil.New(t)
	log.Phase("plan_ids")

	cases := []struct {
		name    string
		mode    plan.VerifyMode
		gitInit bool
		want    []string
	}{
		{
			name: "default_no_git",
			mode: plan.VerifyDefault,
			want: []string{
				generate.PlanStepGoModTidy,
				generate.PlanStepGoModVerify,
				generate.PlanStepGoTest,
				generate.PlanStepGoVet,
			},
		},
		{
			name:    "default_with_git",
			mode:    plan.VerifyDefault,
			gitInit: true,
			want: []string{
				generate.PlanStepGoModTidy,
				generate.PlanStepGoModVerify,
				generate.PlanStepGoTest,
				generate.PlanStepGoVet,
				generate.PlanStepGitInit,
			},
		},
		{
			name: "strict_no_git",
			mode: plan.VerifyStrict,
			want: []string{
				generate.PlanStepGoModTidy,
				generate.PlanStepGoModVerify,
				generate.PlanStepGoTest,
				generate.PlanStepGoVet,
				generate.PlanStepGoStaticcheck,
				generate.PlanStepGoGovulncheck,
			},
		},
		{
			name:    "strict_with_git",
			mode:    plan.VerifyStrict,
			gitInit: true,
			want: []string{
				generate.PlanStepGoModTidy,
				generate.PlanStepGoModVerify,
				generate.PlanStepGoTest,
				generate.PlanStepGoVet,
				generate.PlanStepGoStaticcheck,
				generate.PlanStepGoGovulncheck,
				generate.PlanStepGitInit,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := generate.PlannedToolStepIDs(tc.mode, tc.gitInit)
			log.Assert(tc.name+"_match", generate.StepIDsMatchPlan(tc.want, got),
				strings.Join(tc.want, ","), strings.Join(got, ","))
			// No race detector step in generation gate (FND-004).
			for _, id := range got {
				if strings.Contains(id, "race") {
					log.Fail("race_in_plan", id)
				}
			}
		})
	}
	log.PhaseEnd("plan_ids", testutil.OutcomeOK)
}

// TestProcessTree_EqualsPlan_FakeRunner proves a generate-orchestrator-shaped
// FakePlanRunner observation equals plan external_steps (no extras, no shell).
func TestProcessTree_EqualsPlan_FakeRunner(t *testing.T) {
	log := testutil.New(t)
	log.Phase("process_tree")

	host := toolrun.HostCapture{
		PATH: "/usr/bin:/bin", HOME: "/home/foundry",
		GOMODCACHE: "/tmp/gomodcache", GOCACHE: "/tmp/gocache",
		GOPATH: "/tmp/gopath", GOPROXY: "https://proxy.golang.org,direct", GOSUMDB: "sum.golang.org",
	}
	goEnv := toolrun.ConstructGoEnv(host)
	goBin := "/usr/local/go/bin/go"

	// Build plan-shaped steps matching PlannedToolStepIDs(default, true).
	planned := []plan.ExternalStep{
		{
			ID: generate.PlanStepGoModTidy, Binary: goBin,
			Argv: []string{"go", "mod", "tidy"},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkMay,
			TimeoutS: 600, OutputCapBytes: plan.OutputCapBytes, Env: copyEnv(goEnv),
		},
		{
			ID: generate.PlanStepGoModVerify, Binary: goBin,
			Argv: []string{"go", "mod", "verify"},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 120, OutputCapBytes: plan.OutputCapBytes, Env: copyEnv(goEnv),
		},
		{
			ID: generate.PlanStepGoTest, Binary: goBin,
			Argv: []string{"go", "test", "-count=1", "-buildvcs=false", "-mod=readonly", "./..."},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 300, OutputCapBytes: plan.OutputCapBytes, Env: copyEnv(goEnv),
		},
		{
			ID: generate.PlanStepGoVet, Binary: goBin,
			Argv: []string{"go", "vet", "-buildvcs=false", "-mod=readonly", "./..."},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 300, OutputCapBytes: plan.OutputCapBytes, Env: copyEnv(goEnv),
		},
		{
			ID: generate.PlanStepGitInit, Binary: "/usr/bin/git",
			Argv: []string{"git", "init", "--initial-branch=main", "--template=foundry-owned-git-template-scratch", "."},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 60, OutputCapBytes: plan.OutputCapBytes,
			Env: toolrun.ConstructGitEnv(host.PATH, "foundry-owned-git-template-scratch"),
		},
	}

	// generate stage map → plan ids must cover every planned step.
	stageIDs := generate.PlannedToolStepIDs(plan.VerifyDefault, true)
	planIDs := generate.CollectPlanStepIDs(planned)
	log.Assert("stage_map_equals_plan", generate.StepIDsMatchPlan(stageIDs, planIDs),
		strings.Join(stageIDs, ","), strings.Join(planIDs, ","))

	fake := &toolrun.FakePlanRunner{}
	obs := fake.RunPlan(planned)
	diffs := toolrun.CompareProcessTree(planned, obs)
	log.Assert("tree_equal", len(diffs) == 0, toolrun.FormatTreeDiffs(nil), toolrun.FormatTreeDiffs(diffs))
	log.Step("plan_argv", testutil.OutcomeOK, toolrun.PlannedArgvSummary(planned))

	// Negative: unplanned shell must fail the tree audit.
	fake2 := &toolrun.FakePlanRunner{InjectExtra: []toolrun.ObservedStep{{
		ID: "evil-shell", Binary: "/bin/sh", Argv: []string{"sh", "-c", "true"}, Shell: true,
		Env: map[string]string{},
	}}}
	obs2 := fake2.RunPlan(planned)
	diffs2 := toolrun.CompareProcessTree(planned, obs2)
	if len(diffs2) == 0 {
		log.Fail("expected_diffs", "injected shell not detected")
	}
	var sawExtra, sawShell bool
	for _, d := range diffs2 {
		if d.Kind == "extra" || d.Kind == "count" {
			sawExtra = true
		}
		if d.Kind == "shell" {
			sawShell = true
		}
	}
	log.Assert("detect_extra", sawExtra, true, sawExtra)
	log.Assert("detect_shell", sawShell, true, sawShell)
	log.PhaseEnd("process_tree", testutil.OutcomeOK)
}

// TestProcessTree_NetworkMaySubsetOfPlan ensures disclosure network:may ids
// are exactly the network:may subset of PlannedToolStepIDs (FND-007).
func TestProcessTree_NetworkMaySubsetOfPlan(t *testing.T) {
	log := testutil.New(t)
	log.Phase("network_subset")

	for _, mode := range []plan.VerifyMode{plan.VerifyDefault, plan.VerifyStrict} {
		planned := generate.PlannedToolStepIDs(mode, true)
		may := generate.NetworkMayStepIDs(mode)
		disc := generate.BuildDisclosureFromMode(mode)

		// Disclosure step ids must equal NetworkMayStepIDs.
		log.Assert(string(mode)+"_disclosure_ids",
			generate.StepIDsMatchPlan(may, disc.StepIDs()),
			strings.Join(may, ","), strings.Join(disc.StepIDs(), ","))

		// Every network:may id must appear in the full plan tool list.
		set := map[string]bool{}
		for _, id := range planned {
			set[id] = true
		}
		for _, id := range may {
			if !set[id] {
				log.Fail("may_not_in_plan", id+" mode="+string(mode))
			}
		}
		log.Step(string(mode)+"_subset", testutil.OutcomeOK,
			"may="+strings.Join(may, ",")+" plan_n="+itoa(len(planned)))
	}
	log.PhaseEnd("network_subset", testutil.OutcomeOK)
}

// TestProcessTree_MachineRecordsOnlyPlannedSteps runs the lifecycle with
// injectable stages that record tool step ids; observed sequence must equal
// PlannedToolStepIDs (generate never invents extras — REQ-120).
func TestProcessTree_MachineRecordsOnlyPlannedSteps(t *testing.T) {
	log := testutil.New(t)
	log.Phase("machine_tree")

	mode := plan.VerifyStrict
	gitInit := true
	want := generate.PlannedToolStepIDs(mode, gitInit)

	var observed []string
	stages := defaultStages()
	// Disclose via pure mode helper (ordering before stage).
	stages[generate.StageReportPlanNetwork] = generate.DiscloseFromModeStage(mode)

	// Tool stages append plan step ids in plan order.
	stages[generate.StageGoModTidy] = func(ctx context.Context, rt *generate.Runtime) error {
		_ = ctx
		_ = rt
		observed = append(observed, generate.PlanStepIDsForStage(generate.StageGoModTidy, mode, gitInit)...)
		return nil
	}
	stages[generate.StageVerifyTools] = func(ctx context.Context, rt *generate.Runtime) error {
		_ = ctx
		_ = rt
		observed = append(observed, generate.PlanStepIDsForStage(generate.StageVerifyTools, mode, gitInit)...)
		return nil
	}
	stages[generate.StageGitInit] = func(ctx context.Context, rt *generate.Runtime) error {
		_ = ctx
		_ = rt
		observed = append(observed, generate.PlanStepIDsForStage(generate.StageGitInit, mode, gitInit)...)
		return nil
	}

	m := newMachine(t, stages, nil, nil)
	res := m.Run(context.Background())
	log.Assert("committed", res.Outcome == generate.OutcomeCommitted, generate.OutcomeCommitted, res.Outcome)
	log.Assert("tree_ids", generate.StepIDsMatchPlan(want, observed),
		strings.Join(want, ","), strings.Join(observed, ","))

	// Also audit via toolrun with synthetic plan steps (ids only + empty env ok for match on ids).
	planned := make([]plan.ExternalStep, len(want))
	for i, id := range want {
		planned[i] = plan.ExternalStep{
			ID: id, Binary: "go", Argv: []string{"go", id},
			Cwd: plan.StageDescriptorCWD, Env: map[string]string{"PATH": "/bin"},
		}
		if id == generate.PlanStepGitInit {
			planned[i].Binary = "git"
			planned[i].Argv = []string{"git", "init"}
		}
	}
	// Observed from FakePlanRunner must equal plan when orchestrator uses plan only.
	fake := &toolrun.FakePlanRunner{}
	obs := fake.RunPlan(planned)
	diffs := toolrun.CompareProcessTree(planned, obs)
	log.Assert("fake_tree", len(diffs) == 0, toolrun.FormatTreeDiffs(nil), toolrun.FormatTreeDiffs(diffs))

	log.PhaseEnd("machine_tree", testutil.OutcomeOK)
}

// TestToolStagesOnlyThree asserts exactly the three subprocess lifecycle stages.
func TestToolStagesOnlyThree(t *testing.T) {
	log := testutil.New(t)
	tools := generate.ToolStages()
	log.Assert("count", len(tools) == 3, 3, len(tools))
	for _, id := range generate.AllStageIDs() {
		want := id == generate.StageGoModTidy || id == generate.StageVerifyTools || id == generate.StageGitInit
		log.Assert("is_tool_"+string(id), generate.IsToolStage(id) == want, want, generate.IsToolStage(id))
	}
}

func copyEnv(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
