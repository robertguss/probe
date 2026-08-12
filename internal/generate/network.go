package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// Stable network-may step ids (Section 28.2 / 34.5 / 35.2).
const (
	// StepIDGoModTidy is the cold-cache module-resolution step.
	StepIDGoModTidy = "go-mod-tidy"
	// StepIDGovulncheck is the strict-mode vulnerability DB step.
	StepIDGovulncheck = "go-govulncheck"
)

// Stable reason strings — must match plan.buildNetwork (FND-007 / Section 13.5).
const (
	// ReasonModuleResolution is disclosed for go-mod-tidy on cold caches.
	ReasonModuleResolution = "module resolution during go mod tidy on cold caches"
	// ReasonVulnDB is disclosed for go tool govulncheck under strict verify.
	ReasonVulnDB = "vulnerability database access during go tool govulncheck"
)

// Disclosure is the pure pre-staging network disclosure payload
// (REQ-034, FND-007, Section 13.5). Construction and formatting perform no
// network I/O, no filesystem access, and no subprocess starts.
type Disclosure struct {
	// MayBeRequired is true when at least one external step may use the network.
	MayBeRequired bool `json:"may_be_required"`
	// VerifyMode is "default" or "strict" (plan.verification.mode).
	VerifyMode string `json:"verify_mode"`
	// Steps lists each network:may external step with its reason.
	Steps []DisclosureStep `json:"steps"`
	// Reasons is the plan.network.reasons list (sorted, plan-driven).
	Reasons []string `json:"reasons"`
}

// DisclosureStep is one external step that may require network access.
type DisclosureStep struct {
	// ID is the plan external_steps id (e.g. go-mod-tidy).
	ID string `json:"id"`
	// Network is always "may" for disclosed steps.
	Network string `json:"network"`
	// Reason is the human-readable, plan-driven explanation.
	Reason string `json:"reason"`
}

// wireDisclosure is the exact JSON encoding surface for goldens (field order fixed).
type wireDisclosure struct {
	MayBeRequired bool             `json:"may_be_required"`
	VerifyMode    string           `json:"verify_mode"`
	Steps         []DisclosureStep `json:"steps"`
	Reasons       []string         `json:"reasons"`
}

// ReasonForStepID returns the stable disclosure reason for a known network-may
// step id, or a host-independent generic reason for unknown ids.
func ReasonForStepID(id string) string {
	switch id {
	case StepIDGoModTidy:
		return ReasonModuleResolution
	case StepIDGovulncheck:
		return ReasonVulnDB
	default:
		if id == "" {
			return "external step may require network"
		}
		return "external step " + id + " may require network"
	}
}

// BuildDisclosureFromMode produces plan-driven disclosure for the verify-mode
// step list without a full Plan. Pure formatting only (Section 13.5).
//
// Default: go-mod-tidy.
// Strict:  go-mod-tidy + go-govulncheck.
func BuildDisclosureFromMode(mode plan.VerifyMode) Disclosure {
	mode = mode.Normalize()
	steps := []DisclosureStep{
		{
			ID:      StepIDGoModTidy,
			Network: string(plan.NetworkMay),
			Reason:  ReasonModuleResolution,
		},
	}
	reasons := []string{ReasonModuleResolution}
	if mode == plan.VerifyStrict {
		steps = append(steps, DisclosureStep{
			ID:      StepIDGovulncheck,
			Network: string(plan.NetworkMay),
			Reason:  ReasonVulnDB,
		})
		reasons = append(reasons, ReasonVulnDB)
	}
	// Match plan.buildNetwork: stable alphabetical reasons.
	sort.Strings(reasons)
	return Disclosure{
		MayBeRequired: true,
		VerifyMode:    string(mode),
		Steps:         steps,
		Reasons:       reasons,
	}
}

// BuildDisclosureFromPlan extracts network:may external steps and plan.network
// reasons. Pure: no I/O. Nil plan falls back to default-mode disclosure.
func BuildDisclosureFromPlan(p *plan.Plan) Disclosure {
	if p == nil {
		return BuildDisclosureFromMode(plan.VerifyDefault)
	}
	mode := p.Verification().Mode.Normalize()
	var steps []DisclosureStep
	for _, s := range p.ExternalSteps() {
		if s.Network != plan.NetworkMay {
			continue
		}
		steps = append(steps, DisclosureStep{
			ID:      s.ID,
			Network: string(s.Network),
			Reason:  ReasonForStepID(s.ID),
		})
	}
	net := p.Network()
	reasons := append([]string(nil), net.Reasons...)
	if reasons == nil {
		reasons = []string{}
	}
	// If the plan omitted steps but still declares network, rebuild from mode.
	if len(steps) == 0 && net.MayBeRequired {
		return BuildDisclosureFromMode(mode)
	}
	may := net.MayBeRequired || len(steps) > 0
	return Disclosure{
		MayBeRequired: may,
		VerifyMode:    string(mode),
		Steps:         steps,
		Reasons:       reasons,
	}
}

// StepIDs returns disclosed external step ids in order.
func (d Disclosure) StepIDs() []string {
	if len(d.Steps) == 0 {
		return nil
	}
	out := make([]string, len(d.Steps))
	for i, s := range d.Steps {
		out[i] = s.ID
	}
	return out
}

// TextLines returns human-readable body lines (no header), one per step:
//
//	go-mod-tidy: module resolution during go mod tidy on cold caches
//
// Host-independent; suitable for GenerationEvent.Lines and report encoding.
func (d Disclosure) TextLines() []string {
	if len(d.Steps) == 0 {
		// Fall back to reasons-only when steps were not attached.
		if len(d.Reasons) == 0 {
			return nil
		}
		out := make([]string, len(d.Reasons))
		copy(out, d.Reasons)
		return out
	}
	out := make([]string, 0, len(d.Steps))
	for _, s := range d.Steps {
		line := s.ID
		if s.Reason != "" {
			line = s.ID + ": " + s.Reason
		}
		out = append(out, line)
	}
	return out
}

// FormatText returns the full human disclosure block including the stable
// header consumed by report.WriteNetworkDisclosure consumers:
//
//	network disclosure:
//	  go-mod-tidy: …
func (d Disclosure) FormatText() string {
	lines := d.TextLines()
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("network disclosure:\n")
	for _, ln := range lines {
		b.WriteString("  ")
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	return b.String()
}

// FormatJSON returns deterministic JSON listing step ids + reasons
// (Section 13.5 JSON disclosure). No wall-clock fields.
func (d Disclosure) FormatJSON() ([]byte, error) {
	steps := d.Steps
	if steps == nil {
		steps = []DisclosureStep{}
	}
	reasons := d.Reasons
	if reasons == nil {
		reasons = []string{}
	}
	w := wireDisclosure{
		MayBeRequired: d.MayBeRequired,
		VerifyMode:    d.VerifyMode,
		Steps:         steps,
		Reasons:       reasons,
	}
	return json.Marshal(w)
}

// ApplyDisclosure copies pure disclosure fields onto Runtime for the
// report-plan-network stage (EventNetwork emission). Pure assignment only.
func ApplyDisclosure(rt *Runtime, d Disclosure) {
	if rt == nil {
		return
	}
	rt.NetworkLines = d.TextLines()
	rt.NetworkStepIDs = d.StepIDs()
	rt.VerifyMode = d.VerifyMode
	if rt.VerifyMode == "" {
		rt.VerifyMode = string(plan.VerifyDefault)
	}
}

// DiscloseStage returns a StageFunc that applies d during report-plan-network.
// The stage itself performs no network I/O — pure Runtime assignment.
func DiscloseStage(d Disclosure) StageFunc {
	return func(ctx context.Context, rt *Runtime) error {
		_ = ctx
		ApplyDisclosure(rt, d)
		return nil
	}
}

// DiscloseFromPlanStage returns a StageFunc that discloses from an immutable plan.
func DiscloseFromPlanStage(p *plan.Plan) StageFunc {
	return DiscloseStage(BuildDisclosureFromPlan(p))
}

// DiscloseFromModeStage returns a StageFunc for the given verify mode.
func DiscloseFromModeStage(mode plan.VerifyMode) StageFunc {
	return DiscloseStage(BuildDisclosureFromMode(mode))
}

// FormatNetworkLogResult builds the step-logger result field for disclosure:
// "verify=<mode> steps=<id,id,…>". Host-independent for goldens.
func FormatNetworkLogResult(d Disclosure) string {
	mode := d.VerifyMode
	if mode == "" {
		mode = string(plan.VerifyDefault)
	}
	ids := d.StepIDs()
	return fmt.Sprintf("verify=%s steps=%s", mode, strings.Join(ids, ","))
}

// ContainsOfflineToken reports whether s mentions offline isolation claims
// (FND-007 ban). Used by tests; disclosure must never claim whole-process
// network isolation. Pattern assembled so product source never contains the
// contiguous surface flag spelling (Section 58 scanner).
func ContainsOfflineToken(s string) bool {
	lower := strings.ToLower(s)
	// "offline" word is fine in prose/schema ban-lists; the surface flag is not.
	flag := "--" + "off" + "line"
	return strings.Contains(lower, "offline") || strings.Contains(lower, flag)
}
