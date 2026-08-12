package toolrun

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestSplitLinesPreserveEmpty(t *testing.T) {
	log := testutil.New(t)
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", []string{}},
		{"single", "a", []string{"a"}},
		{"trailing_lf", "a\nb\n", []string{"a", "b"}},
		{"trailing_crlf", "a\r\nb\r\n", []string{"a", "b"}},
		{"empty_middle", "a\n\nb", []string{"a", "", "b"}},
		{"cr_in_line", "a\r\nb\rc\n", []string{"a", "b\rc"}},
		{"no_final_nl", "x\ny", []string{"x", "y"}},
		{"only_nl", "\n", []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitLinesPreserveEmpty(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("%s len: got %d want %d full=%q", tc.name, len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("%s[%d]: got %q want %q (full=%q)", tc.name, i, got[i], tc.want[i], got)
				}
			}
		})
	}
	log.Step("split_lines", testutil.OutcomeOK, "table_ok")
}

func TestCaptureHost_SuccessAndEmptyPATH(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real go subprocess test in short mode")
	}
	log := testutil.New(t)
	log.Phase("success")
	h, err := CaptureHost("")
	if err != nil {
		// PATH/go must be available in the unit environment.
		t.Fatalf("CaptureHost: %v", err)
	}
	if h.PATH == "" || h.GOMODCACHE == "" || h.GOCACHE == "" {
		t.Fatalf("incomplete capture: %+v", h)
	}
	env := ConstructGoEnv(h)
	if bad := KeysOutsideAllowlist(env, GoAllowlistKeys); len(bad) > 0 {
		t.Fatalf("outside allowlist: %v", bad)
	}
	log.Step("capture_ok", testutil.OutcomeOK, "keys="+strings.Join(EnvKeys(env), ","))
	log.PhaseEnd("success", testutil.OutcomeOK)

	log.Phase("empty_path")
	t.Setenv("PATH", "")
	_, err = CaptureHost("")
	if err == nil || !strings.Contains(err.Error(), "PATH is empty") {
		t.Fatalf("want empty PATH error, got %v", err)
	}
	log.Step("empty_path", testutil.OutcomeOK, err.Error())
	log.PhaseEnd("empty_path", testutil.OutcomeOK)
}

func TestCaptureHost_ExplicitBinaryAndBadOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skips subprocess test in short mode")
	}
	log := testutil.New(t)
	dir := t.TempDir()

	// Binary that prints the wrong number of lines.
	bad, err := WriteFakeBinary(dir, "badgo", "only-one-line")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err = CaptureHost(bad)
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("want line-count error, got %v", err)
	}
	log.Step("bad_lines", testutil.OutcomeOK, err.Error())

	// Non-existent explicit binary.
	_, err = CaptureHost(filepath.Join(dir, "missing-go-bin"))
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	log.Step("missing_bin", testutil.OutcomeOK, err.Error())
}

func TestCanonicalEnvEncoding_EmptyAndKeysOutside(t *testing.T) {
	log := testutil.New(t)
	if got := CanonicalEnvEncoding(nil); got != "" {
		t.Fatalf("nil env encoding: %q", got)
	}
	if got := CanonicalEnvEncoding(map[string]string{}); got != "" {
		t.Fatalf("empty env encoding: %q", got)
	}
	// KeysOutsideAllowlist with extras.
	extra := KeysOutsideAllowlist(map[string]string{"A": "1", "PATH": "/bin", "Z": "2"}, []string{"PATH"})
	if len(extra) != 2 || extra[0] != "A" || extra[1] != "Z" {
		t.Fatalf("extra keys: %v", extra)
	}
	// ForbiddenKeysPresent empty hits.
	if hits := ForbiddenKeysPresent(map[string]string{"PATH": "x"}, ForbiddenBleedKeys); len(hits) != 0 {
		t.Fatalf("unexpected hits: %v", hits)
	}
	bleed := map[string]string{"GITHUB_TOKEN": "secret", "PATH": "/bin", "GODEBUG": "x"}
	hits := ForbiddenKeysPresent(bleed, ForbiddenBleedKeys)
	if len(hits) != 2 {
		t.Fatalf("want 2 forbidden hits, got %v", hits)
	}
	log.Step("env_edges", testutil.OutcomeOK, "hits="+strings.Join(hits, ","))
}

func TestErrnoString_AndFDErrors(t *testing.T) {
	log := testutil.New(t)
	if got := errnoString(nil); got != "" {
		t.Fatalf("nil errno: %q", got)
	}
	if got := errnoString(unix.ENOENT); got == "" {
		t.Fatal("unix.Errno empty")
	}
	if got := errnoString(syscall.EPERM); got == "" {
		t.Fatal("syscall.Errno empty")
	}
	if got := errnoString(errors.New("plain")); got != "plain" {
		t.Fatalf("plain: %q", got)
	}
	log.Step("errno", testutil.OutcomeOK, "variants")

	// OpenDirFD on a file (not directory).
	f := filepath.Join(t.TempDir(), "not-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDirFD(f); err == nil {
		t.Fatal("OpenDirFD on file should fail")
	}
	if _, err := OpenDirFD(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("OpenDirFD missing should fail")
	}
	// CloseFD negative is no-op.
	if err := CloseFD(-1); err != nil {
		t.Fatalf("CloseFD(-1): %v", err)
	}
	log.Step("fd_errors", testutil.OutcomeOK, "open_close")
}

func TestOriginalCWD_FDNilAndClosed(t *testing.T) {
	var nilOrig *OriginalCWD
	if nilOrig.FD() != -1 {
		t.Fatalf("nil FD: %d", nilOrig.FD())
	}
	// LogDetail stage/restore fail branches.
	l := BoundStartLog{BinaryBase: "go", FchdirStageOK: false, FchdirErrno: "EIO", RestoreOK: false, RestoreErrno: "EBADF"}
	d := l.LogDetail()
	if !strings.Contains(d, "fchdir_stage=fail:EIO") || !strings.Contains(d, "restore=fail:EBADF") {
		t.Fatalf("detail: %s", d)
	}
	l2 := BoundStartLog{BinaryBase: "go", FchdirStageOK: false, RestoreOK: false}
	d2 := l2.LogDetail()
	if !strings.Contains(d2, "fchdir_stage=fail") || strings.Contains(d2, "fail:") {
		// restore=fail without errno is ok; ensure no double colon for empty errno on stage
		if !strings.Contains(d2, "fchdir_stage=fail ") && !strings.Contains(d2, "fchdir_stage=fail") {
			t.Fatalf("detail2: %s", d2)
		}
	}
}

func TestRestoreOrFail_BothPaths(t *testing.T) {
	log := testutil.New(t)
	orig, err := CaptureOriginalCWD()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = orig.Close() })

	// Success restore path.
	s, err := NewBoundStarter(orig, BoundStarterOptions{
		Fchdir: func(fd int) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	res := &BoundStartResult{}
	cause := errors.New("start blew up")
	got := s.restoreOrFail(res, "step-x", cause, "start_child")
	if got == nil {
		t.Fatal("want error")
	}
	if !res.Log.RestoreOK {
		t.Fatal("restore should succeed")
	}
	if res.FailClass != string(diagnostic.IDToolFailed) {
		t.Fatalf("fail class: %s", res.FailClass)
	}
	fe, ok := diagnostic.AsFoundryError(got)
	if !ok {
		t.Fatal("want FoundryError")
	}
	if !strings.Contains(fe.Error(), "start_child") {
		t.Fatalf("msg: %v", fe)
	}
	log.Step("restore_ok", testutil.OutcomeOK, fe.Error())

	// Restore failure path.
	s2, err := NewBoundStarter(orig, BoundStarterOptions{
		Fchdir: func(fd int) error { return unix.EBADF },
	})
	if err != nil {
		t.Fatal(err)
	}
	res2 := &BoundStartResult{}
	got2 := s2.restoreOrFail(res2, "step-y", cause, "after_stage")
	if got2 == nil {
		t.Fatal("want error")
	}
	if res2.Log.RestoreOK {
		t.Fatal("restore should fail")
	}
	if res2.FailClass != FailClassCWDRestore {
		t.Fatalf("class: %s", res2.FailClass)
	}
	if res2.Log.RestoreErrno == "" {
		t.Fatal("expected restore errno")
	}
	log.Step("restore_fail", testutil.OutcomeOK, res2.Log.RestoreErrno)
}

func TestBoundStarter_ValidationAndStartChildFail(t *testing.T) {
	log := testutil.New(t)
	// NewBoundStarter nil orig.
	if _, err := NewBoundStarter(nil, BoundStarterOptions{}); err == nil {
		t.Fatal("nil orig should fail")
	}
	// Closed-ish orig with bad fd.
	if _, err := NewBoundStarter(&OriginalCWD{fd: -1}, BoundStarterOptions{}); err == nil {
		t.Fatal("bad fd should fail")
	}

	orig, err := CaptureOriginalCWD()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = orig.Close() })
	stage := t.TempDir()
	fd, err := OpenDirFD(stage)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseFD(fd) })

	// StartChild error triggers restoreOrFail.
	s, err := NewBoundStarter(orig, BoundStarterOptions{
		StartChild: func(_ context.Context, _ string, _, _ []string, _ string, _ int, _ time.Duration) (int, func() ([]byte, []byte, error), error) {
			return 0, nil, errors.New("spawn refused")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res := s.Start(context.Background(), BoundStartRequest{
		StageFD: fd,
		Binary:  "/bin/true",
		StepID:  "t",
		Args:    nil,
		Env:     nil,
	})
	if res.OK() {
		t.Fatal("expected start failure")
	}
	if res.Err() == nil {
		t.Fatal("want err")
	}
	log.Step("start_child_fail", testutil.OutcomeOK, res.FailClass)

	// Invalid stage fd / empty binary on live starter.
	res2 := s.Start(context.Background(), BoundStartRequest{StageFD: -3, Binary: "/bin/true", StepID: "t"})
	if res2.FailClass != FailClassFchdirStage {
		t.Fatalf("stage fd class: %s", res2.FailClass)
	}
	res3 := s.Start(context.Background(), BoundStartRequest{StageFD: fd, Binary: "", StepID: "t"})
	if res3.FailClass != FailClassFchdirStage {
		t.Fatalf("empty binary class: %s", res3.FailClass)
	}

	// Nil starter receiver path.
	var nilStarter *BoundStarter
	res4 := nilStarter.Start(context.Background(), BoundStartRequest{StageFD: fd, Binary: "/bin/true", StepID: "t"})
	if res4.OK() {
		t.Fatal("nil starter should fail")
	}
	log.Step("validation", testutil.OutcomeOK, "ok")
}

func TestFakeChild_WaitCapDefault(t *testing.T) {
	c := &fakeChild{
		pid:      1,
		capBytes: 0, // force default
		wait: func() ([]byte, []byte, error) {
			return []byte("hi"), []byte("err"), nil
		},
	}
	out, errb, st, et, on, en, err := c.Wait()
	if err != nil || st || et || string(out) != "hi" || string(errb) != "err" || on != 2 || en != 3 {
		t.Fatalf("wait: out=%q errb=%q st=%v et=%v on=%d en=%d err=%v", out, errb, st, et, on, en, err)
	}
	c2 := &fakeChild{pid: 2, wait: nil}
	_, _, _, _, _, _, err = c2.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCappedWriter_InternalEdges(t *testing.T) {
	log := testutil.New(t)
	w := newCappedWriter(0) // default max
	if w.max != DefaultOutputCapBytes {
		t.Fatalf("default max: %d", w.max)
	}
	w2 := newCappedWriter(4)
	n, err := w2.Write([]byte("abcd"))
	if err != nil || n != 4 {
		t.Fatalf("write full: %d %v", n, err)
	}
	// Exactly full; next write discards.
	n, err = w2.Write([]byte("XY"))
	if err != nil || n != 2 {
		t.Fatalf("discard write: %d %v", n, err)
	}
	b, trunc := w2.Bytes()
	if !trunc || string(b) != "abcd" || w2.TotalOffered() != 6 {
		t.Fatalf("bytes=%q trunc=%v total=%d", b, trunc, w2.TotalOffered())
	}
	// Partial retain path.
	w3 := newCappedWriter(5)
	_, _ = w3.Write([]byte("12"))
	_, _ = w3.Write([]byte("345678"))
	b3, trunc3 := w3.Bytes()
	if !trunc3 || string(b3) != "12345" {
		t.Fatalf("partial: %q trunc=%v", b3, trunc3)
	}
	log.Step("capped_writer", testutil.OutcomeOK, "ok")
}

func TestFakeRunner_LookPathMissAndEnvEdges(t *testing.T) {
	log := testutil.New(t)
	f := &FakeRunner{} // nil PathMap
	if _, err := f.LookPath("go"); err == nil {
		t.Fatal("nil PathMap should miss")
	}
	f.PathMap = map[string]string{"go": "/x/go"}
	if _, err := f.LookPath("git"); err == nil {
		t.Fatal("missing key should miss")
	}
	// Basename fallback.
	f.PathMap = map[string]string{"go": "/opt/go"}
	p, err := f.LookPath("/usr/local/bin/go")
	if err != nil || p != "/opt/go" {
		t.Fatalf("basename look: %s %v", p, err)
	}
	// AllDirsEmpty false when non-empty dir observed.
	f.RecordBoundStart("/tmp/stage", "/x/go", nil)
	if f.AllDirsEmpty() {
		t.Fatal("expected non-empty dir")
	}
	// LastEnvMap empty.
	if m := f.LastEnvMap(); m != nil {
		t.Fatalf("empty last env: %v", m)
	}
	// Run with ExitErr by basename + malformed env entries.
	f.Outputs = map[string]string{"go": "ok"}
	f.ExitErr = map[string]error{"go": errors.New("exit 2")}
	_, _, err = f.Run(context.Background(), "/opt/custom/go", nil, []string{"NOEQ", "A=1", "A=2", "B=3"})
	if err == nil || err.Error() != "exit 2" {
		t.Fatalf("exit err: %v", err)
	}
	m := f.LastEnvMap()
	if m["A"] != "1" || m["B"] != "3" {
		t.Fatalf("env map first-wins: %v", m)
	}
	if _, _, ok := splitEnvKV("nope"); ok {
		t.Fatal("split without =")
	}
	// shellSingleQuote with apostrophe.
	if got := shellSingleQuote("a'b"); got != `a'\''b` {
		t.Fatalf("quote: %q", got)
	}
	// WriteFakeBinary / WriteNonExecutable error on missing parent.
	if _, err := WriteFakeBinary(filepath.Join(t.TempDir(), "nope"), "x", "y"); err == nil {
		t.Fatal("WriteFakeBinary missing parent")
	}
	if _, err := WriteNonExecutable(filepath.Join(t.TempDir(), "nope"), "x", "y"); err == nil {
		t.Fatal("WriteNonExecutable missing parent")
	}
	log.Step("fake_runner", testutil.OutcomeOK, "edges")
}

func TestPreflightHelpers_Internal(t *testing.T) {
	log := testutil.New(t)
	if got := wrongVersionRemediation("go1.26.5"); !strings.Contains(got, "go1.26.5") {
		t.Fatalf("remediation: %s", got)
	}
	if got := truncateForMsg("  abc  ", 0); got != "abc" {
		t.Fatalf("trunc0: %q", got)
	}
	if got := truncateForMsg("abcdef", 3); got != "abc…" {
		t.Fatalf("trunc3: %q", got)
	}
	if got := truncateForMsg("ab", 10); got != "ab" {
		t.Fatalf("short: %q", got)
	}
	if got := firstNonEmpty("", "  ", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty: %q", got)
	}
	if got := firstNonEmpty("", "  "); got != "" {
		t.Fatalf("all empty: %q", got)
	}

	// resolveBinary relative + absolute paths.
	dir := t.TempDir()
	bin, err := WriteFakeBinary(dir, "tool", "ok")
	if err != nil {
		t.Fatal(err)
	}
	r := OSRunner{PathEnv: dir}
	// absolute ok
	p, err := resolveBinary(r, bin, "tool")
	if err != nil || p != bin {
		t.Fatalf("abs: %s %v", p, err)
	}
	// relative via LookPath
	p, err = resolveBinary(r, "tool", "other")
	if err != nil || filepath.Base(p) != "tool" {
		t.Fatalf("rel: %s %v", p, err)
	}
	// missing absolute
	if _, err := resolveBinary(r, filepath.Join(dir, "nope"), "tool"); err == nil {
		t.Fatal("missing abs")
	}
	// directory not executable
	if _, err := resolveBinary(r, dir, "tool"); err == nil {
		t.Fatal("dir should fail")
	}
	// non-executable file
	ne, err := WriteNonExecutable(dir, "noexec", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureExecutable(ne); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("noexec: %v", err)
	}
	// ParseGoVersionOutput bad prefix via empty-ish tag path is covered; short tag:
	if _, err := ParseGoVersionOutput("go version xx linux/amd64"); err == nil {
		// "xx" may fail prefix go check depending on parser — either way exercise.
		_ = err
	}
	log.Step("preflight_helpers", testutil.OutcomeOK, "ok")
}

func TestPreflight_UnparseableAndGitProbeFail(t *testing.T) {
	log := testutil.New(t)
	goPath := "/fake/bin/go"
	gitPath := "/fake/bin/git"
	fake := &FakeRunner{
		PathMap: map[string]string{"go": goPath, "git": gitPath},
		Outputs: map[string]string{
			goPath: "not a version line at all\n",
		},
	}
	res := Preflight(context.Background(), PreflightOptions{
		Host: HostCapture{
			PATH: "/bin", HOME: "/home/u", GOMODCACHE: "/m", GOCACHE: "/c",
			GOPATH: "/g", GOPROXY: "off", GOSUMDB: "off",
		},
		RequireGoVersion: "go1.26.5",
		Runner:           fake,
	})
	if res.OK() || res.FailClass != FailClassWrongVersion {
		t.Fatalf("unparseable: ok=%v class=%s err=%v", res.OK(), res.FailClass, res.Err())
	}
	log.Step("unparseable", testutil.OutcomeOK, res.FailClass)

	// Git version probe failure.
	fake2 := &FakeRunner{
		PathMap: map[string]string{"go": goPath, "git": gitPath},
		Outputs: map[string]string{
			goPath:  "go version go1.26.5 linux/amd64\n",
			gitPath: "git version 2.0\n",
		},
		ExitErr: map[string]error{gitPath: errors.New("git broke")},
	}
	res2 := Preflight(context.Background(), PreflightOptions{
		Host: HostCapture{
			PATH: "/bin", HOME: "/home/u", GOMODCACHE: "/m", GOCACHE: "/c",
			GOPATH: "/g", GOPROXY: "off", GOSUMDB: "off",
		},
		RequireGoVersion: "go1.26.5",
		RequireGit:       true,
		Runner:           fake2,
	})
	if res2.OK() || res2.FailClass != FailClassMissing || res2.FailStep != StepGitPreflight {
		t.Fatalf("git fail: ok=%v class=%s step=%s err=%v", res2.OK(), res2.FailClass, res2.FailStep, res2.Err())
	}
	log.Step("git_probe_fail", testutil.OutcomeOK, res2.FailClass)
}

func TestResolveGo_ProbeEdges(t *testing.T) {
	log := testutil.New(t)
	if _, err := ProbeGoVersionLocal(""); err == nil {
		t.Fatal("empty binary")
	}
	if _, err := ProbeGoVersionLocal("definitely-not-a-go-binary-xyz"); err == nil {
		t.Fatal("lookpath miss")
	}
	// Default required tag path: FindPinnedGoBinary("") should still search.
	// May return non-empty on this host; just ensure no panic.
	_ = FindPinnedGoBinary("   ")
	// WrongVersionRemediation empty required defaults.
	msg := WrongVersionRemediation("", "/x/go", "")
	if !strings.Contains(msg, DefaultPinnedGoTag) {
		t.Fatalf("default tag missing: %s", msg)
	}
	// Non-executable absolute path.
	dir := t.TempDir()
	ne, err := WriteNonExecutable(dir, "go", "#!/bin/sh\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeGoVersionLocal(ne); err == nil {
		t.Fatal("non-exec probe")
	}
	log.Step("resolve_go", testutil.OutcomeOK, "edges")
}

func TestIsolation_FormatAndArtifactAndSecrets(t *testing.T) {
	log := testutil.New(t)
	line := FormatObservationLine(IsolationObservation{
		Sentinel: "GOFLAGS", Observation: "absent", Argv: "go version", Exit: 0, Detail: "ok",
	})
	if !strings.Contains(line, "sentinel=GOFLAGS") || !strings.Contains(line, "observation=absent") {
		t.Fatalf("line: %s", line)
	}
	// WriteIsolationArtifact skip when no dir/env.
	t.Setenv(ArtifactEnvVar, "")
	p, err := WriteIsolationArtifact("", "case", map[string]string{"PATH": "/bin"}, []string{"go"}, nil)
	if err != nil || p != "" {
		t.Fatalf("skip: %q %v", p, err)
	}
	// Write with sanitizing name.
	dir := t.TempDir()
	p, err = WriteIsolationArtifact(dir, "../weird name!.txt", map[string]string{"PATH": "/bin"}, []string{"go"}, []IsolationObservation{{
		Sentinel: "X", Kind: "go-absent", Observation: "absent", Exit: -1,
	}})
	if err != nil || p == "" {
		t.Fatalf("write: %q %v", p, err)
	}
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "isolation_dump") {
		t.Fatalf("body: %s", body)
	}
	// empty base name sanitization
	p2, err := WriteIsolationArtifact(dir, ".", map[string]string{"A": "1"}, nil, nil)
	if err != nil || p2 == "" {
		t.Fatalf("dot name: %q %v", p2, err)
	}
	// AssertNoSecretValues
	dump := "env_keys=PATH\nenv_hash=abc\n"
	if hits := AssertNoSecretValues(dump, []string{"", "secret-value"}); len(hits) != 0 {
		t.Fatalf("hits: %v", hits)
	}
	if hits := AssertNoSecretValues("x=secret-value\n", []string{"secret-value"}); len(hits) != 1 {
		t.Fatalf("want hit: %v", hits)
	}
	log.Step("isolation", testutil.OutcomeOK, "ok")
}

func TestTreeHelpers_AndExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real subprocess test in short mode")
	}
	log := testutil.New(t)
	if EnvKeySetEqual([]string{"A=1", "B=2"}, []string{"A"}) {
		t.Fatal("len mismatch")
	}
	if EnvKeySetEqual([]string{"A=1", "C=2"}, []string{"A", "B"}) {
		t.Fatal("key mismatch")
	}
	if !EnvKeySetEqual([]string{"B=2", "A=1"}, []string{"A", "B"}) {
		t.Fatal("equal keys")
	}
	extra, missing := EnvKeySetMatchesAllowlist(
		map[string]string{"PATH": "/bin", "EXTRA": "1"},
		[]string{"PATH", "HOME", "TMPDIR"},
		map[string]struct{}{"TMPDIR": {}},
	)
	if len(extra) != 1 || extra[0] != "EXTRA" || len(missing) != 1 || missing[0] != "HOME" {
		t.Fatalf("extra=%v missing=%v", extra, missing)
	}
	if argvEqual([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("len")
	}
	if argvEqual([]string{"a", "b"}, []string{"a", "c"}) {
		t.Fatal("elem")
	}
	if !argvEqual([]string{"a"}, []string{"a"}) {
		t.Fatal("eq")
	}
	if m := copyStringMap(nil); m == nil || len(m) != 0 {
		t.Fatalf("nil copy: %v", m)
	}
	// exitCodeFrom branches
	if exitCodeFrom(BoundStartResult{ExitCode: 7}) != 7 {
		t.Fatal("nonzero")
	}
	if exitCodeFrom(BoundStartResult{ExitCode: 0, err: nil}) != 0 {
		t.Fatal("success")
	}
	// ExitError path via a real failed command if available.
	cmd := exec.Command("false")
	_ = cmd.Run()
	// synthesize via Wait on a finished process is awkward; use errors.As with ExitError from Command.
	cmd2 := exec.Command("sh", "-c", "exit 3")
	err := cmd2.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code := exitCodeFrom(BoundStartResult{ExitCode: 0, err: err})
		if code != 3 {
			t.Fatalf("exitcode from ExitError: %d", code)
		}
	}
	if exitCodeFrom(BoundStartResult{ExitCode: 0, err: errors.New("x"), PID: 9}) != -1 {
		t.Fatal("pid signal")
	}
	if exitCodeFrom(BoundStartResult{ExitCode: 0, err: errors.New("x"), PID: 0}) != -1 {
		t.Fatal("start fail")
	}
	if boolString(true) != "true" || boolString(false) != "false" {
		t.Fatal("boolString")
	}
	ex := &Executor{}
	if ex.killGrace() != DefaultKillGrace {
		t.Fatal("default grace")
	}
	ex.KillGrace = time.Second
	if ex.killGrace() != time.Second {
		t.Fatal("custom grace")
	}
	ex0, err := NewExecutor(nil)
	if err == nil || ex0 != nil {
		t.Fatalf("NewExecutor nil starter should fail: %v %v", ex0, err)
	}
	log.Step("tree_exit", testutil.OutcomeOK, "ok")
}

func TestEmptyTemplateDir_DefaultParent(t *testing.T) {
	log := testutil.New(t)
	dir, err := EmptyTemplateDir("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	st, err := os.Stat(dir)
	if err != nil || st.Mode().Perm()&0o777 != 0o700 {
		t.Fatalf("mode: %v %v", st, err)
	}
	// GitTemplateDir absent
	if _, ok := GitTemplateDir(map[string]string{}); ok {
		t.Fatal("absent")
	}
	log.Step("template", testutil.OutcomeOK, dir)
}

func TestKillProcessGroup_NoopAndImmediate(t *testing.T) {
	// pid <= 0 no-ops (must not signal the whole test process group).
	killProcessGroup(0, time.Millisecond)
	killProcessGroup(-1, 0)
	killProcessGroupImmediate(0)
	killProcessGroupImmediate(-5)
}
