package archtest_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestResidualCoverageInventory enforces the residual/coverage allowlist
// (bead go-foundry-cli-ipk.3 / docs/dev/testing.md residual policy).
func TestResidualCoverageInventory(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)
	log.Phase("residual_inventory")
	log.Inputs(map[string]string{
		"allowlist": itoa(len(archtest.ResidualCoverageAllowlist)),
	})

	vs, err := archtest.CheckResidualCoverageInventory(root)
	if err != nil {
		log.Fail("inventory", err.Error())
	}
	for _, v := range vs {
		log.Step("violation", testutil.OutcomeFail, v.String())
		t.Errorf("%s", v)
	}
	log.Assert("clean", len(vs) == 0, 0, len(vs))

	// Sanity: allowlist entries exist and carry non-empty rationale.
	var emptyRationale []string
	for path, why := range archtest.ResidualCoverageAllowlist {
		if strings.TrimSpace(why) == "" {
			emptyRationale = append(emptyRationale, path)
		}
		full := filepath.Join(root, filepath.FromSlash(path))
		if _, err := os.Stat(full); err != nil {
			t.Errorf("allowlisted path missing: %s: %v", path, err)
		}
	}
	sort.Strings(emptyRationale)
	log.Assert("rationale_present", len(emptyRationale) == 0, 0, emptyRationale)
	log.PhaseEnd("residual_inventory", testutil.OutcomeOK)
}

// TestResidualCoverageInventory_RejectsUnknown proves the checker fails closed
// when a residual-named test file is not allowlisted.
func TestResidualCoverageInventory_RejectsUnknown(t *testing.T) {
	log := testutil.New(t)
	log.Phase("reject_unknown")
	tmp := t.TempDir()
	pkgDir := filepath.Join(tmp, "internal", "fakepkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Basename matches residual naming (not residual_policy_*).
	path := filepath.Join(pkgDir, "padding_residual_test.go")
	if err := os.WriteFile(path, []byte("package fakepkg_test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	vs, err := archtest.CheckResidualCoverageInventory(tmp)
	if err != nil {
		log.Fail("check", err.Error())
	}
	log.Assert("has_violation", len(vs) >= 1, true, len(vs))
	found := false
	for _, v := range vs {
		if v.Rule == "residual-coverage-allowlist" && strings.Contains(v.File, "padding_residual_test.go") {
			found = true
			log.Step("detail", testutil.OutcomeOK, v.Detail)
		}
	}
	log.Assert("names_unknown_file", found, true, found)
	log.PhaseEnd("reject_unknown", testutil.OutcomeOK)
}
