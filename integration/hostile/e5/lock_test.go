package e5_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/integration/hostile/e5"
)

func TestE5VersionActionLock(t *testing.T) {
	path, err := e5.FindVersionsTOML()
	if err != nil {
		t.Fatalf("E5PROBE step=locate outcome=fail detail=%v", err)
	}
	t.Logf("E5PROBE step=locate outcome=pass path=%s", path)

	lf, err := e5.ParseLock(path)
	if err != nil {
		t.Fatalf("E5PROBE step=parse outcome=fail detail=%v", err)
	}
	t.Logf("E5PROBE step=parse outcome=pass schema=%s go=%s modules=%d tools=%d actions=%d",
		lf.Schema, lf.Go, len(lf.Modules), len(lf.Tools), len(lf.Actions))

	if issues := e5.ValidateUniqueIDs(lf); len(issues) > 0 {
		for _, i := range issues {
			t.Logf("E5PROBE step=unique_ids outcome=fail detail=%s", i)
		}
		t.Fatalf("unique id validation failed: %d issues", len(issues))
	}
	t.Logf("E5PROBE step=unique_ids outcome=pass")

	if issues := e5.ValidateSection12(lf); len(issues) > 0 {
		for _, i := range issues {
			t.Logf("E5PROBE step=section12 outcome=fail detail=%s", i)
		}
		t.Fatalf("Section 12 coverage failed: %d issues", len(issues))
	}
	t.Logf("E5PROBE step=section12 outcome=pass pins=%d", len(e5.ExpectedSection12))

	if issues := e5.ValidateActions(lf); len(issues) > 0 {
		for _, i := range issues {
			t.Logf("E5PROBE step=actions outcome=fail detail=%s", i)
		}
		t.Fatalf("action lock validation failed: %d issues", len(issues))
	}
	t.Logf("E5PROBE step=actions outcome=pass actions=%d required=%d",
		len(lf.Actions), len(e5.RequiredActionIDs))

	// Single-lock existence: only one versions.toml under catalog/
	catalogDir := filepath.Dir(path)
	entries, err := os.ReadDir(catalogDir)
	if err != nil {
		t.Fatalf("E5PROBE step=single_lock outcome=fail detail=%v", err)
	}
	var versionFiles []string
	for _, e := range entries {
		name := e.Name()
		if name == "versions.toml" || name == "versions.lock" || name == "versions.lock.toml" {
			versionFiles = append(versionFiles, name)
		}
	}
	if len(versionFiles) != 1 || versionFiles[0] != "versions.toml" {
		t.Fatalf("E5PROBE step=single_lock outcome=fail files=%v", versionFiles)
	}
	t.Logf("E5PROBE step=single_lock outcome=pass file=catalog/versions.toml")

	// Emit machine-readable summary line for evidence logs.
	fmt.Fprintf(os.Stderr, "E5SUMMARY ok=true go=%s modules=%d tools=%d actions=%d ts=%s\n",
		lf.Go, len(lf.Modules), len(lf.Tools), len(lf.Actions), time.Now().UTC().Format(time.RFC3339))
}

func TestE5StaticcheckReleaseAlias(t *testing.T) {
	// Section 12 lists Staticcheck as "v0.7.0 / 2026.1" — lock must carry both.
	path, err := e5.FindVersionsTOML()
	if err != nil {
		t.Fatal(err)
	}
	lf, err := e5.ParseLock(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := lf.Raw
	if !containsAll(raw, "v0.7.0", "2026.1", "honnef.co/go/tools") {
		t.Fatalf("E5PROBE step=staticcheck_alias outcome=fail detail=missing v0.7.0/2026.1/module")
	}
	t.Logf("E5PROBE step=staticcheck_alias outcome=pass")
}

func TestE5CVEGoPinDocumented(t *testing.T) {
	path, err := e5.FindVersionsTOML()
	if err != nil {
		t.Fatal(err)
	}
	lf, err := e5.ParseLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(lf.Raw, "CVE-2026-39822", "1.26.5") {
		t.Fatalf("E5PROBE step=cve_doc outcome=fail detail=lock must document CVE-2026-39822 with go 1.26.5")
	}
	t.Logf("E5PROBE step=cve_doc outcome=pass")
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
