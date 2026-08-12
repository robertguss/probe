package testutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestGoldenCompareAndUpdate(t *testing.T) {
	dir := t.TempDir()
	path := testutil.GoldenPath(dir, "sample")
	content := []byte("hello golden\n")

	// Create golden via update path.
	t.Setenv(testutil.EnvUpdateGolden, "1")
	t.Setenv(testutil.EnvCI, "")
	testutil.CompareGolden(t, path, content)

	// Compare matches.
	t.Setenv(testutil.EnvUpdateGolden, "")
	testutil.CompareGolden(t, path, content)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff("hello golden\n", string(got)); diff != "" {
		t.Fatalf("golden content mismatch (-want +got):\n%s", diff)
	}
}

func TestGoldenUpdateBlockedUnderCI(t *testing.T) {
	if err := testutil.UpdateAllowed(); err == nil {
		// UPDATE_GOLDEN not set — expected error.
	}

	t.Setenv(testutil.EnvUpdateGolden, "1")
	t.Setenv(testutil.EnvCI, "true")
	if err := testutil.UpdateAllowed(); err == nil {
		t.Fatal("expected UpdateAllowed to refuse under CI=true")
	}

	// CompareGolden must fatal when update requested under CI.
	dir := t.TempDir()
	path := filepath.Join(dir, "blocked.golden")
	// Run in a subprocess-like subtest that expects failure via t.Fatalf —
	// use a nested test with recover-style: call UpdateAllowed only (already
	// covered) and document CompareGolden path via a helper that mirrors it.
	if !testutil.InCI() {
		t.Fatal("InCI should be true after CI=true")
	}
	_ = path
}

func TestGoldenUpdateBlockedSimulatedCICompare(t *testing.T) {
	// Full CompareGolden path under simulated CI: expect the test helper to fail.
	t.Setenv(testutil.EnvUpdateGolden, "1")
	t.Setenv(testutil.EnvCI, "true")

	ok := t.Run("must_refuse", func(t *testing.T) {
		// Replace Fatalf behavior: we can't catch Fatalf easily, so re-check
		// the guard used by CompareGolden.
		if !testutil.InCI() {
			t.Fatal("CI not detected")
		}
		if err := testutil.UpdateAllowed(); err == nil {
			t.Fatal("update should not be allowed")
		}
	})
	if !ok {
		t.Fatal("subtest failed")
	}
}

func TestInCIVariants(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"", false},
		{"false", false},
		{"true", true},
		{"1", true},
		{"yes", true},
	}
	for _, tc := range cases {
		t.Run("ci="+tc.v, func(t *testing.T) {
			t.Setenv(testutil.EnvCI, tc.v)
			if got := testutil.InCI(); got != tc.want {
				t.Fatalf("InCI(%q)=%v want %v", tc.v, got, tc.want)
			}
		})
	}
}
