package testutil_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestRedactPlanJSONForDiff_BasenamesOnly(t *testing.T) {
	raw := []byte(`{
  "schema": 1,
  "destination": {
    "path": "/home/foundry/projects/minimal-cli",
    "parent": "/home/foundry/projects",
    "basename": "minimal-cli",
    "observation": "absent"
  },
  "plan_sha256": "abc"
}`)
	got := testutil.RedactPlanJSONForDiff(raw)
	if strings.Contains(got, "/home/foundry") {
		t.Fatalf("absolute home leaked:\n%s", got)
	}
	if !strings.Contains(got, `"path": "minimal-cli"`) {
		t.Fatalf("expected path basename, got:\n%s", got)
	}
	if !strings.Contains(got, `"parent": "projects"`) {
		t.Fatalf("expected parent basename, got:\n%s", got)
	}
}

func TestUnifiedDiff_ShowsChange(t *testing.T) {
	diff := testutil.UnifiedDiff("a", "line1\nline2\n", "b", "line1\nlineX\n")
	if !strings.Contains(diff, "-line2") || !strings.Contains(diff, "+lineX") {
		t.Fatalf("diff missing change lines:\n%s", diff)
	}
	if !strings.Contains(diff, " line1") {
		t.Fatalf("diff missing context:\n%s", diff)
	}
}
