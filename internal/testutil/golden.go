package testutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// EnvUpdateGolden is the sole opt-in for rewriting golden files (REQ-219).
const EnvUpdateGolden = "UPDATE_GOLDEN"

// EnvCI is the standard CI indicator. When set to a truthy value, golden
// updates are refused even if UPDATE_GOLDEN=1.
const EnvCI = "CI"

// CompareGolden compares got to the checked-in golden file at
// testdata/<name>.golden relative to the calling test package directory
// (pass an explicit path via GoldenPath if needed).
//
// Update workflow (Section 45.4 / REQ-219):
//
//	UPDATE_GOLDEN=1 go test ./path -run TestName
//
// Refuses to update when CI is truthy (true/1/yes). Prints every changed path.
// Callers that bulk-update many files should enforce suite thresholds themselves.
func CompareGolden(t *testing.T, goldenPath string, got []byte) {
	t.Helper()
	got = normalizeGolden(got)

	if shouldUpdateGolden() {
		if InCI() {
			t.Fatalf("golden update refused: %s is set (CI must never auto-update goldens; REQ-219)", EnvCI)
		}
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		t.Logf("GOLDEN updated path=%s bytes=%d", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (set %s=1 to create)", goldenPath, err, EnvUpdateGolden)
	}
	want = normalizeGolden(want)
	if !bytes.Equal(want, got) {
		t.Errorf("golden mismatch path=%s\n--- want (%d bytes)\n%s\n--- got (%d bytes)\n%s",
			goldenPath, len(want), truncate(string(want), 512), len(got), truncate(string(got), 512))
		t.Logf("update with: %s=1 go test (refused when %s=true)", EnvUpdateGolden, EnvCI)
	}
}

// GoldenPath joins the test package's testdata dir with name + ".golden".
// dir should be the directory containing the test file (e.g. from test file
// path or an explicit "testdata").
func GoldenPath(dir, name string) string {
	if filepath.Ext(name) == ".golden" {
		return filepath.Join(dir, name)
	}
	return filepath.Join(dir, name+".golden")
}

// InCI reports whether the process appears to be running under CI.
func InCI() bool {
	v := os.Getenv(EnvCI)
	switch v {
	case "true", "TRUE", "True", "1", "yes", "YES", "y", "Y", "on", "ON":
		return true
	default:
		return false
	}
}

// shouldUpdateGolden reports whether UPDATE_GOLDEN requests an update.
// Does not consult CI — CompareGolden refuses the write separately.
func shouldUpdateGolden() bool {
	v := os.Getenv(EnvUpdateGolden)
	switch v {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	default:
		return false
	}
}

// UpdateAllowed is a pure helper for tests: returns nil if an update may
// proceed, or an error explaining the block (missing flag or CI).
func UpdateAllowed() error {
	if !shouldUpdateGolden() {
		return fmt.Errorf("%s not set to 1", EnvUpdateGolden)
	}
	if InCI() {
		return fmt.Errorf("golden update refused under CI (%s is set)", EnvCI)
	}
	return nil
}

func normalizeGolden(b []byte) []byte {
	// Normalize to LF-only for cross-platform stability.
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	// Ensure trailing newline for POSIX text files.
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return b
}
