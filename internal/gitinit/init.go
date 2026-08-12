package gitinit

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

// DefaultTimeout is the plan-declared git init timeout (Section 34.3: 1 min).
const DefaultTimeout = 60 * time.Second

// GitRunner executes one plan-declared external step for git init. *toolrun.Executor
// satisfies this interface. Tests inject fakes that record argv/env without
// starting real processes.
type GitRunner interface {
	Run(ctx context.Context, req toolrun.StepRequest) toolrun.StepResult
}

// Options configures isolated git init (Section 39 / REQ-128).
//
// When Init is false the call is a no-op: no subprocess, no template, and
// the stage is left without a Foundry-created `.git`.
//
// StageFD binds child cwd via toolrun (Section 34.4). StageDir is the absolute
// path of the same stage object for post-init filesystem inspection
// (snapshot + semantic validation). Both are required when Init is true.
type Options struct {
	// Init is plan.git.init. When false, Init() skips all Git work.
	Init bool
	// InitialBranch is plan.git.initial_branch (default "main").
	InitialBranch string
	// GitBinary is the absolute preflight-resolved git path.
	GitBinary string
	// PATH is the host PATH fragment used in the closed git env.
	PATH string
	// StageFD is the stage directory descriptor for bound child start.
	StageFD int
	// StageDir is the absolute stage path (same object as StageFD).
	StageDir string
	// Timeout is the plan-declared timeout; zero uses DefaultTimeout.
	Timeout time.Duration
	// OutputCapBytes is the per-stream cap; zero uses plan.OutputCapBytes.
	OutputCapBytes int
	// TempRoot is the Foundry temporary root for the empty template scratch.
	// Empty uses os.TempDir(). Never the destination-parent namespace.
	TempRoot string
	// TemplateDir, when non-empty, is an already-created Foundry template
	// path (tests). When empty, Init creates one via EmptyTemplate and removes
	// it after the step (stage 16).
	TemplateDir string
	// OwnTemplate is true when TemplateDir was created by this call and must
	// be removed. Set automatically when TemplateDir is empty at entry.
	// Callers that pass a pre-built TemplateDir leave this false.
	// (Exported for tests that force ownership.)
	OwnTemplate bool
	// Logger receives the init → scratch remove → re-conformance → semantic
	// timeline. Optional.
	Logger StepLogger
	// SkipSnapshot, when true, skips pre/post non-.git snapshot compare.
	// Production leaves this false. Tests that only exercise argv may set true.
	SkipSnapshot bool
}

// Result is the outcome of Init (toolrun step + stage-16 post-work).
//
// On failure Err is a *diagnostic.FoundryError with id git.failed (or, for
// skip/cancel paths, a wrapped tool class). The stage is never deleted.
type Result struct {
	// Skipped is true when Options.Init was false.
	Skipped bool
	// Step is the toolrun step result (zero when skipped).
	Step toolrun.StepResult
	// Argv is the planned full argv (git basename + args) for process-tree.
	Argv []string
	// EnvHash is the allowlist SHA-256 (never env values).
	EnvHash string
	// EnvKeys are sorted allowlist keys.
	EnvKeys []string
	// TemplateDir is the absolute owned template path used (empty when skipped).
	TemplateDir string
	// TemplateRemoved is true when the owned scratch was successfully removed.
	TemplateRemoved bool
	// SnapshotOK is true when non-.git re-conformance passed (or was skipped).
	SnapshotOK bool
	// SnapshotDiffs lists non-.git divergences when SnapshotOK is false.
	SnapshotDiffs []Diff
	// Semantic is the .git semantic validation report (zero when skipped).
	Semantic SemanticReport
	// FailClass is "git.failed" or empty on success/skip.
	FailClass string
	err       error
}

// Err returns the failure, if any.
func (r Result) Err() error { return r.err }

// OK reports overall success (skip counts as OK — no Git work requested).
func (r Result) OK() bool {
	return r.err == nil && r.FailClass == ""
}

// LogDetail is a single-line summary for steplog (no env values, no homes).
func (r Result) LogDetail() string {
	if r.Skipped {
		return "skipped init=false"
	}
	return fmt.Sprintf(
		"step=%s argv=%q env_hash=%s exit=%d template_removed=%v snapshot_ok=%v semantic_ok=%v fail_class=%s",
		StepID, strings.Join(r.Argv, " "), r.EnvHash, r.Step.ExitCode,
		r.TemplateRemoved, r.SnapshotOK, r.Semantic.OK, r.FailClass,
	)
}

// Init runs stages 15–16 of Section 29.2: optional isolated git init, scratch
// removal, non-.git re-conformance, and semantic `.git` validation.
//
// runner must be non-nil when opts.Init is true (*toolrun.Executor in production).
// Failure never deletes the stage.
func Init(ctx context.Context, runner GitRunner, opts Options) Result {
	log := loggerOrNop(opts.Logger)
	res := Result{SnapshotOK: true}

	if !opts.Init {
		log.GitStep("skip", "skip", "git.init=false")
		res.Skipped = true
		// Document that we did not create .git; if one already exists that is
		// outside this package's authority (stage should be clean pre-init).
		if opts.StageDir != "" && GitDirExists(opts.StageDir) {
			log.GitStep("skip_existing_git", "info", ".git present while init=false (not created by this call)")
		}
		return res
	}

	branch := opts.InitialBranch
	if branch == "" {
		branch = "main"
	}
	if opts.GitBinary == "" {
		return failResult(res, log, "precheck", "empty git binary path")
	}
	if opts.StageFD < 0 {
		return failResult(res, log, "precheck", fmt.Sprintf("invalid stage fd %d", opts.StageFD))
	}
	if opts.StageDir == "" {
		return failResult(res, log, "precheck", "empty stage dir")
	}
	if opts.PATH == "" {
		return failResult(res, log, "precheck", "empty PATH for git env")
	}
	if runner == nil {
		return failResult(res, log, "precheck", "nil GitRunner")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	capN := opts.OutputCapBytes
	if capN <= 0 {
		capN = plan.OutputCapBytes
	}

	// --- stage 15 prep: empty owned template ---
	templateDir := opts.TemplateDir
	ownTemplate := opts.OwnTemplate
	if templateDir == "" {
		var err error
		templateDir, err = EmptyTemplate(opts.TempRoot)
		if err != nil {
			return failResult(res, log, "template_create", err.Error())
		}
		ownTemplate = true
	}
	res.TemplateDir = templateDir
	if !IsFoundryTemplate(templateDir) && ownTemplate {
		// Pre-built test templates may use the foundry-git-template- prefix
		// via EmptyTemplate; if OwnTemplate and not our prefix, still allow
		// but log. RemoveTemplate will refuse non-prefix paths.
	}
	log.GitStep("template_ready", "ok", fmt.Sprintf(
		"path_basename=%s owned=%v foundry_prefix=%v",
		filepath.Base(templateDir), ownTemplate, IsFoundryTemplate(templateDir),
	))

	// Pre-init non-.git snapshot (stage 16 re-conformance baseline).
	var before Snapshot
	if !opts.SkipSnapshot {
		var err error
		before, err = SnapshotNonGit(opts.StageDir)
		if err != nil {
			_ = removeOwned(ownTemplate, templateDir, &res, log)
			return failResult(res, log, "snapshot_before", err.Error())
		}
		log.GitStep("snapshot_before", "ok", fmt.Sprintf("entries=%d", len(before.Keys)))
	}

	// Closed env + planned argv (Section 34.2).
	env := toolrun.ConstructGitEnv(opts.PATH, templateDir)
	args := PlannedArgs(branch, templateDir)
	res.Argv = PlannedArgv(branch, templateDir)
	res.EnvHash = toolrun.AllowlistHash(env)
	res.EnvKeys = toolrun.EnvKeys(env)

	log.GitStep("init_start", "ok", fmt.Sprintf(
		"argv=%q env_hash=%s env_keys=%s timeout_ms=%d",
		strings.Join(res.Argv, " "), res.EnvHash, strings.Join(res.EnvKeys, ","),
		timeout.Milliseconds(),
	))

	step := runner.Run(ctx, toolrun.StepRequest{
		StepID:         StepID,
		Binary:         opts.GitBinary,
		Args:           args,
		Env:            env,
		StageFD:        opts.StageFD,
		Timeout:        timeout,
		OutputCapBytes: capN,
	})
	res.Step = step

	// Log step outcome with lengths; first/last lines only on failure.
	first, last := firstLastLines(step.Stdout, step.Stderr)
	if step.OK() {
		log.GitStep("init_run", "ok", fmt.Sprintf(
			"argv=%q env_hash=%s exit=%d stdout_n=%d stderr_n=%d duration_ms=%d",
			strings.Join(step.Argv, " "), step.EnvHash, step.ExitCode,
			step.StdoutBytes, step.StderrBytes, step.Duration.Milliseconds(),
		))
	} else {
		log.GitStep("init_run", "fail", fmt.Sprintf(
			"argv=%q env_hash=%s exit=%d stdout_n=%d stderr_n=%d fail_class=%s first=%q last=%q",
			strings.Join(step.Argv, " "), step.EnvHash, step.ExitCode,
			step.StdoutBytes, step.StderrBytes, step.FailClass,
			truncate(first, 120), truncate(last, 120),
		))
	}

	// Always remove owned scratch (stage 16) — even on init failure — so we
	// do not leak temp dirs. Destination stage is never touched by this.
	if err := removeOwned(ownTemplate, templateDir, &res, log); err != nil && res.err == nil {
		// Prefer original step failure; only surface removal error if init OK.
		if step.OK() {
			return failResult(res, log, "template_remove", err.Error())
		}
		log.GitStep("template_remove", "fail", err.Error())
	}

	if !step.OK() {
		return mapStepFailure(res, log, step)
	}

	// --- stage 16: re-conformance + semantic ---
	if !opts.SkipSnapshot {
		after, err := SnapshotNonGit(opts.StageDir)
		if err != nil {
			return failResult(res, log, "snapshot_after", err.Error())
		}
		diffs := CompareNonGit(before, after)
		if len(diffs) > 0 {
			res.SnapshotOK = false
			res.SnapshotDiffs = diffs
			detail := formatDiffs(diffs)
			log.GitStep("snapshot_recheck", "fail", detail)
			return failResult(res, log, "snapshot_recheck",
				"non-.git conformance failed after git init: "+detail)
		}
		log.GitStep("snapshot_recheck", "ok", fmt.Sprintf("entries=%d", len(after.Keys)))
		res.SnapshotOK = true
	}

	sem := ValidateSemanticGit(opts.StageDir, branch)
	res.Semantic = sem
	if !sem.OK {
		log.GitStep("semantic", "fail", strings.Join(sem.Failures, "; "))
		return failResult(res, log, "semantic",
			"semantic .git validation failed: "+strings.Join(sem.Failures, "; "))
	}
	log.GitStep("semantic", "ok", fmt.Sprintf(
		"HEAD=%s branch=%s objects=%v refs=%v index_empty=%v",
		sem.HEADRef, sem.ExpectedBranch, sem.HasObjectsDir, sem.HasRefsDir, sem.IndexEmpty,
	))

	log.GitStep("complete", "ok", res.LogDetail())
	return res
}

func removeOwned(own bool, dir string, res *Result, log StepLogger) error {
	if !own || dir == "" {
		if !own {
			log.GitStep("template_remove", "skip", "not owned by this call")
		}
		return nil
	}
	if err := RemoveTemplate(dir); err != nil {
		log.GitStep("template_remove", "fail", err.Error())
		return err
	}
	res.TemplateRemoved = true
	log.GitStep("template_remove", "ok", "basename="+filepath.Base(dir))
	return nil
}

func failResult(res Result, log StepLogger, step, detail string) Result {
	log.GitStep(step, "fail", detail)
	res.FailClass = string(diagnostic.IDGitFailed)
	res.err = diagnostic.Newf(
		diagnostic.IDGitFailed,
		diagnostic.StepLocation(StepID),
		"isolated git init failed at %s: %s", step, detail,
	).WithRemediation(
		"Ensure `git` is available and functional. Inspect the preserved stage's " +
			".git layout; fix host git issues and re-run. Foundry only runs isolated " +
			"`git init` with a scratch template (Section 39). The stage was not deleted.",
	)
	return res
}

func mapStepFailure(res Result, log StepLogger, step toolrun.StepResult) Result {
	res.FailClass = string(diagnostic.IDGitFailed)
	// Prefer wrapping the toolrun error when present; always surface git.failed.
	detail := fmt.Sprintf(
		"git init subprocess failed: exit=%d fail_class=%s timed_out=%v cancelled=%v",
		step.ExitCode, step.FailClass, step.TimedOut, step.Cancelled,
	)
	first, last := firstLastLines(step.Stdout, step.Stderr)
	if first != "" || last != "" {
		detail += fmt.Sprintf(" first=%q last=%q", truncate(first, 120), truncate(last, 120))
	}
	log.GitStep("map_failure", "fail", detail)

	cause := step.Err()
	msg := fmt.Sprintf("isolated git init failed: %s", detail)
	var fe *diagnostic.FoundryError
	if cause != nil && errors.As(cause, &fe) {
		res.err = diagnostic.Wrapf(
			diagnostic.IDGitFailed,
			diagnostic.StepLocation(StepID),
			cause,
			"%s", msg,
		).WithRemediation(
			"Read the step id, argv, env-allowlist hash, and bounded output. " +
				"Inspect the preserved stage; fix the git/tool failure and re-run generate. " +
				"Foundry never commits, sets identity, installs hooks, or pushes.",
		)
		return res
	}
	if cause != nil {
		res.err = diagnostic.Wrapf(
			diagnostic.IDGitFailed,
			diagnostic.StepLocation(StepID),
			cause,
			"%s", msg,
		).WithRemediation(diagnostic.RemediationFor(diagnostic.IDGitFailed))
		return res
	}
	res.err = diagnostic.Newf(
		diagnostic.IDGitFailed,
		diagnostic.StepLocation(StepID),
		"%s", msg,
	).WithRemediation(diagnostic.RemediationFor(diagnostic.IDGitFailed))
	return res
}

func firstLastLines(stdout, stderr []byte) (first, last string) {
	// Prefer stderr for git diagnostics; fall back to stdout.
	combined := string(stderr)
	if combined == "" {
		combined = string(stdout)
	}
	if combined == "" {
		return "", ""
	}
	lines := strings.Split(strings.ReplaceAll(combined, "\r\n", "\n"), "\n")
	// Drop trailing empty from final newline.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return "", ""
	}
	first = lines[0]
	last = lines[len(lines)-1]
	return first, last
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func formatDiffs(diffs []Diff) string {
	parts := make([]string, 0, len(diffs))
	for i, d := range diffs {
		if i >= 8 {
			parts = append(parts, fmt.Sprintf("…+%d more", len(diffs)-i))
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%s", d.Rel, d.Reason))
	}
	return strings.Join(parts, ",")
}
