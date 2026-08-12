package generate

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/gitinit"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
	"github.com/robertguss/go-foundry-cli/internal/verify"
)

// Orchestrator builds production StageFuncs that drive fsx/toolrun/render/
// verify/gitinit for one generate run (Section 29 / REQ-034).
//
// Stages 1–5 are no-ops when Plan is already constructed through the shared
// pure pipeline (plan/generate equality). Progress events still fire for those
// stages so human progress names remain complete.
//
// Not safe for concurrent use. Call Close after Machine.Run returns.
type Orchestrator struct {
	Plan    *plan.Plan
	Catalog *catalog.Catalog
	Host    toolrun.HostCapture
	// GoBinary / GitBinary override LookPath when non-empty (tests / preflight).
	GoBinary  string
	GitBinary string
	// TempRoot for git template scratch (optional).
	TempRoot string
	Log      StepLogger

	// Filled during Run.
	parent   *fsx.ParentHandle
	stage    *fsx.Stage
	starter  *toolrun.BoundStarter
	origCWD  *toolrun.OriginalCWD
	executor *toolrun.Executor
	goEnv    map[string]string
	goPath   string
	gitPath  string
	// baselineDigest is the post-verify aggregate for GenerateResult.
	baselineDigest string
	// verifyDone is true after stages 11–14 complete via verify.Run.
	verifyDone bool
	closed     bool
}

// Stages returns the full Section 29.2 stage map for Machine.Config.Stages.
func (o *Orchestrator) Stages() map[StageID]StageFunc {
	if o == nil {
		return map[StageID]StageFunc{}
	}
	return map[StageID]StageFunc{
		StageReadParseSpec:      o.nopStage,
		StageValidateSpec:       o.nopStage,
		StageLoadCatalog:        o.nopStage,
		StageResolve:            o.nopStage,
		StageBuildPlan:          o.nopStage,
		StageToolPreflight:      o.stageToolPreflight,
		StageReportPlanNetwork:  o.stageNetwork,
		StageAcquireParent:      o.stageAcquireParent,
		StageCreateStage:        o.stageCreateStage,
		StageRender:             o.stageRender,
		StageGoModTidy:          o.stageVerifyPipeline,
		StageFreezeTree:         o.stageVerifyAlreadyDone,
		StageVerifyTools:        o.stageVerifyAlreadyDone,
		StageFinalConformance:   o.stageVerifyAlreadyDone,
		StageGitInit:            o.stageGitInit,
		StageGitTemplateCleanup: o.nopStage, // gitinit.Init covers stage 16
		StageParentReobserve:    o.stageParentReobserve,
		StageCommit:             o.stageCommit,
		StageReport:             o.nopStage, // CLI owns final report encoding
	}
}

// BaselineDigest returns the frozen conformance aggregate after verify.
func (o *Orchestrator) BaselineDigest() string {
	if o == nil {
		return ""
	}
	return o.baselineDigest
}

// Close releases retained handles (stage is never deleted — Section 31.6).
func (o *Orchestrator) Close() error {
	if o == nil || o.closed {
		return nil
	}
	o.closed = true
	var first error
	if o.stage != nil {
		if err := o.stage.Close(); err != nil && first == nil {
			first = err
		}
	}
	if o.parent != nil {
		if err := o.parent.Close(); err != nil && first == nil {
			first = err
		}
	}
	if o.origCWD != nil {
		// OriginalCWD has no Close on all platforms; ignore.
		_ = o.origCWD
	}
	return first
}

func (o *Orchestrator) nopStage(ctx context.Context, rt *Runtime) error {
	_ = ctx
	_ = rt
	return nil
}

func (o *Orchestrator) stageToolPreflight(ctx context.Context, rt *Runtime) error {
	_ = rt
	if o.Plan == nil {
		return &StageError{
			Exit:   diagnostic.ExitFailure,
			Detail: "orchestrator: plan is nil",
			Err: diagnostic.New(
				diagnostic.IDInternalBug,
				"generate orchestrator requires a pre-built plan",
				diagnostic.Location{},
			),
		}
	}
	requireGit := o.Plan.Git().Init
	res := toolrun.Preflight(ctx, toolrun.PreflightOptions{
		Host:       o.Host,
		GoBinary:   o.GoBinary,
		GitBinary:  o.GitBinary,
		RequireGit: requireGit,
		Timeout:    toolrun.DefaultPreflightTimeout,
	})
	if err := res.Err(); err != nil {
		return &StageError{
			Exit:   diagnostic.ExitCode(err),
			Detail: "tool-preflight",
			Err:    err,
		}
	}
	o.goPath = res.GoPath
	o.gitPath = res.GitPath
	o.goEnv = toolrun.ConstructGoEnv(o.Host)

	// Bound starter for later tool stages.
	orig, err := toolrun.CaptureOriginalCWD()
	if err != nil {
		return &StageError{
			Exit: diagnostic.ExitFailure,
			Err: diagnostic.Wrap(
				diagnostic.IDInternalBug,
				"cannot capture original working directory for bound tool starts",
				diagnostic.Location{},
				err,
			),
		}
	}
	o.origCWD = orig
	starter, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{})
	if err != nil {
		return &StageError{
			Exit: diagnostic.ExitFailure,
			Err: diagnostic.Wrap(
				diagnostic.IDInternalBug,
				"cannot create bound starter",
				diagnostic.Location{},
				err,
			),
		}
	}
	o.starter = starter
	exec, err := toolrun.NewExecutor(starter)
	if err != nil {
		return &StageError{
			Exit: diagnostic.ExitFailure,
			Err:  diagnostic.Wrap(diagnostic.IDInternalBug, "cannot create tool executor", diagnostic.Location{}, err),
		}
	}
	o.executor = exec
	logStep(o.Log, string(LifePlanned), "tool-preflight", "ok", 0)
	return nil
}

func (o *Orchestrator) stageNetwork(ctx context.Context, rt *Runtime) error {
	_ = ctx
	d := BuildDisclosureFromPlan(o.Plan)
	ApplyDisclosure(rt, d)
	return nil
}

func (o *Orchestrator) stageAcquireParent(ctx context.Context, rt *Runtime) error {
	_ = ctx
	_ = rt
	dest := o.Plan.Destination().Path
	parent, err := fsx.Preflight(dest, fsx.PreflightOptions{})
	if err != nil {
		return &StageError{
			Exit:   diagnostic.ExitCode(err),
			Detail: "acquire-parent",
			Err:    err,
		}
	}
	o.parent = parent
	return nil
}

func (o *Orchestrator) stageCreateStage(ctx context.Context, rt *Runtime) error {
	_ = ctx
	if o.parent == nil {
		return &StageError{
			Exit:   diagnostic.ExitFailure,
			Detail: "create-stage without parent",
			Err: diagnostic.New(
				diagnostic.IDInternalBug,
				"create-stage requires acquired parent",
				diagnostic.Location{},
			),
		}
	}
	project := o.Plan.Project().Name
	if project == "" {
		project = o.Plan.Destination().Basename
	}
	stage, err := fsx.CreateStage(o.parent, project)
	if err != nil {
		// Stage may exist partially; report path if available.
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Detail:   "create-stage",
			Err:      err,
		}
	}
	o.stage = stage
	rt.StageExists = true
	rt.StagePath = stage.Path()
	return nil
}

func (o *Orchestrator) stageRender(ctx context.Context, rt *Runtime) error {
	_ = ctx
	if o.stage == nil || o.Catalog == nil || o.Plan == nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: rt.StageExists,
			Detail:   "render preconditions",
			Err: diagnostic.New(
				diagnostic.IDRenderFailed,
				"render requires stage, catalog, and plan",
				diagnostic.Location{},
			),
		}
	}
	jobs, err := JobsFromPlan(o.Plan, o.Catalog)
	if err != nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Detail:   "render jobs",
			Err:      err,
		}
	}
	w := o.stage.Writer()
	if w == nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Err: diagnostic.New(
				diagnostic.IDRenderFailed,
				"stage writer is nil",
				diagnostic.Location{},
			),
		}
	}
	if _, err := render.RenderAll(o.Catalog, jobs, w); err != nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Detail:   "render",
			Err:      err,
		}
	}
	return nil
}

// stageVerifyPipeline runs stages 11–14 as a single verify.Run (Section 35).
// Subsequent freeze/verify-tools/final-conformance stages are progress nops.
func (o *Orchestrator) stageVerifyPipeline(ctx context.Context, rt *Runtime) error {
	if o.verifyDone {
		return nil
	}
	if o.stage == nil || o.executor == nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: rt.StageExists,
			Err: diagnostic.New(
				diagnostic.IDVerifyFailed,
				"verify requires stage and tool executor",
				diagnostic.Location{},
			),
		}
	}
	stageFD, err := o.dupStageFD()
	if err != nil {
		return &StageError{Exit: diagnostic.ExitFailure, Preserve: true, Err: err}
	}
	defer func() { _ = toolrun.CloseFD(stageFD) }()

	mode := verify.ModeDefault
	if o.Plan.Verification().Mode == plan.VerifyStrict {
		mode = verify.ModeStrict
	}
	pins := pinsFromPlan(o.Plan)
	vres := verify.Run(ctx, o.executor, verify.Options{
		Mode:           mode,
		GoBinary:       o.goPath,
		Env:            o.goEnv,
		StageFD:        stageFD,
		StageDir:       o.stage.Path(),
		ExpectedPins:   pins,
		OutputCapBytes: plan.OutputCapBytes,
	})
	if err := vres.Err(); err != nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Detail:   "verify:" + vres.FailedStep,
			Err:      err,
		}
	}
	o.baselineDigest = vres.BaselineDigest()
	o.verifyDone = true
	return nil
}

func (o *Orchestrator) stageVerifyAlreadyDone(ctx context.Context, rt *Runtime) error {
	_ = ctx
	if !o.verifyDone {
		// Defensive: if freeze runs without tidy completing, fail closed.
		return o.stageVerifyPipeline(ctx, rt)
	}
	return nil
}

func (o *Orchestrator) stageGitInit(ctx context.Context, rt *Runtime) error {
	if o.stage == nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: rt.StageExists,
			Err:      diagnostic.New(diagnostic.IDGitFailed, "git-init requires stage", diagnostic.Location{}),
		}
	}
	git := o.Plan.Git()
	if !git.Init {
		// Progress still fires; no-op body.
		return nil
	}
	if o.executor == nil || o.gitPath == "" {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Err: diagnostic.New(
				diagnostic.IDToolMissing,
				"git binary not available for git-init",
				diagnostic.PathLocation("git"),
			),
		}
	}
	stageFD, err := o.dupStageFD()
	if err != nil {
		return &StageError{Exit: diagnostic.ExitFailure, Preserve: true, Err: err}
	}
	defer func() { _ = toolrun.CloseFD(stageFD) }()

	gres := gitinit.Init(ctx, o.executor, gitinit.Options{
		Init:           true,
		InitialBranch:  git.InitialBranch,
		GitBinary:      o.gitPath,
		PATH:           o.Host.PATH,
		StageFD:        stageFD,
		StageDir:       o.stage.Path(),
		Timeout:        gitinit.DefaultTimeout,
		OutputCapBytes: plan.OutputCapBytes,
		TempRoot:       o.TempRoot,
	})
	if err := gres.Err(); err != nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Detail:   "git-init",
			Err:      err,
		}
	}
	return nil
}

func (o *Orchestrator) stageParentReobserve(ctx context.Context, rt *Runtime) error {
	_ = ctx
	if o.parent == nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: rt.StageExists,
			Err: diagnostic.New(
				diagnostic.IDFSParentMoved,
				"parent handle missing at reobserve",
				diagnostic.Location{},
			),
		}
	}
	if err := o.parent.Reobserve(); err != nil {
		return &StageError{
			Exit:     diagnostic.ExitFailure,
			Preserve: true,
			Detail:   "parent-reobserve",
			Err:      err,
		}
	}
	return nil
}

func (o *Orchestrator) stageCommit(ctx context.Context, rt *Runtime) error {
	_ = ctx
	if o.stage == nil {
		return &StageError{
			Exit: diagnostic.ExitFailure,
			Err: diagnostic.New(
				diagnostic.IDFSCommitAmbiguous,
				"commit requires stage",
				diagnostic.Location{},
			),
		}
	}
	res := fsx.Commit(o.stage)
	switch res.Class {
	case fsx.ClassCommitted:
		rt.CommitOutcome = OutcomeCommitted
		rt.Destination = filepath.Join(o.Plan.Destination().Parent, o.Plan.Destination().Basename)
		if res.DestName != "" {
			// Prefer full destination path from plan for reports.
			rt.Destination = o.Plan.Destination().Path
		}
		return nil
	case fsx.ClassConflict:
		rt.CommitOutcome = OutcomeConflicted
		rt.StageExists = true
		rt.StagePath = res.StagePath
		return &StageError{
			Exit:     res.Exit,
			Preserve: true,
			Outcome:  OutcomeConflicted,
			Detail:   "commit conflict",
			Err:      res.Err,
		}
	case fsx.ClassAmbiguous:
		rt.CommitOutcome = OutcomeAmbiguous
		rt.StageExists = true
		rt.StagePath = res.StagePath
		return &StageError{
			Exit:     res.Exit,
			Preserve: true,
			Outcome:  OutcomeAmbiguous,
			Detail:   "commit ambiguous",
			Err:      res.Err,
		}
	default:
		// Uncommitted / failed-preserved
		rt.CommitOutcome = OutcomeFailedPreserved
		rt.StageExists = true
		rt.StagePath = res.StagePath
		exit := res.Exit
		if exit == 0 {
			exit = diagnostic.ExitFailure
		}
		return &StageError{
			Exit:     exit,
			Preserve: true,
			Outcome:  OutcomeFailedPreserved,
			Detail:   "commit failed",
			Err:      res.Err,
		}
	}
}

func (o *Orchestrator) dupStageFD() (int, error) {
	if o.stage == nil {
		return -1, diagnostic.New(
			diagnostic.IDInternalBug,
			"no stage for descriptor dup",
			diagnostic.Location{},
		)
	}
	// Prefer Transaction-style dup via stage path identity re-verify + Fcntl.
	// Stage exposes DirFD; we duplicate through a tiny helper Transaction-less
	// path: open a CLOEXEC dup of the stage dir fd when available.
	//
	// fsx.Stage does not export Dup; use Transaction wrapper when possible.
	// Begin is wrong here (would create a second stage). Use stage.DirFD +
	// toolrun only when DirFD is live.
	//
	// Production: construct a one-shot Transaction-like dup via Commit path
	// APIs. The Stage's Writer/identity path uses the same object; toolrun
	// needs a separate CLOEXEC fd it may close.
	//
	// We use fsx via a temporary Transaction surface: only DuplicateStageHandle
	// is available on Transaction. Rebuild a minimal Transaction is not
	// exported. Use CreateStage's stage + public Commit(stage) only.
	//
	// Fallback: open stage path as O_DIRECTORY for tools when DirFD is -1
	// (should not happen on unix). Prefer Descriptor from stage.
	return dupStageDescriptor(o.stage)
}

// pinsFromPlan converts plan dependencies to verify pins (runtime+test modules).
func pinsFromPlan(p *plan.Plan) []verify.Pin {
	if p == nil {
		return nil
	}
	deps := p.Dependencies()
	out := make([]verify.Pin, 0, len(deps))
	for _, d := range deps {
		out = append(out, verify.Pin{Path: d.Module, Version: d.Version})
	}
	return out
}

// FormatOrchestratorLog is a host-independent debug line for tests.
func FormatOrchestratorLog(o *Orchestrator) string {
	if o == nil {
		return "orchestrator=nil"
	}
	dest := ""
	if o.Plan != nil {
		dest = o.Plan.Destination().Basename
	}
	return fmt.Sprintf("dest=%s stage=%v verify_done=%v", dest, o.stage != nil, o.verifyDone)
}
