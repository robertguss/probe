package archtest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ResidualCoverageAllowlist is the inventory of residual_*_test.go and
// coverage_*_test.go files permitted under the residual/coverage policy
// (docs/dev/testing.md, bead go-foundry-cli-ipk.3).
//
// New residual/coverage files MUST NOT be added without:
//  1. Public-contract tests covering the same behavior (not only *ForTest /
//     export_test branch hits), and
//  2. An explicit allowlist update in this map with a short rationale.
//
// Prefer rewriting pure line-chasing tests into scenario tests, or deleting
// them when public packages already meet coverage floors without them.
var ResidualCoverageAllowlist = map[string]string{
	"internal/catalog/residual_test.go":       "nil/load public accessors",
	"internal/cli/residual_test.go":           "CLI residual edges; prefer command scenarios",
	"internal/diagnostic/coverage_test.go":    "nil FoundryError + location contracts",
	"internal/fsx/coverage_accessors_test.go": "fsx edge paths; keep floors via commit/stage scenarios",
	"internal/plan/residual_test.go":          "Construct/VerifyMode edge contracts",
	"internal/render/residual_test.go":        "render residual; prefer golden/scenario tests",
	"internal/report/coverage_test.go":        "encoder/report residual",
	"internal/resolve/coverage_test.go":       "resolve residual; prefer property/suite tests",
	"internal/spec/coverage_test.go":          "spec external coverage; prefer suite/validate",
	"internal/spec/internal_coverage_test.go": "white-box helpers; stdlib only (no testutil)",
	"internal/testutil/residual_test.go":      "logger/plandiff helper edges",
	"internal/verify/residual_test.go":        "verify pathset/only-steps contracts",
	"internal/version/coverage_test.go":       "version.Read/WithCatalog public shape",
}

// residualCoverageName reports whether a basename matches residual/coverage
// test file naming conventions used by the inventory gate.
//
// Matches: residual_test.go, *_residual_test.go, coverage_test.go,
// coverage_*_test.go, *_coverage_test.go. Does not match incidental names
// like residual_policy_test.go (policy gate itself).
func residualCoverageName(base string) bool {
	if !strings.HasSuffix(base, "_test.go") {
		return false
	}
	switch {
	case base == "residual_test.go", base == "coverage_test.go":
		return true
	case strings.HasSuffix(base, "_residual_test.go"):
		return true
	case strings.HasPrefix(base, "coverage_") && strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasSuffix(base, "_coverage_test.go"):
		return true
	default:
		return false
	}
}

// CheckResidualCoverageInventory walks internal/ and cmd/ for residual_* and
// coverage_* test files and returns violations for unlisted paths or missing
// allowlisted paths (deleted without updating the allowlist).
func CheckResidualCoverageInventory(root string) ([]Violation, error) {
	var found []string
	for _, sub := range []string{"internal", "cmd"} {
		base := filepath.Join(root, sub)
		if _, err := fsStat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Skip vendor-like and large generated trees if any.
				name := d.Name()
				if name == "testdata" || name == "vendor" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !residualCoverageName(d.Name()) {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			// Normalize to slash form for allowlist keys.
			found = append(found, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(found)

	var vs []Violation
	seen := make(map[string]bool, len(found))
	for _, path := range found {
		seen[path] = true
		if _, ok := ResidualCoverageAllowlist[path]; !ok {
			vs = append(vs, Violation{
				Rule:    "residual-coverage-allowlist",
				Package: filepath.Dir(path),
				File:    path,
				Detail: fmt.Sprintf(
					"new residual/coverage test %q is not on ResidualCoverageAllowlist; "+
						"add public-contract tests for the same behavior, then update the allowlist "+
						"with rationale (docs/dev/testing.md residual policy)",
					path,
				),
			})
		}
	}
	// Detect allowlist drift when a file was removed without updating the map.
	var missing []string
	for path := range ResidualCoverageAllowlist {
		if !seen[path] {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	for _, path := range missing {
		vs = append(vs, Violation{
			Rule:    "residual-coverage-allowlist",
			Package: filepath.Dir(path),
			File:    path,
			Detail: fmt.Sprintf(
				"allowlisted residual/coverage file %q is missing on disk; remove it from ResidualCoverageAllowlist",
				path,
			),
		})
	}
	return vs, nil
}

// fsStat is a thin os.Stat wrapper for optional tree roots.
func fsStat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}
