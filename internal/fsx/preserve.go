package fsx

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// RSK310 is the residual-risk identifier for preserved-stage debris burden
// (SPEC-FOUNDRY-002 Section 55 / RSK-310). User-facing remediations cite it so
// operators and agents know Foundry never auto-deletes stages.
const RSK310 = "RSK-310"

// StageLocation returns the diagnostic path for a stage entry under the
// authored parent path (Section 31.6). It is for reporting only — never use
// the result for pathname-based mutation of the destination namespace.
//
// Empty components are omitted; a lone basename is returned when parent is
// unknown (fail-closed diagnostics still name the stage entry).
func StageLocation(parentAuthored, stageName string) string {
	parentAuthored = strings.TrimSpace(parentAuthored)
	stageName = strings.TrimSpace(stageName)
	switch {
	case parentAuthored == "" && stageName == "":
		return ""
	case parentAuthored == "":
		return stageName
	case stageName == "":
		return parentAuthored
	default:
		return filepath.Join(parentAuthored, stageName)
	}
}

// ManualRemovalRemediation returns the Section 31.6 / RSK-310 one-line owner
// remediation: inspect the preserved stage, then remove or rename it
// deliberately. Foundry never auto-deletes stages.
//
// When stagePath is non-empty it is embedded so the owner can copy-paste the
// inspect/remove commands after review.
func ManualRemovalRemediation(stagePath string) string {
	stagePath = strings.TrimSpace(stagePath)
	if stagePath == "" {
		return "Inspect any preserved stage under the destination parent " +
			"(basename .foundry-<name>-<random>, mode 0700). " +
			"Foundry never auto-deletes stages (" + RSK310 + "); " +
			"after inspection, remove with rm -rf <stage> or rename into place with mv."
	}
	return fmt.Sprintf(
		"Inspect preserved stage at %s (mode 0700), then remove with `rm -rf %s` "+
			"or rename into place with `mv` after identity check. "+
			"Foundry never auto-deletes stages (%s).",
		stagePath, stagePath, RSK310,
	)
}

// PreserveReport is the Section 31.6 reporting payload for a preserved stage.
// Human text and JSON surfaces MUST include StagePath when non-empty
// (REQ-130/158). Callers (generate/report) pass StagePath into
// report.Encoder.Failure / GenerateResult without re-deriving it.
type PreserveReport struct {
	// StagePath is parent-authored path joined with the stage basename.
	StagePath string `json:"stage_path,omitempty"`
	// StageName is the stage basename under the parent (.foundry-<name>-<random>).
	StageName string `json:"stage_name,omitempty"`
	// ParentPath is the authored destination parent path (diagnostics only).
	ParentPath string `json:"parent_path,omitempty"`
	// DestName is the intended destination basename when known.
	DestName string `json:"dest_name,omitempty"`
	// Class is the outcome class token (commit class or failure class label).
	Class string `json:"class,omitempty"`
	// ErrorID is the Appendix D identifier when applicable.
	ErrorID string `json:"error_id,omitempty"`
	// Exit is the process exit code for this outcome.
	Exit int `json:"exit,omitempty"`
	// Remediation is the RSK-310 manual inspect/remove text.
	Remediation string `json:"remediation,omitempty"`
	// RiskID is always RSK-310 when a stage is preserved.
	RiskID string `json:"risk_id,omitempty"`
}

// Preserved reports whether this commit result leaves a stage entry that the
// owner must inspect (not committed). Empty StageName means no stage was known.
func (r CommitResult) Preserved() bool {
	return r.Class != ClassCommitted && r.StageName != ""
}

// Report returns the Section 31.6 PreserveReport for this CommitResult.
// StagePath is populated whenever StageName is known (including committed,
// where the path names the pre-rename stage entry for logs only). RiskID and
// Remediation are set when the stage is preserved (not committed).
func (r CommitResult) Report() PreserveReport {
	pr := PreserveReport{
		StagePath: r.StagePath,
		StageName: r.StageName,
		DestName:  r.DestName,
		Class:     string(r.Class),
		Exit:      r.Exit,
		ErrorID:   string(r.ErrorID()),
	}
	if pr.StagePath == "" {
		pr.StagePath = StageLocation("", r.StageName)
	}
	if r.Preserved() {
		pr.RiskID = RSK310
		pr.Remediation = ManualRemovalRemediation(pr.StagePath)
		// Prefer explicit remediation from the FoundryError when present
		// (includes stage path + RSK-310 from newCommitError).
		if fe, ok := diagnostic.AsFoundryError(r.Err); ok {
			if rem := fe.Remediation(); rem != "" {
				pr.Remediation = rem
			}
		}
	}
	return pr
}

// attachPreserveRemediation returns a FoundryError with RSK-310 manual-removal
// remediation naming stagePath. If fe is nil, returns nil.
func attachPreserveRemediation(fe *diagnostic.FoundryError, stagePath string) *diagnostic.FoundryError {
	if fe == nil {
		return nil
	}
	// Compose: registry/default meaning first when present, then RSK-310 line
	// with the concrete stage path so agents always see exact location + commands.
	base := fe.Remediation()
	rsk := ManualRemovalRemediation(stagePath)
	if base == "" || base == rsk {
		return fe.WithRemediation(rsk)
	}
	// Avoid duplicating when base already cites RSK-310 / stage path.
	if strings.Contains(base, RSK310) && (stagePath == "" || strings.Contains(base, stagePath)) {
		return fe
	}
	return fe.WithRemediation(base + " " + rsk)
}

// withPreserveRemediation is attachPreserveRemediation for error values.
func withPreserveRemediation(err error, stagePath string) error {
	if err == nil {
		return nil
	}
	fe, ok := diagnostic.AsFoundryError(err)
	if !ok {
		return err
	}
	return attachPreserveRemediation(fe, stagePath)
}
