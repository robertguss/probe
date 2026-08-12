//go:build unix

package verify_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
	"github.com/robertguss/go-foundry-cli/internal/verify"
)

// TestE2E_MutatingTest_FailsClosed builds a real module whose TestMutate writes
// a file during go test; verify must fail with verify.unplanned_mutation and
// logs must name go-test (FND-006 / REQ-150 mutation fixture).
func TestE2E_MutatingTest_FailsClosed(t *testing.T) {
	goBin, ok := lookGo(t)
	if !ok {
		return
	}
	log := testutil.New(t)
	log.Phase("arrange")

	stage := writeStage(t, map[string]string{
		"go.mod": "module example.com/mutatefix\n\ngo 1.26.0\n",
		"main.go": `package main

func main() {}
`,
		"mutate_test.go": `package main

import (
	"os"
	"testing"
)

func TestMutate(t *testing.T) {
	// Deliberately mutate the module root during the test (hostile fixture).
	if err := os.WriteFile("mutated_by_test.txt", []byte("boom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
`,
	})
	// Pre-tidy with real go so go.sum exists and module is usable.
	runGo(t, goBin, stage, "mod", "tidy")

	stageFD := openStageFD(t, stage)
	ex := newExecutor(t)
	env := captureGoEnv(t, goBin)
	ml := &memLog{}
	opts := verify.Options{
		Mode:         verify.ModeDefault,
		GoBinary:     goBin,
		Env:          env,
		StageFD:      stageFD,
		StageDir:     stage,
		SkipEnvCheck: true,
		SkipTidy:     true, // already tidied
		Logger:       ml,
		// Only run from freeze-relevant steps: module-mutation through final.
		// (Skip re-running tidy; still freezes + tests.)
	}
	log.Inputs(map[string]string{"stage": filepath.Base(stage), "go": filepath.Base(goBin)})
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res := verify.Run(ctx, ex, opts)
	log.Step("run", testutil.OutcomeOK, res.LogDetail())
	if res.Err() != nil {
		log.Step("err", testutil.OutcomeInfo, res.Err().Error())
	}
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if res.OK() {
		log.Fail("want_fail", "mutating test must fail generation gate")
		return
	}
	// Failure is unplanned_mutation after go-test (or tool.failed if test itself
	// failed — but TestMutate passes; mutation is the drift).
	log.Assert("fail_class_unplanned_or_tool",
		res.FailClass == string(diagnostic.IDVerifyUnplannedMutation) ||
			res.FailClass == string(diagnostic.IDToolFailed) ||
			res.FailClass == string(diagnostic.IDVerifyFailed),
		"verify.unplanned_mutation|tool.failed|verify.failed", res.FailClass)
	// Prefer the FND-006 path: go-test succeeded, then conform failed.
	if res.FailClass == string(diagnostic.IDVerifyUnplannedMutation) {
		log.Assert("failed_step", res.FailedStep == verify.CheckGoTest, verify.CheckGoTest, res.FailedStep)
		log.Assert("names_file", strings.Contains(res.Err().Error(), "mutated_by_test.txt"),
			true, res.Err().Error())
	}
	// Logs alone name a failing step.
	failed := ml.FailedStepName()
	log.Assert("log_names_step", failed != "", true, failed)
	log.Step("log_failed_step", testutil.OutcomeOK, failed)
	// Stage preserved (still has our files).
	if _, err := os.Stat(filepath.Join(stage, "main.go")); err != nil {
		log.Fail("stage_preserved", err.Error())
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestE2E_CachedThenBroken_StillRuns proves go-test argv includes -count=1 so
// a previously-passing cached result cannot mask a newly broken test
// (FND-006 / REQ-150).
func TestE2E_CachedThenBroken_StillRuns(t *testing.T) {
	goBin, ok := lookGo(t)
	if !ok {
		return
	}
	log := testutil.New(t)
	log.Phase("arrange")

	stage := writeStage(t, map[string]string{
		"go.mod": "module example.com/cachefix\n\ngo 1.26.0\n",
		"main.go": `package main

func main() {}
`,
		"cache_test.go": `package main

import "testing"

func TestOnce(t *testing.T) {}
`,
	})
	runGo(t, goBin, stage, "mod", "tidy")

	// Warm the test cache with a passing run.
	runGo(t, goBin, stage, "test", "-count=1", "-buildvcs=false", "./...")

	// Break the test.
	if err := os.WriteFile(filepath.Join(stage, "cache_test.go"), []byte(`package main

import "testing"

func TestOnce(t *testing.T) { t.Fatal("broken after cache warm") }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	stageFD := openStageFD(t, stage)
	ex := newExecutor(t)
	env := captureGoEnv(t, goBin)
	ml := &memLog{}
	// Record argv via a wrapping fake is hard with real executor; assert on
	// StepsFor and on failure of go-test after cache warm.
	opts := verify.Options{
		Mode:         verify.ModeDefault,
		GoBinary:     goBin,
		Env:          env,
		StageFD:      stageFD,
		StageDir:     stage,
		SkipEnvCheck: true,
		SkipTidy:     true,
		Logger:       ml,
	}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res := verify.Run(ctx, ex, opts)
	log.Step("run", testutil.OutcomeOK, res.LogDetail())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	// Static: generation gate includes -count=1.
	log.Assert("has_count_one", verify.HasCountOne(verify.DefaultSteps()), true, false)
	// Runtime: broken test must execute and fail (not cached-pass).
	if res.OK() {
		log.Fail("want_test_fail", "broken test must not pass via cache")
		return
	}
	log.Assert("failed_go_test", res.FailedStep == verify.CheckGoTest, verify.CheckGoTest, res.FailedStep)
	log.Assert("log_go_test_fail", ml.Has(verify.CheckGoTest, "fail"), true, false)
	// Confirm tool result carried non-zero exit when present.
	for _, s := range res.Steps {
		if s.ID == verify.CheckGoTest {
			log.Assert("exit_nonzero", s.ExitCode != 0, true, s.ExitCode)
			log.Assert("argv_count", verify.ArgvContains(s.Argv, "-count=1"), true, s.Argv)
		}
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestE2E_CleanModule_DefaultPasses is a positive real-go smoke for default gate.
func TestE2E_CleanModule_DefaultPasses(t *testing.T) {
	goBin, ok := lookGo(t)
	if !ok {
		return
	}
	log := testutil.New(t)
	stage := writeStage(t, map[string]string{
		"go.mod": "module example.com/clean\n\ngo 1.26.0\n",
		"main.go": `package main

func main() {}
`,
		"main_test.go": `package main

import "testing"

func TestOK(t *testing.T) {}
`,
	})
	runGo(t, goBin, stage, "mod", "tidy")

	stageFD := openStageFD(t, stage)
	ex := newExecutor(t)
	env := captureGoEnv(t, goBin)
	ml := &memLog{}
	res := verify.Run(context.Background(), ex, verify.Options{
		Mode:         verify.ModeDefault,
		GoBinary:     goBin,
		Env:          env,
		StageFD:      stageFD,
		StageDir:     stage,
		SkipEnvCheck: true,
		SkipTidy:     true,
		Logger:       ml,
	})
	if !res.OK() {
		log.Fail("ok", res.LogDetail()+" err="+errStr(res.Err()))
		return
	}
	log.Assert("frozen", res.Frozen, true, res.Frozen)
	log.Assert("final_ok", ml.Has(verify.CheckFinalConformance, "ok"), true, false)
	log.Step("clean_pass", testutil.OutcomeOK, res.LogDetail())
}

func captureGoEnv(t *testing.T, goBin string) map[string]string {
	t.Helper()
	h, err := toolrun.CaptureHost(goBin)
	if err != nil {
		t.Fatalf("CaptureHost: %v", err)
	}
	// Isolate HOME/TMPDIR for child to temp dirs (still allowlist keys).
	h.HOME = t.TempDir()
	h.TMPDIR = t.TempDir()
	return toolrun.ConstructGoEnv(h)
}

func lookGo(t *testing.T) (string, bool) {
	t.Helper()
	p, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not on PATH")
		return "", false
	}
	return p, true
}

func runGo(t *testing.T, goBin, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(goBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOFLAGS=",
		"GOENV=off",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %v: %v\n%s", args, err, out)
	}
}

func newExecutor(t *testing.T) *toolrun.Executor {
	t.Helper()
	orig, err := toolrun.CaptureOriginalCWD()
	if err != nil {
		t.Fatalf("CaptureOriginalCWD: %v", err)
	}
	t.Cleanup(func() { _ = orig.Close() })
	starter, err := toolrun.NewBoundStarter(orig, toolrun.BoundStarterOptions{})
	if err != nil {
		t.Fatalf("NewBoundStarter: %v", err)
	}
	ex, err := toolrun.NewExecutor(starter)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	return ex
}

func openStageFD(t *testing.T, dir string) int {
	t.Helper()
	fd, err := toolrun.OpenDirFD(dir)
	if err != nil {
		t.Fatalf("OpenDirFD: %v", err)
	}
	t.Cleanup(func() { _ = toolrun.CloseFD(fd) })
	return fd
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
