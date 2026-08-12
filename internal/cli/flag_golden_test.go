package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestFlagCombinationGoldens snapshots representative command outputs produced
// by the CLI surface with common flag combinations (Section 47.2 / REQ-219).
// These complement the help goldens and are validated by the golden-matrix CI
// job. Updates are only permitted with UPDATE_GOLDEN=1 and never under CI=true.
func TestFlagCombinationGoldens(t *testing.T) {
	log := testutil.New(t)
	log.Phase("flag_combination_goldens")

	dir := filepath.Join("testdata", "snapshots")
	cases := []struct {
		name      string
		normalize func(string) string
		args      []string
	}{
		{
			name:      "version_text",
			normalize: normalizeHelp,
			args:      []string{"version"},
		},
		{
			name:      "version_json",
			normalize: func(s string) string { return s },
			args:      []string{"version", "--output", "json"},
		},
		{
			name:      "catalog_list_text",
			normalize: normalizeHelp,
			args:      []string{"catalog", "list"},
		},
		{
			name:      "catalog_list_json",
			normalize: func(s string) string { return s },
			args:      []string{"catalog", "list", "--output", "json"},
		},
		{
			name:      "catalog_show_cli_text",
			normalize: normalizeHelp,
			args:      []string{"catalog", "show", "cli"},
		},
		{
			name:      "catalog_show_cli_json",
			normalize: func(s string) string { return s },
			args:      []string{"catalog", "show", "cli", "--output", "json"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			res := runCLI(t, tc.args...)
			if res.Code != 0 {
				t.Fatalf("unexpected exit code %d: stdout=%q stderr=%q", res.Code, res.Stdout, res.Stderr)
			}
			got := tc.normalize(res.Stdout)
			path := testutil.GoldenPath(dir, tc.name)
			testutil.CompareGolden(t, path, []byte(got))
			sub.Step("golden", testutil.OutcomeOK, path)
		})
	}
	log.PhaseEnd("flag_combination_goldens", testutil.OutcomeOK)
}
