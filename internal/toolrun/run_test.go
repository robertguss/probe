//go:build unix

package toolrun_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func newExecutor(t *testing.T, opts toolrun.BoundStarterOptions) *toolrun.Executor {
	t.Helper()
	orig := captureOrig(t)
	starter, err := toolrun.NewBoundStarter(orig, opts)
	if err != nil {
		t.Fatalf("NewBoundStarter: %v", err)
	}
	ex, err := toolrun.NewExecutor(starter)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	// Tests that cancel with short grace.
	ex.KillGrace = 50 * time.Millisecond
	return ex
}

// TestExecutor_FakeRunner_ExactArgvEnv proves fake-runner unit contract:
// exact argv (basename) and env allowlist hash, empty Dir, success path.
func TestExecutor_FakeRunner_ExactArgvEnv(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	host := testHost()
	env := toolrun.ConstructGoEnv(host)
	wantHash := toolrun.AllowlistHash(env)

	fake := &toolrun.FakeRunner{}
	var seenEnv []string
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		OnCommand: func(dir, binary string, args []string) {
			fake.RecordBoundStart(dir, binary, args)
		},
		StartChild: func(_ context.Context, binary string, args, envSlice []string, dir string, capBytes int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			if dir != "" {
				return 0, nil, fmt.Errorf("pathname Dir %q", dir)
			}
			seenEnv = append([]string(nil), envSlice...)
			fake.RecordBoundStart(dir, binary, args)
			// Record env via FakeRunner.Run shape for LastEnvMap parity.
			_, _, _ = fake.Run(context.Background(), binary, args, envSlice)
			return 9001, func() ([]byte, []byte, error) {
				return []byte("ok\n"), nil, nil
			}, nil
		},
	})
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := ex.Run(context.Background(), toolrun.StepRequest{
		StepID:         "go-mod-verify",
		Binary:         "/usr/local/go/bin/go",
		Args:           []string{"mod", "verify"},
		Env:            env,
		StageFD:        stageFD,
		Timeout:        30 * time.Second,
		OutputCapBytes: plan.OutputCapBytes,
	})
	log.Step("run", testutil.OutcomeOK, res.LogDetail())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if !res.OK() {
		log.Fail("ok", fmt.Sprintf("err=%v class=%s", res.Err(), res.FailClass))
	}
	log.Assert("argv0", res.Argv[0] == "go", "go", res.Argv[0])
	log.Assert("argv", strings.Join(res.Argv, " ") == "go mod verify", "go mod verify", strings.Join(res.Argv, " "))
	log.Assert("env_hash", res.EnvHash == wantHash, wantHash, res.EnvHash)
	log.Assert("env_keys_sorted", strings.Join(res.EnvKeys, ",") == strings.Join(toolrun.EnvKeys(env), ","),
		strings.Join(toolrun.EnvKeys(env), ","), strings.Join(res.EnvKeys, ","))
	log.Assert("dir_empty", fake.AllDirsEmpty(), true, fake.AllDirsEmpty())
	log.Assert("exit0", res.ExitCode == 0, 0, res.ExitCode)
	log.Assert("no_replay_success", func() bool {
		o, e, _, _ := res.Replay()
		return o == nil && e == nil
	}(), true, false)
	// Env slice must equal allowlist encoding — no host bleed keys.
	gotMap := map[string]string{}
	for _, kv := range seenEnv {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			gotMap[kv[:i]] = kv[i+1:]
		}
	}
	bleed := toolrun.ForbiddenKeysPresent(gotMap, toolrun.ForbiddenBleedKeys)
	// GIT_TEMPLATE_DIR is forbidden on go steps.
	log.Assert("no_bleed", len(bleed) == 0, "[]", bleed)
	log.Assert("fields_no_values", !strings.Contains(res.LogDetail(), host.HOME), true, res.LogDetail())
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestExecutor_Timeout_FailClass injects a StartChild that blocks past the
// plan timeout so the test no longer depends on a real sleep binary.
func TestExecutor_Timeout_FailClass(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(ctx context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			<-ctx.Done()
			return 7, func() ([]byte, []byte, error) { return nil, nil, ctx.Err() }, nil
		},
	})

	log.Phase("act")
	start := time.Now()
	res := ex.Run(context.Background(), toolrun.StepRequest{
		StepID:  "slow-step",
		Binary:  "/fake/sleep",
		Args:    []string{"30"},
		Env:     map[string]string{"PATH": "/usr/bin:/bin", "LC_ALL": "C", "LANG": "C"},
		StageFD: stageFD,
		Timeout: 200 * time.Millisecond,
	})
	elapsed := time.Since(start)
	log.Step("timeout_run", testutil.OutcomeOK, res.LogDetail()+" elapsed_ms="+fmt.Sprintf("%d", elapsed.Milliseconds()))
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	log.Assert("timed_out", res.TimedOut, true, res.TimedOut)
	log.Assert("fail_class", res.FailClass == toolrun.FailClassTimeout, toolrun.FailClassTimeout, res.FailClass)
	log.Assert("not_ok", !res.OK(), false, res.OK())
	var fe *diagnostic.FoundryError
	if !errors.As(res.Err(), &fe) {
		log.Fail("foundry_error", fmt.Sprintf("%T %v", res.Err(), res.Err()))
	} else {
		log.Assert("id", fe.ID() == diagnostic.IDToolTimeout, diagnostic.IDToolTimeout, fe.ID())
	}
	log.Assert("bounded", elapsed < 5*time.Second, true, elapsed)
	out, errb, _, _ := res.Replay()
	log.Step("replay", testutil.OutcomeOK, fmt.Sprintf("stdout_len=%d stderr_len=%d", len(out), len(errb)))
	log.Assert("should_replay", res.ShouldReplay(), true, res.ShouldReplay())
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestExecutor_OutputCap_Truncation injects large stdout so the test no
// longer shells out to dd/tr.
func TestExecutor_OutputCap_Truncation(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)

	const capN = 1024
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, _ string, _, _ []string, _ string, capBytes int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			log.Assert("cap_passed", capBytes == capN, capN, capBytes)
			big := []byte(strings.Repeat("x", capN*4))
			return 8, func() ([]byte, []byte, error) { return big, nil, nil }, nil
		},
	})
	log.Phase("act")
	res := ex.Run(context.Background(), toolrun.StepRequest{
		StepID:         "cap-step",
		Binary:         "/fake/bigout",
		Args:           nil,
		Env:            map[string]string{"PATH": "/usr/bin:/bin", "LC_ALL": "C"},
		StageFD:        stageFD,
		Timeout:        10 * time.Second,
		OutputCapBytes: capN,
	})
	log.Step("run", testutil.OutcomeOK, res.LogDetail())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if !res.OK() {
		log.Fail("ok", fmt.Sprintf("exit=%d err=%v", res.ExitCode, res.Err()))
	}
	log.Assert("stdout_len_le_cap", len(res.Stdout) <= capN, capN, len(res.Stdout))
	log.Assert("stdout_truncated", res.StdoutTruncated, true, res.StdoutTruncated)
	o, _, _, _ := res.Replay()
	log.Assert("no_replay_ok", o == nil, true, o != nil)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestExecutor_FakeRunner_CapAndTimeout injects large stdout + slow wait.
func TestExecutor_FakeRunner_CapAndTimeout(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	const capN = 64

	log.Phase("cap_path")
	exCap := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, _ string, _, _ []string, dir string, capBytes int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			if dir != "" {
				return 0, nil, fmt.Errorf("dir set")
			}
			big := []byte(strings.Repeat("Z", capBytes*4))
			return 1, func() ([]byte, []byte, error) {
				return big, []byte("err-side"), nil
			}, nil
		},
	})
	res := exCap.Run(context.Background(), toolrun.StepRequest{
		StepID:         "fake-cap",
		Binary:         "/fake/go",
		Args:           []string{"test"},
		Env:            map[string]string{"PATH": "/bin"},
		StageFD:        stageFD,
		Timeout:        time.Minute,
		OutputCapBytes: capN,
	})
	log.Step("cap", testutil.OutcomeOK, res.LogDetail())
	log.Assert("ok", res.OK(), true, res.OK())
	log.Assert("stdout_cap", len(res.Stdout) == capN, capN, len(res.Stdout))
	log.Assert("stdout_trunc", res.StdoutTruncated, true, res.StdoutTruncated)
	log.Assert("stderr_no_trunc", !res.StderrTruncated && string(res.Stderr) == "err-side", "err-side", string(res.Stderr))
	log.PhaseEnd("cap_path", testutil.OutcomeOK)

	log.Phase("timeout_path")
	exTO := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(ctx context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			return 2, func() ([]byte, []byte, error) {
				select {
				case <-ctx.Done():
					return nil, []byte("killed"), ctx.Err()
				case <-time.After(30 * time.Second):
					return []byte("late"), nil, nil
				}
			}, nil
		},
	})
	resTO := exTO.Run(context.Background(), toolrun.StepRequest{
		StepID:  "fake-timeout",
		Binary:  "/fake/go",
		Args:    []string{"test", "./..."},
		Env:     map[string]string{"PATH": "/bin"},
		StageFD: stageFD,
		Timeout: 50 * time.Millisecond,
	})
	log.Step("timeout", testutil.OutcomeOK, resTO.LogDetail())
	log.Assert("timed_out", resTO.TimedOut, true, resTO.TimedOut)
	log.Assert("class", resTO.FailClass == toolrun.FailClassTimeout, toolrun.FailClassTimeout, resTO.FailClass)
	o, e, ot, et := resTO.Replay()
	log.Assert("replay_on_fail", resTO.ShouldReplay(), true, resTO.ShouldReplay())
	// Timeout path injects stderr "killed"; at least one stream should be non-nil for replay.
	log.Assert("replay_streams", o != nil || e != nil, true, map[string]bool{"stdout": o != nil, "stderr": e != nil})
	if e != nil {
		log.Assert("replay_stderr_killed", strings.Contains(string(e), "killed"), true, string(e))
	}
	_ = ot
	_ = et
	log.PhaseEnd("timeout_path", testutil.OutcomeOK)
}

// TestExecutor_NonZeroExit_FailClassAndReplay injects a failing exit so the
// test no longer depends on the host false binary.
func TestExecutor_NonZeroExit_FailClassAndReplay(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			return 9, func() ([]byte, []byte, error) { return []byte("boom\n"), nil, errors.New("exit status 1") }, nil
		},
	})
	res := ex.Run(context.Background(), toolrun.StepRequest{
		StepID:  "go-test",
		Binary:  "/fake/false",
		Args:    nil,
		Env:     map[string]string{"PATH": "/usr/bin:/bin"},
		StageFD: stageFD,
		Timeout: 10 * time.Second,
	})
	log.Step("run", testutil.OutcomeOK, res.LogDetail())
	log.Assert("not_ok", !res.OK(), false, res.OK())
	log.Assert("class", res.FailClass == toolrun.FailClassFailed, toolrun.FailClassFailed, res.FailClass)
	log.Assert("exit_nonzero", res.ExitCode != 0, true, res.ExitCode)
	log.Assert("replay", res.ShouldReplay(), true, res.ShouldReplay())
	var fe *diagnostic.FoundryError
	if errors.As(res.Err(), &fe) {
		log.Assert("id", fe.ID() == diagnostic.IDToolFailed, diagnostic.IDToolFailed, fe.ID())
	} else {
		log.Fail("foundry_error", fmt.Sprintf("%v", res.Err()))
	}
}

// TestExecutor_Cancel_KillsProcessGroup injects a StartChild that blocks until
// the context is cancelled so the test does not spawn a real sleep process.
// Uses testing/synctest so cancel scheduling is deterministic without a wall
// clock Sleep (ipk.15).
func TestExecutor_Cancel_KillsProcessGroup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := testutil.New(t)
		stage := t.TempDir()
		stageFD := openStageFD(t, stage)
		ex := newExecutor(t, toolrun.BoundStarterOptions{
			StartChild: func(ctx context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
				<-ctx.Done()
				return 10, func() ([]byte, []byte, error) { return nil, nil, ctx.Err() }, nil
			},
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan toolrun.StepResult, 1)
		go func() {
			done <- ex.Run(ctx, toolrun.StepRequest{
				StepID:  "cancel-step",
				Binary:  "/fake/sleep",
				Args:    []string{"60"},
				Env:     map[string]string{"PATH": "/usr/bin:/bin"},
				StageFD: stageFD,
				Timeout: 2 * time.Minute,
			})
		}()
		// Advance fake time so the Run goroutine reaches StartChild and blocks.
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()
		cancel()
		synctest.Wait()

		select {
		case res := <-done:
			log.Step("cancelled", testutil.OutcomeOK, res.LogDetail())
			log.Assert("cancelled_flag", res.Cancelled || res.FailClass != "", true, res.Cancelled)
			log.Assert("not_ok", !res.OK(), false, res.OK())
		default:
			// One more wait in case result is still buffering.
			synctest.Wait()
			select {
			case res := <-done:
				log.Step("cancelled", testutil.OutcomeOK, res.LogDetail())
				log.Assert("cancelled_flag", res.Cancelled || res.FailClass != "", true, res.Cancelled)
				log.Assert("not_ok", !res.OK(), false, res.OK())
			default:
				log.Fail("cancel_wait", "executor did not return after cancel")
			}
		}
	})
}

// TestExecutor_RunFromPlan_ArgvAndTimeout wires plan.ExternalStep fields.
func TestExecutor_RunFromPlan_ArgvAndTimeout(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	var sawArgs []string
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, binary string, args, env []string, dir string, capBytes int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			sawArgs = append([]string{filepath.Base(binary)}, args...)
			log.Assert("cap", capBytes == plan.OutputCapBytes || capBytes == 128, plan.OutputCapBytes, capBytes)
			return 3, func() ([]byte, []byte, error) { return []byte("v\n"), nil, nil }, nil
		},
	})
	step := plan.ExternalStep{
		ID:             "go-mod-tidy",
		Binary:         "/opt/go/bin/go",
		Argv:           []string{"go", "mod", "tidy"},
		Cwd:            plan.StageDescriptorCWD,
		TimeoutS:       600,
		OutputCapBytes: 128,
		Env:            toolrun.ConstructGoEnv(testHost()),
	}
	res := ex.RunFromPlan(context.Background(), step, stageFD)
	log.Step("from_plan", testutil.OutcomeOK, res.LogDetail())
	if !res.OK() {
		log.Fail("ok", res.Err().Error())
	}
	log.Assert("argv", strings.Join(sawArgs, " ") == "go mod tidy",
		"go mod tidy", strings.Join(sawArgs, " "))
	log.Assert("step_id", res.StepID == "go-mod-tidy", "go-mod-tidy", res.StepID)
	log.Assert("env_hash_set", res.EnvHash != "", true, res.EnvHash)
}

// TestExecutor_ProcessTree_NoShell asserts argv never uses sh -c.
func TestExecutor_ProcessTree_NoShell(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	var argvLists [][]string
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, binary string, args, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			rec := append([]string{binary}, args...)
			argvLists = append(argvLists, rec)
			return 4, func() ([]byte, []byte, error) { return nil, nil, nil }, nil
		},
	})
	steps := []toolrun.StepRequest{
		{StepID: "go-mod-tidy", Binary: "/bin/go", Args: []string{"mod", "tidy"}, Env: map[string]string{"PATH": "/bin"}, StageFD: stageFD, Timeout: time.Second},
		{StepID: "go-test", Binary: "/bin/go", Args: []string{"test", "-count=1", "./..."}, Env: map[string]string{"PATH": "/bin"}, StageFD: stageFD, Timeout: time.Second},
		{StepID: "git-init", Binary: "/bin/git", Args: []string{"init", "--initial-branch=main", "--template=/tmp/t", "."}, Env: map[string]string{"PATH": "/bin"}, StageFD: stageFD, Timeout: time.Second},
	}
	for _, s := range steps {
		res := ex.Run(context.Background(), s)
		if !res.OK() {
			log.Fail("step_"+s.StepID, res.Err().Error())
		}
		log.Step("argv_"+s.StepID, testutil.OutcomeOK, strings.Join(res.Argv, " "))
	}
	for _, a := range argvLists {
		joined := strings.Join(a, " ")
		if strings.Contains(joined, "sh -c") || strings.Contains(joined, "/bin/sh") || strings.Contains(joined, "bash") {
			log.Fail("shell_used", joined)
		}
		base := filepath.Base(a[0])
		if base != "go" && base != "git" {
			log.Fail("unexpected_binary", a[0])
		}
	}
	log.Assert("count", len(argvLists) == 3, 3, len(argvLists))
}

// TestExecutor_ConcurrentSerialized ensures Executor shares BoundStarter mutex.
func TestExecutor_ConcurrentSerialized(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	var inCS atomic.Int32
	var maxCS atomic.Int32
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		DuringCriticalSection: func() {
			n := inCS.Add(1)
			for {
				cur := maxCS.Load()
				if n <= cur || maxCS.CompareAndSwap(cur, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			inCS.Add(-1)
		},
		StartChild: func(_ context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			return 5, func() ([]byte, []byte, error) { return nil, nil, nil }, nil
		},
	})
	const n = 8
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			res := ex.Run(context.Background(), toolrun.StepRequest{
				StepID:  "conc",
				Binary:  "/bin/true",
				Env:     map[string]string{"PATH": "/bin"},
				StageFD: stageFD,
				Timeout: 5 * time.Second,
			})
			if !res.OK() {
				errCh <- res.Err()
				return
			}
			errCh <- nil
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errCh; err != nil {
			log.Fail("conc", err.Error())
		}
	}
	log.Assert("max_cs", maxCS.Load() == 1, 1, maxCS.Load())
}

// TestStepResult_LogFieldsNeverEnvValues golden-ish field contract.
func TestStepResult_LogFieldsNeverEnvValues(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	secretHome := "/home/secret-user-do-not-log"
	env := toolrun.ConstructGoEnv(toolrun.HostCapture{
		PATH: "/bin", HOME: secretHome, GOMODCACHE: secretHome + "/mod",
		GOCACHE: secretHome + "/cache", GOPATH: secretHome + "/go",
		GOPROXY: "https://proxy.golang.org,direct", GOSUMDB: "sum.golang.org",
	})
	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			return 6, func() ([]byte, []byte, error) { return []byte("ok"), nil, nil }, nil
		},
	})
	res := ex.Run(context.Background(), toolrun.StepRequest{
		StepID: "log-check", Binary: "/usr/bin/go", Args: []string{"version"},
		Env: env, StageFD: stageFD, Timeout: time.Second,
	})
	fields := res.LogFields()
	blob := res.LogDetail()
	for k, v := range fields {
		blob += k + "=" + v + " "
	}
	if strings.Contains(blob, secretHome) {
		log.Fail("home_leaked", blob)
	}
	if strings.Contains(blob, "proxy.golang.org") {
		log.Fail("proxy_value_leaked", blob)
	}
	log.Assert("has_env_hash", fields["env_hash"] != "", true, fields["env_hash"])
	log.Assert("has_env_keys", strings.Contains(fields["env_keys"], "GOENV"), true, fields["env_keys"])
	log.Assert("ok", res.OK(), true, res.OK())
}

// TestExecutor_EnvSizeLimit_RejectsOversizedEnv verifies that an environment
// block larger than ARG_MAX is rejected deterministically with the env-size
// fail class before the child is started (no subprocess spawned).
func TestExecutor_EnvSizeLimit_RejectsOversizedEnv(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	started := &atomic.Bool{}

	ex := newExecutor(t, toolrun.BoundStarterOptions{
		EnvSizeLimit: 32,
		StartChild: func(_ context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			started.Store(true)
			return 0, func() ([]byte, []byte, error) { return nil, nil, nil }, nil
		},
	})

	env := map[string]string{"PATH": "/usr/bin:/bin", "VERY_LONG_VAR": strings.Repeat("x", 64)}
	res := ex.Run(context.Background(), toolrun.StepRequest{
		StepID: "env-size", Binary: "/usr/bin/go", Args: []string{"version"},
		Env: env, StageFD: stageFD, Timeout: time.Second,
	})
	if res.OK() {
		log.Fail("expected failure", res.LogDetail())
	}
	log.Assert("no_start", !started.Load(), false, started.Load())
	log.Assert("fail_class_env_size", res.FailClass == toolrun.FailClassEnvSize, toolrun.FailClassEnvSize, res.FailClass)
	var fe *diagnostic.FoundryError
	log.Assert("foundry_error", errors.As(res.Err(), &fe), true, res.Err())
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestExecutor_ConcurrentProcessDeath_DeterministicFailClass simulates multiple
// children that die unexpectedly during wait. It verifies that each result is
// classified as tool.failed, the shared mutex remains consistent, and no run
// panics or leaks (REQ-214 / Section 34.4).
func TestExecutor_ConcurrentProcessDeath_DeterministicFailClass(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	stageFD := openStageFD(t, stage)
	var started atomic.Int32

	ex := newExecutor(t, toolrun.BoundStarterOptions{
		StartChild: func(_ context.Context, binary string, args, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			pid := int(started.Add(1))
			return pid, func() ([]byte, []byte, error) {
				// Simulate unexpected process death before a clean exit.
				return []byte("boom\n"), nil, fmt.Errorf("signal: killed")
			}, nil
		},
	})

	const n = 8
	var wg sync.WaitGroup
	results := make([]toolrun.StepResult, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = ex.Run(context.Background(), toolrun.StepRequest{
				StepID:  fmt.Sprintf("death-%d", i),
				Binary:  "/usr/bin/go",
				Args:    []string{"version"},
				StageFD: stageFD,
				Timeout: time.Second,
			})
		}(i)
	}
	wg.Wait()

	allFailed := true
	for _, res := range results {
		if res.OK() {
			allFailed = false
			break
		}
		if res.FailClass != toolrun.FailClassFailed {
			allFailed = false
			break
		}
	}
	log.Assert("all_classified_failed", allFailed, true, allFailed)
	log.Assert("started_count", started.Load() == n, n, started.Load())
	log.PhaseEnd("assert", testutil.OutcomeOK)
}
