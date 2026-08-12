package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/archtest"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestProfileAdmissionDocHeadings ensures the Section 19.4 / REQ-078 admission
// process doc exists and retains the checklist structure reviewers depend on.
// Vague profile proposals are blocked by process; this test keeps the process
// document from rotting (go-foundry-cli-tpa).
func TestProfileAdmissionDocHeadings(t *testing.T) {
	log := testutil.New(t)
	root := repoRoot(t)

	log.Phase("load")
	rel := archtest.DocProfileAdmissionPath
	log.Fixture("profile_admission", rel)
	path := filepath.Join(root, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fail("read_doc", err.Error())
	}
	body := string(data)
	log.Inputs(map[string]string{
		"bytes":    itoa(len(data)),
		"headings": itoa(len(archtest.RequiredProfileAdmissionHeadings)),
		"needles":  itoa(len(archtest.RequiredProfileAdmissionNeedles)),
	})
	log.PhaseEnd("load", testutil.OutcomeOK)

	log.Phase("assert_headings")
	var missingHeadings []string
	for _, h := range archtest.RequiredProfileAdmissionHeadings {
		present := linePresent(body, h)
		log.Assert("heading:"+sanitize(h), present, true, present)
		if !present {
			missingHeadings = append(missingHeadings, h)
		}
	}
	if len(missingHeadings) > 0 {
		t.Fatalf("docs/dev/profile-admission.md missing required headings:\n  - %s\n"+
			"Restore the Section 19.4 process headings; see archtest.RequiredProfileAdmissionHeadings.",
			strings.Join(missingHeadings, "\n  - "))
	}
	log.PhaseEnd("assert_headings", testutil.OutcomeOK)

	log.Phase("assert_needles")
	var missingNeedles []string
	for _, n := range archtest.RequiredProfileAdmissionNeedles {
		ok := strings.Contains(body, n)
		log.Assert("needle:"+sanitize(n), ok, true, ok)
		if !ok {
			missingNeedles = append(missingNeedles, n)
		}
	}
	if len(missingNeedles) > 0 {
		t.Fatalf("docs/dev/profile-admission.md missing required content needles:\n  - %s\n"+
			"Checklist / REQ-078 / recipe holdouts must stay explicit.",
			strings.Join(missingNeedles, "\n  - "))
	}
	log.PhaseEnd("assert_needles", testutil.OutcomeOK)

	// Authority link shape: relative path to the specification from docs/dev/.
	log.Phase("assert_authority_link")
	linkOK := strings.Contains(body, "02-definitive-foundry-specification-revised-fable-5.md")
	log.Assert("spec_link", linkOK, true, linkOK)
	if !linkOK {
		t.Fatal("profile-admission.md must link to the SPEC-FOUNDRY-002 authority document")
	}
	log.PhaseEnd("assert_authority_link", testutil.OutcomeOK)
}

// linePresent reports whether exactLine appears as a full line (trimmed match
// allowed only for trailing whitespace). Headings must not be buried mid-line.
func linePresent(body, exactLine string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimRight(line, "\r\t ") == exactLine {
			return true
		}
	}
	return false
}
