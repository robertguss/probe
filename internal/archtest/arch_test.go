package archtest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
)

func TestArchitecture(t *testing.T) {
	// P1.8 (4hi) entry point: layer/import rules + Section 58 red-lines.
	root := repoRoot(t)
	t.Logf("archtest root=%s", root)

	violations, err := archtest.Check(root)
	if err != nil {
		t.Fatalf("arch check error: %v", err)
	}
	for _, v := range violations {
		// Red-line hits use Rule=RL-58-*; always print full violation text.
		t.Errorf("%s", v)
	}
	if len(violations) > 0 {
		t.Fatalf("%d architecture violation(s)", len(violations))
	}
}

func TestForbiddenPackagesAbsent(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range archtest.ForbiddenPackageDirs {
		p := filepath.Join(root, rel)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			id := archtest.ForbiddenRedlinePackageDirs[rel]
			if id == "" {
				id = "forbidden_package"
			}
			t.Errorf("%s: forbidden package directory exists: %s; %s",
				id, rel, archtest.RedlineRemediation)
		}
	}
}

func TestLayerOrderCoversSection42(t *testing.T) {
	// Sanity: generate is above fsx/toolrun; diagnostic is lowest pure layer.
	idx := map[string]int{}
	for i, l := range archtest.LayerOrder {
		idx[l] = i
	}
	required := []string{
		"internal/diagnostic",
		"internal/spec",
		"internal/catalog",
		"internal/resolve",
		"internal/render",
		"internal/plan",
		"internal/fsx",
		"internal/toolrun",
		"internal/generate",
		"internal/report",
		"internal/cli",
	}
	for _, r := range required {
		if _, ok := idx[r]; !ok {
			t.Errorf("LayerOrder missing %s", r)
		}
	}
	if idx["internal/diagnostic"] >= idx["internal/spec"] {
		t.Error("diagnostic must be below spec")
	}
	if idx["internal/fsx"] >= idx["internal/generate"] {
		t.Error("fsx must be below generate")
	}
	if idx["internal/cli"] <= idx["internal/generate"] {
		t.Error("cli must be outermost (above generate)")
	}
}

func TestCmdFoundryIsSoleExitSite(t *testing.T) {
	root := repoRoot(t)
	main := filepath.Join(root, "cmd", "foundry", "main.go")
	if _, err := os.Stat(main); err != nil {
		t.Fatalf("cmd/foundry/main.go missing: %v", err)
	}
	// Full scan is in TestArchitecture; this guards the scaffold file exists.
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/archtest → repo root
	start := filepath.Join(filepath.Dir(file), "..", "..")
	root, err := archtest.RepoRoot(start)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
