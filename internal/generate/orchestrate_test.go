package generate_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func mustCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return c
}

func mustPlanCLI(t *testing.T, destPath string, gitInit bool) *plan.Plan {
	t.Helper()
	toml := `
schema = 1
name = "orch-cli"
module = "github.com/example/orch-cli"
description = "orchestrator unit fixture"
archetype = "cli"
destination = "./orch-cli"
profiles = []
`
	if !gitInit {
		toml += "\n[git]\ninit = false\n"
	}
	raw, err := spec.Decode("orch-cli.toml", []byte(toml))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	cat := mustCatalog(t)
	// Destination observation: parent exists, dest absent.
	parent := filepath.Dir(destPath)
	base := filepath.Base(destPath)
	host := plan.HostEnv{
		PATH:       os.Getenv("PATH"),
		HOME:       os.Getenv("HOME"),
		TMPDIR:     os.TempDir(),
		GOMODCACHE: firstEnv("GOMODCACHE", filepath.Join(os.TempDir(), "gomodcache")),
		GOCACHE:    firstEnv("GOCACHE", filepath.Join(os.TempDir(), "gocache")),
		GOPATH:     firstEnv("GOPATH", filepath.Join(os.TempDir(), "gopath")),
		GOPROXY:    firstEnv("GOPROXY", "https://proxy.golang.org,direct"),
		GOSUMDB:    firstEnv("GOSUMDB", "sum.golang.org"),
	}
	goBin := pinnedGo(t)
	gitBin, _ := exec.LookPath("git")
	opts := plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  "testcommit",
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       "orch-cli.toml",
		Destination: plan.DestinationInfo{
			Path:        destPath,
			Parent:      parent,
			Basename:    base,
			Observation: plan.ObservationAbsent,
		},
		Verify:         plan.VerifyDefault,
		GoBinary:       goBin,
		GitBinary:      gitBin,
		GitTemplateDir: "",
		Host:           host,
	}
	p, err := plan.Pipeline(vs, cat, opts)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	return p
}

func firstEnv(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func pinnedGo(t *testing.T) string {
	t.Helper()
	if p := toolrun.FindPinnedGoBinary(toolrun.DefaultPinnedGoTag); p != "" {
		return p
	}
	p, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not on PATH")
	}
	return p
}

func privateParent(t *testing.T) string {
	t.Helper()
	// Prefer sticky /tmp for custody-safe parents on Linux.
	base := "/tmp"
	if runtime.GOOS != "linux" {
		base = os.TempDir()
	}
	dir, err := os.MkdirTemp(base, "foundry-orch-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir
}

func TestJobsFromPlanMinimalCLI(t *testing.T) {
	log := testutil.New(t)
	log.Phase("jobs_from_plan")
	cat := mustCatalog(t)
	parent := privateParent(t)
	dest := filepath.Join(parent, "orch-cli")
	p := mustPlanCLI(t, dest, false)

	jobs, err := generate.JobsFromPlan(p, cat)
	if err != nil {
		log.Fail("jobs", err.Error())
	}
	log.Assert("non_empty", len(jobs) > 5, ">5", len(jobs))

	var staticN, tmplN, gomodN int
	for _, j := range jobs {
		switch j.Mechanism {
		case render.MechanismStatic:
			staticN++
			if j.Static == nil || j.Static.Path == "" {
				t.Fatalf("static job incomplete: %+v", j)
			}
		case render.MechanismTemplate:
			tmplN++
			if j.Template == nil || j.Template.Data.Name != "orch-cli" {
				t.Fatalf("template job data: %+v", j.Template)
			}
			log.Assert("dist_off", !j.Template.Data.DistributionEnabled, false, j.Template.Data.DistributionEnabled)
		case render.MechanismGomod:
			gomodN++
			if j.Gomod == nil || j.Gomod.Module != "github.com/example/orch-cli" {
				t.Fatalf("gomod: %+v", j.Gomod)
			}
		default:
			t.Fatalf("unknown mechanism %v", j.Mechanism)
		}
	}
	log.Assert("has_static", staticN > 0, true, staticN)
	log.Assert("has_tmpl", tmplN > 0, true, tmplN)
	log.Assert("has_gomod", gomodN == 1, 1, gomodN)

	_, err = generate.JobsFromPlan(nil, cat)
	log.Assert("nil_plan_err", err != nil, true, err != nil)
	_, err = generate.JobsFromPlan(p, nil)
	log.Assert("nil_cat_err", err != nil, true, err != nil)

	log.PhaseEnd("jobs_from_plan", testutil.OutcomeOK)
}

func TestJobsFromPlanDistributionFlag(t *testing.T) {
	log := testutil.New(t)
	log.Phase("jobs_distribution")
	cat := mustCatalog(t)
	toml := `
schema = 1
name = "pub-cli"
module = "github.com/example/pub-cli"
description = "public distribution fixture"
archetype = "cli"
destination = "./pub-cli"
visibility = "public"
profiles = ["distribution"]
[git]
init = false
`
	raw, err := spec.Decode("pub.toml", []byte(toml))
	if err != nil {
		t.Fatal(err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatal(err)
	}
	parent := privateParent(t)
	dest := filepath.Join(parent, "pub-cli")
	host := plan.HostEnv{
		PATH: os.Getenv("PATH"), HOME: os.Getenv("HOME"), TMPDIR: os.TempDir(),
		GOMODCACHE: firstEnv("GOMODCACHE", filepath.Join(os.TempDir(), "gomodcache")),
		GOCACHE:    firstEnv("GOCACHE", filepath.Join(os.TempDir(), "gocache")),
		GOPATH:     firstEnv("GOPATH", filepath.Join(os.TempDir(), "gopath")),
		GOPROXY:    "https://proxy.golang.org,direct", GOSUMDB: "sum.golang.org",
	}
	p, err := plan.Pipeline(vs, cat, plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       "pub.toml",
		Destination: plan.DestinationInfo{
			Path: dest, Parent: parent, Basename: "pub-cli", Observation: plan.ObservationAbsent,
		},
		Verify: plan.VerifyDefault, GoBinary: pinnedGo(t), Host: host,
	})
	if err != nil {
		// distribution may require resolve predicates — fail loudly
		t.Fatalf("Pipeline: %v", err)
	}
	jobs, err := generate.JobsFromPlan(p, cat)
	if err != nil {
		log.Fail("jobs", err.Error())
	}
	found := false
	for _, j := range jobs {
		if j.Mechanism == render.MechanismTemplate && j.Template != nil && j.Template.Data.DistributionEnabled {
			found = true
			break
		}
	}
	log.Assert("distribution_enabled", found, true, found)
	// resolve path also lists profiles
	log.Assert("profiles_has_dist", resolveContains(p.Profiles(), "distribution"), true, p.Profiles())
	log.PhaseEnd("jobs_distribution", testutil.OutcomeOK)
}

func resolveContains(ss []string, id string) bool {
	for _, s := range ss {
		if s == id {
			return true
		}
	}
	return false
}

func TestOrchestratorHappyPathNoGit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("orch_happy")
	if testing.Short() {
		t.Skip("orchestrator e2e-ish")
	}
	cat := mustCatalog(t)
	parent := privateParent(t)
	dest := filepath.Join(parent, "orch-cli")
	p := mustPlanCLI(t, dest, false)

	rec := &generate.RecordingLogger{}
	orch := &generate.Orchestrator{
		Plan:     p,
		Catalog:  cat,
		Host:     toolrun.HostCaptureFromPlan(hostFromPlan(p)),
		GoBinary: pinnedGo(t),
		Log:      rec,
	}
	defer func() { _ = orch.Close() }()

	log.Assert("format_before", strings.Contains(generate.FormatOrchestratorLog(orch), "dest=orch-cli"), true, generate.FormatOrchestratorLog(orch))
	log.Assert("format_nil", generate.FormatOrchestratorLog(nil) == "orchestrator=nil", true, generate.FormatOrchestratorLog(nil))
	log.Assert("baseline_empty", orch.BaselineDigest() == "", true, orch.BaselineDigest())

	m := generate.New(generate.Config{
		Stages: orch.Stages(),
		Log:    rec,
	})
	res := m.Run(context.Background())
	log.Assert("exit", res.Exit == diagnostic.ExitSuccess, diagnostic.ExitSuccess, res.Exit)
	log.Assert("outcome", res.Outcome == generate.OutcomeCommitted, generate.OutcomeCommitted, res.Outcome)
	log.Assert("dest_exists", dirExists(dest), true, dest)
	log.Assert("baseline_set", orch.BaselineDigest() != "", true, orch.BaselineDigest())
	log.Assert("format_after", strings.Contains(generate.FormatOrchestratorLog(orch), "verify_done=true"), true, generate.FormatOrchestratorLog(orch))

	// go.mod rendered
	if _, err := os.Stat(filepath.Join(dest, "go.mod")); err != nil {
		log.Fail("go.mod", err.Error())
	}
	log.PhaseEnd("orch_happy", testutil.OutcomeOK)
}

func hostFromPlan(p *plan.Plan) plan.HostEnv {
	// Reconstruct ambient-like host from process env for toolrun.
	return plan.HostEnv{
		PATH:       os.Getenv("PATH"),
		HOME:       os.Getenv("HOME"),
		TMPDIR:     os.TempDir(),
		GOMODCACHE: firstEnv("GOMODCACHE", filepath.Join(os.TempDir(), "gomodcache")),
		GOCACHE:    firstEnv("GOCACHE", filepath.Join(os.TempDir(), "gocache")),
		GOPATH:     firstEnv("GOPATH", filepath.Join(os.TempDir(), "gopath")),
		GOPROXY:    firstEnv("GOPROXY", "https://proxy.golang.org,direct"),
		GOSUMDB:    firstEnv("GOSUMDB", "sum.golang.org"),
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func TestOrchestratorNilPlanPreflight(t *testing.T) {
	log := testutil.New(t)
	log.Phase("nil_plan")
	orch := &generate.Orchestrator{Catalog: mustCatalog(t), GoBinary: pinnedGo(t)}
	stages := orch.Stages()
	err := stages[generate.StageToolPreflight](context.Background(), &generate.Runtime{})
	log.Assert("err", err != nil, true, err != nil)
	se, ok := generate.AsStageError(err)
	log.Assert("stage_err", ok, true, ok)
	if ok {
		log.Assert("exit", se.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, se.Exit)
	}
	log.Assert("close_nil_ok", orch.Close() == nil, true, false)
	log.Assert("baseline_nil_o", (*generate.Orchestrator)(nil).BaselineDigest() == "", true, false)
	log.Assert("stages_nil_o", len((*generate.Orchestrator)(nil).Stages()) == 0, true, false)
	log.PhaseEnd("nil_plan", testutil.OutcomeOK)
}

func TestOrchestratorCreateStageWithoutParent(t *testing.T) {
	log := testutil.New(t)
	log.Phase("create_no_parent")
	parent := privateParent(t)
	dest := filepath.Join(parent, "orch-cli")
	p := mustPlanCLI(t, dest, false)
	orch := &generate.Orchestrator{Plan: p, Catalog: mustCatalog(t), GoBinary: pinnedGo(t)}
	rt := &generate.Runtime{}
	err := orch.Stages()[generate.StageCreateStage](context.Background(), rt)
	log.Assert("err", err != nil, true, err != nil)
	se, ok := generate.AsStageError(err)
	log.Assert("stage_err", ok && se.Detail == "create-stage without parent", true, se)
	log.PhaseEnd("create_no_parent", testutil.OutcomeOK)
}

func TestOrchestratorRenderPreconditions(t *testing.T) {
	log := testutil.New(t)
	log.Phase("render_pre")
	orch := &generate.Orchestrator{}
	err := orch.Stages()[generate.StageRender](context.Background(), &generate.Runtime{StageExists: true})
	log.Assert("err", err != nil, true, err != nil)
	log.PhaseEnd("render_pre", testutil.OutcomeOK)
}

func TestOrchestratorCommitWithoutStage(t *testing.T) {
	log := testutil.New(t)
	log.Phase("commit_no_stage")
	parent := privateParent(t)
	dest := filepath.Join(parent, "orch-cli")
	p := mustPlanCLI(t, dest, false)
	orch := &generate.Orchestrator{Plan: p}
	err := orch.Stages()[generate.StageCommit](context.Background(), &generate.Runtime{})
	log.Assert("err", err != nil, true, err != nil)
	log.PhaseEnd("commit_no_stage", testutil.OutcomeOK)
}

func TestOrchestratorParentReobserveMissing(t *testing.T) {
	log := testutil.New(t)
	log.Phase("reobserve_missing")
	orch := &generate.Orchestrator{}
	err := orch.Stages()[generate.StageParentReobserve](context.Background(), &generate.Runtime{StageExists: true})
	log.Assert("err", err != nil, true, err != nil)
	log.PhaseEnd("reobserve_missing", testutil.OutcomeOK)
}

func TestOrchestratorNetworkDisclosure(t *testing.T) {
	log := testutil.New(t)
	log.Phase("network")
	parent := privateParent(t)
	dest := filepath.Join(parent, "orch-cli")
	p := mustPlanCLI(t, dest, false)
	orch := &generate.Orchestrator{Plan: p}
	rt := &generate.Runtime{}
	if err := orch.Stages()[generate.StageReportPlanNetwork](context.Background(), rt); err != nil {
		log.Fail("network", err.Error())
	}
	log.Assert("lines", len(rt.NetworkLines) > 0, true, rt.NetworkLines)
	// DiscloseFromPlanStage helper
	fn := generate.DiscloseFromPlanStage(p)
	rt2 := &generate.Runtime{}
	if err := fn(context.Background(), rt2); err != nil {
		log.Fail("disclose", err.Error())
	}
	log.Assert("disclose_lines", len(rt2.NetworkLines) > 0, true, len(rt2.NetworkLines))
	log.PhaseEnd("network", testutil.OutcomeOK)
}

func TestStageHelpersCoverage(t *testing.T) {
	log := testutil.New(t)
	log.Phase("stage_helpers")
	log.Assert("valid", generate.ValidStageID(generate.StageCommit), true, false)
	log.Assert("invalid", !generate.ValidStageID(generate.StageID("nope")), true, false)
	log.Assert("creates_disk", generate.StageCreatesDisk(generate.StageCreateStage), true, false)
	log.Assert("no_disk", !generate.StageCreatesDisk(generate.StageReadParseSpec), true, false)
	log.Assert("fail_preserves", generate.FailurePreservesStage(generate.StageRender), true, false)
	log.Assert("fail_no_preserve", !generate.FailurePreservesStage(generate.StageReadParseSpec), true, false)
	// OutcomeFromLifecycle edges
	for _, tc := range []struct {
		life generate.LifecycleState
		want generate.CommitOutcome
	}{
		{generate.LifeCommitted, generate.OutcomeCommitted},
		{generate.LifeConflicted, generate.OutcomeConflicted},
		{generate.LifeFailedPreserved, generate.OutcomeFailedPreserved},
		{generate.LifeAmbiguous, generate.OutcomeAmbiguous},
		{generate.LifeCancelled, generate.OutcomeCancelled},
		{generate.LifePlanned, generate.OutcomeNotStarted},
	} {
		tc := tc
		t.Run("outcome_"+string(tc.life), func(t *testing.T) {
			got := generate.OutcomeFromLifecycle(tc.life)
			log.Assert("outcome_"+string(tc.life), got == tc.want, tc.want, got)
		})
	}
	log.PhaseEnd("stage_helpers", testutil.OutcomeOK)
}

// silence unused import if resolve is only used via profiles helper
var _ = resolve.Resolve

func TestOrchestratorHappyPathWithGit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("orch_git")
	if testing.Short() {
		t.Skip("orchestrator e2e-ish")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	cat := mustCatalog(t)
	parent := privateParent(t)
	dest := filepath.Join(parent, "orch-cli")
	p := mustPlanCLI(t, dest, true)
	rec := &generate.RecordingLogger{}
	orch := &generate.Orchestrator{
		Plan:      p,
		Catalog:   cat,
		Host:      toolrun.HostCaptureFromPlan(hostFromPlan(p)),
		GoBinary:  pinnedGo(t),
		GitBinary: func() string { g, _ := exec.LookPath("git"); return g }(),
		Log:       rec,
	}
	defer func() { _ = orch.Close() }()
	m := generate.New(generate.Config{Stages: orch.Stages(), Log: rec})
	res := m.Run(context.Background())
	log.Assert("exit", res.Exit == diagnostic.ExitSuccess, diagnostic.ExitSuccess, res.Exit)
	log.Assert("outcome", res.Outcome == generate.OutcomeCommitted, generate.OutcomeCommitted, res.Outcome)
	// .git must exist after commit
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		log.Fail("git_dir", err.Error())
	}
	log.PhaseEnd("orch_git", testutil.OutcomeOK)
}

func TestResidualMachineAndEventHelpers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("residual")
	// LifecycleFromCommitOutcome all arms
	for _, o := range []generate.CommitOutcome{
		generate.OutcomeCommitted, generate.OutcomeConflicted, generate.OutcomeFailedPreserved,
		generate.OutcomeAmbiguous, generate.OutcomeCancelled, generate.OutcomeNotStarted, generate.CommitOutcome("x"),
	} {
		o := o
		t.Run("lifecycle_"+string(o), func(t *testing.T) {
			got := generate.LifecycleFromCommitOutcome(o)
			log.Assert("lifecycle_"+string(o), got != "", true, got)
		})
	}
	// StageError Error/Unwrap
	se := &generate.StageError{Detail: "d", Err: context.Canceled, Exit: 1}
	_ = se.Error()
	_ = se.Unwrap()
	se2 := &generate.StageError{}
	_ = se2.Error()
	// Nop sink OnEvent
	var nop generate.NopSink
	nop.OnEvent(generate.GenerationEvent{Kind: generate.EventProgress})
	// Collecting sink
	cs := &generate.CollectingSink{}
	cs.OnEvent(generate.GenerationEvent{Kind: generate.EventTerminal})
	// Machine State/Runtime/Events/FormatMachineDebug
	m := generate.New(generate.Config{Stages: defaultStages()})
	_ = m.State()
	_ = m.Runtime()
	_ = m.Events()
	_ = generate.FormatMachineDebug(m)
	_ = generate.FormatMachineDebug(nil)
	// Double-run should refuse: machine already finished.
	res := m.Run(context.Background())
	log.Assert("first_ok", res.Exit == 0, 0, res.Exit)
	res2 := m.Run(context.Background())
	log.Assert("second_exit", res2.Exit != 0, true, res2.Exit)
	secondErr := ""
	if res2.Err != nil {
		secondErr = res2.Err.Error()
	}
	log.Assert("second_err", res2.Err != nil && strings.Contains(secondErr, "already finished"), true, secondErr)
	// StepIDsMatchPlan / CollectPlanStepIDs
	log.Assert("match_nil", generate.StepIDsMatchPlan(nil, nil), true, false)
	log.Assert("match_diff", !generate.StepIDsMatchPlan([]string{"a"}, []string{"b"}), true, false)
	log.Assert("collect_nil", generate.CollectPlanStepIDs(nil) == nil, true, false)
	// network helpers
	d := generate.BuildDisclosureFromMode(plan.VerifyDefault)
	_ = d.TextLines()
	_ = d.StepIDs()
	_ = generate.FormatNetworkLogResult(d)
	generate.ApplyDisclosure(nil, d)
	log.PhaseEnd("residual", testutil.OutcomeOK)
}
