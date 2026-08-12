//go:build unix

package sentinel_test

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// blackHoleListener starts a loopback-only TCP listener that accepts
// connections and never reads/writes/closes them, simulating a
// deliberately slow or unreachable proxy without any real network
// dependency (bead go-foundry-cli-wet.4.3).
//
// Accepted connections are tracked under a mutex and closed only from
// test-scoped Cleanup on the test goroutine — never via t.Cleanup from
// the Accept goroutine (go-foundry-cli-ipk.19).
func blackHoleListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("blackHoleListener: listen: %v", err)
	}
	var (
		mu    sync.Mutex
		conns []net.Conn
	)
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
		conns = nil
	})
	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			// Deliberately never read/write: the peer hangs until the
			// caller's own timeout fires. Close is deferred to test Cleanup.
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	return ln.Addr().String()
}

// sentinelExecutor builds a production *toolrun.Executor bound at dir,
// mirroring internal/toolrun's own test helper via exported APIs only.
func sentinelExecutor(t *testing.T, dir string) (*toolrun.Executor, int) {
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
	ex.KillGrace = 50 * time.Millisecond
	fd, err := toolrun.OpenDirFD(dir)
	if err != nil {
		t.Fatalf("OpenDirFD(%s): %v", dir, err)
	}
	t.Cleanup(func() { _ = toolrun.CloseFD(fd) })
	return ex, fd
}

// TestSentinel_NetworkTimeout_StableClassification exercises the
// production step-execution path (the same toolrun.Executor go/git steps
// use) against a deliberately unreachable local "proxy" and asserts stable,
// deterministic timeout classification plus proxy-key isolation for both
// the go and git constructed environments (REQ-154 / REQ-214).
//
// No real network dependency: the "proxy" is a loopback listener that never
// responds, so the probe subprocess blocks on a TCP read until the
// executor's own plan timeout fires and kills the process group.
func TestSentinel_NetworkTimeout_StableClassification(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH; cannot simulate a hung proxy probe via /dev/tcp")
	}
	addr := blackHoleListener(t)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%s): %v", addr, err)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "proxy-probe.sh")
	body := "#!/usr/bin/env bash\n" +
		"exec 3<>/dev/tcp/" + host + "/" + port + "\n" +
		"read -r _ <&3\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write probe script: %v", err)
	}
	h := testHost(t)
	// Deliberately point the host-effective GOPROXY at the unreachable
	// listener, as if a developer's machine or CI runner had a slow/dead
	// proxy configured.
	h.GOPROXY = "http://" + addr
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("go_env_isolation")
	goEnv := toolrun.ConstructGoEnv(h)
	goBleed := toolrun.ForbiddenKeysPresent(goEnv, toolrun.ForbiddenBleedKeys)
	log.Assert("go_env_no_proxy_bleed", len(goBleed) == 0, "[]", strings0(goBleed))
	log.PhaseEnd("go_env_isolation", testutil.OutcomeOK)

	log.Phase("go_timeout")
	ex, stageFD := sentinelExecutor(t, dir)
	start := time.Now()
	res := ex.Run(context.Background(), toolrun.StepRequest{
		StepID:  "go-network-timeout-sentinel",
		Binary:  bash,
		Args:    []string{script},
		Env:     goEnv,
		StageFD: stageFD,
		Timeout: 300 * time.Millisecond,
	})
	elapsed := time.Since(start)
	log.Step("go_probe", testutil.OutcomeOK, res.LogDetail())
	log.Assert("go_timed_out", res.TimedOut, true, res.TimedOut)
	log.Assert("go_fail_class", res.FailClass == toolrun.FailClassTimeout, toolrun.FailClassTimeout, res.FailClass)
	log.Assert("go_bounded", elapsed < 5*time.Second, true, elapsed)
	var fe *diagnostic.FoundryError
	if !errors.As(res.Err(), &fe) {
		log.Fail("go_foundry_error", res.LogDetail())
	} else {
		log.Assert("go_error_id", fe.ID() == diagnostic.IDToolTimeout, diagnostic.IDToolTimeout, fe.ID())
	}
	log.PhaseEnd("go_timeout", testutil.OutcomeOK)

	log.Phase("git_env_isolation")
	gitEnv := toolrun.ConstructGitEnv(h.PATH, dir)
	// GIT_TEMPLATE_DIR is a legitimate git-step key (forbidden only on go
	// steps), so check against the git allowlist rather than the shared
	// forbidden-bleed set: no proxy key (HTTP_PROXY/HTTPS_PROXY/etc.) or any
	// other undeclared key may appear.
	gitBad := toolrun.KeysOutsideAllowlist(gitEnv, toolrun.GitAllowlistKeys)
	log.Assert("git_env_no_proxy_bleed", len(gitBad) == 0, "[]", strings0(gitBad))
	log.PhaseEnd("git_env_isolation", testutil.OutcomeOK)

	log.Phase("git_timeout")
	res2 := ex.Run(context.Background(), toolrun.StepRequest{
		StepID:  "git-init-network-timeout-sentinel",
		Binary:  bash,
		Args:    []string{script},
		Env:     gitEnv,
		StageFD: stageFD,
		Timeout: 300 * time.Millisecond,
	})
	log.Step("git_probe", testutil.OutcomeOK, res2.LogDetail())
	log.Assert("git_timed_out", res2.TimedOut, true, res2.TimedOut)
	log.Assert("git_fail_class", res2.FailClass == toolrun.FailClassTimeout, toolrun.FailClassTimeout, res2.FailClass)
	var fe2 *diagnostic.FoundryError
	if !errors.As(res2.Err(), &fe2) {
		log.Fail("git_foundry_error", res2.LogDetail())
	} else {
		log.Assert("git_error_id", fe2.ID() == diagnostic.IDToolTimeout, diagnostic.IDToolTimeout, fe2.ID())
	}
	log.PhaseEnd("git_timeout", testutil.OutcomeOK)
}

func strings0(v []string) string {
	if len(v) == 0 {
		return ""
	}
	out := v[0]
	for _, s := range v[1:] {
		out += "," + s
	}
	return out
}
