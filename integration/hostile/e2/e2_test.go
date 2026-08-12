//go:build unix

package e2_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/integration/hostile/e2"
)

func testHost(t *testing.T) e2.HostCapture {
	t.Helper()
	h, err := e2.CaptureHost()
	if err != nil {
		t.Fatalf("CaptureHost: %v", err)
	}
	return h
}

func helpers(t *testing.T) (dir, marker string) {
	t.Helper()
	dir = t.TempDir()
	marker = filepath.Join(dir, "marker.log")
	if err := e2.WriteSentinelHelpers(dir, marker); err != nil {
		t.Fatalf("WriteSentinelHelpers: %v", err)
	}
	// Ensure marker starts absent.
	_ = os.Remove(marker)
	return dir, marker
}

// TestE2_ConstructGoEnv_EmptyBaseAllowlist proves constructed env has only
// allowlist keys and normative fixed values (Section 34.2).
func TestE2_ConstructGoEnv_EmptyBaseAllowlist(t *testing.T) {
	log := e2.NewProbeLog()
	h := testHost(t)
	env := e2.ConstructGoEnv(h)
	m := e2.EnvMap(env)

	// Outside allowlist → fail.
	if bad := e2.KeysOutsideAllowlist(m, e2.GoAllowlistKeys); len(bad) > 0 {
		t.Fatalf("keys outside allowlist: %v", bad)
	}
	log.Record(e2.ProbeEntry{
		Probe: "construct_go", Step: "allowlist_keys", Outcome: "pass",
		Detail: "key count=" + itoa(len(m)),
		Args:   strings.Join(e2.EnvKeys(m), ","),
	})

	// Fixed values.
	for k, want := range e2.FixedGoEnv {
		got := m[k]
		leak := got != want
		outcome := "pass"
		if leak {
			outcome = "fail"
		}
		log.Record(e2.ProbeEntry{
			Probe: "construct_go", Step: "fixed_value", Key: k, Leak: e2.BoolPtr(leak),
			Outcome: outcome, Detail: "want normative; mismatch=" + boolStr(leak),
		})
		if leak {
			t.Errorf("fixed key %s: got %q want %q", k, got, want)
		}
	}

	// Host-captured present.
	for _, k := range []string{"PATH", "HOME", "GOMODCACHE", "GOCACHE", "GOPATH", "GOPROXY", "GOSUMDB"} {
		if m[k] == "" && k != "TMPDIR" {
			// PATH/HOME must be non-empty; caches should be non-empty on normal hosts.
			if k == "PATH" || k == "HOME" {
				t.Errorf("host-captured %s is empty", k)
			}
		}
	}
	// TMPDIR only present when host set it.
	if h.TMPDIR == "" {
		if _, ok := m["TMPDIR"]; ok {
			t.Error("TMPDIR must be absent when host TMPDIR unset")
		}
	} else if m["TMPDIR"] != h.TMPDIR {
		t.Errorf("TMPDIR: got %q want %q", m["TMPDIR"], h.TMPDIR)
	}

	pass, fail := log.SummaryPassFail()
	if fail != 0 {
		t.Fatalf("probe log fail=%d pass=%d", fail, pass)
	}
}

// TestE2_GoSentinelMatrix_NoLeak proves every Go sentinel key is closed under
// construction even when the *test process* is polluted (simulating hostile host).
func TestE2_GoSentinelMatrix_NoLeak(t *testing.T) {
	log := e2.NewProbeLog()
	dir, marker := helpers(t)
	h := testHost(t)

	// Pollute the ambient environment of Capture is already done; construction
	// must not read GoSentinelKeys from the host process env.
	for _, k := range e2.GoSentinelKeys {
		t.Setenv(k, "SHOULD_NOT_APPEAR_"+e2.SentinelValue)
	}
	t.Setenv("GOFLAGS", "-toolexec="+filepath.Join(dir, "toolexec-sentinel.sh"))
	t.Setenv("GOCACHEPROG", filepath.Join(dir, "cacheprog-sentinel.sh"))
	t.Setenv("GOAUTH", filepath.Join(dir, "goauth-sentinel.sh"))
	t.Setenv("GOENV", filepath.Join(dir, "hostile-goenv"))

	// Re-capture only host-permitted fields via go env — CaptureHost uses ambient
	// go env which is fine; ConstructGoEnv ignores polluted GO* for fixed keys.
	env := e2.ConstructGoEnv(h)
	m := e2.EnvMap(env)

	for _, plant := range e2.FullSentinelMatrix() {
		if plant.Kind != "go-fixed" && plant.Kind != "go-absent" {
			continue
		}
		k := plant.Key
		switch plant.Kind {
		case "go-fixed":
			want, ok := e2.NormativeGoValue(k)
			if !ok {
				t.Fatalf("go-fixed key %s missing from FixedGoEnv", k)
			}
			got := m[k]
			leak := got != want
			// Also ensure sentinel marker string is not present.
			if strings.Contains(got, e2.SentinelValue) || strings.Contains(got, "SHOULD_NOT_APPEAR") {
				leak = true
			}
			outcome := "pass"
			if leak {
				outcome = "fail"
			}
			log.Record(e2.ProbeEntry{
				Probe: "sentinel_matrix", Step: "go_fixed", Key: k, Leak: e2.BoolPtr(leak),
				Outcome: outcome,
				Detail:  plant.Description + "; normative_match=" + boolStr(!leak),
			})
			if leak {
				t.Errorf("sentinel leak on %s: got %q want normative %q", k, got, want)
			}
		case "go-absent":
			_, present := m[k]
			leak := present
			outcome := "pass"
			if leak {
				outcome = "fail"
			}
			log.Record(e2.ProbeEntry{
				Probe: "sentinel_matrix", Step: "go_absent", Key: k, Leak: e2.BoolPtr(leak),
				Outcome: outcome, Detail: plant.Description,
			})
			if leak {
				t.Errorf("undeclared key %s present in constructed env", k)
			}
		}
	}

	// Extra: keys outside allowlist must be empty set.
	if bad := e2.KeysOutsideAllowlist(m, e2.GoAllowlistKeys); len(bad) > 0 {
		t.Fatalf("outside allowlist: %v", bad)
	}

	// Marker must still be absent (construction does not execute helpers).
	if e2.MarkerContains(marker, "sentinel") {
		t.Error("helper marker touched during construction alone")
	}

	pass, fail := log.SummaryPassFail()
	if fail != 0 {
		t.Fatalf("sentinel matrix failures: fail=%d pass=%d", fail, pass)
	}
}

// TestE2_GoEnv_LiveToolValues runs real `go env` under ConstructGoEnv and
// asserts cmd/go reports the normative closed values.
func TestE2_GoEnv_LiveToolValues(t *testing.T) {
	log := e2.NewProbeLog()
	h := testHost(t)
	// Pollute ambient — child must not inherit.
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPRIVATE", "*.evil.example")
	t.Setenv("GOVCS", "public:all")
	t.Setenv("GOAUTH", "netrc")
	t.Setenv("GOWORK", "off") // same value but we check constructed
	t.Setenv("GOCACHEPROG", "/nonexistent/cacheprog")
	t.Setenv("GOENV", "off") // even if host sets off, we force via construct

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	keys := []string{
		"GOENV", "GOFLAGS", "GOCACHEPROG", "GOTOOLCHAIN", "GOWORK",
		"GOVCS", "GOAUTH", "CGO_ENABLED", "GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GOINSECURE",
	}
	vals, childEnv, err := e2.RunGoEnv(ctx, "", h, keys...)
	if err != nil {
		t.Fatalf("RunGoEnv: %v", err)
	}

	// cmd/go reports GOENV as empty when the process env is GOENV=off
	// (disabled file); the isolation contract is: not a path, and no file read.
	want := map[string]string{
		"GOENV":       "", // effective: off/disabled (see also child env GOENV=off)
		"GOFLAGS":     "",
		"GOCACHEPROG": "",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
		"GOVCS":       "*:off",
		"GOAUTH":      "off",
		"CGO_ENABLED": "0",
		"GOPRIVATE":   "",
		"GONOPROXY":   "",
		"GONOSUMDB":   "",
		"GOINSECURE":  "",
	}
	// Child process env must still set GOENV=off (construction contract).
	if e2.EnvMap(childEnv)["GOENV"] != "off" {
		t.Fatalf("child env GOENV=%q want off", e2.EnvMap(childEnv)["GOENV"])
	}
	for k, w := range want {
		got := vals[k]
		leak := got != w
		// GOENV must not be a filesystem path to a hostile env file.
		if k == "GOENV" && (got == "off" || got == "") {
			leak = false
		}
		if k == "GOENV" && strings.Contains(got, string(filepath.Separator)) {
			leak = true
		}
		outcome := "pass"
		if leak {
			outcome = "fail"
		}
		log.Record(e2.ProbeEntry{
			Probe: "live_go_env", Step: "go_env", Key: k, Leak: e2.BoolPtr(leak),
			Outcome: outcome, Args: "go env " + k,
			Detail: "observed_tool_value_match=" + boolStr(!leak) + " raw=" + got,
		})
		if leak {
			t.Errorf("go env %s = %q want %q", k, got, w)
		}
	}
}

// TestE2_GOFLAGS_Toolexec_DoesNotRun proves a planted GOFLAGS -toolexec helper
// does not execute under constructed env (contrast: does run when inherited).
func TestE2_GOFLAGS_Toolexec_DoesNotRun(t *testing.T) {
	log := e2.NewProbeLog()
	dir, marker := helpers(t)
	h := testHost(t)
	toolexec := filepath.Join(dir, "toolexec-sentinel.sh")

	// Control: inherited pollution CAN run toolexec (documents threat).
	// Use `go env GOFLAGS` only — toolexec runs on compile, not plain `go env`.
	// Use a tiny compile: `go test -c` of a trivial package is heavy; instead
	// `go list -f '{{.ImportPath}}' std` may not invoke compile.
	// Per cmd/go docs, -toolexec wraps tool invocations during build.
	// Minimal: `go test` of a temp module with a trivial test.
	modDir := filepath.Join(dir, "mod")
	if err := os.MkdirAll(modDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module e2sentinel\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "x.go"), []byte("package x\nfunc F() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- Control: host-style inherit (expect leak / helper runs) ---
	_ = os.Remove(marker)
	inheritEnv := append(os.Environ(), "GOFLAGS=-toolexec="+toolexec)
	cmd := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "x.out"), ".")
	cmd.Dir = modDir
	cmd.Env = inheritEnv
	if out, err := cmd.CombinedOutput(); err != nil {
		// Still useful if compile failed after toolexec ran.
		t.Logf("control compile err (ok if marker written): %v\n%s", err, out)
	}
	controlLeaked := e2.MarkerContains(marker, "toolexec-sentinel-ran")
	log.Record(e2.ProbeEntry{
		Probe: "toolexec", Step: "control_inherit", Key: "GOFLAGS",
		Leak: e2.BoolPtr(controlLeaked), Outcome: "info",
		Detail: "threat model: inherited GOFLAGS -toolexec ran=" + boolStr(controlLeaked),
	})
	if !controlLeaked {
		// On some hosts go may not invoke compile tools if cached; force -a.
		_ = os.Remove(marker)
		cmd = exec.Command("go", "test", "-a", "-c", "-o", filepath.Join(dir, "x2.out"), ".")
		cmd.Dir = modDir
		cmd.Env = inheritEnv
		_, _ = cmd.CombinedOutput()
		controlLeaked = e2.MarkerContains(marker, "toolexec-sentinel-ran")
		log.Record(e2.ProbeEntry{
			Probe: "toolexec", Step: "control_inherit_a", Key: "GOFLAGS",
			Leak: e2.BoolPtr(controlLeaked), Outcome: "info",
			Detail: "retry -a: ran=" + boolStr(controlLeaked),
		})
	}
	if !controlLeaked {
		t.Log("warning: control did not invoke toolexec (cache?); isolation still asserted below")
	}

	// --- Isolated: constructed env must not run helper ---
	_ = os.Remove(marker)
	// Also plant hostile GOFLAGS in ambient for good measure.
	t.Setenv("GOFLAGS", "-toolexec="+toolexec)
	childEnv := e2.ConstructGoEnv(h)
	// Double-check GOFLAGS closed.
	if e2.EnvMap(childEnv)["GOFLAGS"] != "" {
		t.Fatalf("constructed GOFLAGS not empty: %q", e2.EnvMap(childEnv)["GOFLAGS"])
	}
	cmd = exec.Command("go", "test", "-a", "-c", "-o", filepath.Join(dir, "x3.out"), ".")
	cmd.Dir = modDir
	cmd.Env = childEnv
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated compile: %v\n%s", err, out)
	}
	isolatedLeaked := e2.MarkerContains(marker, "toolexec-sentinel-ran")
	outcome := "pass"
	if isolatedLeaked {
		outcome = "fail"
	}
	log.Record(e2.ProbeEntry{
		Probe: "toolexec", Step: "isolated_construct", Key: "GOFLAGS",
		Leak: e2.BoolPtr(isolatedLeaked), Outcome: outcome,
		Detail: "constructed env must not execute host toolexec",
		Args:   "go test -a -c",
	})
	if isolatedLeaked {
		t.Fatal("GOFLAGS -toolexec sentinel executed under constructed env")
	}
}

// TestE2_GOCACHEPROG_DoesNotRun proves empty GOCACHEPROG under construction
// prevents a planted cache program from running.
func TestE2_GOCACHEPROG_DoesNotRun(t *testing.T) {
	log := e2.NewProbeLog()
	dir, marker := helpers(t)
	h := testHost(t)
	prog := filepath.Join(dir, "cacheprog-sentinel.sh")

	// Control: with GOCACHEPROG set, go should attempt to run it on build cache ops.
	_ = os.Remove(marker)
	modDir := filepath.Join(dir, "mod2")
	_ = os.MkdirAll(modDir, 0o700)
	_ = os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module e2cache\n\ngo 1.26\n"), 0o644)
	_ = os.WriteFile(filepath.Join(modDir, "x.go"), []byte("package x\n"), 0o644)
	cmd := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "c.out"), ".")
	cmd.Dir = modDir
	cmd.Env = append(os.Environ(), "GOCACHEPROG="+prog)
	_, _ = cmd.CombinedOutput()
	control := e2.MarkerContains(marker, "cacheprog-sentinel-ran")
	log.Record(e2.ProbeEntry{
		Probe: "gocacheprog", Step: "control_inherit", Key: "GOCACHEPROG",
		Leak: e2.BoolPtr(control), Outcome: "info",
		Detail: "threat model: GOCACHEPROG ran=" + boolStr(control),
	})

	// Isolated.
	_ = os.Remove(marker)
	t.Setenv("GOCACHEPROG", prog)
	childEnv := e2.ConstructGoEnv(h)
	if e2.EnvMap(childEnv)["GOCACHEPROG"] != "" {
		t.Fatalf("constructed GOCACHEPROG not empty")
	}
	cmd = exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "c2.out"), ".")
	cmd.Dir = modDir
	cmd.Env = childEnv
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated: %v\n%s", err, out)
	}
	leaked := e2.MarkerContains(marker, "cacheprog-sentinel-ran")
	outcome := "pass"
	if leaked {
		outcome = "fail"
	}
	log.Record(e2.ProbeEntry{
		Probe: "gocacheprog", Step: "isolated_construct", Key: "GOCACHEPROG",
		Leak: e2.BoolPtr(leaked), Outcome: outcome,
		Detail: "empty GOCACHEPROG must not invoke host helper",
	})
	if leaked {
		t.Fatal("GOCACHEPROG sentinel ran under constructed env")
	}
}

// TestE2_GOENV_FileNotRead proves GOENV=off prevents reading a hostile go env file.
func TestE2_GOENV_FileNotRead(t *testing.T) {
	log := e2.NewProbeLog()
	dir, marker := helpers(t)
	h := testHost(t)
	goenvFile := filepath.Join(dir, "hostile-goenv")

	// Control: GOENV=file should inject GOFLAGS from file.
	_ = os.Remove(marker)
	cmd := exec.Command("go", "env", "GOFLAGS")
	cmd.Env = append(os.Environ(), "GOENV="+goenvFile, "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("control go env: %v\n%s", err, out)
	}
	controlVal := strings.TrimSpace(string(out))
	controlLeaked := strings.Contains(controlVal, "toolexec")
	log.Record(e2.ProbeEntry{
		Probe: "goenv_file", Step: "control_read", Key: "GOENV",
		Leak: e2.BoolPtr(controlLeaked), Outcome: "info",
		Detail: "threat: GOENV file injects GOFLAGS containing toolexec=" + boolStr(controlLeaked),
	})

	// Isolated: ConstructGoEnv forces GOENV=off; go env GOFLAGS empty.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t.Setenv("GOENV", goenvFile)
	vals, childEnv, err := e2.RunGoEnv(ctx, "", h, "GOENV", "GOFLAGS")
	if err != nil {
		t.Fatal(err)
	}
	// Construction sets GOENV=off; cmd/go reports GOENV as empty when disabled.
	// Isolation proof: child env is off, tool GOFLAGS empty, no path to hostile file.
	childGOENV := e2.EnvMap(childEnv)["GOENV"]
	toolGOENV := vals["GOENV"]
	toolGOFLAGS := vals["GOFLAGS"]
	leak := childGOENV != "off" || toolGOFLAGS != "" || strings.Contains(toolGOENV, string(filepath.Separator))
	outcome := "pass"
	if leak {
		outcome = "fail"
	}
	log.Record(e2.ProbeEntry{
		Probe: "goenv_file", Step: "isolated_off", Key: "GOENV",
		Leak: e2.BoolPtr(leak), Outcome: outcome,
		Detail: "child_GOENV=" + childGOENV + " tool_GOENV=" + toolGOENV + " tool_GOFLAGS_empty=" + boolStr(toolGOFLAGS == ""),
		Args:   "go env GOENV GOFLAGS",
	})
	if leak {
		t.Fatalf("GOENV isolation failed: child=%q tool_GOENV=%q tool_GOFLAGS=%q", childGOENV, toolGOENV, toolGOFLAGS)
	}
	if e2.MarkerContains(marker, "toolexec") {
		t.Fatal("toolexec ran via GOENV file under isolation")
	}
}

// TestE2_ConstructGitEnv_Allowlist proves git env shape (Section 34.2).
func TestE2_ConstructGitEnv_Allowlist(t *testing.T) {
	log := e2.NewProbeLog()
	tmpl := t.TempDir()
	env := e2.ConstructGitEnv("/usr/bin:/bin", tmpl)
	m := e2.EnvMap(env)

	if bad := e2.KeysOutsideAllowlist(m, e2.GitAllowlistKeys); len(bad) > 0 {
		t.Fatalf("git env outside allowlist: %v", bad)
	}
	want := map[string]string{
		"PATH":                "/usr/bin:/bin",
		"LC_ALL":              "C",
		"LANG":                "C",
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_SYSTEM":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TEMPLATE_DIR":    tmpl,
	}
	for k, w := range want {
		if m[k] != w {
			t.Errorf("git env %s=%q want %q", k, m[k], w)
		}
	}
	// Forbidden keys absent.
	for _, k := range e2.GitSentinelKeys {
		if _, ok := m[k]; ok {
			t.Errorf("git env must not contain %s", k)
			log.Record(e2.ProbeEntry{
				Probe: "construct_git", Step: "absent", Key: k, Leak: e2.BoolPtr(true), Outcome: "fail",
			})
		} else {
			log.Record(e2.ProbeEntry{
				Probe: "construct_git", Step: "absent", Key: k, Leak: e2.BoolPtr(false), Outcome: "pass",
			})
		}
	}
	log.Record(e2.ProbeEntry{
		Probe: "construct_git", Step: "allowlist", Outcome: "pass",
		Detail: "keys=" + strings.Join(e2.EnvKeys(m), ","),
	})
}

// TestE2_GitInit_EmptyTemplate_NoHostCopy proves isolated git init does not
// copy hostile host template files or hooks.
func TestE2_GitInit_EmptyTemplate_NoHostCopy(t *testing.T) {
	log := e2.NewProbeLog()
	dir, marker := helpers(t)
	hostileTmpl := filepath.Join(dir, "hostile-template")

	// Contrast: init WITH hostile template copies sentinel.
	contrastDir := filepath.Join(dir, "contrast-repo")
	if err := os.MkdirAll(contrastDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--template="+hostileTmpl, contrastDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("contrast git init: %v\n%s", err, out)
	}
	contrast, err := e2.InspectGitTemplateLeak(contrastDir)
	if err != nil {
		t.Fatal(err)
	}
	if !contrast.SentinelFilePresent {
		t.Fatal("contrast setup broken: hostile template did not copy SENTINEL_TEMPLATE_FILE")
	}
	log.Record(e2.ProbeEntry{
		Probe: "git_template", Step: "contrast_hostile", Key: "GIT_TEMPLATE",
		Leak: e2.BoolPtr(true), Outcome: "info",
		Detail: "threat model: host template copies SENTINEL_TEMPLATE_FILE",
	})

	// Isolated path.
	owned, err := e2.EmptyTemplateDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(owned)

	repo := filepath.Join(dir, "isolated-repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	// Plant hostile GIT_* in ambient; ConstructGitEnv must ignore.
	t.Setenv("GIT_TEMPLATE_DIR", hostileTmpl)
	t.Setenv("HOME", filepath.Join(dir, "fake-home"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "hostile-gitconfig"))

	path := os.Getenv("PATH")
	env, err := e2.IsolatedGitInit("", repo, "main", owned, path)
	if err != nil {
		t.Fatalf("IsolatedGitInit: %v", err)
	}
	m := e2.EnvMap(env)
	if m["GIT_TEMPLATE_DIR"] != owned {
		t.Fatalf("GIT_TEMPLATE_DIR=%q want owned %q", m["GIT_TEMPLATE_DIR"], owned)
	}
	if _, ok := m["HOME"]; ok {
		t.Fatal("HOME must be absent from git env")
	}

	rep, err := e2.InspectGitTemplateLeak(repo)
	if err != nil {
		t.Fatal(err)
	}
	leak := rep.SentinelFilePresent || len(rep.HostileHooks) > 0
	outcome := "pass"
	if leak {
		outcome = "fail"
	}
	log.Record(e2.ProbeEntry{
		Probe: "git_template", Step: "isolated_empty", Key: "GIT_TEMPLATE",
		Leak: e2.BoolPtr(leak), Outcome: outcome,
		Detail: "sentinel_file=" + boolStr(rep.SentinelFilePresent) +
			" hostile_hooks=" + strings.Join(rep.HostileHooks, ",") +
			" hooks_entries=" + strings.Join(rep.HooksDirEntries, ","),
		Args: "git init --initial-branch=main --template=<owned-empty>",
	})
	if leak {
		t.Fatalf("template leak: %+v", rep)
	}
	// Prefer empty or absent hooks dir.
	if rep.NonEmptyHooksDir {
		// Empty owned template should not install sample hooks; if any entries
		// exist they must not be our hostile ones (already checked). Log info.
		log.Record(e2.ProbeEntry{
			Probe: "git_template", Step: "hooks_dir", Outcome: "info",
			Detail: "hooks dir non-empty but no hostile content: " + strings.Join(rep.HooksDirEntries, ","),
		})
	}

	// git config must not show system/global origins from hostile files.
	cfgOut, cfgErr := e2.GitConfigEffective("", repo, env)
	if cfgErr != nil {
		// empty config can still be exit 0; log if error
		log.Record(e2.ProbeEntry{
			Probe: "git_config", Step: "list", Outcome: "info",
			Errno: cfgErr.Error(), Detail: "config list err (may be ok if empty)",
		})
	}
	if strings.Contains(cfgOut, "hostile-gitconfig") || strings.Contains(cfgOut, e2.SentinelValue) {
		t.Fatalf("git config leaked hostile origin:\n%s", cfgOut)
	}
	if strings.Contains(cfgOut, "e2leak") {
		t.Fatalf("git alias from hostile config visible:\n%s", cfgOut)
	}
	log.Record(e2.ProbeEntry{
		Probe: "git_config", Step: "isolated_list", Key: "GIT_CONFIG_GLOBAL",
		Leak: e2.BoolPtr(false), Outcome: "pass",
		Detail: "no hostile config origin; bytes=" + itoa(len(cfgOut)),
	})
	_ = marker
}

// TestE2_ProcessTree_EqualsPlan proves FakeRunner observations match plan.
func TestE2_ProcessTree_EqualsPlan(t *testing.T) {
	log := e2.NewProbeLog()
	h := testHost(t)

	steps := e2.ApplyHostToSteps(e2.DefaultExternalSteps(), h)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	steps, err = e2.ResolveBinaries(steps, func(name string) (string, error) {
		if name == "go" {
			return goBin, nil
		}
		return exec.LookPath(name)
	})
	if err != nil {
		t.Fatal(err)
	}

	fake := &e2.FakeRunner{}
	observed := fake.RunPlan(steps)
	diffs := e2.CompareProcessTree(steps, observed)
	outcome := "pass"
	if len(diffs) != 0 {
		outcome = "fail"
	}
	log.Record(e2.ProbeEntry{
		Probe: "process_tree", Step: "default_plan", Outcome: outcome,
		Detail: e2.FormatTreeDiffs(diffs),
		Args:   e2.PlannedArgvSummary(steps),
	})
	if len(diffs) != 0 {
		t.Fatalf("process tree diffs: %s", e2.FormatTreeDiffs(diffs))
	}

	// Negative: extra unplanned step detected.
	fake2 := &e2.FakeRunner{InjectExtra: []e2.ObservedStep{{
		ID: "evil-shell", Binary: "/bin/sh", Argv: []string{"sh", "-c", "true"}, Shell: true,
		Env: map[string]string{},
	}}}
	obs2 := fake2.RunPlan(steps)
	diffs2 := e2.CompareProcessTree(steps, obs2)
	if len(diffs2) == 0 {
		t.Fatal("expected diffs for injected extra step")
	}
	var sawExtra, sawShell bool
	for _, d := range diffs2 {
		if d.Kind == "extra" || d.Kind == "count" {
			sawExtra = true
		}
		if d.Kind == "shell" {
			sawShell = true
		}
	}
	if !sawExtra {
		t.Errorf("expected count/extra diff, got %s", e2.FormatTreeDiffs(diffs2))
	}
	if !sawShell {
		t.Errorf("expected shell diff for injected sh step, got %s", e2.FormatTreeDiffs(diffs2))
	}
	log.Record(e2.ProbeEntry{
		Probe: "process_tree", Step: "detect_extra_shell", Outcome: "pass",
		Detail: "detected: " + e2.FormatTreeDiffs(diffs2) + " shell_flag=" + boolStr(sawShell),
	})

	// Negative: env pollution in observed detected.
	fake3 := &e2.FakeRunner{MutateEnv: func(id string, env map[string]string) {
		env["GOFLAGS"] = "-toolexec=/evil"
		env["EVIL_EXTRA"] = "1"
	}}
	obs3 := fake3.RunPlan(steps)
	diffs3 := e2.CompareProcessTree(steps, obs3)
	if len(diffs3) == 0 {
		t.Fatal("expected env diffs")
	}
	log.Record(e2.ProbeEntry{
		Probe: "process_tree", Step: "detect_env_pollution", Outcome: "pass",
		Detail: e2.FormatTreeDiffs(diffs3),
	})
}

// TestE2_ProcessTree_LiveGoEnvStep records a real go-env step and compares
// a single-step mini-plan (promotable audit pattern).
func TestE2_ProcessTree_LiveGoEnvStep(t *testing.T) {
	log := e2.NewProbeLog()
	h := testHost(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	envMap := e2.EnvMap(e2.ConstructGoEnv(h))
	plan := []e2.ExternalStep{{
		ID: "go-env-probe", Binary: goBin,
		Argv: []string{"go", "env", "GOFLAGS", "GOENV", "GOTOOLCHAIN"},
		Cwd:  "stage-descriptor", Network: "no", TimeoutS: 60,
		OutputCapBytes: 4 << 20, Env: envMap,
	}}
	runner := &e2.RealRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stdout, stderr, err := runner.Run(ctx, plan[0], "")
	if err != nil {
		t.Fatalf("run: %v\nstderr=%s", err, stderr)
	}
	// go env GOFLAGS GOENV GOTOOLCHAIN → empty, empty-or-off, local
	raw := string(stdout)
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("unexpected go env line count %d: %q", len(lines), raw)
	}
	if lines[0] != "" {
		t.Fatalf("GOFLAGS want empty, got %q", lines[0])
	}
	if lines[1] != "" && lines[1] != "off" {
		t.Fatalf("GOENV want empty|off (disabled), got %q", lines[1])
	}
	if lines[2] != "local" {
		t.Fatalf("GOTOOLCHAIN want local, got %q", lines[2])
	}
	obs := runner.Snapshot()
	diffs := e2.CompareProcessTree(plan, obs)
	outcome := "pass"
	if len(diffs) != 0 {
		outcome = "fail"
	}
	log.Record(e2.ProbeEntry{
		Probe: "process_tree", Step: "live_go_env", Outcome: outcome,
		Detail: e2.FormatTreeDiffs(diffs),
		Args:   strings.Join(plan[0].Argv, " "),
	})
	if len(diffs) != 0 {
		t.Fatalf("diffs: %s", e2.FormatTreeDiffs(diffs))
	}
}

// TestE2_FullMatrix_EndToEnd is the single evidence entrypoint covering the
// full sentinel list + process-tree dump for the E2 record.
func TestE2_FullMatrix_EndToEnd(t *testing.T) {
	log := e2.NewProbeLog()
	dir, marker := helpers(t)
	h := testHost(t)

	// Plant full hostile host env in ambient.
	hostile := e2.HostileHostEnv(os.Environ(), dir)
	for _, e := range hostile {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		// Only set sentinel keys to avoid breaking the test binary's runtime.
		if strings.HasPrefix(k, "GO") || strings.HasPrefix(k, "GIT") ||
			k == "CC" || k == "CXX" || k == "GCCGO" || k == "EMAIL" ||
			k == "XDG_CONFIG_HOME" || k == "HOME" {
			// Do not override HOME for the test process itself (breaks modules).
			if k == "HOME" {
				continue
			}
			t.Setenv(k, v)
		}
	}

	// 1) Go construction matrix
	env := e2.ConstructGoEnv(h)
	m := e2.EnvMap(env)
	for _, plant := range e2.FullSentinelMatrix() {
		switch plant.Kind {
		case "go-fixed":
			want, _ := e2.NormativeGoValue(plant.Key)
			got := m[plant.Key]
			leak := got != want
			outcome := "pass"
			if leak {
				outcome = "fail"
				t.Errorf("go-fixed %s leak", plant.Key)
			}
			log.Record(e2.ProbeEntry{
				Probe: "e2e_matrix", Step: plant.Kind, Key: plant.Key,
				Leak: e2.BoolPtr(leak), Outcome: outcome, Detail: plant.Description,
			})
		case "go-absent":
			_, present := m[plant.Key]
			leak := present
			outcome := "pass"
			if leak {
				outcome = "fail"
				t.Errorf("go-absent %s present", plant.Key)
			}
			log.Record(e2.ProbeEntry{
				Probe: "e2e_matrix", Step: plant.Kind, Key: plant.Key,
				Leak: e2.BoolPtr(leak), Outcome: outcome, Detail: plant.Description,
			})
		case "git-absent":
			// checked against git env below
		}
	}

	// 2) Live go env
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	vals, _, err := e2.RunGoEnv(ctx, "", h,
		"GOENV", "GOFLAGS", "GOCACHEPROG", "GOAUTH", "GOVCS",
		"GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GOINSECURE", "GOTOOLCHAIN", "CGO_ENABLED")
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		// GOENV: cmd/go prints empty when process env is GOENV=off (disabled file).
		"GOENV": "", "GOFLAGS": "", "GOCACHEPROG": "", "GOAUTH": "off",
		"GOVCS": "*:off", "GOPRIVATE": "", "GONOPROXY": "", "GONOSUMDB": "",
		"GOINSECURE": "", "GOTOOLCHAIN": "local", "CGO_ENABLED": "0",
	} {
		got := vals[k]
		leak := got != want
		if k == "GOENV" && (got == "" || got == "off") {
			leak = false
		}
		if k == "GOENV" && strings.Contains(got, string(filepath.Separator)) {
			leak = true
		}
		outcome := "pass"
		if leak {
			outcome = "fail"
			t.Errorf("live %s=%q want %q", k, got, want)
		}
		log.Record(e2.ProbeEntry{
			Probe: "e2e_matrix", Step: "live_go_env", Key: k,
			Leak: e2.BoolPtr(leak), Outcome: outcome,
		})
	}
	// Construction still places GOENV=off in the child env map.
	if e2.EnvMap(e2.ConstructGoEnv(h))["GOENV"] != "off" {
		t.Error("constructed child env must set GOENV=off")
	}

	// 3) Git template isolation
	owned, err := e2.EmptyTemplateDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(owned)
	repo := filepath.Join(dir, "e2e-repo")
	_ = os.MkdirAll(repo, 0o700)
	gitEnv, err := e2.IsolatedGitInit("", repo, "main", owned, h.PATH)
	if err != nil {
		t.Fatal(err)
	}
	gm := e2.EnvMap(gitEnv)
	for _, plant := range e2.FullSentinelMatrix() {
		if plant.Kind != "git-absent" {
			continue
		}
		_, present := gm[plant.Key]
		leak := present
		outcome := "pass"
		if leak {
			outcome = "fail"
			t.Errorf("git-absent %s present", plant.Key)
		}
		log.Record(e2.ProbeEntry{
			Probe: "e2e_matrix", Step: "git_absent", Key: plant.Key,
			Leak: e2.BoolPtr(leak), Outcome: outcome, Detail: plant.Description,
		})
	}
	rep, err := e2.InspectGitTemplateLeak(repo)
	if err != nil {
		t.Fatal(err)
	}
	leak := rep.SentinelFilePresent || len(rep.HostileHooks) > 0
	outcome := "pass"
	if leak {
		outcome = "fail"
		t.Errorf("template leak: %+v", rep)
	}
	log.Record(e2.ProbeEntry{
		Probe: "e2e_matrix", Step: "git_template", Key: "GIT_TEMPLATE",
		Leak: e2.BoolPtr(leak), Outcome: outcome,
	})

	// 4) Process tree vs plan
	steps := e2.ApplyHostToSteps(e2.DefaultExternalSteps(), h)
	// Append git-init as plan would when git.init=true
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	steps = append(steps, e2.GitInitStep(gitBin, "main", owned, h.PATH))
	steps, err = e2.ResolveBinaries(steps, func(name string) (string, error) {
		switch name {
		case "go":
			return goBin, nil
		case "git":
			return gitBin, nil
		default:
			return exec.LookPath(name)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	// Fix git-init binary to absolute (GitInitStep already has abs from lookpath).
	for i := range steps {
		if steps[i].ID == "git-init" {
			steps[i].Binary = gitBin
		}
	}
	fake := &e2.FakeRunner{}
	obs := fake.RunPlan(steps)
	diffs := e2.CompareProcessTree(steps, obs)
	treeOK := len(diffs) == 0
	outcome = "pass"
	if !treeOK {
		outcome = "fail"
		t.Errorf("tree: %s", e2.FormatTreeDiffs(diffs))
	}
	log.Record(e2.ProbeEntry{
		Probe: "e2e_matrix", Step: "process_tree", Outcome: outcome,
		Detail: e2.FormatTreeDiffs(diffs),
		Args:   e2.PlannedArgvSummary(steps),
	})

	// 5) Helper marker clean for construction+git (no compile helpers expected)
	if e2.MarkerContains(marker, "toolexec-sentinel-ran") ||
		e2.MarkerContains(marker, "cacheprog-sentinel-ran") ||
		e2.MarkerContains(marker, "goauth-sentinel-ran") ||
		e2.MarkerContains(marker, "host-template-hook-ran") {
		t.Error("sentinel helper/hook executed during e2e matrix")
		log.Record(e2.ProbeEntry{
			Probe: "e2e_matrix", Step: "marker_clean", Leak: e2.BoolPtr(true), Outcome: "fail",
		})
	} else {
		log.Record(e2.ProbeEntry{
			Probe: "e2e_matrix", Step: "marker_clean", Leak: e2.BoolPtr(false), Outcome: "pass",
			Detail: "no helper execution",
		})
	}

	pass, fail := log.SummaryPassFail()
	t.Logf("E2 full matrix: pass=%d fail=%d os=%s/%s", pass, fail, runtime.GOOS, runtime.GOARCH)
	if fail != 0 {
		t.Fatalf("E2 matrix had %d failures", fail)
	}

	// Emit JSON summary line for evidence capture.
	if b, err := log.JSON(); err == nil {
		t.Logf("E2_JSON_BYTES=%d", len(b))
	}
}

// TestE2_PlanClosedWorld_NoShell asserts plan steps never use a shell argv.
func TestE2_PlanClosedWorld_NoShell(t *testing.T) {
	for _, steps := range [][]e2.ExternalStep{e2.DefaultExternalSteps(), e2.StrictExternalSteps()} {
		for _, s := range steps {
			for _, a := range s.Argv {
				low := strings.ToLower(a)
				if low == "sh" || low == "bash" || low == "zsh" || strings.Contains(low, "/sh") {
					t.Errorf("step %s uses shell token %q", s.ID, a)
				}
			}
			if s.Network != "no" && s.Network != "may" {
				t.Errorf("step %s bad network %q", s.ID, s.Network)
			}
			if s.Cwd != "stage-descriptor" {
				t.Errorf("step %s cwd %q want stage-descriptor", s.ID, s.Cwd)
			}
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var neg bool
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
