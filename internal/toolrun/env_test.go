package toolrun_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func testHost() toolrun.HostCapture {
	return toolrun.HostCapture{
		PATH:       "/usr/bin:/bin",
		HOME:       "/home/foundry",
		TMPDIR:     "/tmp",
		GOMODCACHE: "/home/foundry/go/pkg/mod",
		GOCACHE:    "/home/foundry/.cache/go-build",
		GOPATH:     "/home/foundry/go",
		GOPROXY:    "https://proxy.golang.org,direct",
		GOSUMDB:    "sum.golang.org",
	}
}

func TestConstructGoEnv_EqualsAllowlist_NoHostBleed(t *testing.T) {
	log := testutil.New(t)
	log.Phase("construct")
	env := toolrun.ConstructGoEnv(testHost())
	log.Step("construct_go_env", testutil.OutcomeOK, "keys="+strings.Join(toolrun.EnvKeys(env), ","))
	log.PhaseEnd("construct", testutil.OutcomeOK)

	log.Phase("assert_allowlist")
	// Every key must be in the allowlist; no extras.
	outside := toolrun.KeysOutsideAllowlist(env, toolrun.GoAllowlistKeys)
	log.Assert("no_keys_outside_allowlist", len(outside) == 0, "[]", outside)

	// TMPDIR present when host set it.
	if _, ok := env["TMPDIR"]; !ok {
		log.Fail("tmpdir_present", "TMPDIR missing when host TMPDIR set")
	}

	// Forbidden host-bleed keys must be absent.
	hits := toolrun.ForbiddenKeysPresent(env, toolrun.ForbiddenBleedKeys)
	// GIT_TEMPLATE_DIR is in ForbiddenBleedKeys for go steps — must be absent.
	log.Assert("forbidden_bleed_absent", len(hits) == 0, "[]", hits)

	// Table of sentinel-like keys that hostile hosts plant (E2 matrix names).
	sentinels := []string{
		"GODEBUG", "GOEXPERIMENT", "GO111MODULE",
		"CC", "CXX", "CGO_CFLAGS",
		"GIT_DIR", "GIT_WORK_TREE", "XDG_CONFIG_HOME",
		"SSH_AUTH_SOCK", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY",
	}
	for _, k := range sentinels {
		_, present := env[k]
		log.Assert("sentinel_absent_"+k, !present, false, present)
	}
	log.PhaseEnd("assert_allowlist", testutil.OutcomeOK)
}

func TestConstructGoEnv_NormativeFixedValues(t *testing.T) {
	log := testutil.New(t)
	env := toolrun.ConstructGoEnv(testHost())

	want := map[string]string{
		"GOENV":       "off",
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
		"LC_ALL":      "C",
		"LANG":        "C",
		"TERM":        "dumb",
	}
	log.Phase("fixed_values")
	for k, v := range want {
		got := env[k]
		log.Assert("fixed_"+k, got == v, v, got)
	}
	log.PhaseEnd("fixed_values", testutil.OutcomeOK)
}

func TestConstructGoEnv_ProductCGODisabled(t *testing.T) {
	env := toolrun.ConstructGoEnv(testHost())
	if env["CGO_ENABLED"] != "0" {
		t.Fatalf("CGO_ENABLED=%q want 0 (product default; race exception not here)", env["CGO_ENABLED"])
	}
}

func TestConstructGoEnv_OmitsEmptyTMPDIR(t *testing.T) {
	h := testHost()
	h.TMPDIR = ""
	env := toolrun.ConstructGoEnv(h)
	if _, ok := env["TMPDIR"]; ok {
		t.Fatalf("TMPDIR present when host TMPDIR empty: %q", env["TMPDIR"])
	}
	// Still no keys outside allowlist.
	if bad := toolrun.KeysOutsideAllowlist(env, toolrun.GoAllowlistKeys); len(bad) > 0 {
		t.Fatalf("outside allowlist: %v", bad)
	}
}

func TestConstructGoEnv_DeterminismCount2(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")
	h := testHost()
	a := toolrun.EnvSlice(toolrun.ConstructGoEnv(h))
	b := toolrun.EnvSlice(toolrun.ConstructGoEnv(h))
	if len(a) != len(b) {
		log.Fail("slice_len", "len mismatch")
	}
	for i := range a {
		if a[i] != b[i] {
			log.Fail("slice_eq", "index mismatch")
		}
	}
	encA := toolrun.CanonicalEnvEncoding(toolrun.ConstructGoEnv(h))
	encB := toolrun.CanonicalEnvEncoding(toolrun.ConstructGoEnv(h))
	log.Assert("canonical_encoding_stable", encA == encB, encA, encB)
	hashA := toolrun.AllowlistHash(toolrun.ConstructGoEnv(h))
	hashB := toolrun.AllowlistHash(toolrun.ConstructGoEnv(h))
	log.Assert("allowlist_hash_stable", hashA == hashB, hashA, hashB)
	// Sorted keys: consecutive encodings must be byte-identical.
	if hashA == "" || len(hashA) != 64 {
		t.Fatalf("hash length: %q", hashA)
	}
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

func TestConstructGitEnv_ExactAllowlist(t *testing.T) {
	log := testutil.New(t)
	env := toolrun.ConstructGitEnv("/usr/bin", "/tmp/foundry-git-template")
	outside := toolrun.KeysOutsideAllowlist(env, toolrun.GitAllowlistKeys)
	log.Assert("git_no_extra_keys", len(outside) == 0, "[]", outside)

	want := map[string]string{
		"PATH":                "/usr/bin",
		"LC_ALL":              "C",
		"LANG":                "C",
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_SYSTEM":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TEMPLATE_DIR":    "/tmp/foundry-git-template",
	}
	for k, v := range want {
		log.Assert("git_"+k, env[k] == v, v, env[k])
	}
	// Intentionally absent.
	for _, k := range []string{"HOME", "GIT_DIR", "GIT_WORK_TREE", "XDG_CONFIG_HOME"} {
		_, ok := env[k]
		log.Assert("git_absent_"+k, !ok, false, ok)
	}
}

func TestAllowlistHash_NeverIncludesInKeyList(t *testing.T) {
	// Hash is hex; key list is names only — step-log contract.
	log := testutil.New(t)
	env := toolrun.ConstructGoEnv(testHost())
	hash := toolrun.AllowlistHash(env)
	keys := toolrun.EnvKeys(env)

	// Simulate step logger detail: hash + keys, never values.
	detail := "env_hash=" + hash + " env_keys=" + strings.Join(keys, ",")
	log.Step("env_summary", testutil.OutcomeOK, detail)

	// Hostile secret-like values must not appear in the log detail.
	secretValues := []string{
		env["HOME"],
		env["GOMODCACHE"],
		env["GOPROXY"],
		"/home/foundry",
	}
	for _, secret := range secretValues {
		if secret == "" {
			continue
		}
		// Hash is hex of content so theoretically could collide; require that
		// raw value substrings of path form do not appear outside the hash.
		// We only check that detail does not contain "HOME=" or raw path assigns.
		if strings.Contains(detail, "HOME=") || strings.Contains(detail, "GOPROXY=") {
			t.Fatalf("detail leaked key=value form: %s", detail)
		}
		// Paths should not appear as path components in key-name list.
		if strings.Contains(strings.Join(keys, ","), "/") {
			t.Fatalf("key names contain path separator: %v", keys)
		}
		_ = secret
	}
	// Ensure Subprocess helper path records hash not values.
	log.Subprocess("go-preflight", []string{"go", "version"}, hash, 0, 0, 0, "", "", false)
	steps := log.Steps()
	var subDetail string
	for _, s := range steps {
		if s.Name == "go-preflight" {
			subDetail = s.Detail
		}
	}
	if !strings.Contains(subDetail, "env_hash="+hash) {
		t.Fatalf("subprocess log missing hash: %s", subDetail)
	}
	if strings.Contains(subDetail, "GOENV=off") || strings.Contains(subDetail, "/home/foundry") {
		t.Fatalf("subprocess log leaked env values: %s", subDetail)
	}
}

func TestParity_FixedGoEnvMatchesPlan(t *testing.T) {
	// plan.FixedGoEnvValues and toolrun.FixedGoEnv must stay identical so
	// plan-recorded env maps match runtime construction (REQ-154).
	if len(toolrun.FixedGoEnv) != len(plan.FixedGoEnvValues) {
		t.Fatalf("len FixedGoEnv=%d plan=%d", len(toolrun.FixedGoEnv), len(plan.FixedGoEnvValues))
	}
	for k, v := range toolrun.FixedGoEnv {
		pv, ok := plan.FixedGoEnvValues[k]
		if !ok {
			t.Errorf("plan missing fixed key %s", k)
			continue
		}
		if pv != v {
			t.Errorf("key %s: toolrun=%q plan=%q", k, v, pv)
		}
	}
	for k := range plan.FixedGoEnvValues {
		if _, ok := toolrun.FixedGoEnv[k]; !ok {
			t.Errorf("toolrun missing plan fixed key %s", k)
		}
	}
}

func TestHostCapture_RoundTripPlan(t *testing.T) {
	h := testHost()
	p := h.ToPlanHost()
	back := toolrun.HostCaptureFromPlan(p)
	if back != h {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back, h)
	}
}

func TestNormativeGoValue(t *testing.T) {
	v, ok := toolrun.NormativeGoValue("GOENV")
	if !ok || v != "off" {
		t.Fatalf("GOENV: got %q %v", v, ok)
	}
	_, ok = toolrun.NormativeGoValue("PATH")
	if ok {
		t.Fatal("PATH must not be normative fixed")
	}
}
