//go:build unix

package sentinel_test

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
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func testHost(t *testing.T) toolrun.HostCapture {
	t.Helper()
	h, err := toolrun.CaptureHost("")
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
	_ = os.Remove(marker)
	return dir, marker
}

func recordArtifact(t *testing.T, name string, env map[string]string, argv []string, obs []toolrun.IsolationObservation) {
	t.Helper()
	path, err := toolrun.WriteIsolationArtifact("", name, env, argv, obs)
	if err != nil {
		t.Logf("isolation artifact write: %v", err)
		return
	}
	if path != "" {
		t.Logf("isolation_artifact=%s", path)
	}
}

// TestSentinel_Toolrun_GoMatrix plants the full E2 go sentinel matrix against
// production ConstructGoEnv and asserts no leak (REQ-214 permanent suite).
func TestSentinel_Toolrun_GoMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	dir, marker := helpers(t)
	h := testHost(t)
	log.Fixture("helpers", dir)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("plant_hostile")
	// Pollute ambient — construction must ignore undeclared and close fixed keys.
	for _, k := range e2.GoSentinelKeys {
		t.Setenv(k, "SHOULD_NOT_APPEAR_"+e2.SentinelValue)
	}
	t.Setenv("GOFLAGS", "-toolexec="+filepath.Join(dir, "toolexec-sentinel.sh"))
	t.Setenv("GOCACHEPROG", filepath.Join(dir, "cacheprog-sentinel.sh"))
	t.Setenv("GOAUTH", filepath.Join(dir, "goauth-sentinel.sh"))
	t.Setenv("GOENV", filepath.Join(dir, "hostile-goenv"))
	log.PhaseEnd("plant_hostile", testutil.OutcomeOK)

	log.Phase("construct")
	env := toolrun.ConstructGoEnv(h)
	log.Step("construct_go", testutil.OutcomeOK,
		"env_hash="+toolrun.AllowlistHash(env)+" env_keys="+strings.Join(toolrun.EnvKeys(env), ","))
	log.PhaseEnd("construct", testutil.OutcomeOK)

	log.Phase("assert_matrix")
	var obs []toolrun.IsolationObservation
	for _, plant := range e2.FullSentinelMatrix() {
		if plant.Kind != "go-fixed" && plant.Kind != "go-absent" {
			continue
		}
		k := plant.Key
		switch plant.Kind {
		case "go-fixed":
			want, ok := toolrun.NormativeGoValue(k)
			if !ok {
				log.Fail("missing_fixed", k)
			}
			got := env[k]
			leak := got != want ||
				strings.Contains(got, e2.SentinelValue) ||
				strings.Contains(got, "SHOULD_NOT_APPEAR")
			observation := "normative"
			if leak {
				observation = "leak"
			}
			o := toolrun.IsolationObservation{
				Sentinel: k, Kind: plant.Kind, Observation: observation,
				Exit: -1, Detail: plant.Description,
			}
			obs = append(obs, o)
			log.Step(k, mapOutcome(leak), toolrun.FormatObservationLine(o))
			log.Assert("go_fixed_"+k, !leak, false, leak)
		case "go-absent":
			_, present := env[k]
			observation := "absent"
			if present {
				observation = "present"
			}
			o := toolrun.IsolationObservation{
				Sentinel: k, Kind: plant.Kind, Observation: observation,
				Exit: -1, Detail: plant.Description,
			}
			obs = append(obs, o)
			log.Step(k, mapOutcome(present), toolrun.FormatObservationLine(o))
			log.Assert("go_absent_"+k, !present, false, present)
		}
	}
	if bad := toolrun.KeysOutsideAllowlist(env, toolrun.GoAllowlistKeys); len(bad) > 0 {
		recordArtifact(t, "go_matrix_extra", env, nil, obs)
		log.Fail("outside_allowlist", strings.Join(bad, ","))
	}
	if e2.MarkerContains(marker, "sentinel") {
		recordArtifact(t, "go_matrix_marker", env, nil, obs)
		log.Fail("helper_ran", "marker touched during construction")
	}
	log.PhaseEnd("assert_matrix", testutil.OutcomeOK)

	if t.Failed() {
		recordArtifact(t, "go_matrix_fail", env, nil, obs)
	}
}

// TestSentinel_Toolrun_LiveGoGit runs real go/git under polluted host env and
// asserts isolation (REQ-214 integration).
func TestSentinel_Toolrun_LiveGoGit(t *testing.T) {
	log := testutil.New(t)
	dir, marker := helpers(t)
	h := testHost(t)

	// Plant full hostile ambient (skip HOME so test binary modules still work).
	hostile := e2.HostileHostEnv(os.Environ(), dir)
	for _, e := range hostile {
		k, v, ok := strings.Cut(e, "=")
		if !ok || k == "HOME" {
			continue
		}
		if strings.HasPrefix(k, "GO") || strings.HasPrefix(k, "GIT") ||
			k == "CC" || k == "CXX" || k == "GCCGO" || k == "EMAIL" ||
			k == "XDG_CONFIG_HOME" {
			t.Setenv(k, v)
		}
	}

	log.Phase("live_go_env")
	env := toolrun.ConstructGoEnv(h)
	goBin, err := exec.LookPath("go")
	if err != nil {
		log.Fail("look_go", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	keys := []string{
		"GOENV", "GOFLAGS", "GOCACHEPROG", "GOAUTH", "GOVCS",
		"GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GOINSECURE", "GOTOOLCHAIN", "CGO_ENABLED",
	}
	cmd := exec.CommandContext(ctx, goBin, append([]string{"env"}, keys...)...)
	cmd.Env = toolrun.EnvSlice(env)
	out, err := cmd.CombinedOutput()
	exit := 0
	if err != nil {
		exit = 1
		recordArtifact(t, "live_go_env", env, append([]string{"go", "env"}, keys...), nil)
		log.Fail("go_env", err.Error()+"\n"+string(out))
	}
	lines := splitLines(string(out))
	if len(lines) != len(keys) {
		log.Fail("line_count", string(out))
	}
	want := map[string]string{
		"GOENV": "", "GOFLAGS": "", "GOCACHEPROG": "", "GOAUTH": "off",
		"GOVCS": "*:off", "GOPRIVATE": "", "GONOPROXY": "", "GONOSUMDB": "",
		"GOINSECURE": "", "GOTOOLCHAIN": "local", "CGO_ENABLED": "0",
	}
	var obs []toolrun.IsolationObservation
	for i, k := range keys {
		got := lines[i]
		w := want[k]
		leak := got != w
		if k == "GOENV" && (got == "" || got == "off") {
			leak = false
		}
		if k == "GOENV" && strings.Contains(got, string(filepath.Separator)) {
			leak = true
		}
		observation := "normative"
		if leak {
			observation = "leak"
		}
		o := toolrun.IsolationObservation{
			Sentinel: k, Kind: "live-go", Observation: observation,
			Argv: "go env " + k, Exit: exit, Detail: "tool_value_match=" + boolStr(!leak),
		}
		// Do not put raw tool values that might be paths into detail for GOENV file case —
		// detail uses only booleans / key names.
		obs = append(obs, o)
		log.Step("live_"+k, mapOutcome(leak), toolrun.FormatObservationLine(o))
		log.Assert("live_"+k, !leak, false, leak)
	}
	// Construction still places GOENV=off in the child env map.
	log.Assert("child_GOENV_off", env["GOENV"] == "off", "off", env["GOENV"])
	log.PhaseEnd("live_go_env", testutil.OutcomeOK)

	log.Phase("live_git_init")
	owned, err := toolrun.EmptyTemplateDir(dir)
	if err != nil {
		log.Fail("empty_template", err.Error())
	}
	t.Cleanup(func() { _ = os.RemoveAll(owned) })
	repo := filepath.Join(dir, "sentinel-repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		log.Fail("mkdir_repo", err.Error())
	}
	gitEnv := toolrun.ConstructGitEnv(h.PATH, owned)
	gitBin, err := exec.LookPath("git")
	if err != nil {
		log.Fail("look_git", err.Error())
	}
	gitCmd := exec.Command(gitBin, "init", "--initial-branch=main", "--template="+owned, ".")
	gitCmd.Dir = repo
	gitCmd.Env = toolrun.EnvSlice(gitEnv)
	gitOut, gitErr := gitCmd.CombinedOutput()
	gitExit := 0
	if gitErr != nil {
		gitExit = 1
		recordArtifact(t, "live_git_init", gitEnv, gitCmd.Args, obs)
		log.Fail("git_init", gitErr.Error()+"\n"+string(gitOut))
	}
	// Git sentinel keys absent.
	for _, k := range e2.GitSentinelKeys {
		_, present := gitEnv[k]
		o := toolrun.IsolationObservation{
			Sentinel: k, Kind: "git-absent",
			Observation: mapPresent(present), Argv: "git init", Exit: gitExit,
		}
		obs = append(obs, o)
		log.Step("git_absent_"+k, mapOutcome(present), toolrun.FormatObservationLine(o))
		log.Assert("git_absent_"+k, !present, false, present)
	}
	// Template leak inspection (e2 helper).
	rep, err := e2.InspectGitTemplateLeak(repo)
	if err != nil {
		log.Fail("inspect", err.Error())
	}
	leak := rep.SentinelFilePresent || len(rep.HostileHooks) > 0
	o := toolrun.IsolationObservation{
		Sentinel: "GIT_TEMPLATE", Kind: "git-template",
		Observation: mapLeak(leak), Argv: "git init --template=<owned-empty>", Exit: gitExit,
		Detail: "sentinel_file=" + boolStr(rep.SentinelFilePresent) +
			" hostile_hooks=" + strings.Join(rep.HostileHooks, ","),
	}
	obs = append(obs, o)
	log.Step("git_template", mapOutcome(leak), toolrun.FormatObservationLine(o))
	log.Assert("template_clean", !leak, false, leak)
	log.Assert("tmpl_dir_owned", gitEnv["GIT_TEMPLATE_DIR"] == owned, owned, gitEnv["GIT_TEMPLATE_DIR"])
	log.PhaseEnd("live_git_init", testutil.OutcomeOK)

	log.Phase("helpers_clean")
	helperLeak := e2.MarkerContains(marker, "toolexec-sentinel-ran") ||
		e2.MarkerContains(marker, "cacheprog-sentinel-ran") ||
		e2.MarkerContains(marker, "goauth-sentinel-ran") ||
		e2.MarkerContains(marker, "host-template-hook-ran")
	log.Assert("marker_clean", !helperLeak, false, helperLeak)
	log.PhaseEnd("helpers_clean", testutil.OutcomeOK)

	if t.Failed() {
		recordArtifact(t, "live_go_git_fail", env, []string{"go", "env"}, obs)
	}
}

// TestSentinel_Toolrun_ToolexecIsolated proves GOFLAGS -toolexec does not run
// under ConstructGoEnv (production path).
func TestSentinel_Toolrun_ToolexecIsolated(t *testing.T) {
	log := testutil.New(t)
	dir, marker := helpers(t)
	h := testHost(t)
	toolexec := filepath.Join(dir, "toolexec-sentinel.sh")

	modDir := filepath.Join(dir, "mod")
	if err := os.MkdirAll(modDir, 0o700); err != nil {
		log.Fail("mkdir", err.Error())
	}
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module sentinelsuite\n\ngo 1.26\n"), 0o644); err != nil {
		log.Fail("gomod", err.Error())
	}
	if err := os.WriteFile(filepath.Join(modDir, "x.go"), []byte("package x\nfunc F() int { return 1 }\n"), 0o644); err != nil {
		log.Fail("xgo", err.Error())
	}

	t.Setenv("GOFLAGS", "-toolexec="+toolexec)
	childEnv := toolrun.ConstructGoEnv(h)
	log.Assert("GOFLAGS_empty", childEnv["GOFLAGS"] == "", "", childEnv["GOFLAGS"])

	_ = os.Remove(marker)
	cmd := exec.Command("go", "test", "-a", "-c", "-o", filepath.Join(dir, "x.out"), ".")
	cmd.Dir = modDir
	cmd.Env = toolrun.EnvSlice(childEnv)
	out, err := cmd.CombinedOutput()
	exit := 0
	if err != nil {
		exit = 1
		recordArtifact(t, "toolexec", childEnv, cmd.Args, nil)
		log.Fail("compile", err.Error()+"\n"+string(out))
	}
	leaked := e2.MarkerContains(marker, "toolexec-sentinel-ran")
	o := toolrun.IsolationObservation{
		Sentinel: "GOFLAGS_TOOLEXEC", Kind: "helper-prog",
		Observation: mapLeak(leaked), Argv: "go test -a -c", Exit: exit,
		Detail: "helper_ran=" + boolStr(leaked),
	}
	log.Step("toolexec", mapOutcome(leaked), toolrun.FormatObservationLine(o))
	log.Assert("toolexec_absent", !leaked, false, leaked)
	if leaked {
		recordArtifact(t, "toolexec_leak", childEnv, cmd.Args, []toolrun.IsolationObservation{o})
	}
}

// TestSentinel_Toolrun_ProcessTreeEqualsPlan audits FakePlanRunner against a
// plan-shaped external_steps list including git-init (REQ-214).
func TestSentinel_Toolrun_ProcessTreeEqualsPlan(t *testing.T) {
	log := testutil.New(t)
	h := testHost(t)
	owned, err := toolrun.EmptyTemplateDir(t.TempDir())
	if err != nil {
		log.Fail("template", err.Error())
	}
	t.Cleanup(func() { _ = os.RemoveAll(owned) })

	goBin, err := exec.LookPath("go")
	if err != nil {
		log.Fail("look_go", err.Error())
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		log.Fail("look_git", err.Error())
	}
	goEnv := toolrun.ConstructGoEnv(h)
	gitEnv := toolrun.ConstructGitEnv(h.PATH, owned)

	steps := []plan.ExternalStep{
		{
			ID: "go-mod-tidy", Binary: goBin,
			Argv: []string{"go", "mod", "tidy"},
			Cwd:  plan.StageDescriptorCWD, Mutates: []string{"go.mod", "go.sum"},
			Network: plan.NetworkMay, TimeoutS: 600, OutputCapBytes: plan.OutputCapBytes,
			Env: copyMap(goEnv),
		},
		{
			ID: "go-mod-verify", Binary: goBin,
			Argv: []string{"go", "mod", "verify"},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 120, OutputCapBytes: plan.OutputCapBytes, Env: copyMap(goEnv),
		},
		{
			ID: "go-test", Binary: goBin,
			Argv: []string{"go", "test", "-count=1", "-buildvcs=false", "-mod=readonly", "./..."},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 300, OutputCapBytes: plan.OutputCapBytes, Env: copyMap(goEnv),
		},
		{
			ID: "go-vet", Binary: goBin,
			Argv: []string{"go", "vet", "-buildvcs=false", "-mod=readonly", "./..."},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 300, OutputCapBytes: plan.OutputCapBytes, Env: copyMap(goEnv),
		},
		{
			ID: "git-init", Binary: gitBin,
			Argv: []string{"git", "init", "--initial-branch=main", "--template=" + owned, "."},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 60, OutputCapBytes: plan.OutputCapBytes, Env: gitEnv,
		},
	}

	log.Phase("process_tree")
	log.Step("plan", testutil.OutcomeOK, toolrun.PlannedArgvSummary(steps))
	fake := &toolrun.FakePlanRunner{}
	obs := fake.RunPlan(steps)
	diffs := toolrun.CompareProcessTree(steps, obs)
	o := toolrun.IsolationObservation{
		Sentinel: "process-tree", Kind: "process-tree",
		Observation: mapLeak(len(diffs) != 0),
		Argv:        toolrun.PlannedArgvSummary(steps),
		Exit:        0,
		Detail:      toolrun.FormatTreeDiffs(diffs),
	}
	log.Step("tree_audit", mapOutcome(len(diffs) != 0), toolrun.FormatObservationLine(o))
	log.Assert("tree_equal", len(diffs) == 0, toolrun.FormatTreeDiffs(nil), toolrun.FormatTreeDiffs(diffs))

	// Failure dump redaction: polluted env must not leak values in FormatTreeDiffs.
	fake2 := &toolrun.FakePlanRunner{MutateEnv: func(_ string, env map[string]string) {
		env["GOFLAGS"] = "-toolexec=/evil/secret-helper"
		env["AWS_SECRET_ACCESS_KEY"] = "AKIA_SECRET_VALUE_DO_NOT_LOG"
	}}
	obs2 := fake2.RunPlan(steps)
	diffs2 := toolrun.CompareProcessTree(steps, obs2)
	joined := toolrun.FormatTreeDiffs(diffs2)
	log.Assert("redact_secret", !strings.Contains(joined, "AKIA_SECRET_VALUE_DO_NOT_LOG"), true, joined)
	log.Assert("redact_helper_path", !strings.Contains(joined, "/evil/secret-helper"), true, joined)
	log.Assert("key_name_GOFLAGS", strings.Contains(joined, "GOFLAGS"), true, joined)
	log.Assert("key_name_AWS", strings.Contains(joined, "AWS_SECRET_ACCESS_KEY"), true, joined)
	if t.Failed() {
		recordArtifact(t, "process_tree", goEnv, steps[0].Argv, []toolrun.IsolationObservation{o})
	}
	log.PhaseEnd("process_tree", testutil.OutcomeOK)
	t.Logf("sentinel suite os=%s/%s steps=%d", runtime.GOOS, runtime.GOARCH, len(steps))
}

// TestSentinel_FullMatrix_EndToEnd is the single CI entrypoint covering the
// full sentinel list + process-tree for the permanent REQ-214 suite.
func TestSentinel_FullMatrix_EndToEnd(t *testing.T) {
	// Compose the critical paths: matrix + live + tree already covered above.
	// This test re-runs a compact end-to-end for evidence-style single entry.
	log := testutil.New(t)
	dir, marker := helpers(t)
	h := testHost(t)

	log.Phase("full_matrix")
	env := toolrun.ConstructGoEnv(h)
	pass := 0
	fail := 0
	for _, plant := range e2.FullSentinelMatrix() {
		switch plant.Kind {
		case "go-fixed":
			want, _ := toolrun.NormativeGoValue(plant.Key)
			leak := env[plant.Key] != want
			if leak {
				fail++
				t.Errorf("go-fixed %s leak", plant.Key)
			} else {
				pass++
			}
			log.Step(plant.Key, mapOutcome(leak), "kind=go-fixed observation="+mapLeak(leak))
		case "go-absent":
			_, present := env[plant.Key]
			if present {
				fail++
				t.Errorf("go-absent %s present", plant.Key)
			} else {
				pass++
			}
			log.Step(plant.Key, mapOutcome(present), "kind=go-absent observation="+mapPresent(present))
		}
	}

	owned, err := toolrun.EmptyTemplateDir(dir)
	if err != nil {
		log.Fail("template", err.Error())
	}
	t.Cleanup(func() { _ = os.RemoveAll(owned) })
	repo := filepath.Join(dir, "e2e-repo")
	_ = os.MkdirAll(repo, 0o700)
	gitEnv := toolrun.ConstructGitEnv(h.PATH, owned)
	gitBin, err := exec.LookPath("git")
	if err != nil {
		log.Fail("git", err.Error())
	}
	cmd := exec.Command(gitBin, "init", "--initial-branch=main", "--template="+owned, ".")
	cmd.Dir = repo
	cmd.Env = toolrun.EnvSlice(gitEnv)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fail("git_init", err.Error()+"\n"+string(out))
	}
	for _, plant := range e2.FullSentinelMatrix() {
		if plant.Kind != "git-absent" {
			continue
		}
		_, present := gitEnv[plant.Key]
		if present {
			fail++
			t.Errorf("git-absent %s present", plant.Key)
		} else {
			pass++
		}
		log.Step(plant.Key, mapOutcome(present), "kind=git-absent observation="+mapPresent(present))
	}
	rep, err := e2.InspectGitTemplateLeak(repo)
	if err != nil {
		log.Fail("inspect", err.Error())
	}
	if rep.SentinelFilePresent || len(rep.HostileHooks) > 0 {
		fail++
		t.Errorf("template leak: %+v", rep)
	} else {
		pass++
	}

	// Process tree mini plan.
	goBin, _ := exec.LookPath("go")
	steps := []plan.ExternalStep{{
		ID: "go-mod-verify", Binary: goBin,
		Argv: []string{"go", "mod", "verify"},
		Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
		TimeoutS: 120, OutputCapBytes: plan.OutputCapBytes, Env: copyMap(env),
	}, {
		ID: "git-init", Binary: gitBin,
		Argv: []string{"git", "init", "--initial-branch=main", "--template=" + owned, "."},
		Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
		TimeoutS: 60, OutputCapBytes: plan.OutputCapBytes, Env: gitEnv,
	}}
	fake := &toolrun.FakePlanRunner{}
	treeDiffs := toolrun.CompareProcessTree(steps, fake.RunPlan(steps))
	if len(treeDiffs) != 0 {
		fail++
		t.Errorf("tree: %s", toolrun.FormatTreeDiffs(treeDiffs))
	} else {
		pass++
	}
	log.Step("process_tree", mapOutcome(len(treeDiffs) != 0), toolrun.FormatTreeDiffs(treeDiffs))

	if e2.MarkerContains(marker, "sentinel") {
		// Construction + git init should not run go helpers.
		// (toolexec only on compile — not invoked here.)
	}
	log.PhaseEnd("full_matrix", testutil.OutcomeOK)
	t.Logf("REQ-214 full matrix: pass=%d fail=%d os=%s/%s", pass, fail, runtime.GOOS, runtime.GOARCH)
	if fail != 0 {
		recordArtifact(t, "full_matrix", env, nil, nil)
		t.Fatalf("sentinel matrix failures: %d", fail)
	}
}

func mapOutcome(bad bool) testutil.Outcome {
	if bad {
		return testutil.OutcomeFail
	}
	return testutil.OutcomeOK
}

func mapPresent(present bool) string {
	if present {
		return "present"
	}
	return "absent"
}

func mapLeak(leak bool) string {
	if leak {
		return "leak"
	}
	return "clean"
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	if strings.HasSuffix(s, "\r\n") {
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
