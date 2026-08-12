package gitinit_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// FakeGitRunner is a test double for GitRunner. It records every Run call and
// returns a scripted result. Tests can optionally make it plant a minimal
// semantic-valid .git layout so Init's snapshot + semantic validation passes.
//
// Production code must not use FakeGitRunner (test helpers only by convention).
type FakeGitRunner struct {
	mu sync.Mutex

	// Calls records every StepRequest passed to Run.
	Calls []toolrun.StepRequest

	// Result is returned when ResultFn is nil.
	Result toolrun.StepResult

	// ResultFn, when set, computes the result for each call.
	ResultFn func(toolrun.StepRequest) toolrun.StepResult

	// PlantGitDir, when non-empty, causes Run to create a minimal valid .git
	// layout in that directory before returning. The initial branch is parsed
	// from --initial-branch= in the request args; it defaults to "main".
	PlantGitDir string

	// OnRun runs (while holding the call lock) just before returning the result.
	// Use it to simulate side effects such as mutating a non-.git stage file.
	OnRun func(toolrun.StepRequest)
}

// Run implements GitRunner.
func (f *FakeGitRunner) Run(_ context.Context, req toolrun.StepRequest) toolrun.StepResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	cp := req
	cp.Args = append([]string(nil), req.Args...)
	if req.Env != nil {
		cp.Env = make(map[string]string, len(req.Env))
		for k, v := range req.Env {
			cp.Env[k] = v
		}
	}
	f.Calls = append(f.Calls, cp)

	if f.PlantGitDir != "" {
		branch := "main"
		for _, a := range req.Args {
			if strings.HasPrefix(a, "--initial-branch=") {
				branch = strings.TrimPrefix(a, "--initial-branch=")
			}
		}
		PlantMinimalGitDir(f.PlantGitDir, branch)
	}

	if f.OnRun != nil {
		f.OnRun(req)
	}

	if f.ResultFn != nil {
		return f.ResultFn(req)
	}
	return f.Result
}

// CallCount returns the number of recorded calls.
func (f *FakeGitRunner) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}

// Last returns the most recent recorded request, or a zero value if none.
func (f *FakeGitRunner) Last() toolrun.StepRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.Calls) == 0 {
		return toolrun.StepRequest{}
	}
	return f.Calls[len(f.Calls)-1]
}

// ObservedArgv returns the full argv (binary basename + args) for the call at
// index idx, or nil if out of range.
func (f *FakeGitRunner) ObservedArgv(idx int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if idx < 0 || idx >= len(f.Calls) {
		return nil
	}
	req := f.Calls[idx]
	return append([]string{filepath.Base(req.Binary)}, req.Args...)
}

// PlantMinimalGitDir creates the smallest .git layout that
// ValidateSemanticGit accepts. It is exported for test helpers and fakes.
func PlantMinimalGitDir(dir, branch string) error {
	if branch == "" {
		branch = "main"
	}
	git := filepath.Join(dir, ".git")
	for _, d := range []string{
		filepath.Join(git, "objects", "info"),
		filepath.Join(git, "objects", "pack"),
		filepath.Join(git, "refs", "heads"),
		filepath.Join(git, "refs", "tags"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(git, "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(git, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644)
}
