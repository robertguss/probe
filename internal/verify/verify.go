package verify

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// StepRunner executes one plan-declared external step. *toolrun.Executor
// satisfies this interface. Tests inject fakes that record argv/env without
// starting real processes.
type StepRunner interface {
	Run(ctx context.Context, req toolrun.StepRequest) toolrun.StepResult
}

// Options configures the staging verification pipeline (Section 35).
//
// StageFD binds child cwd via toolrun (Section 34.4). StageDir is the absolute
// path of the same stage object for in-process filesystem checks. Both are
// required for external steps.
type Options struct {
	// Mode is default or strict (REQ-152). Empty normalizes to default.
	Mode Mode
	// GoBinary is the absolute preflight-resolved go path.
	GoBinary string
	// Env is the closed Go environment (Section 34.2 ConstructGoEnv).
	Env map[string]string
	// StageFD is the stage directory descriptor for bound child start.
	StageFD int
	// StageDir is the absolute stage path (same object as StageFD).
	StageDir string
	// ExpectedPins are exact module pins that must survive tidy reparse.
	// Empty skips pin presence checks (still rejects replace/exclude).
	ExpectedPins []Pin
	// OutputCapBytes is the per-stream cap; zero uses plan.OutputCapBytes.
	OutputCapBytes int
	// Logger receives the per-step timeline. Optional.
	Logger StepLogger
	// SkipEnvCheck, when true, skips AssertNoEnvWeaken (tests only).
	SkipEnvCheck bool
	// SkipTidy, when true, assumes go.mod/go.sum already post-tidy and freezes
	// the current tree as the baseline after mutation/pin checks against a
	// synthetic empty pre-snapshot (tests for post-tidy steps only).
	// Production leaves this false.
	SkipTidy bool
	// OnlySteps, when non-empty, runs only these step IDs (tests). Production
	// leaves nil to run the full mode list.
	OnlySteps []string
}

// StepOutcome is the recorded result of one verify step for logs/report.
type StepOutcome struct {
	ID             string
	Kind           Kind
	Argv           []string
	Duration       time.Duration
	ExitCode       int
	StdoutBytes    int64
	StderrBytes    int64
	BaselineBefore string
	BaselineAfter  string
	MutationPaths  []string
	FailClass      string
	OK             bool
	Tool           toolrun.StepResult // zero when in-proc
}

// Result is the outcome of Run (stages 11–14).
//
// On failure Err is a *diagnostic.FoundryError. The stage is never deleted.
type Result struct {
	// Mode is the normalized mode used.
	Mode Mode
	// Baseline is the frozen post-tidy tree (zero when freeze never ran).
	Baseline ConformanceBaseline
	// Frozen is true when ConformanceBaseline was successfully frozen.
	Frozen bool
	// Steps lists per-step outcomes in execution order.
	Steps []StepOutcome
	// Mutation is the tidy mutation-set report (zero when tidy skipped without check).
	Mutation MutationReport
	// FailedStep is the first failing step id (empty on success).
	FailedStep string
	// FailClass is the diagnostic id string on failure.
	FailClass string
	err       error
}

// Err returns the failure, if any.
func (r Result) Err() error { return r.err }

// OK reports overall success.
func (r Result) OK() bool {
	return r.err == nil && r.FailClass == ""
}

// BaselineDigest returns the aggregate digest or empty when not frozen.
func (r Result) BaselineDigest() string {
	if !r.Frozen {
		return ""
	}
	return r.Baseline.AggregateDigest
}

// LogDetail is a single-line summary for steplog (no env values, no homes).
func (r Result) LogDetail() string {
	return fmt.Sprintf(
		"mode=%s frozen=%v baseline=%s steps=%d failed_step=%s fail_class=%s",
		r.Mode, r.Frozen, shortDigest(r.BaselineDigest()), len(r.Steps),
		r.FailedStep, r.FailClass,
	)
}

// Run executes the full verification pipeline for opts.Mode (Section 35).
//
// runner must be non-nil when any external step runs (*toolrun.Executor in
// production). Failure never deletes the stage.
func Run(ctx context.Context, runner StepRunner, opts Options) Result {
	log := loggerOrNop(opts.Logger)
	mode := opts.Mode.Normalize()
	res := Result{Mode: mode}

	if !opts.SkipEnvCheck {
		if err := AssertNoEnvWeaken(); err != nil {
			return fail(res, log, "env_weaken", err)
		}
	}
	if !mode.Valid() {
		return fail(res, log, "mode", diagnostic.Newf(
			diagnostic.IDVerifyFailed,
			diagnostic.Location{},
			"invalid verify mode %q", mode,
		))
	}
	if opts.StageDir == "" {
		return fail(res, log, "precheck", diagnostic.Newf(
			diagnostic.IDVerifyFailed,
			diagnostic.Location{},
			"empty stage dir",
		))
	}

	steps := StepsFor(mode)
	if len(opts.OnlySteps) > 0 {
		steps = filterSteps(steps, opts.OnlySteps)
	}

	// Race must never appear (static audit at runtime).
	if ContainsRace(steps) {
		return fail(res, log, "race_audit", diagnostic.Newf(
			diagnostic.IDVerifyFailed,
			diagnostic.Location{},
			"generation gate must not include race detector (FND-004)",
		))
	}

	capN := opts.OutputCapBytes
	if capN <= 0 {
		capN = plan.OutputCapBytes
	}

	var baseline ConformanceBaseline
	frozen := false
	var preTidy ConformanceBaseline

	for _, step := range steps {
		start := time.Now()
		outcome := StepOutcome{
			ID:   step.ID,
			Kind: step.Kind,
			Argv: append([]string(nil), step.Argv...),
		}
		if frozen {
			outcome.BaselineBefore = baseline.AggregateDigest
		}

		log.VerifyStep(step.ID+"_start", "ok", fmt.Sprintf(
			"kind=%s argv=%q baseline_before=%s",
			step.Kind, strings.Join(step.Argv, " "), shortDigest(outcome.BaselineBefore),
		))

		var stepErr error
		switch step.ID {
		case StepGoModTidy:
			if opts.SkipTidy {
				log.VerifyStep(step.ID, "skip", "SkipTidy=true")
				outcome.OK = true
				outcome.Duration = time.Since(start)
				res.Steps = append(res.Steps, outcome)
				continue
			}
			var snapErr error
			preTidy, snapErr = SnapshotNonGit(opts.StageDir)
			if snapErr != nil {
				stepErr = diagnostic.Wrapf(
					diagnostic.IDVerifyFailed,
					diagnostic.StepLocation(StepGoModTidy),
					snapErr,
					"pre-tidy snapshot failed: %v", snapErr,
				)
				break
			}
			log.VerifyStep("pre_tidy_snapshot", "ok", fmt.Sprintf(
				"entries=%d digest=%s", len(preTidy.Keys), shortDigest(preTidy.AggregateDigest),
			))
			tr, err := runExternal(ctx, runner, opts, step, capN)
			outcome.Tool = tr
			outcome.ExitCode = tr.ExitCode
			outcome.StdoutBytes = tr.StdoutBytes
			outcome.StderrBytes = tr.StderrBytes
			if err != nil {
				stepErr = err
				break
			}
			outcome.OK = true

		case CheckModuleMutation:
			// If tidy was skipped, treat current tree as post and pre=current
			// for path-set equality; still reparse pins.
			var post ConformanceBaseline
			var err error
			post, err = SnapshotNonGit(opts.StageDir)
			if err != nil {
				stepErr = diagnostic.Wrapf(
					diagnostic.IDVerifyFailed,
					diagnostic.StepLocation(CheckModuleMutation),
					err,
					"post-tidy snapshot failed: %v", err,
				)
				break
			}
			pre := preTidy
			if opts.SkipTidy || len(pre.Entries) == 0 {
				// No tidy ran: require zero extra paths vs self (changed empty).
				pre = post
			}
			rep, merr := ValidateMutationAndPins(pre, post, opts.StageDir, opts.ExpectedPins)
			res.Mutation = rep
			outcome.MutationPaths = append([]string(nil), rep.ChangedPaths...)
			if merr != nil {
				stepErr = merr
				break
			}
			// Freeze baseline after successful mutation-set (REQ-126).
			baseline = post
			// Recompute aggregate is already on post.
			frozen = true
			res.Baseline = baseline
			res.Frozen = true
			outcome.BaselineAfter = baseline.AggregateDigest
			outcome.OK = true
			log.VerifyStep("freeze", "ok", fmt.Sprintf(
				"entries=%d digest=%s changed=%v",
				len(baseline.Keys), shortDigest(baseline.AggregateDigest), rep.ChangedPaths,
			))

		case CheckGofmt:
			fails, err := CheckGoFmt(opts.StageDir)
			if err != nil {
				stepErr = err
				_ = fails
				break
			}
			outcome.OK = true

		case CheckFinalConformance:
			if !frozen {
				stepErr = diagnostic.Newf(
					diagnostic.IDVerifyFailed,
					diagnostic.StepLocation(CheckFinalConformance),
					"final conformance requires frozen baseline",
				)
				break
			}
			diffs, err := Conform(baseline, opts.StageDir)
			if err != nil {
				outcome.MutationPaths = diffPaths(diffs)
				stepErr = err
				break
			}
			after, _ := SnapshotNonGit(opts.StageDir)
			outcome.BaselineAfter = after.AggregateDigest
			outcome.OK = true

		default:
			// External post-freeze tools.
			if step.Kind != KindExternal {
				stepErr = diagnostic.Newf(
					diagnostic.IDVerifyFailed,
					diagnostic.StepLocation(step.ID),
					"unknown in-process step %s", step.ID,
				)
				break
			}
			tr, err := runExternal(ctx, runner, opts, step, capN)
			outcome.Tool = tr
			outcome.ExitCode = tr.ExitCode
			outcome.StdoutBytes = tr.StdoutBytes
			outcome.StderrBytes = tr.StderrBytes
			if err != nil {
				stepErr = err
				break
			}
			// Conformance after every external tool (FND-006).
			if step.AfterToolConform {
				if !frozen {
					stepErr = diagnostic.Newf(
						diagnostic.IDVerifyFailed,
						diagnostic.StepLocation(step.ID),
						"post-tool conformance requires frozen baseline before %s", step.ID,
					)
					break
				}
				diffs, cerr := ConformAfterStep(baseline, opts.StageDir, step.ID)
				if cerr != nil {
					outcome.MutationPaths = diffPaths(diffs)
					stepErr = cerr
					break
				}
				after, _ := SnapshotNonGit(opts.StageDir)
				outcome.BaselineAfter = after.AggregateDigest
			}
			outcome.OK = true
		}

		outcome.Duration = time.Since(start)
		if frozen && outcome.BaselineAfter == "" {
			outcome.BaselineAfter = baseline.AggregateDigest
		}

		if stepErr != nil {
			outcome.OK = false
			outcome.FailClass = failClassOf(stepErr)
			res.Steps = append(res.Steps, outcome)
			log.VerifyStep(step.ID, "fail", fmt.Sprintf(
				"argv=%q duration_ms=%d exit=%d fail_class=%s mutation=%v baseline_before=%s baseline_after=%s err=%s",
				strings.Join(outcome.Argv, " "), outcome.Duration.Milliseconds(),
				outcome.ExitCode, outcome.FailClass, outcome.MutationPaths,
				shortDigest(outcome.BaselineBefore), shortDigest(outcome.BaselineAfter),
				stepErr.Error(),
			))
			return fail(res, log, step.ID, stepErr)
		}

		log.VerifyStep(step.ID, "ok", fmt.Sprintf(
			"argv=%q duration_ms=%d exit=%d stdout_n=%d stderr_n=%d baseline_before=%s baseline_after=%s mutation=%v",
			strings.Join(outcome.Argv, " "), outcome.Duration.Milliseconds(),
			outcome.ExitCode, outcome.StdoutBytes, outcome.StderrBytes,
			shortDigest(outcome.BaselineBefore), shortDigest(outcome.BaselineAfter),
			outcome.MutationPaths,
		))
		res.Steps = append(res.Steps, outcome)
	}

	log.VerifyStep("complete", "ok", res.LogDetail())
	return res
}

func runExternal(ctx context.Context, runner StepRunner, opts Options, step Step, capN int) (toolrun.StepResult, error) {
	if runner == nil {
		return toolrun.StepResult{}, diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(step.ID),
			"nil StepRunner for external step %s", step.ID,
		)
	}
	if opts.GoBinary == "" {
		return toolrun.StepResult{}, diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(step.ID),
			"empty go binary for step %s", step.ID,
		)
	}
	if opts.StageFD < 0 {
		return toolrun.StepResult{}, diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(step.ID),
			"invalid stage fd %d for step %s", opts.StageFD, step.ID,
		)
	}
	args := step.Argv
	if len(args) > 0 {
		// Argv[0] is "go" basename; toolrun wants argv[1:].
		args = args[1:]
	}
	timeout := step.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	tr := runner.Run(ctx, toolrun.StepRequest{
		StepID:         step.ID,
		Binary:         opts.GoBinary,
		Args:           args,
		Env:            opts.Env,
		StageFD:        opts.StageFD,
		Timeout:        timeout,
		OutputCapBytes: capN,
	})
	if !tr.OK() {
		// Prefer toolrun's diagnostic; map to tool.* ids.
		if tr.Err() != nil {
			return tr, tr.Err()
		}
		return tr, diagnostic.Newf(
			diagnostic.IDToolFailed,
			diagnostic.StepLocation(step.ID),
			"external step %s failed: exit=%d fail_class=%s",
			step.ID, tr.ExitCode, tr.FailClass,
		)
	}
	return tr, nil
}

func fail(res Result, log StepLogger, step string, err error) Result {
	res.FailedStep = step
	res.FailClass = failClassOf(err)
	res.err = err
	if log != nil {
		log.VerifyStep("pipeline_fail", "fail", fmt.Sprintf(
			"failed_step=%s fail_class=%s err=%s", step, res.FailClass, errString(err),
		))
	}
	return res
}

func failClassOf(err error) string {
	if err == nil {
		return ""
	}
	var fe *diagnostic.FoundryError
	if errors.As(err, &fe) {
		return string(fe.ID())
	}
	return string(diagnostic.IDVerifyFailed)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func filterSteps(steps []Step, only []string) []Step {
	want := make(map[string]struct{}, len(only))
	for _, id := range only {
		want[id] = struct{}{}
	}
	var out []Step
	for _, s := range steps {
		if _, ok := want[s.ID]; ok {
			out = append(out, s)
		}
	}
	return out
}

func diffPaths(diffs []Diff) []string {
	out := make([]string, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, d.Rel)
	}
	return out
}

// StageBasename returns the stage directory basename for logging (never full
// host homes when stage lives under a user path).
func StageBasename(stageDir string) string {
	return filepath.Base(stageDir)
}
