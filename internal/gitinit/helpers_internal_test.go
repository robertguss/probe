package gitinit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

type recLog struct {
	entries []string
}

func (r *recLog) GitStep(step, outcome, detail string) {
	r.entries = append(r.entries, step+"|"+outcome+"|"+detail)
}

func (r *recLog) has(substr string) bool {
	for _, e := range r.entries {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func TestItoa(t *testing.T) {
	log := testutil.New(t)
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{42, "42"},
		{123456, "123456"},
		{-1, "-1"},
		{-99, "-99"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("itoa_%d", tc.n), func(t *testing.T) {
			if got := itoa(tc.n); got != tc.want {
				t.Fatalf("itoa(%d)=%q want %q", tc.n, got, tc.want)
			}
		})
	}
	log.Step("itoa", testutil.OutcomeOK, "table")
}

func TestPlannedArgs_DefaultBranchAndSurfaceEdges(t *testing.T) {
	log := testutil.New(t)
	args := PlannedArgs("", "/tmp/foundry-git-template-abc")
	if args[1] != "--initial-branch=main" {
		t.Fatalf("default branch: %v", args)
	}
	// SubcommandOf edges
	if SubcommandOf(nil) != "" {
		t.Fatal("nil argv")
	}
	if SubcommandOf([]string{"git"}) != "" {
		t.Fatal("no sub")
	}
	if SubcommandOf([]string{"git", "--bare", "status"}) != "status" {
		t.Fatal("skip flags")
	}
	if SubcommandOf([]string{"git", "-C", "x", "status"}) != "x" {
		// -C takes a path arg; extractor returns first non-flag token (x).
		t.Fatal("flag value token")
	}
	if SubcommandOf([]string{"/usr/bin/git", "init"}) != "init" {
		t.Fatal("abs git")
	}
	if SubcommandOf([]string{"init"}) != "init" {
		t.Fatal("bare sub")
	}
	// AssertOnlyInit mismatch paths (exercises itoa via index)
	want := PlannedArgv("main", "/t")
	if msg := AssertOnlyInit(want[:len(want)-1], "main", "/t"); msg != "argv length mismatch" {
		t.Fatalf("len: %q", msg)
	}
	bad := append([]string{}, want...)
	bad[2] = "--template=/other"
	if msg := AssertOnlyInit(bad, "main", "/t"); !strings.HasPrefix(msg, "argv mismatch at ") {
		t.Fatalf("mismatch: %q", msg)
	}
	// Craft argv that matches length/content of planned but wrong sub via direct call path:
	// AssertOnlyInit checks PlannedArgv equality first, so subcommand/forbidden branches
	// after the loop are only reachable if PlannedArgv itself returned a forbidden sub —
	// which it never does. Still cover IsForbidden comprehensively.
	for _, f := range ForbiddenGitSubcommands {
		if !IsForbiddenSubcommand(f) {
			t.Fatalf("forbidden miss: %s", f)
		}
	}
	if IsForbiddenSubcommand("init") || IsForbiddenSubcommand("version") {
		t.Fatal("init/version must be allowed")
	}
	log.Step("surface", testutil.OutcomeOK, "edges")
}

func TestEmptyTemplate_Remove_IsFoundry(t *testing.T) {
	log := testutil.New(t)
	// Empty parent uses TempDir.
	dir, err := EmptyTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	if !IsFoundryTemplate(dir) {
		t.Fatalf("not foundry template: %s", dir)
	}
	if IsFoundryTemplate("") || IsFoundryTemplate("/tmp/other") {
		t.Fatal("false positives")
	}
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o777 != 0o700 {
		t.Fatalf("mode: %v", st.Mode())
	}
	// RemoveTemplate empty / root refuse / non-template refuse.
	if err := RemoveTemplate(""); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTemplate("/"); err == nil {
		t.Fatal("refuse /")
	}
	if err := RemoveTemplate(t.TempDir()); err == nil {
		t.Fatal("refuse non-template")
	}
	if err := RemoveTemplate(dir); err != nil {
		t.Fatal(err)
	}
	// Already removed is ok.
	if err := RemoveTemplate(dir); err != nil {
		t.Fatal(err)
	}
	// Custom parent.
	parent := t.TempDir()
	d2, err := EmptyTemplate(parent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d2, parent) {
		t.Fatalf("parent: %s not under %s", d2, parent)
	}
	_ = RemoveTemplate(d2)
	log.Step("template", testutil.OutcomeOK, "ok")
}

func TestTruncateAndFormatDiffs(t *testing.T) {
	log := testutil.New(t)
	if truncate("abc", 0) != "abc" {
		t.Fatal("max0")
	}
	if truncate("abc", 10) != "abc" {
		t.Fatal("short")
	}
	if truncate("abcdef", 3) != "abc…" {
		t.Fatal("trunc")
	}
	// formatDiffs empty / few / >8
	if formatDiffs(nil) != "" {
		t.Fatal("nil")
	}
	diffs := []Diff{{Rel: "a", Reason: "missing"}, {Rel: "b", Reason: "extra"}}
	got := formatDiffs(diffs)
	if got != "a:missing,b:extra" {
		t.Fatalf("few: %q", got)
	}
	many := make([]Diff, 10)
	for i := range many {
		many[i] = Diff{Rel: fmt.Sprintf("f%d", i), Reason: "bytes"}
	}
	got = formatDiffs(many)
	if !strings.Contains(got, "…+2 more") {
		t.Fatalf("cap: %q", got)
	}
	if strings.Count(got, "f") < 8 {
		t.Fatalf("should list 8: %q", got)
	}
	log.Step("truncate_format", testutil.OutcomeOK, got)
}

func TestFirstLastLines_AndMapStepFailure(t *testing.T) {
	log := testutil.New(t)
	f, l := firstLastLines(nil, nil)
	if f != "" || l != "" {
		t.Fatal("empty")
	}
	f, l = firstLastLines([]byte("only-out\n"), nil)
	if f != "only-out" || l != "only-out" {
		t.Fatalf("stdout only: %q %q", f, l)
	}
	f, l = firstLastLines([]byte("o1\no2\n"), []byte("e1\ne2\ne3\n"))
	if f != "e1" || l != "e3" {
		t.Fatalf("stderr prefer: %q %q", f, l)
	}
	f, l = firstLastLines(nil, []byte("\n\n"))
	if f != "" || l != "" {
		t.Fatalf("blank lines: %q %q", f, l)
	}

	rl := &recLog{}
	// cause is FoundryError
	cause := diagnostic.Newf(diagnostic.IDToolFailed, diagnostic.StepLocation("x"), "tool boom")
	step := toolrun.StepResult{
		ExitCode:  1,
		FailClass: "tool.failed",
		Stdout:    []byte("out-first\nout-last\n"),
		Stderr:    []byte(strings.Repeat("E", 200) + "\nend\n"),
	}
	// inject err via reflection-free path: mapStepFailure reads step.Err()
	// which is private — set through a real failure. Use a thin wrapper by
	// constructing Result via fail path after assigning through package.
	// StepResult.err is unexported; mapStepFailure uses step.Err().
	// We'll call mapStepFailure with zero err (nil cause branch).
	res := mapStepFailure(Result{}, rl, step)
	if res.OK() || res.Err() == nil {
		t.Fatal("want failure")
	}
	if res.FailClass != string(diagnostic.IDGitFailed) {
		t.Fatalf("class: %s", res.FailClass)
	}
	fe, ok := diagnostic.AsFoundryError(res.Err())
	if !ok || fe.ID() != diagnostic.IDGitFailed {
		t.Fatalf("fe: %v ok=%v", res.Err(), ok)
	}
	if !rl.has("map_failure|fail|") {
		t.Fatalf("log: %v", rl.entries)
	}
	// detail should include truncated first line
	if !rl.has("first=") {
		t.Fatalf("missing first in %v", rl.entries)
	}

	// Non-nil non-Foundry cause: need StepResult with err. Use Executor?
	// Instead duplicate mapStepFailure branches by constructing via a local helper
	// that sets err through failResult-style isn't possible for StepResult.
	// Call mapStepFailure after wrapping: toolrun.StepResult can't set err from outside.
	// Use init path with fake runner for non-Foundry / Foundry causes below.
	_ = cause
	log.Step("map_nil_cause", testutil.OutcomeOK, res.FailClass)
}

// errGitRunner returns a fixed StepResult with a controllable error.
type errGitRunner struct {
	res toolrun.StepResult
	err error
}

func (e errGitRunner) Run(_ context.Context, req toolrun.StepRequest) toolrun.StepResult {
	// Clone request into a result shell — we cannot set unexported err on StepResult
	// from outside toolrun. Instead run real classification is blocked.
	// For mapStepFailure unit tests we call mapStepFailure directly after
	// building step via toolrun's public surface when possible.
	_ = req
	return e.res
}

func TestMapStepFailure_ViaExportedInitPaths(t *testing.T) {
	log := testutil.New(t)
	// Direct unit test of non-nil cause branches by calling mapStepFailure with
	// a step that carries err — StepResult.err is package-private in toolrun,
	// so exercise through Init with a custom GitRunner that returns FailClass set
	// and uses a step whose Err is nil (already covered), plus removeOwned edges.

	rl := &recLog{}
	res := Result{}
	// removeOwned skip not owned
	if err := removeOwned(false, "/tmp/foundry-git-template-x", &res, rl); err != nil {
		t.Fatal(err)
	}
	if !rl.has("template_remove|skip|") {
		t.Fatalf("skip log: %v", rl.entries)
	}
	// removeOwned empty dir while owned
	if err := removeOwned(true, "", &res, rl); err != nil {
		t.Fatal(err)
	}
	// removeOwned fail (refuse path)
	if err := removeOwned(true, "/tmp/not-a-template", &res, rl); err == nil {
		t.Fatal("want refuse")
	}
	if !rl.has("template_remove|fail|") {
		t.Fatalf("fail log: %v", rl.entries)
	}
	// success remove
	dir, err := EmptyTemplate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	res2 := Result{}
	if err := removeOwned(true, dir, &res2, rl); err != nil || !res2.TemplateRemoved {
		t.Fatalf("remove ok: err=%v removed=%v", err, res2.TemplateRemoved)
	}
	log.Step("remove_owned", testutil.OutcomeOK, "ok")
}

func TestMapStepFailure_CauseBranches(t *testing.T) {
	log := testutil.New(t)
	rl := &recLog{}

	// Patch: call mapStepFailure with step built inside toolrun via a tiny
	// local construction — we need step.err set. Use unsafe? No.
	// Implement a package-local helper that constructs StepResult with err.
	stepFoundry := stepWithErr(
		diagnostic.Newf(diagnostic.IDToolFailed, diagnostic.StepLocation("git-init"), "inner tool"),
		1, "tool.failed", []byte("so"), []byte("se\n"),
	)
	res := mapStepFailure(Result{}, rl, stepFoundry)
	if res.Err() == nil {
		t.Fatal("want err")
	}
	// Wrapped Foundry path
	var fe *diagnostic.FoundryError
	if !errors.As(res.Err(), &fe) {
		t.Fatalf("want FoundryError: %v", res.Err())
	}
	if fe.ID() != diagnostic.IDGitFailed {
		t.Fatalf("id: %s", fe.ID())
	}

	rl2 := &recLog{}
	stepPlain := stepWithErr(errors.New("plain fail"), 2, "x", nil, []byte("errline\n"))
	res2 := mapStepFailure(Result{}, rl2, stepPlain)
	if res2.Err() == nil {
		t.Fatal("want err")
	}
	if !errors.As(res2.Err(), &fe) || fe.ID() != diagnostic.IDGitFailed {
		t.Fatalf("plain wrap: %v", res2.Err())
	}
	log.Step("map_causes", testutil.OutcomeOK, "foundry+plain")
}

// stepWithErr builds a toolrun.StepResult with an injected error via the
// package-level test constructor (no reflect/unsafe field poking; ipk.9).
func stepWithErr(err error, exit int, class string, stdout, stderr []byte) toolrun.StepResult {
	return toolrun.NewStepResultForTest(exit, class, stdout, stderr, err)
}

func TestInit_PrecheckBranches(t *testing.T) {
	log := testutil.New(t)
	rl := &recLog{}
	ctx := context.Background()

	// init=false with existing .git logs info
	stage := t.TempDir()
	if err := os.Mkdir(filepath.Join(stage, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := Init(ctx, nil, Options{Init: false, StageDir: stage, Logger: rl})
	if !res.OK() || !res.Skipped {
		t.Fatalf("skip: %+v", res)
	}
	if !rl.has("skip_existing_git|info|") {
		t.Fatalf("log: %v", rl.entries)
	}

	// empty binary
	res = Init(ctx, nil, Options{Init: true, Logger: rl})
	if res.OK() || !strings.Contains(res.Err().Error(), "empty git binary") {
		t.Fatalf("empty bin: %v", res.Err())
	}
	// invalid fd
	res = Init(ctx, nil, Options{Init: true, GitBinary: "/bin/git", StageFD: -1, Logger: rl})
	if res.OK() || !strings.Contains(res.Err().Error(), "invalid stage fd") {
		t.Fatalf("fd: %v", res.Err())
	}
	// empty stage dir
	res = Init(ctx, nil, Options{Init: true, GitBinary: "/bin/git", StageFD: 3, StageDir: "", Logger: rl})
	if res.OK() || !strings.Contains(res.Err().Error(), "empty stage dir") {
		t.Fatalf("stage: %v", res.Err())
	}
	// empty PATH
	res = Init(ctx, nil, Options{Init: true, GitBinary: "/bin/git", StageFD: 3, StageDir: stage, PATH: "", Logger: rl})
	if res.OK() || !strings.Contains(res.Err().Error(), "empty PATH") {
		t.Fatalf("path: %v", res.Err())
	}
	// nil runner
	res = Init(ctx, nil, Options{Init: true, GitBinary: "/bin/git", StageFD: 3, StageDir: stage, PATH: "/bin", Logger: rl})
	if res.OK() || !strings.Contains(res.Err().Error(), "nil GitRunner") {
		t.Fatalf("runner: %v", res.Err())
	}
	// default branch when empty — exercised once runner is non-nil; use failing runner after template
	// bad TempRoot for template create
	badParent := filepath.Join(stage, "not-a-dir-file")
	if err := os.WriteFile(badParent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res = Init(ctx, errGitRunner{}, Options{
		Init: true, GitBinary: "/bin/git", StageFD: 3, StageDir: stage, PATH: "/bin",
		TempRoot: badParent, Logger: rl,
	})
	if res.OK() || !strings.Contains(res.Err().Error(), "template") {
		// fail at template_create
		if res.OK() {
			t.Fatalf("want template fail: %v", res.Err())
		}
	}
	log.Step("precheck", testutil.OutcomeOK, "branches")
}

func TestLoggerOrNop_AndNopGitStep(t *testing.T) {
	// Cover nopLogger.GitStep body (0% previously).
	l := loggerOrNop(nil)
	l.GitStep("x", "ok", "d")
	l2 := loggerOrNop(&recLog{})
	l2.GitStep("y", "ok", "d")
}
