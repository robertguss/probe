//go:build unix

package e3_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/integration/hostile/e3"
	"github.com/robertguss/go-foundry-cli/integration/hostile/e3/knownrace"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	// integration/hostile/e3 → repo root
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func baseEnv() []string {
	// Start from process env but drop shell-alias noise; PATH comes from OS.
	return os.Environ()
}

// TestE3_Preflight_HostCompiler proves preflight succeeds on a well-equipped host.
func TestE3_Preflight_HostCompiler(t *testing.T) {
	log := e3.NewProbeLog()
	env := baseEnv()
	// Ensure we do not force CGO off for the race preflight path.
	env = stripEnv(env, "CGO_ENABLED")

	pf := e3.PreflightWithEnv(env)
	log.Record(e3.ProbeEntry{
		Probe: "preflight", Step: "host_compiler", Outcome: outcome(pf.OK),
		CGO: pf.CGOEnabled, Compiler: pf.CompilerPath,
		Skip:   pf.FormatSkipNotice(),
		Detail: pf.Detail + "; version=" + pf.CompilerOut,
		Args:   "PreflightWithEnv(host)",
	})

	if !pf.OK {
		t.Fatalf("expected preflight OK on host with gcc/clang; got skip=%q detail=%q",
			pf.FormatSkipNotice(), pf.Detail)
	}
	if pf.CompilerPath == "" {
		t.Fatal("OK preflight must record CompilerPath")
	}
	if pf.CGOEnabled != "1" {
		t.Fatalf("race preflight must set CGO_ENABLED=1, got %q", pf.CGOEnabled)
	}
	if pf.FormatSkipNotice() != "" {
		t.Fatalf("OK preflight must not emit skip notice, got %q", pf.FormatSkipNotice())
	}
	if pf.CompilerOut == "" {
		t.Log("warning: compiler version line empty (non-fatal)")
	}

	pass, fail := log.SummaryPassFail()
	if fail != 0 || pass < 1 {
		t.Fatalf("probe log: pass=%d fail=%d", pass, fail)
	}
}

// TestE3_Preflight_MissingCompiler_SkipNotice proves missing compiler → explicit skip, never silent pass.
func TestE3_Preflight_MissingCompiler_SkipNotice(t *testing.T) {
	log := e3.NewProbeLog()

	// Empty PATH and no CC → cannot find a compiler.
	env := []string{
		"PATH=/nonexistent-e3-empty-path",
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
	}

	pf := e3.PreflightWithEnv(env)
	notice := pf.FormatSkipNotice()
	log.Record(e3.ProbeEntry{
		Probe: "preflight", Step: "missing_compiler", Outcome: "pass",
		CGO: pf.CGOEnabled, Compiler: pf.Compiler,
		Skip:   notice,
		Detail: pf.Detail,
		Args:   "PATH=/nonexistent-e3-empty-path",
	})

	if pf.OK {
		t.Fatal("preflight must not be OK with empty PATH and no CC")
	}
	if notice == "" {
		t.Fatal("missing compiler must produce non-empty skip notice (never silent pass)")
	}
	if notice != e3.SkipNoticeMissingCompiler {
		t.Fatalf("skip notice golden mismatch:\n got: %q\nwant: %q", notice, e3.SkipNoticeMissingCompiler)
	}
	if !strings.Contains(notice, "not a silent pass") {
		t.Fatalf("skip notice must state it is not a silent pass: %q", notice)
	}

	// Golden substring for CI templates.
	if !strings.Contains(notice, "race detector skipped") {
		t.Fatalf("skip notice must start with stable prefix: %q", notice)
	}
}

// TestE3_Preflight_CGODisabled_SkipNotice proves CGO_ENABLED=0 → explicit skip.
func TestE3_Preflight_CGODisabled_SkipNotice(t *testing.T) {
	env := append(baseEnv(), "CGO_ENABLED=0")
	// PreflightWithEnv checks CGO_ENABLED=0 before compiler lookup.
	pf := e3.PreflightWithEnv(env)
	if pf.OK {
		t.Fatal("CGO_ENABLED=0 must not pass race preflight")
	}
	if pf.FormatSkipNotice() != e3.SkipNoticeCGODisabled {
		t.Fatalf("CGO skip notice mismatch: %q", pf.FormatSkipNotice())
	}
	e3.NewProbeLog().Record(e3.ProbeEntry{
		Probe: "preflight", Step: "cgo_disabled", Outcome: "pass",
		CGO: "0", Skip: pf.FormatSkipNotice(), Detail: pf.Detail,
	})
}

// TestE3_Preflight_BrokenCompiler_SkipNotice proves a non-working CC is not a silent pass.
func TestE3_Preflight_BrokenCompiler_SkipNotice(t *testing.T) {
	dir := t.TempDir()
	// Script that exists and is executable but fails compilation.
	bad := filepath.Join(dir, "badcc")
	script := "#!/bin/sh\necho 'badcc: refusing compile' >&2\nexit 1\n"
	if err := os.WriteFile(bad, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + dir,
		"CC=" + bad,
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
	}
	pf := e3.PreflightWithEnv(env)
	if pf.OK {
		t.Fatal("broken compiler must not pass preflight")
	}
	notice := pf.FormatSkipNotice()
	if notice != e3.SkipNoticeMissingCompiler {
		t.Fatalf("broken compiler skip notice: %q", notice)
	}
	e3.NewProbeLog().Record(e3.ProbeEntry{
		Probe: "preflight", Step: "broken_compiler", Outcome: "pass",
		Compiler: bad, Skip: notice, Detail: pf.Detail,
	})
}

// TestE3_KnownRace_Detected runs the known-race fixture under -race and expects DATA RACE.
func TestE3_KnownRace_Detected(t *testing.T) {
	log := e3.NewProbeLog()
	env := stripEnv(baseEnv(), "CGO_ENABLED")
	pf := e3.PreflightWithEnv(env)
	if !pf.OK {
		// Host cannot race — record skip notice and fail the probe suite so
		// CI does not silently green this evidence on an unequipped machine.
		// (The dedicated skip-notice unit tests still pass without a compiler.)
		log.Record(e3.ProbeEntry{
			Probe: "known_race", Step: "preflight", Outcome: "skip",
			Skip: pf.FormatSkipNotice(), Detail: pf.Detail,
		})
		t.Fatalf("cannot prove known-race fixture without compiler: %s", pf.FormatSkipNotice())
	}

	log.Record(e3.ProbeEntry{
		Probe: "known_race", Step: "preflight", Outcome: "pass",
		CGO: "1", Compiler: pf.CompilerPath, Detail: pf.Detail,
	})

	root := repoRoot(t)
	pkg := "./integration/hostile/e3/knownrace/"
	args := []string{"test", "-race", "-count=1", "-timeout", "60s", pkg}

	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = e3.SanitizeEnvForRace(env, pf)
	// Pin toolchain like other evidence spikes; arm intentional race body.
	cmd.Env = append(cmd.Env, "GOTOOLCHAIN=go1.26.5", knownrace.EnvKnownRace+"=1")

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()

	log.Record(e3.ProbeEntry{
		Probe: "known_race", Step: "go_test_race",
		Args: strings.Join(append([]string{"go"}, args...), " ") + " " + knownrace.EnvKnownRace + "=1",
		CGO:  "1", Compiler: pf.CompilerPath,
		Outcome: "info",
		Detail:  truncate(out, 2000),
	})

	if err == nil {
		log.Record(e3.ProbeEntry{
			Probe: "known_race", Step: "expect_data_race", Outcome: "fail",
			Detail: "go test -race exited 0; instrumentation inactive or fixture fixed",
		})
		t.Fatalf("known-race fixture must fail under -race; output:\n%s", out)
	}
	if !strings.Contains(out, "DATA RACE") {
		log.Record(e3.ProbeEntry{
			Probe: "known_race", Step: "expect_data_race", Outcome: "fail",
			Errno:  err.Error(),
			Detail: "exit non-zero but no DATA RACE marker; output:\n" + truncate(out, 1500),
		})
		t.Fatalf("expected DATA RACE in output; err=%v\n%s", err, out)
	}

	log.Record(e3.ProbeEntry{
		Probe: "known_race", Step: "expect_data_race", Outcome: "pass",
		Detail: "DATA RACE reported; race instrumentation active",
	})
}

// TestE3_KnownRace_WithoutRaceFlag typically passes (proves -race is required).
func TestE3_KnownRace_WithoutRaceFlag(t *testing.T) {
	root := repoRoot(t)
	pkg := "./integration/hostile/e3/knownrace/"
	cmd := exec.Command("go", "test", "-count=1", "-timeout", "30s", pkg)
	cmd.Dir = root
	cmd.Env = append(stripEnv(baseEnv(), "CGO_ENABLED"), "CGO_ENABLED=0", "GOTOOLCHAIN=go1.26.5")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	e3.NewProbeLog().Record(e3.ProbeEntry{
		Probe: "known_race", Step: "without_race_flag",
		Args:    "go test -count=1 (no -race) CGO_ENABLED=0",
		CGO:     "0",
		Outcome: outcome(err == nil),
		Detail:  truncate(out, 500),
	})
	if err != nil {
		t.Fatalf("uninstrumented knownrace tests should pass: %v\n%s", err, out)
	}
	if strings.Contains(out, "DATA RACE") {
		t.Fatal("DATA RACE must not appear without -race")
	}
}

// TestE3_GenerationGate_NeverIncludesRace proves Section 35.4 placement.
func TestE3_GenerationGate_NeverIncludesRace(t *testing.T) {
	log := e3.NewProbeLog()

	def := e3.DefaultGenerationGate()
	strict := e3.StrictGenerationGate()

	if e3.GenerationGateContainsRace(def) {
		t.Fatal("default generation gate must not contain race")
	}
	if e3.GenerationGateContainsRace(strict) {
		t.Fatal("strict generation gate must not contain race")
	}

	// Explicit inventory check against Appendix E go-test argv.
	for _, s := range strict {
		if s.IsRace {
			t.Fatalf("step %s marked IsRace", s.ID)
		}
		if strings.Contains(s.Argv, "-race") {
			t.Fatalf("step %s argv contains -race: %s", s.ID, s.Argv)
		}
	}

	// Default go-test is uncached count=1, not race.
	var goTest *e3.GenerationStep
	for i := range def {
		if def[i].ID == "go-test" {
			goTest = &def[i]
			break
		}
	}
	if goTest == nil {
		t.Fatal("default gate missing go-test step")
	}
	if !strings.Contains(goTest.Argv, "-count=1") {
		t.Fatalf("go-test must use -count=1: %s", goTest.Argv)
	}
	if strings.Contains(goTest.Argv, "-race") {
		t.Fatalf("go-test must not use -race: %s", goTest.Argv)
	}

	// Strict adds staticcheck + govulncheck only.
	ids := map[string]bool{}
	for _, s := range strict {
		ids[s.ID] = true
	}
	for _, want := range []string{"go-staticcheck", "go-govulncheck"} {
		if !ids[want] {
			t.Fatalf("strict gate missing %s", want)
		}
	}
	if ids["go-test-race"] || ids["go-race"] {
		t.Fatal("strict generation must not invent a race step id")
	}

	log.Record(e3.ProbeEntry{
		Probe: "generation_gate", Step: "default_and_strict", Outcome: "pass",
		Detail: "Section 35.1/35.2 steps exclude -race; race is CI/strict-workflow only (35.4)",
		Args:   "DefaultGenerationGate+StrictGenerationGate",
	})
}

// TestE3_ProductCGO_Zero proves product-style builds stay CGO_ENABLED=0.
func TestE3_ProductCGO_Zero(t *testing.T) {
	root := repoRoot(t)
	// Compile e3 package (and knownrace) with CGO off — product/release posture.
	outPath := filepath.Join(t.TempDir(), "e3-cgo0.test")
	cmd := exec.Command("go", "test", "-c", "-o", outPath, "./integration/hostile/e3/")
	cmd.Dir = root
	cmd.Env = append(stripEnv(baseEnv(), "CGO_ENABLED"), "CGO_ENABLED=0", "GOTOOLCHAIN=go1.26.5")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("CGO_ENABLED=0 test compile failed: %v\n%s", err, buf.String())
	}
	// Binary must exist.
	if st, err := os.Stat(outPath); err != nil || st.Size() == 0 {
		t.Fatalf("CGO_ENABLED=0 artifact missing: %v", err)
	}
	e3.NewProbeLog().Record(e3.ProbeEntry{
		Probe: "product_cgo", Step: "cgo_enabled_0_compile", Outcome: "pass",
		CGO:    "0",
		Detail: "go test -c with CGO_ENABLED=0 succeeded (product/release posture)",
		Args:   "go test -c -o … ./integration/hostile/e3/",
	})
}

// TestE3_SkipNotice_NeverSilentPass is a table over all failure modes.
func TestE3_SkipNotice_NeverSilentPass(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		want string
	}{
		{
			name: "empty-path",
			env:  []string{"PATH=/no-compilers-here-e3"},
			want: e3.SkipNoticeMissingCompiler,
		},
		{
			name: "cgo-zero",
			env:  []string{"PATH=" + os.Getenv("PATH"), "CGO_ENABLED=0"},
			want: e3.SkipNoticeCGODisabled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf := e3.PreflightWithEnv(tc.env)
			if pf.OK {
				t.Fatal("expected !OK")
			}
			if pf.FormatSkipNotice() != tc.want {
				t.Fatalf("notice=%q want=%q", pf.FormatSkipNotice(), tc.want)
			}
			// Contract: !OK ⇒ non-empty notice (never silent).
			if strings.TrimSpace(pf.FormatSkipNotice()) == "" {
				t.Fatal("silent pass forbidden")
			}
		})
	}
}

// TestE3_Matrix_EndToEnd runs the full evidence matrix with structured logging.
func TestE3_Matrix_EndToEnd(t *testing.T) {
	log := e3.NewProbeLog()
	env := stripEnv(baseEnv(), "CGO_ENABLED")

	// 1. Preflight
	pf := e3.PreflightWithEnv(env)
	log.Record(e3.ProbeEntry{
		Probe: "matrix", Step: "preflight",
		Outcome: outcome(pf.OK), CGO: pf.CGOEnabled,
		Compiler: pf.CompilerPath, Skip: pf.FormatSkipNotice(),
		Detail: pf.Detail + " | " + pf.CompilerOut,
	})
	if !pf.OK {
		t.Fatalf("matrix requires host compiler: %s", pf.FormatSkipNotice())
	}

	// 2. Known race under -race
	root := repoRoot(t)
	raceCmd := exec.Command("go", "test", "-race", "-count=1", "-timeout", "60s",
		"./integration/hostile/e3/knownrace/")
	raceCmd.Dir = root
	raceCmd.Env = append(e3.SanitizeEnvForRace(env, pf),
		"GOTOOLCHAIN=go1.26.5", knownrace.EnvKnownRace+"=1")
	var raceBuf bytes.Buffer
	raceCmd.Stdout = &raceBuf
	raceCmd.Stderr = &raceBuf
	raceErr := raceCmd.Run()
	raceOut := raceBuf.String()
	raceOK := raceErr != nil && strings.Contains(raceOut, "DATA RACE")
	log.Record(e3.ProbeEntry{
		Probe: "matrix", Step: "known_race",
		Outcome: outcome(raceOK), CGO: "1", Compiler: pf.CompilerPath,
		Args:   "go test -race ./integration/hostile/e3/knownrace/",
		Detail: truncate(raceOut, 800),
	})
	if !raceOK {
		t.Fatalf("known race not detected: err=%v\n%s", raceErr, raceOut)
	}

	// 3. Generation gate
	gateOK := !e3.GenerationGateContainsRace(e3.StrictGenerationGate())
	log.Record(e3.ProbeEntry{
		Probe: "matrix", Step: "generation_gate",
		Outcome: outcome(gateOK),
		Detail:  "strict generation steps exclude -race (Section 35.4)",
	})
	if !gateOK {
		t.Fatal("generation gate unexpectedly contains race")
	}

	// 4. Product CGO=0 compile of knownrace
	outBin := filepath.Join(t.TempDir(), "knownrace-cgo0.test")
	c0 := exec.Command("go", "test", "-c", "-o", outBin, "./integration/hostile/e3/knownrace/")
	c0.Dir = root
	c0.Env = append(stripEnv(env, "CGO_ENABLED"), "CGO_ENABLED=0", "GOTOOLCHAIN=go1.26.5")
	var c0buf bytes.Buffer
	c0.Stdout = &c0buf
	c0.Stderr = &c0buf
	if err := c0.Run(); err != nil {
		log.Record(e3.ProbeEntry{
			Probe: "matrix", Step: "product_cgo0", Outcome: "fail",
			CGO: "0", Detail: c0buf.String(),
		})
		t.Fatalf("CGO_ENABLED=0 compile: %v\n%s", err, c0buf.String())
	}
	log.Record(e3.ProbeEntry{
		Probe: "matrix", Step: "product_cgo0", Outcome: "pass", CGO: "0",
		Detail: "knownrace package compiles with CGO_ENABLED=0",
	})

	// 5. Skip-notice golden still holds under forced failure
	skipPF := e3.PreflightWithEnv([]string{"PATH=/e3-none", "CGO_ENABLED=1"})
	skipOK := !skipPF.OK && skipPF.FormatSkipNotice() == e3.SkipNoticeMissingCompiler
	log.Record(e3.ProbeEntry{
		Probe: "matrix", Step: "skip_notice_golden",
		Outcome: outcome(skipOK), Skip: skipPF.FormatSkipNotice(),
		Detail: "forced missing compiler",
	})
	if !skipOK {
		t.Fatalf("skip notice golden failed: ok=%v notice=%q", skipPF.OK, skipPF.FormatSkipNotice())
	}

	pass, fail := log.SummaryPassFail()
	if fail != 0 {
		t.Fatalf("matrix had failures: pass=%d fail=%d", pass, fail)
	}
	t.Logf("E3 matrix complete: pass=%d fail=%d os=%s/%s compiler=%s",
		pass, fail, runtime.GOOS, runtime.GOARCH, pf.CompilerPath)
}

func stripEnv(env []string, keys ...string) []string {
	drop := map[string]bool{}
	for _, k := range keys {
		drop[k] = true
	}
	out := make([]string, 0, len(env))
	for _, e := range env {
		k, _, ok := strings.Cut(e, "=")
		if !ok || drop[k] {
			continue
		}
		out = append(out, e)
	}
	return out
}

func outcome(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}
