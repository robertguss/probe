package toolrun_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

func TestPreflight_Success_FakeRunner(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	goPath := "/fake/bin/go"
	gitPath := "/fake/bin/git"
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{
			"go":  goPath,
			"git": gitPath,
		},
		Outputs: map[string]string{
			goPath:  "go version go1.26.5 linux/amd64\n",
			gitPath: "git version 2.43.0\n",
		},
	}
	host := testHost()
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:             host,
		RequireGoVersion: "go1.26.5",
		RequireGit:       true,
		Runner:           fake,
	})
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	log.Assert("ok", res.OK(), true, res.OK())
	if err := res.Err(); err != nil {
		log.Fail("err_nil", err.Error())
	}
	log.Assert("go_path", res.GoPath == goPath, goPath, res.GoPath)
	log.Assert("git_path", res.GitPath == gitPath, gitPath, res.GitPath)
	log.Assert("go_version", res.GoVersion == "go1.26.5", "go1.26.5", res.GoVersion)
	log.Assert("fail_class_empty", res.FailClass == "", "", res.FailClass)
	log.Assert("env_hash_len", len(res.GoEnvHash) == 64, 64, len(res.GoEnvHash))

	// Closed env used for go version is the first Run (git --version is second).
	if len(fake.ObservedEnv) < 1 {
		log.Fail("observed_env", "no runs recorded")
	}
	obs := envSliceToMapTest(fake.ObservedEnv[0])
	if bad := toolrun.KeysOutsideAllowlist(obs, toolrun.GoAllowlistKeys); len(bad) > 0 {
		log.Fail("observed_bleed", strings.Join(bad, ","))
	}
	log.Assert("observed_GOENV", obs["GOENV"] == "off", "off", obs["GOENV"])
	log.Assert("observed_CGO", obs["CGO_ENABLED"] == "0", "0", obs["CGO_ENABLED"])
	log.Assert("observed_GOTOOLCHAIN", obs["GOTOOLCHAIN"] == "local", "local", obs["GOTOOLCHAIN"])
	log.Assert("observed_GOWORK", obs["GOWORK"] == "off", "off", obs["GOWORK"])
	log.Assert("observed_GOFLAGS_empty", obs["GOFLAGS"] == "", "", obs["GOFLAGS"])
	log.Assert("observed_GOCACHEPROG_empty", obs["GOCACHEPROG"] == "", "", obs["GOCACHEPROG"])

	// Log fields never carry env values.
	fields := res.LogFields()
	log.Inputs(fields)
	for k, v := range fields {
		if strings.Contains(v, "/home/foundry/go/pkg/mod") {
			t.Fatalf("log field %s leaked cache path: %s", k, v)
		}
		if strings.Contains(k, "GOFLAGS") || strings.HasPrefix(v, "GOENV=") {
			t.Fatalf("log field leaked env assign: %s=%s", k, v)
		}
	}
	log.Step("preflight_log", testutil.OutcomeOK,
		"go_path="+fields["go_path"]+
			" go_version="+fields["go_version"]+
			" env_hash="+fields["env_hash"]+
			" fail_class="+fields["fail_class"])
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestPreflight_MissingBinary(t *testing.T) {
	log := testutil.New(t)
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{}, // nothing
		Outputs: map[string]string{},
	}
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:   testHost(),
		Runner: fake,
	})
	log.Assert("not_ok", !res.OK(), false, res.OK())
	log.Assert("fail_class", res.FailClass == toolrun.FailClassMissing, toolrun.FailClassMissing, res.FailClass)
	log.Assert("fail_step", res.FailStep == toolrun.StepGoPreflight, toolrun.StepGoPreflight, res.FailStep)
	fe, ok := diagnostic.AsFoundryError(res.Err())
	if !ok {
		log.Fail("foundry_error", "expected FoundryError")
	}
	log.Assert("id", fe.ID() == diagnostic.IDToolMissing, diagnostic.IDToolMissing, fe.ID())
	log.Assert("exit", fe.ExitCode() == 1, 1, fe.ExitCode())
}

func TestPreflight_WrongVersion(t *testing.T) {
	log := testutil.New(t)
	goPath := "/fake/bin/go"
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{"go": goPath},
		Outputs: map[string]string{
			goPath: "go version go1.22.0 linux/amd64\n",
		},
	}
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:             testHost(),
		RequireGoVersion: "go1.26.5",
		Runner:           fake,
	})
	log.Assert("not_ok", !res.OK(), false, res.OK())
	log.Assert("fail_class", res.FailClass == toolrun.FailClassWrongVersion,
		toolrun.FailClassWrongVersion, res.FailClass)
	log.Assert("observed", res.GoVersion == "go1.22.0", "go1.22.0", res.GoVersion)
	fe, ok := diagnostic.AsFoundryError(res.Err())
	if !ok {
		log.Fail("foundry_error", "expected FoundryError")
	}
	log.Assert("id", fe.ID() == diagnostic.IDToolWrongVersion, diagnostic.IDToolWrongVersion, fe.ID())
	if !strings.Contains(fe.Message(), "go1.22.0") || !strings.Contains(fe.Message(), "go1.26.5") {
		t.Fatalf("message should name both versions: %s", fe.Message())
	}
	// Fail-closed: no partial success.
	if res.GitPath != "" {
		t.Fatal("git must not be resolved after go version failure")
	}
}

func TestPreflight_NonExecutable(t *testing.T) {
	log := testutil.New(t)
	dir := t.TempDir()
	path, err := toolrun.WriteNonExecutable(dir, "go", "#!/bin/sh\necho no\n")
	if err != nil {
		t.Fatal(err)
	}
	// Absolute path to non-executable → tool.missing before Run.
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:     testHost(),
		GoBinary: path,
		Runner:   &toolrun.FakeRunner{PathMap: map[string]string{}, Outputs: map[string]string{}},
	})
	log.Assert("not_ok", !res.OK(), false, res.OK())
	log.Assert("fail_class", res.FailClass == toolrun.FailClassMissing, toolrun.FailClassMissing, res.FailClass)
	fe, ok := diagnostic.AsFoundryError(res.Err())
	if !ok {
		log.Fail("foundry_error", "expected FoundryError")
	}
	log.Assert("id", fe.ID() == diagnostic.IDToolMissing, diagnostic.IDToolMissing, fe.ID())
	if !strings.Contains(fe.Message(), "not executable") && !strings.Contains(fe.Error(), "not executable") {
		// ensureExecutable error is wrapped into message
		if !strings.Contains(fe.Error(), "not executable") {
			t.Fatalf("expected not executable in error: %v", fe)
		}
	}
}

func TestPreflight_MissingGitWhenRequired(t *testing.T) {
	goPath := "/fake/bin/go"
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{"go": goPath},
		Outputs: map[string]string{goPath: "go version go1.26.5 linux/amd64\n"},
	}
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:       testHost(),
		RequireGit: true,
		Runner:     fake,
	})
	if res.OK() {
		t.Fatal("expected failure when git required but missing")
	}
	if res.FailClass != toolrun.FailClassMissing {
		t.Fatalf("fail class %q", res.FailClass)
	}
	if res.FailStep != toolrun.StepGitPreflight {
		t.Fatalf("fail step %q", res.FailStep)
	}
	fe, ok := diagnostic.AsFoundryError(res.Err())
	if !ok || fe.ID() != diagnostic.IDToolMissing {
		t.Fatalf("want tool.missing, got %v", res.Err())
	}
}

func TestPreflight_ProbeFailure_ToolMissing(t *testing.T) {
	goPath := "/fake/bin/go"
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{"go": goPath},
		Outputs: map[string]string{goPath: ""},
		ExitErr: map[string]error{goPath: errors.New("exit status 1")},
	}
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:   testHost(),
		Runner: fake,
	})
	if res.OK() || res.FailClass != toolrun.FailClassMissing {
		t.Fatalf("want tool.missing on probe failure, got ok=%v class=%s err=%v", res.OK(), res.FailClass, res.Err())
	}
}

func TestPreflight_Success_RealFakeBinaryOnPATH(t *testing.T) {
	// Exercises OSRunner + real executable scripts (not FakeRunner).
	log := testutil.New(t)
	dir := t.TempDir()
	goBin, err := toolrun.WriteFakeBinary(dir, "go", "go version go1.26.5 linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	gitBin, err := toolrun.WriteFakeBinary(dir, "git", "git version 2.99.0")
	if err != nil {
		t.Fatal(err)
	}
	_ = goBin
	_ = gitBin

	host := testHost()
	host.PATH = dir
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:             host,
		RequireGoVersion: "go1.26.5",
		RequireGit:       true,
		// Runner nil → OSRunner with Host.PATH
	})
	log.Assert("ok", res.OK(), true, res.OK())
	if !res.OK() {
		log.Fail("preflight", res.Err().Error())
	}
	log.Assert("go_version", res.GoVersion == "go1.26.5", "go1.26.5", res.GoVersion)
	if !strings.Contains(res.GitVersion, "2.99.0") {
		t.Fatalf("git version %q", res.GitVersion)
	}
	// Resolved paths under temp dir.
	if filepath.Dir(res.GoPath) != dir {
		t.Fatalf("go path %q not under %s", res.GoPath, dir)
	}
	log.Step("real_fake_ok", testutil.OutcomeOK,
		"go_path="+res.GoPath+" git_path="+res.GitPath+" env_hash="+res.GoEnvHash)
}

func TestPreflight_DefaultPinnedTag(t *testing.T) {
	if toolrun.DefaultPinnedGoTag != "go1.26.5" {
		t.Fatalf("DefaultPinnedGoTag=%q", toolrun.DefaultPinnedGoTag)
	}
	// Empty RequireGoVersion uses default; wrong output fails closed.
	goPath := "/fake/bin/go"
	fake := &toolrun.FakeRunner{
		PathMap: map[string]string{"go": goPath},
		Outputs: map[string]string{goPath: "go version go1.26.4 linux/amd64\n"},
	}
	res := toolrun.Preflight(context.Background(), toolrun.PreflightOptions{
		Host:   testHost(),
		Runner: fake,
	})
	if res.OK() || res.FailClass != toolrun.FailClassWrongVersion {
		t.Fatalf("nearby version must fail: ok=%v class=%s", res.OK(), res.FailClass)
	}
	if res.RequiredGo != toolrun.DefaultPinnedGoTag {
		t.Fatalf("required %q", res.RequiredGo)
	}
}

func TestParseGoVersionOutput(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"go version go1.26.5 linux/amd64\n", "go1.26.5", false},
		{"go version go1.26.5 linux/amd64", "go1.26.5", false},
		{"go version go1.22.0 windows/amd64", "go1.22.0", false},
		{"go version devel go1.27-abc linux/amd64", "", true}, // non-go* first token rejected
		{"", "", true},
		{"not a version", "", true},
	}
	for _, tc := range tests {
		got, err := toolrun.ParseGoVersionOutput(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("input %q: want err", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("input %q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("input %q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestOSRunner_LookPathAbsoluteMissing(t *testing.T) {
	r := toolrun.OSRunner{}
	_, err := r.LookPath(filepath.Join(t.TempDir(), "no-such-go"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteFakeBinary_Executable(t *testing.T) {
	dir := t.TempDir()
	p, err := toolrun.WriteFakeBinary(dir, "tool", "hello")
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&0o111 == 0 {
		t.Fatal("not executable")
	}
}

func envSliceToMapTest(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return m
}
