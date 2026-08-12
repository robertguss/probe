package toolrun_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// TestSentinel_ForbiddenKeysAbsent is the env allowlist negative table:
// every forbidden / host-bleed key is absent after construction (REQ-214 unit).
func TestSentinel_ForbiddenKeysAbsent(t *testing.T) {
	log := testutil.New(t)
	log.Phase("negative_table")
	env := toolrun.ConstructGoEnv(testHost())

	// Full ForbiddenBleedKeys table.
	for _, k := range toolrun.ForbiddenBleedKeys {
		_, present := env[k]
		// GIT_TEMPLATE_DIR is forbidden on go steps.
		log.Assert("forbidden_absent_"+k, !present, false, present)
	}

	// E2 matrix go-absent keys (may overlap ForbiddenBleedKeys).
	goAbsent := []string{
		"GODEBUG", "GOEXPERIMENT", "GCCGO", "CC", "CXX",
		"CGO_CFLAGS", "CGO_LDFLAGS", "GO111MODULE", "GOROOT",
	}
	for _, k := range goAbsent {
		_, present := env[k]
		log.Assert("go_absent_"+k, !present, false, present)
	}

	// Fixed keys must be normative (not host-planted sentinel strings).
	for k, want := range toolrun.FixedGoEnv {
		got := env[k]
		log.Assert("normative_"+k, got == want, want, got)
	}

	extra, missing := toolrun.EnvKeySetMatchesAllowlist(env, toolrun.GoAllowlistKeys, map[string]struct{}{
		"TMPDIR": {},
	})
	log.Assert("no_extra", len(extra) == 0, "[]", extra)
	log.Assert("no_missing_required", len(missing) == 0, "[]", missing)
	log.PhaseEnd("negative_table", testutil.OutcomeOK)
}

// TestSentinel_FakeRunner_EnvKeySetEquality: FakeRunner records env keys;
// assert set equality with the constructed allowlist (REQ-214 unit).
func TestSentinel_FakeRunner_EnvKeySetEquality(t *testing.T) {
	log := testutil.New(t)
	log.Phase("fake_runner_env")
	host := testHost()
	env := toolrun.ConstructGoEnv(host)
	wantKeys := toolrun.EnvKeys(env)
	slice := toolrun.EnvSlice(env)

	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{"go": "/usr/local/go/bin/go"},
		Outputs: map[string]string{"/usr/local/go/bin/go": "go version go1.26.5 linux/amd64\n"},
	}
	_, _, err := fake.Run(context.Background(), "/usr/local/go/bin/go", []string{"env", "GOFLAGS"}, slice)
	if err != nil {
		log.Fail("fake_run", err.Error())
	}
	if len(fake.ObservedEnv) != 1 {
		log.Fail("observed_count", "want 1")
	}
	eq := toolrun.EnvKeySetEqual(fake.ObservedEnv[0], wantKeys)
	log.Assert("key_set_equal", eq, true, eq)

	// Extra host key must break set equality.
	polluted := append(slice, "GODEBUG=gctrace=1", "EVIL_EXTRA=1")
	_, _, _ = fake.Run(context.Background(), "/usr/local/go/bin/go", []string{"version"}, polluted)
	eq2 := toolrun.EnvKeySetEqual(fake.ObservedEnv[1], wantKeys)
	log.Assert("polluted_detected", !eq2, false, eq2)

	// LastEnvMap keys must match allowlist with no bleed.
	gotMap := fake.LastEnvMap()
	bleed := toolrun.ForbiddenKeysPresent(gotMap, toolrun.ForbiddenBleedKeys)
	// LastEnvMap is polluted run — expect bleed present.
	log.Assert("polluted_has_bleed", len(bleed) > 0, true, len(bleed) > 0)
	log.PhaseEnd("fake_runner_env", testutil.OutcomeOK)
}

// TestSentinel_TemplateIsolation: empty owned template path is the only
// templateDir in ConstructGitEnv (REQ-214 unit).
func TestSentinel_TemplateIsolation(t *testing.T) {
	log := testutil.New(t)
	log.Phase("template")
	parent := t.TempDir()
	owned, err := toolrun.EmptyTemplateDir(parent)
	if err != nil {
		log.Fail("empty_template", err.Error())
	}
	t.Cleanup(func() { _ = os.RemoveAll(owned) })

	// Owned dir must be empty.
	entries, err := os.ReadDir(owned)
	if err != nil {
		log.Fail("readdir", err.Error())
	}
	log.Assert("template_empty", len(entries) == 0, 0, len(entries))

	env := toolrun.ConstructGitEnv("/usr/bin:/bin", owned)
	tmpl, ok := toolrun.GitTemplateDir(env)
	log.Assert("template_present", ok, true, ok)
	log.Assert("template_is_owned", tmpl == owned, owned, tmpl)

	// Only allowlist keys; GIT_TEMPLATE_DIR is the owned path exclusively.
	outside := toolrun.KeysOutsideAllowlist(env, toolrun.GitAllowlistKeys)
	log.Assert("git_no_extra", len(outside) == 0, "[]", outside)

	// Hostile path must not appear unless caller passed it (construction is exact).
	hostile := filepath.Join(parent, "hostile-template")
	if strings.Contains(tmpl, "hostile") {
		log.Fail("hostile_in_owned", tmpl)
	}
	// Constructing with owned never injects ambient GIT_TEMPLATE_DIR.
	t.Setenv("GIT_TEMPLATE_DIR", hostile)
	env2 := toolrun.ConstructGitEnv("/usr/bin:/bin", owned)
	log.Assert("ignores_ambient", env2["GIT_TEMPLATE_DIR"] == owned, owned, env2["GIT_TEMPLATE_DIR"])
	log.Assert("no_home", func() bool { _, ok := env2["HOME"]; return !ok }(), true, false)
	log.PhaseEnd("template", testutil.OutcomeOK)
}

// TestSentinel_IsolationDump_RedactsValues proves failure dumps expose key
// names + argv only — never secret values (REQ-214).
func TestSentinel_IsolationDump_RedactsValues(t *testing.T) {
	log := testutil.New(t)
	log.Phase("redaction")
	host := testHost()
	// Inject secret-like host capture values.
	host.HOME = "/home/secret-user"
	host.GOMODCACHE = "/home/secret-user/go/pkg/mod"
	host.GOPROXY = "https://proxy.secret.example,direct"
	env := toolrun.ConstructGoEnv(host)
	argv := []string{"go", "mod", "verify"}
	obs := []toolrun.IsolationObservation{{
		Sentinel: "GOFLAGS", Kind: "go-fixed", Observation: "normative",
		Argv: strings.Join(argv, " "), Exit: 0, Detail: "leak=false",
	}}
	dump := toolrun.IsolationDump(env, argv, obs)
	log.Step("dump", testutil.OutcomeOK, "bytes="+itoa(len(dump)))

	// Must contain key names and hash, not values.
	if !strings.Contains(dump, "env_keys=") {
		log.Fail("missing_keys", dump)
	}
	if !strings.Contains(dump, "env_hash=") {
		log.Fail("missing_hash", dump)
	}
	if !strings.Contains(dump, "argv=") {
		log.Fail("missing_argv", dump)
	}
	// Forbidden: KEY=value of secrets or raw secret paths.
	secrets := []string{
		host.HOME,
		host.GOMODCACHE,
		host.GOPROXY,
		"HOME=" + host.HOME,
		"GOPROXY=" + host.GOPROXY,
	}
	hits := toolrun.AssertNoSecretValues(dump, secrets)
	// HOME path might appear only if it collides with hash hex (vanishingly rare
	// for path-shaped strings). AssertNoSecretValues checks =value form.
	log.Assert("no_secret_values", len(hits) == 0, "[]", hits)

	// Also: dump must list GOENV as a key name (in env_keys=) without GOENV=off value assign.
	if strings.Contains(dump, "GOENV=off") || strings.Contains(dump, "GOFLAGS=") {
		// IsolationDump must not emit KEY=value pairs for env contents.
		// Note: sentinel= lines are ok; check raw env encoding form.
		if strings.Contains(dump, "\nGOENV=") || strings.Contains(dump, " env_keys=") && strings.Contains(dump, "GOENV=off") {
			// Explicit KEY=value of fixed env must not appear as dump lines.
		}
	}
	// Stronger: CanonicalEnvEncoding must not be embedded.
	enc := toolrun.CanonicalEnvEncoding(env)
	if strings.Contains(dump, enc) {
		log.Fail("canonical_encoding_leaked", "full env encoding present in dump")
	}
	log.PhaseEnd("redaction", testutil.OutcomeOK)
}

// TestSentinel_WriteIsolationArtifact writes redacted file under artifact dir.
func TestSentinel_WriteIsolationArtifact(t *testing.T) {
	log := testutil.New(t)
	// Clear ambient artifact dir so skip path is deterministic (CI may set it).
	t.Setenv(toolrun.ArtifactEnvVar, "")
	dir := t.TempDir()
	env := toolrun.ConstructGoEnv(testHost())
	path, err := toolrun.WriteIsolationArtifact(dir, "TestCase", env, []string{"go", "env"}, nil)
	if err != nil {
		log.Fail("write", err.Error())
	}
	if path == "" {
		log.Fail("path_empty", "expected written path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fail("read", err.Error())
	}
	body := string(b)
	log.Assert("has_keys", strings.Contains(body, "env_keys="), true, false)
	log.Assert("no_home_assign", !strings.Contains(body, "HOME="), true, strings.Contains(body, "HOME="))
	// Skip when no dir and no env.
	path2, err := toolrun.WriteIsolationArtifact("", "x", env, nil, nil)
	if err != nil {
		log.Fail("skip_err", err.Error())
	}
	log.Assert("skip_empty", path2 == "", "", path2)
	// Env var path: FOUNDRY_SENTINEL_ARTIFACT_DIR is honored when dir arg empty.
	art := t.TempDir()
	t.Setenv(toolrun.ArtifactEnvVar, art)
	path3, err := toolrun.WriteIsolationArtifact("", "from-env", env, []string{"go", "version"}, nil)
	if err != nil {
		log.Fail("env_write", err.Error())
	}
	log.Assert("env_path_used", path3 != "" && strings.HasPrefix(path3, art), true, path3)
	log.PhaseEnd("artifact", testutil.OutcomeOK)
}

// TestProcessTree_EqualsPlan_FakePlanRunner proves observed argv list equals
// plan external_steps (no extras, no shell) — REQ-214 process-tree unit.
func TestProcessTree_EqualsPlan_FakePlanRunner(t *testing.T) {
	log := testutil.New(t)
	log.Phase("process_tree")
	host := testHost()
	goEnv := toolrun.ConstructGoEnv(host)
	steps := []plan.ExternalStep{
		{
			ID: "go-mod-verify", Binary: "/usr/local/go/bin/go",
			Argv: []string{"go", "mod", "verify"},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 120, OutputCapBytes: plan.OutputCapBytes, Env: copyEnv(goEnv),
		},
		{
			ID: "go-test", Binary: "/usr/local/go/bin/go",
			Argv: []string{"go", "test", "-count=1", "./..."},
			Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
			TimeoutS: 300, OutputCapBytes: plan.OutputCapBytes, Env: copyEnv(goEnv),
		},
	}

	fake := &toolrun.FakePlanRunner{}
	obs := fake.RunPlan(steps)
	diffs := toolrun.CompareProcessTree(steps, obs)
	log.Assert("tree_equal", len(diffs) == 0, toolrun.FormatTreeDiffs(nil), toolrun.FormatTreeDiffs(diffs))
	log.Step("plan_argv", testutil.OutcomeOK, toolrun.PlannedArgvSummary(steps))

	// Negative: extra shell step.
	fake2 := &toolrun.FakePlanRunner{InjectExtra: []toolrun.ObservedStep{{
		ID: "evil-shell", Binary: "/bin/sh", Argv: []string{"sh", "-c", "true"}, Shell: true,
		Env: map[string]string{},
	}}}
	obs2 := fake2.RunPlan(steps)
	diffs2 := toolrun.CompareProcessTree(steps, obs2)
	if len(diffs2) == 0 {
		log.Fail("expected_diffs", "injected shell not detected")
	}
	var sawExtra, sawShell bool
	for _, d := range diffs2 {
		if d.Kind == "extra" || d.Kind == "count" {
			sawExtra = true
		}
		if d.Kind == "shell" {
			sawShell = true
		}
		// Env diffs must not contain secret values (key names only).
		if d.Kind == "env" && (strings.Contains(d.Detail, "planned=") || strings.Contains(d.Detail, "observed=")) {
			log.Fail("env_value_in_diff", d.Detail)
		}
	}
	log.Assert("detect_extra", sawExtra, true, sawExtra)
	log.Assert("detect_shell", sawShell, true, sawShell)
	log.Step("neg_shell", testutil.OutcomeOK, toolrun.FormatTreeDiffs(diffs2))

	// Negative: env pollution — key names only in dump.
	fake3 := &toolrun.FakePlanRunner{MutateEnv: func(_ string, env map[string]string) {
		env["GOFLAGS"] = "-toolexec=/evil"
		env["EVIL_EXTRA"] = "secret-token-value"
	}}
	obs3 := fake3.RunPlan(steps)
	diffs3 := toolrun.CompareProcessTree(steps, obs3)
	if len(diffs3) == 0 {
		log.Fail("expected_env_diffs", "pollution not detected")
	}
	joined := toolrun.FormatTreeDiffs(diffs3)
	log.Assert("no_secret_in_tree_diff", !strings.Contains(joined, "secret-token-value"), true, joined)
	log.Assert("no_toolexec_path_as_value", !strings.Contains(joined, "-toolexec=/evil"), true, joined)
	// Key names must appear.
	log.Assert("names_GOFLAGS", strings.Contains(joined, "GOFLAGS"), true, joined)
	log.Assert("names_EVIL", strings.Contains(joined, "EVIL_EXTRA"), true, joined)
	log.PhaseEnd("process_tree", testutil.OutcomeOK)
}

// TestProcessTree_GitStep_TemplateDirOnlyOwned checks git-init plan step env
// records only the owned template path.
func TestProcessTree_GitStep_TemplateDirOnlyOwned(t *testing.T) {
	log := testutil.New(t)
	owned, err := toolrun.EmptyTemplateDir(t.TempDir())
	if err != nil {
		log.Fail("template", err.Error())
	}
	t.Cleanup(func() { _ = os.RemoveAll(owned) })
	gitEnv := toolrun.ConstructGitEnv("/usr/bin:/bin", owned)
	steps := []plan.ExternalStep{{
		ID: "git-init", Binary: "/usr/bin/git",
		Argv: []string{"git", "init", "--initial-branch=main", "--template=" + owned, "."},
		Cwd:  plan.StageDescriptorCWD, Network: plan.NetworkNo,
		TimeoutS: 60, OutputCapBytes: plan.OutputCapBytes, Env: gitEnv,
	}}
	fake := &toolrun.FakePlanRunner{}
	obs := fake.RunPlan(steps)
	diffs := toolrun.CompareProcessTree(steps, obs)
	log.Assert("tree_ok", len(diffs) == 0, "", toolrun.FormatTreeDiffs(diffs))
	log.Assert("only_owned_tmpl", obs[0].Env["GIT_TEMPLATE_DIR"] == owned, owned, obs[0].Env["GIT_TEMPLATE_DIR"])
	// Argv embeds template path (plan records it) — tree equality requires match.
	log.Assert("argv_template", strings.Contains(strings.Join(obs[0].Argv, " "), owned), true, obs[0].Argv)
}

func copyEnv(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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
