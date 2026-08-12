package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
	"github.com/spf13/cobra"
)

// reportSink streams generate.GenerationEvent values into a report.Encoder
// (REQ-155/156). generate owns events; report owns encoding.
//
// Thread safety: the generate.Machine calls OnEvent synchronously from its
// single-threaded Run loop — no concurrent writes to firstStreamErr occur.
// If Machine becomes concurrent in the future, add a mutex here.
type reportSink struct {
	enc *report.Encoder
	// firstStreamErr is the first pre-commit stream failure (blocks placement).
	firstStreamErr error
}

// OnEvent implements generate.EventSink.
func (s *reportSink) OnEvent(ev generate.GenerationEvent) {
	if s == nil || s.enc == nil {
		return
	}
	if err := s.enc.ApplyEvent(toReportEvent(ev)); err != nil && s.firstStreamErr == nil {
		s.firstStreamErr = err
	}
}

// toReportEvent maps generate events to report's type (parallel field names).
func toReportEvent(ev generate.GenerationEvent) report.GenerationEvent {
	return report.GenerationEvent{
		Kind:        string(ev.Kind),
		Stage:       report.StageID(ev.Stage),
		State:       report.LifecycleState(ev.State),
		Detail:      ev.Detail,
		Lines:       append([]string(nil), ev.Lines...),
		Outcome:     report.CommitOutcome(ev.Outcome),
		StagePath:   ev.StagePath,
		Destination: ev.Destination,
	}
}

// runGenerate is the generate command body (Section 13 / 29–30 / REQ-034/036/155).
//
//  1. Shared pure pipeline (byte-equal plan to `plan` under same flags)
//  2. Machine lifecycle with production or injected stages
//  3. Stream events to report; final Success/Failure with commit-dominated exit
func (r *root) runGenerate(cmd *cobra.Command, f *planGenerateFlags) error {
	ctx := cmd.Context()
	if err := checkCancelled(ctx); err != nil {
		return err
	}

	// Same pure path as plan (REQ-033 / plan/generate equality defect class).
	out, err := r.runPurePipeline(cmd.InOrStdin(), f.Spec, f.Dest, f.Verify)
	if err != nil {
		return err
	}
	p := out.Plan

	enc := newEncoder(cmd.OutOrStdout(), cmd.ErrOrStderr(), r.g)
	sink := &reportSink{enc: enc}
	recLog := &generate.RecordingLogger{}

	stages, closer, baselineFn := r.buildGenerateStages(p, recLog)
	if closer != nil {
		defer func() { _ = closer() }()
	}

	// Verbose: plan identity (text mode only; never secrets).
	_ = enc.Verbose(fmt.Sprintf(
		"verify=%s plan_sha256=%s dest=%s",
		p.Verification().Mode, p.PlanSHA256(), p.Destination().Basename,
	))

	m := generate.New(generate.Config{
		Stages: stages,
		Sink:   sink,
		Log:    recLog,
	})
	res := m.Run(ctx)

	// Pre-commit stream failure: block success reporting; preserve stage.
	if sink.firstStreamErr != nil && res.Outcome != generate.OutcomeCommitted {
		stagePath := res.StagePath
		code, _ := enc.Failure("generate", diagnostic.New(
			diagnostic.IDReportFailed,
			fmt.Sprintf("output stream failed before commit: %v", sink.firstStreamErr),
			diagnostic.Location{},
		).WithRemediation("Fix the stdout/stderr consumer and re-run generate. If a stage was preserved, inspect stage_path."), stagePath)
		return exitWith(code)
	}

	baseline := ""
	if baselineFn != nil {
		baseline = baselineFn()
	}
	return r.finishGenerate(enc, p, res, baseline)
}

// buildGenerateStages returns stage funcs: Options.GenerateStages when set
// (unit tests), otherwise production Orchestrator stages.
//
// closer releases orchestration handles; baselineFn returns the verify
// aggregate digest after Run (production only).
func (r *root) buildGenerateStages(p *plan.Plan, log generate.StepLogger) (
	stages map[generate.StageID]generate.StageFunc,
	closer func() error,
	baselineFn func() string,
) {
	if r.opts.GenerateStages != nil {
		// Test injection: copy map; fill network disclosure from plan when absent.
		stages = make(map[generate.StageID]generate.StageFunc, len(r.opts.GenerateStages)+1)
		for k, v := range r.opts.GenerateStages {
			stages[k] = v
		}
		if _, ok := stages[generate.StageReportPlanNetwork]; !ok {
			stages[generate.StageReportPlanNetwork] = generate.DiscloseFromPlanStage(p)
		}
		return stages, nil, func() string { return "" }
	}

	cat, err := r.loadCatalog()
	if err != nil {
		return map[generate.StageID]generate.StageFunc{
			generate.StageLoadCatalog: generate.FailStage(&generate.StageError{
				Exit: diagnostic.ExitFailure,
				Err:  err,
			}),
		}, nil, func() string { return "" }
	}

	host := r.generatePipelineHost(p.Git().Init)
	orch := &generate.Orchestrator{
		Plan:      p,
		Catalog:   cat,
		Host:      toolrun.HostCaptureFromPlan(host.Host),
		GoBinary:  host.GoBinary,
		GitBinary: host.GitBinary,
		Log:       log,
	}
	return orch.Stages(), orch.Close, orch.BaselineDigest
}

// finishGenerate encodes the terminal report and returns exitError / nil.
func (r *root) finishGenerate(enc *report.Encoder, p *plan.Plan, res generate.Result, baseline string) error {
	networkIDs := networkStepIDs(p)

	switch res.Outcome {
	case generate.OutcomeCommitted:
		enc.MarkCommitted()
		result := report.GenerateResult{
			CommitOutcome:             report.OutcomeCommitted,
			Destination:               res.Destination,
			PlanSHA256:                p.PlanSHA256(),
			ConformanceBaselineDigest: baseline,
			NetworkDisclosure:         networkIDs,
			NextSteps:                 generateNextSteps(p),
		}
		text := formatGenerateSuccess(result, p.Git().Init)
		// Post-commit stream failure is absorbed by MarkCommitted (exit 0).
		_ = enc.Success("generate", result, text)
		return nil

	case generate.OutcomeCancelled:
		stagePath := res.StagePath
		err := res.Err
		if err == nil {
			err = diagnostic.ErrCancelled
		}
		code, _ := enc.Failure("generate", err, stagePath)
		if code == 0 {
			code = diagnostic.ExitCancelled
		}
		return exitWith(code)

	default:
		// conflicted / failed-preserved / ambiguous / not-started
		stagePath := res.StagePath
		err := res.Err
		if err == nil {
			err = diagnostic.New(
				diagnostic.IDInternalBug,
				fmt.Sprintf("generate failed with outcome %s", res.Outcome),
				diagnostic.Location{},
			)
		}
		// Prefer machine exit when set (conflict → 2, etc.).
		code, werr := enc.Failure("generate", err, stagePath)
		_ = werr
		if res.Exit != 0 {
			code = res.Exit
		}
		return exitWith(code)
	}
}

func networkStepIDs(p *plan.Plan) []string {
	if p == nil {
		return nil
	}
	d := generate.BuildDisclosureFromPlan(p)
	return d.StepIDs()
}

func generateNextSteps(p *plan.Plan) []string {
	if p == nil {
		return nil
	}
	dest := p.Destination().Basename
	if dest == "" {
		dest = p.Project().Name
	}
	steps := []string{
		fmt.Sprintf("cd %s", dest),
	}
	if p.Git().Init {
		steps = append(steps, `git add . && git commit -m "Initial commit"`)
	}
	steps = append(steps, "go test ./...")
	return steps
}

func formatGenerateSuccess(r report.GenerateResult, gitInit bool) string {
	var b strings.Builder
	// "wrote" = filesystem transaction succeeded (exclusive rename). Not a git commit.
	fmt.Fprintf(&b, "generate: wrote destination=%s", r.Destination)
	if r.PlanSHA256 != "" {
		fmt.Fprintf(&b, " plan_sha256=%s", r.PlanSHA256)
	}
	if r.StagePath != "" {
		fmt.Fprintf(&b, " stage_path=%s", r.StagePath)
	}
	if gitInit {
		fmt.Fprintf(&b, "\ngenerate: git initialized (no commits); run: git add . && git commit")
	}
	return b.String()
}

// redactDestForLog returns destination basename only (goldens / logs).
func redactDestForLog(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
