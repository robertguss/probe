package e3

import (
	"strings"
	"testing"
)

func TestSkipNoticeGoldens(t *testing.T) {
	// Immutable contract strings for CI template promotion.
	if !strings.HasPrefix(SkipNoticeMissingCompiler, "race detector skipped:") {
		t.Fatalf("missing-compiler notice prefix: %q", SkipNoticeMissingCompiler)
	}
	if !strings.Contains(SkipNoticeMissingCompiler, "not a silent pass") {
		t.Fatalf("missing-compiler notice must forbid silent pass: %q", SkipNoticeMissingCompiler)
	}
	if !strings.HasPrefix(SkipNoticeCGODisabled, "race detector skipped:") {
		t.Fatalf("cgo notice prefix: %q", SkipNoticeCGODisabled)
	}
	if !strings.Contains(SkipNoticeCGODisabled, "not a silent pass") {
		t.Fatalf("cgo notice must forbid silent pass: %q", SkipNoticeCGODisabled)
	}
}

func TestFormatSkipNotice_OKEmpty(t *testing.T) {
	r := PreflightResult{OK: true, CGOEnabled: "1", Compiler: "gcc"}
	if r.FormatSkipNotice() != "" {
		t.Fatalf("OK result must not skip: %q", r.FormatSkipNotice())
	}
}

func TestFormatSkipNotice_DefaultFallback(t *testing.T) {
	r := PreflightResult{OK: false}
	if r.FormatSkipNotice() != SkipNoticeMissingCompiler {
		t.Fatalf("empty SkipNotice must fall back to missing-compiler golden")
	}
}

func TestSanitizeEnvForRace(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"CGO_ENABLED=0",
		"CC=/old/cc",
		"HOME=/tmp",
	}
	pf := PreflightResult{OK: true, CompilerPath: "/usr/bin/gcc"}
	got := SanitizeEnvForRace(base, pf)
	m := map[string]string{}
	for _, e := range got {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	if m["CGO_ENABLED"] != "1" {
		t.Fatalf("CGO_ENABLED=%q", m["CGO_ENABLED"])
	}
	if m["CC"] != "/usr/bin/gcc" {
		t.Fatalf("CC=%q", m["CC"])
	}
	if m["HOME"] != "/tmp" {
		t.Fatalf("HOME lost")
	}
}

func TestGenerationGateSteps_AppendixE(t *testing.T) {
	// Appendix E go-test argv fragment.
	for _, s := range DefaultGenerationGate() {
		if s.ID == "go-test" && !strings.Contains(s.Argv, "-count=1") {
			t.Fatalf("Appendix E requires -count=1: %s", s.Argv)
		}
	}
	if GenerationGateContainsRace(StrictGenerationGate()) {
		t.Fatal("strict gate must not contain race")
	}
}
