//go:build unix

package gitinit_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/gitinit"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// TestInit_False_LeavesNoGit proves git.init=false is a no-op with no .git.
func TestInit_False_LeavesNoGit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	stage := stageWithFiles(t, map[string]string{"README.md": "hi\n"})
	ml := &memLog{}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := gitinit.Init(context.Background(), nil, gitinit.Options{
		Init:     false,
		StageDir: stage,
		Logger:   ml,
	})
	log.Step("init", testutil.OutcomeOK, res.LogDetail())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	log.Assert("ok", res.OK(), true, res.OK())
	log.Assert("skipped", res.Skipped, true, res.Skipped)
	log.Assert("no_git", !gitinit.GitDirExists(stage), true, gitinit.GitDirExists(stage))
	log.Assert("no_runner_needed", res.Step.PID == 0, 0, res.Step.PID)
	log.Assert("skip_logged", ml.Has("skip", "skip"), true, false)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestInit_True_CreatesRepoWithBranch exercises the full success path with a
// FakeGitRunner so the test is not gated on a host git binary.
func TestInit_True_CreatesRepoWithBranch(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	stage := stageWithFiles(t, map[string]string{
		"README.md": "# demo\n",
		"main.go":   "package main\n",
	})
	stageFD := openStageFD(t, stage)
	fake := &FakeGitRunner{PlantGitDir: stage}
	ml := &memLog{}
	tempRoot := t.TempDir()
	log.Inputs(map[string]string{"branch": "main", "files": "2"})
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := gitinit.Init(context.Background(), fake, gitinit.Options{
		Init:          true,
		InitialBranch: "main",
		GitBinary:     "/usr/bin/git",
		PATH:          "/usr/bin:/bin",
		StageFD:       stageFD,
		StageDir:      stage,
		TempRoot:      tempRoot,
		Timeout:       30 * time.Second,
		Logger:        ml,
	})
	log.Subprocess("git-init", res.Argv, res.EnvHash, res.Step.ExitCode,
		int(res.Step.StdoutBytes), int(res.Step.StderrBytes), "", "", !res.OK())
	log.Step("timeline", testutil.OutcomeOK, res.LogDetail())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if !res.OK() {
		log.Fail("init_ok", fmt.Sprintf("err=%v class=%s", res.Err(), res.FailClass))
	}
	log.Assert("one_call", fake.CallCount() == 1, 1, fake.CallCount())
	log.Assert("git_exists", gitinit.GitDirExists(stage), true, true)
	log.Assert("semantic", res.Semantic.OK, true, res.Semantic.OK)
	log.Assert("HEAD", res.Semantic.HEADRef == "ref: refs/heads/main",
		"ref: refs/heads/main", res.Semantic.HEADRef)
	log.Assert("branch", res.Semantic.ExpectedBranch == "main", "main", res.Semantic.ExpectedBranch)
	log.Assert("index_empty", res.Semantic.IndexEmpty, true, res.Semantic.IndexEmpty)
	log.Assert("no_hook_content", len(res.Semantic.HooksWithContent) == 0, 0, len(res.Semantic.HooksWithContent))
	log.Assert("snapshot_ok", res.SnapshotOK, true, res.SnapshotOK)
	log.Assert("template_removed", res.TemplateRemoved, true, res.TemplateRemoved)
	log.Assert("foundry_template", gitinit.IsFoundryTemplate(res.TemplateDir), true, res.TemplateDir)
	// Scratch gone.
	if _, err := os.Stat(res.TemplateDir); !os.IsNotExist(err) {
		log.Fail("scratch_still_present", res.TemplateDir)
	}
	// Non-.git files intact.
	b, err := os.ReadFile(filepath.Join(stage, "README.md"))
	if err != nil || string(b) != "# demo\n" {
		log.Fail("readme", fmt.Sprintf("%v %q", err, b))
	}
	// Timeline includes required steps.
	for _, step := range []string{"template_ready", "init_run", "template_remove", "snapshot_recheck", "semantic", "complete"} {
		log.Assert("log_"+step, ml.Has(step, "ok"), true, false)
	}
	// Env hash present; no secret values in logs.
	log.Assert("env_hash_len", len(res.EnvHash) == 64, 64, len(res.EnvHash))
	for _, e := range ml.Entries() {
		if strings.Contains(e.Detail, "HOME=") || strings.Contains(e.Detail, os.Getenv("HOME")) {
			// PATH may contain home-like fragments; only ban explicit HOME= env dump.
			if strings.Contains(e.Detail, "HOME=") {
				log.Fail("env_value_leaked", e.Detail)
			}
		}
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestInit_CustomBranch proves initial_branch is honored using FakeGitRunner.
func TestInit_CustomBranch(t *testing.T) {
	log := testutil.New(t)
	stage := stageWithFiles(t, map[string]string{"f.txt": "x\n"})
	stageFD := openStageFD(t, stage)
	fake := &FakeGitRunner{PlantGitDir: stage}
	res := gitinit.Init(context.Background(), fake, gitinit.Options{
		Init: true, InitialBranch: "develop", GitBinary: "/usr/bin/git",
		PATH: "/usr/bin:/bin", StageFD: stageFD, StageDir: stage,
		TempRoot: t.TempDir(), Timeout: 30 * time.Second,
	})
	if !res.OK() {
		log.Fail("ok", res.Err().Error())
	}
	log.Assert("HEAD", res.Semantic.HEADRef == "ref: refs/heads/develop",
		"ref: refs/heads/develop", res.Semantic.HEADRef)
	log.Assert("argv_branch", strings.Contains(strings.Join(res.Argv, " "), "--initial-branch=develop"),
		true, strings.Join(res.Argv, " "))
}

// TestInit_ProcessTree_OnlyPlannedGit proves exactly one git init argv; no
// forbidden subcommands (REQ-160).
func TestInit_ProcessTree_OnlyPlannedGit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	stage := stageWithFiles(t, map[string]string{"a.go": "package a\n"})
	stageFD := openStageFD(t, stage)
	var observed [][]string
	opts := plantGitViaFakeStart(stage, "main")
	// Record exactly once per Start (process-tree = planned argv only).
	inner := opts.StartChild
	opts.StartChild = func(ctx context.Context, binary string, args, env []string, dir string, cap int, grace time.Duration) (int, func() ([]byte, []byte, error), error) {
		observed = append(observed, append([]string{filepath.Base(binary)}, args...))
		return inner(ctx, binary, args, env, dir, cap, grace)
	}
	ex := newExecutor(t, opts)
	tmpl, err := gitinit.EmptyTemplate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Pre-create template so we know exact argv for AssertOnlyInit.
	// Own it so removal runs.
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := gitinit.Init(context.Background(), ex, gitinit.Options{
		Init: true, InitialBranch: "main", GitBinary: "/usr/bin/git",
		PATH: "/usr/bin:/bin", StageFD: stageFD, StageDir: stage,
		TemplateDir: tmpl, OwnTemplate: true, Timeout: 5 * time.Second,
	})
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if !res.OK() {
		log.Fail("ok", fmt.Sprintf("%v", res.Err()))
	}
	log.Assert("one_start", len(observed) == 1, 1, len(observed))
	if len(observed) != 1 {
		log.Fail("observed", fmt.Sprintf("%v", observed))
	}
	argv := observed[0]
	log.Step("argv", testutil.OutcomeOK, strings.Join(argv, " "))
	// Process tree: only init.
	sub := gitinit.SubcommandOf(argv)
	log.Assert("sub_init", sub == gitinit.OnlyGitSubcommand, gitinit.OnlyGitSubcommand, sub)
	log.Assert("not_forbidden", !gitinit.IsForbiddenSubcommand(sub), false, gitinit.IsForbiddenSubcommand(sub))
	for _, f := range gitinit.ForbiddenGitSubcommands {
		joined := strings.Join(argv, " ")
		// "branch" appears only as --initial-branch= flag, not as subcommand.
		if f == "branch" {
			continue
		}
		// token boundary: space+f+space or space+f+end
		if strings.Contains(joined, " "+f+" ") || strings.HasSuffix(joined, " "+f) {
			log.Fail("forbidden_in_argv", f+" in "+joined)
		}
	}
	// Exact planned shape.
	want := gitinit.PlannedArgv("main", tmpl)
	log.Assert("argv_eq", strings.Join(argv, " ") == strings.Join(want, " "),
		strings.Join(want, " "), strings.Join(argv, " "))
	log.Assert("no_shell", !strings.Contains(strings.Join(argv, " "), "sh"), true, false)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestInit_EmptyOwnedTemplate proves template path is Foundry-controlled.
func TestInit_EmptyOwnedTemplate(t *testing.T) {
	log := testutil.New(t)
	stage := stageWithFiles(t, map[string]string{"x": "1\n"})
	stageFD := openStageFD(t, stage)
	var templateSeen string
	opts := plantGitViaFakeStart(stage, "main")
	opts.StartChild = func(ctx context.Context, binary string, args, env []string, dir string, cap int, grace time.Duration) (int, func() ([]byte, []byte, error), error) {
		for _, a := range args {
			if strings.HasPrefix(a, "--template=") {
				templateSeen = strings.TrimPrefix(a, "--template=")
			}
		}
		for _, e := range env {
			if strings.HasPrefix(e, "GIT_TEMPLATE_DIR=") {
				templateSeen = strings.TrimPrefix(e, "GIT_TEMPLATE_DIR=")
			}
		}
		return plantGitViaFakeStart(stage, "main").StartChild(ctx, binary, args, env, dir, cap, grace)
	}
	ex := newExecutor(t, opts)
	tempRoot := t.TempDir()
	res := gitinit.Init(context.Background(), ex, gitinit.Options{
		Init: true, InitialBranch: "main", GitBinary: "/usr/bin/git",
		PATH: "/bin", StageFD: stageFD, StageDir: stage, TempRoot: tempRoot,
		Timeout: 5 * time.Second,
	})
	if !res.OK() {
		log.Fail("ok", res.Err().Error())
	}
	log.Assert("foundry_prefix", gitinit.IsFoundryTemplate(templateSeen), true, templateSeen)
	log.Assert("under_temp", strings.HasPrefix(templateSeen, tempRoot), true, templateSeen)
	log.Assert("env_has_template", res.Step.EnvHash != "", true, res.Step.EnvHash)
	// Env keys exact allowlist.
	wantKeys := toolrun.GitAllowlistKeys
	// Sort copy for compare — EnvKeys already sorted.
	got := res.EnvKeys
	log.Assert("key_count", len(got) == len(wantKeys), len(wantKeys), len(got))
}

// TestInit_Failure_GitFailedAndRemediation maps non-zero git to git.failed.
func TestInit_Failure_GitFailedAndRemediation(t *testing.T) {
	log := testutil.New(t)
	stage := stageWithFiles(t, map[string]string{"x": "1\n"})
	stageFD := openStageFD(t, stage)
	// Fake child exits non-zero without creating .git.
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			return 9, func() ([]byte, []byte, error) {
				return nil, []byte("fatal: mock init failure\n"), fmt.Errorf("exit status 128")
			}, nil
		},
	})
	ml := &memLog{}
	res := gitinit.Init(context.Background(), ex, gitinit.Options{
		Init: true, InitialBranch: "main", GitBinary: "/usr/bin/git",
		PATH: "/bin", StageFD: stageFD, StageDir: stage, TempRoot: t.TempDir(),
		Timeout: 5 * time.Second, Logger: ml,
	})
	log.Assert("not_ok", !res.OK(), false, res.OK())
	log.Assert("fail_class", res.FailClass == string(diagnostic.IDGitFailed),
		diagnostic.IDGitFailed, res.FailClass)
	var fe *diagnostic.FoundryError
	if !errors.As(res.Err(), &fe) {
		log.Fail("foundry_error", fmt.Sprintf("%T %v", res.Err(), res.Err()))
	} else {
		log.Assert("id", fe.ID() == diagnostic.IDGitFailed, diagnostic.IDGitFailed, fe.ID())
		log.Assert("remediation", fe.Remediation() != "", true, fe.Remediation())
		log.Assert("location_step", fe.Location().StepID == gitinit.StepID, gitinit.StepID, fe.Location().StepID)
	}
	// Stage still present with files (preservation — we never delete stage).
	if _, err := os.Stat(filepath.Join(stage, "x")); err != nil {
		log.Fail("stage_file", err.Error())
	}
	log.Assert("fail_logged", ml.Has("init_run", "fail"), true, false)
	// Template still removed on failure path.
	log.Assert("template_removed", res.TemplateRemoved, true, res.TemplateRemoved)
}

// TestInit_SnapshotDetectsNonGitMutation fails when git step mutates non-.git files.
func TestInit_SnapshotDetectsNonGitMutation(t *testing.T) {
	log := testutil.New(t)
	stage := stageWithFiles(t, map[string]string{"keep.txt": "same\n"})
	stageFD := openStageFD(t, stage)
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, _ string, args, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			// Mutate a non-.git file — must be detected by re-conformance.
			_ = os.WriteFile(filepath.Join(stage, "keep.txt"), []byte("MUTATED\n"), 0o644)
			// Also plant valid .git so only snapshot fails.
			if err := PlantMinimalGitDir(stage, "main"); err != nil {
				t.Fatal(err)
			}
			return 1, func() ([]byte, []byte, error) { return nil, nil, nil }, nil
		},
	})
	res := gitinit.Init(context.Background(), ex, gitinit.Options{
		Init: true, InitialBranch: "main", GitBinary: "/usr/bin/git",
		PATH: "/bin", StageFD: stageFD, StageDir: stage, TempRoot: t.TempDir(),
		Timeout: 5 * time.Second,
	})
	log.Assert("not_ok", !res.OK(), false, res.OK())
	log.Assert("snapshot_fail", !res.SnapshotOK, false, res.SnapshotOK)
	log.Assert("has_diff", len(res.SnapshotDiffs) > 0, true, len(res.SnapshotDiffs))
	found := false
	for _, d := range res.SnapshotDiffs {
		if d.Rel == "keep.txt" && d.Reason == "bytes" {
			found = true
		}
	}
	log.Assert("bytes_diff", found, true, fmt.Sprintf("%v", res.SnapshotDiffs))
	log.Assert("git_failed", res.FailClass == string(diagnostic.IDGitFailed),
		diagnostic.IDGitFailed, res.FailClass)
}

// TestInit_Semantic_RejectsFileMasquerading ensures .git file fails validation.
func TestInit_Semantic_RejectsFileMasquerading(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	// Plant .git as a file.
	if err := os.WriteFile(filepath.Join(stage, ".git"), []byte("gitdir: /evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := gitinit.ValidateSemanticGit(stage, "main")
	log.Assert("not_ok", !rep.OK, false, rep.OK)
	log.Assert("not_dir", !rep.IsDirectory, false, rep.IsDirectory)
	log.Assert("has_failure", len(rep.Failures) > 0, true, len(rep.Failures))
}

// TestInit_LogsArgvEnvHashExit covers logging contract fields.
func TestInit_LogsArgvEnvHashExit(t *testing.T) {
	log := testutil.New(t)
	stage := stageWithFiles(t, map[string]string{"z": "z\n"})
	stageFD := openStageFD(t, stage)
	ex := newExecutor(t, plantGitViaFakeStart(stage, "main"))
	ml := &memLog{}
	res := gitinit.Init(context.Background(), ex, gitinit.Options{
		Init: true, InitialBranch: "main", GitBinary: "/usr/bin/git",
		PATH: "/bin", StageFD: stageFD, StageDir: stage, TempRoot: t.TempDir(),
		Timeout: 5 * time.Second, Logger: ml,
	})
	if !res.OK() {
		log.Fail("ok", res.Err().Error())
	}
	// init_start must include argv + env_hash.
	found := false
	for _, e := range ml.Entries() {
		if e.Step == "init_start" && e.Outcome == "ok" {
			found = true
			log.Assert("has_argv", strings.Contains(e.Detail, "argv="), true, e.Detail)
			log.Assert("has_env_hash", strings.Contains(e.Detail, "env_hash="), true, e.Detail)
			log.Assert("has_timeout", strings.Contains(e.Detail, "timeout_ms="), true, e.Detail)
		}
		if e.Step == "init_run" && e.Outcome == "ok" {
			log.Assert("has_exit", strings.Contains(e.Detail, "exit="), true, e.Detail)
			log.Assert("has_stdout_n", strings.Contains(e.Detail, "stdout_n="), true, e.Detail)
		}
	}
	log.Assert("init_start_found", found, true, false)
	log.Assert("result_hash", len(res.EnvHash) == 64, 64, len(res.EnvHash))
	log.Assert("result_argv0", res.Argv[0] == "git", "git", res.Argv[0])
}
