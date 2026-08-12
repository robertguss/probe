package archtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DocTestingPath is the maintainer testing guide (golden discipline docs).
const DocTestingPath = "docs/dev/testing.md"

// EnvUpdateGolden is the sole golden-update opt-in (must match testutil).
const EnvUpdateGolden = "UPDATE_GOLDEN"

// EnvCI is the CI indicator that refuses golden updates (must match testutil).
const EnvCI = "CI"

// GoldenBulkThreshold is the documented maximum number of golden files a single
// named suite may rewrite without a second explicit opt-in (REQ-219 / §45.4).
// Suites that bulk-update more files must document a second env flag themselves.
// The archtest suite enforces documentation of this constant; automatic bulk
// rewriting is never performed by CompareGolden.
const GoldenBulkThreshold = 20

// GoldenDisciplineViolation is one REQ-219 CI / policy failure.
type GoldenDisciplineViolation struct {
	Rule   string
	File   string
	Detail string
}

func (v GoldenDisciplineViolation) String() string {
	if v.File != "" {
		return fmt.Sprintf("golden_discipline: rule=%s file=%s detail=%s", v.Rule, v.File, v.Detail)
	}
	return fmt.Sprintf("golden_discipline: rule=%s detail=%s", v.Rule, v.Detail)
}

// CheckGoldenDiscipline enforces REQ-219 golden update policy against the tree:
//
//  1. CI workflows must set CI=true (so CompareGolden refuses UPDATE_GOLDEN).
//  2. CI workflows must never set UPDATE_GOLDEN (no auto-update path).
//  3. docs/dev/testing.md must document UPDATE_GOLDEN and the CI refuse rule.
//  4. Bulk threshold is documented via GoldenBulkThreshold (>0).
func CheckGoldenDiscipline(root string) ([]GoldenDisciplineViolation, error) {
	var all []GoldenDisciplineViolation

	if GoldenBulkThreshold <= 0 {
		all = append(all, GoldenDisciplineViolation{
			Rule:   "bulk_threshold",
			Detail: "GoldenBulkThreshold must be a positive documented suite threshold",
		})
	}

	// Testing guide must document the discipline.
	docPath := filepath.Join(root, DocTestingPath)
	doc, err := os.ReadFile(docPath)
	if err != nil {
		all = append(all, GoldenDisciplineViolation{
			Rule:   "testing_doc",
			File:   DocTestingPath,
			Detail: "docs/dev/testing.md missing: " + err.Error(),
		})
	} else {
		text := string(doc)
		if !strings.Contains(text, EnvUpdateGolden) {
			all = append(all, GoldenDisciplineViolation{
				Rule:   "testing_doc",
				File:   DocTestingPath,
				Detail: "testing guide must document " + EnvUpdateGolden + " opt-in",
			})
		}
		// Accept any of the documented CI refuse phrasings from the guide/spec.
		ciRefuse := strings.Contains(text, "never auto-update") ||
			strings.Contains(text, "CI must never") ||
			strings.Contains(text, "must never auto-update") ||
			(strings.Contains(text, "CI") && strings.Contains(text, "refuses"))
		if !ciRefuse {
			all = append(all, GoldenDisciplineViolation{
				Rule:   "testing_doc",
				File:   DocTestingPath,
				Detail: "testing guide must state CI never auto-updates goldens",
			})
		}
		if !strings.Contains(text, "one named suite") && !strings.Contains(text, "one suite at a time") {
			all = append(all, GoldenDisciplineViolation{
				Rule:   "testing_doc",
				File:   DocTestingPath,
				Detail: "testing guide must require one named suite at a time",
			})
		}
	}

	// Workflow scan.
	wfDir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(wfDir)
	if err != nil {
		if os.IsNotExist(err) {
			all = append(all, GoldenDisciplineViolation{
				Rule:   "ci_workflow",
				File:   ".github/workflows",
				Detail: "no CI workflows directory; CI must set CI=true and refuse golden updates",
			})
			return all, nil
		}
		return all, err
	}

	var workflowCount int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		workflowCount++
		rel := filepath.ToSlash(filepath.Join(".github", "workflows", name))
		body, err := os.ReadFile(filepath.Join(wfDir, name))
		if err != nil {
			return all, err
		}
		text := string(body)

		// Must not set UPDATE_GOLDEN anywhere in CI config.
		if strings.Contains(text, EnvUpdateGolden) {
			all = append(all, GoldenDisciplineViolation{
				Rule:   "ci_no_update_golden",
				File:   rel,
				Detail: EnvUpdateGolden + " must never appear in CI workflows (REQ-219)",
			})
		}

		// Must set CI indicator so testutil.InCI() is true.
		// Accept env: CI: "true" / CI: true / CI=true forms.
		if !workflowSetsCI(text) {
			all = append(all, GoldenDisciplineViolation{
				Rule:   "ci_sets_ci_env",
				File:   rel,
				Detail: "workflow must set " + EnvCI + "=true so golden updates are refused",
			})
		}
	}
	if workflowCount == 0 {
		all = append(all, GoldenDisciplineViolation{
			Rule:   "ci_workflow",
			File:   ".github/workflows",
			Detail: "no CI workflow files found",
		})
	}

	return all, nil
}

func workflowSetsCI(text string) bool {
	// Common GitHub Actions forms:
	//   CI: "true"
	//   CI: true
	//   CI: 'true'
	// Also export-style in run scripts is weaker; require env: block style.
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		// Skip comments.
		if strings.HasPrefix(trim, "#") {
			continue
		}
		// CI: "true" | CI: true | CI: 'true' | CI: 1
		if strings.HasPrefix(trim, "CI:") {
			rest := strings.TrimSpace(strings.TrimPrefix(trim, "CI:"))
			rest = strings.Trim(rest, `"'`)
			switch rest {
			case "true", "TRUE", "True", "1", "yes", "YES":
				return true
			}
		}
	}
	return false
}
