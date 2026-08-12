package generatee2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ArtifactEnvVar names the CI directory for generate e2e failure dumps.
// Set by the linux-generate-e2e / macos-generate-e2e jobs.
const ArtifactEnvVar = "FOUNDRY_GENERATE_E2E_ARTIFACT_DIR"

// FailureArtifact is a redacted, agent-actionable dump for one matrix cell.
// Never includes secret-bearing env values (REQ-214 style redaction).
type FailureArtifact struct {
	// Case is the matrix case name (basename-safe).
	Case string
	// Stage is the Section 29.2 stage id active at failure (or "none").
	Stage string
	// Cause is a short machine-facing reason (error id, CommitResult, cancel class).
	Cause string
	// PlanSHA256 is the plan identity when known.
	PlanSHA256 string
	// CommitResult is the Section 31.9 outcome enum when known.
	CommitResult string
	// Exit is the process exit code.
	Exit int
	// StagePath is the preserved stage location when present (basename-preferred in body).
	StagePath string
	// Destination is the planned destination basename.
	Destination string
	// Remediation is operator guidance when present.
	Remediation string
	// Timeline is stage/progress lines (already host-independent when possible).
	Timeline []string
	// ExternalSteps summarizes plan external_steps ids (no env values).
	ExternalSteps []string
	// Detail is free-form non-secret diagnostic text.
	Detail string
	// Stream notes which stream failed if any (stdout/stderr/"").
	Stream string
}

// FormatFailureArtifact renders a stable multi-line dump.
func FormatFailureArtifact(a FailureArtifact) string {
	var b strings.Builder
	b.WriteString("generate_e2e_failure\n")
	fmt.Fprintf(&b, "case=%s\n", sanitizeBase(a.Case))
	fmt.Fprintf(&b, "stage=%s\n", emptyDash(a.Stage))
	fmt.Fprintf(&b, "cause=%s\n", emptyDash(a.Cause))
	fmt.Fprintf(&b, "plan_sha256=%s\n", emptyDash(a.PlanSHA256))
	fmt.Fprintf(&b, "commit_result=%s\n", emptyDash(a.CommitResult))
	fmt.Fprintf(&b, "exit=%d\n", a.Exit)
	fmt.Fprintf(&b, "destination=%s\n", emptyDash(filepath.Base(a.Destination)))
	if a.StagePath != "" {
		// Keep path for operator remount; note basename for grepping.
		fmt.Fprintf(&b, "stage_path=%s\n", a.StagePath)
		fmt.Fprintf(&b, "stage_basename=%s\n", filepath.Base(a.StagePath))
	} else {
		b.WriteString("stage_path=\n")
	}
	if a.Stream != "" {
		fmt.Fprintf(&b, "stream=%s\n", a.Stream)
	}
	if a.Remediation != "" {
		fmt.Fprintf(&b, "remediation=%s\n", oneLine(a.Remediation))
	}
	if len(a.ExternalSteps) > 0 {
		fmt.Fprintf(&b, "external_steps=%s\n", strings.Join(a.ExternalSteps, ","))
	}
	if a.Detail != "" {
		fmt.Fprintf(&b, "detail=%s\n", oneLine(a.Detail))
	}
	if len(a.Timeline) > 0 {
		b.WriteString("timeline:\n")
		for _, ln := range a.Timeline {
			fmt.Fprintf(&b, "  %s\n", oneLine(ln))
		}
	}
	return b.String()
}

// WriteFailureArtifact writes FormatFailureArtifact under dir (or ArtifactEnvVar).
// Returns the path written, or "" when no artifact directory is configured.
func WriteFailureArtifact(dir string, a FailureArtifact) (string, error) {
	if dir == "" {
		dir = os.Getenv(ArtifactEnvVar)
	}
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := sanitizeBase(a.Case)
	if base == "" {
		base = "generate_e2e"
	}
	// Filename encodes stage + cause so CI artifact browsers identify the cell.
	stageTok := sanitizeBase(a.Stage)
	if stageTok == "" {
		stageTok = "none"
	}
	causeTok := sanitizeBase(a.Cause)
	if causeTok == "" {
		causeTok = "unknown"
	}
	name := fmt.Sprintf("%s__stage-%s__cause-%s__%d.txt",
		base, stageTok, causeTok, time.Now().UTC().UnixNano())
	path := filepath.Join(dir, name)
	body := FormatFailureArtifact(a)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func sanitizeBase(s string) string {
	s = filepath.Base(s)
	if s == "" || s == "." || s == "/" {
		return ""
	}
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > 500 {
		return s[:500] + "…"
	}
	return s
}
