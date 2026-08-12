package plan

import (
	"fmt"
	"strings"
)

// Plan is the immutable Generation Plan (Section 28 / REQ-120–122).
//
// Construct only via Construct / Pipeline. Safe for concurrent read after
// construction. Accessors return defensive copies of collections.
type Plan struct {
	schema            int
	foundry           FoundryMeta
	specification     SpecificationRef
	project           ProjectInfo
	destination       DestinationInfo
	profiles          []string
	files             []FileEntry
	dependencies      []DependencyEntry
	tools             []ToolEntry
	externalSteps     []ExternalStep
	toolOutputs       []ToolOutput
	verification      Verification
	network           NetworkDisclosure
	git               GitInfo
	commitResultModel string
	planSHA256        string
	warnings          []string

	// canonicalJSON is the full marshaled form including plan_sha256 (cached).
	canonicalJSON []byte
}

// Schema returns the plan schema version (always 1).
func (p *Plan) Schema() int {
	if p == nil {
		return 0
	}
	return p.schema
}

// Foundry returns Foundry identity metadata.
func (p *Plan) Foundry() FoundryMeta {
	if p == nil {
		return FoundryMeta{}
	}
	return p.foundry
}

// Specification returns the specification source reference.
func (p *Plan) Specification() SpecificationRef {
	if p == nil {
		return SpecificationRef{}
	}
	return p.specification
}

// Project returns project identity fields.
func (p *Plan) Project() ProjectInfo {
	if p == nil {
		return ProjectInfo{}
	}
	return p.project
}

// Destination returns destination identity and observation.
func (p *Plan) Destination() DestinationInfo {
	if p == nil {
		return DestinationInfo{}
	}
	return p.destination
}

// Profiles returns a copy of the sorted selected profile IDs (never nil after Construct).
func (p *Plan) Profiles() []string {
	if p == nil {
		return nil
	}
	return copyStrings(p.profiles)
}

// Files returns a copy of planned files sorted by path.
func (p *Plan) Files() []FileEntry {
	if p == nil {
		return nil
	}
	out := make([]FileEntry, len(p.files))
	copy(out, p.files)
	return out
}

// FileCount returns the number of planned output files.
func (p *Plan) FileCount() int {
	if p == nil {
		return 0
	}
	return len(p.files)
}

// Dependencies returns a copy of dependency entries.
func (p *Plan) Dependencies() []DependencyEntry {
	if p == nil {
		return nil
	}
	out := make([]DependencyEntry, len(p.dependencies))
	copy(out, p.dependencies)
	return out
}

// Tools returns a copy of tool entries.
func (p *Plan) Tools() []ToolEntry {
	if p == nil {
		return nil
	}
	out := make([]ToolEntry, len(p.tools))
	copy(out, p.tools)
	return out
}

// ExternalSteps returns a deep copy of planned subprocess steps.
func (p *Plan) ExternalSteps() []ExternalStep {
	if p == nil {
		return nil
	}
	return copySteps(p.externalSteps)
}

// ExternalStepCount returns the number of external steps.
func (p *Plan) ExternalStepCount() int {
	if p == nil {
		return 0
	}
	return len(p.externalSteps)
}

// ToolOutputs returns a copy of tool-output declarations.
func (p *Plan) ToolOutputs() []ToolOutput {
	if p == nil {
		return nil
	}
	out := make([]ToolOutput, len(p.toolOutputs))
	for i, t := range p.toolOutputs {
		out[i] = ToolOutput{Path: t.Path, Steps: copyStrings(t.Steps)}
	}
	return out
}

// Verification returns the verification mode and ordered checks.
func (p *Plan) Verification() Verification {
	if p == nil {
		return Verification{}
	}
	return Verification{
		Mode:   p.verification.Mode,
		Checks: copyStrings(p.verification.Checks),
	}
}

// Network returns network disclosure.
func (p *Plan) Network() NetworkDisclosure {
	if p == nil {
		return NetworkDisclosure{}
	}
	return NetworkDisclosure{
		MayBeRequired: p.network.MayBeRequired,
		Reasons:       copyStrings(p.network.Reasons),
	}
}

// Git returns git plan fields.
func (p *Plan) Git() GitInfo {
	if p == nil {
		return GitInfo{}
	}
	return p.git
}

// CommitResultModel returns the Section 31.9 matrix reference.
func (p *Plan) CommitResultModel() string {
	if p == nil {
		return ""
	}
	return p.commitResultModel
}

// PlanSHA256 returns the plan digest (SHA-256 hex of canonical JSON excluding this field).
func (p *Plan) PlanSHA256() string {
	if p == nil {
		return ""
	}
	return p.planSHA256
}

// Warnings returns a copy of sorted stable warnings.
func (p *Plan) Warnings() []string {
	if p == nil {
		return nil
	}
	return copyStrings(p.warnings)
}

// JSON returns the canonical plan JSON bytes (including plan_sha256 and trailing newline).
// The slice is a defensive copy; mutating it does not affect the Plan.
func (p *Plan) JSON() []byte {
	if p == nil {
		return nil
	}
	out := make([]byte, len(p.canonicalJSON))
	copy(out, p.canonicalJSON)
	return out
}

// Equal reports whether two plans have identical canonical JSON (REQ-122).
func (p *Plan) Equal(o *Plan) bool {
	if p == nil || o == nil {
		return p == o
	}
	if len(p.canonicalJSON) != len(o.canonicalJSON) {
		return false
	}
	for i := range p.canonicalJSON {
		if p.canonicalJSON[i] != o.canonicalJSON[i] {
			return false
		}
	}
	return true
}

// String returns a short debug summary (not a serialization format).
func (p *Plan) String() string {
	if p == nil {
		return "Plan(nil)"
	}
	return fmt.Sprintf(
		"Plan{schema=%d project=%q files=%d steps=%d sha256=%s}",
		p.schema, p.project.Name, len(p.files), len(p.externalSteps), truncateHex(p.planSHA256, 12),
	)
}

// Summary returns a log-friendly one-line description for step logs.
func Summary(p *Plan) string {
	if p == nil {
		return "plan: <nil>"
	}
	return fmt.Sprintf(
		"project=%s archetype=%s files=%d steps=%d profiles=%s plan_sha256=%s",
		p.project.Name, p.project.Archetype, len(p.files), len(p.externalSteps),
		formatIDSet(p.profiles), p.planSHA256,
	)
}

func formatIDSet(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}
	return "[" + strings.Join(ids, ", ") + "]"
}

func truncateHex(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copySteps(in []ExternalStep) []ExternalStep {
	out := make([]ExternalStep, len(in))
	for i, s := range in {
		out[i] = ExternalStep{
			ID:             s.ID,
			Binary:         s.Binary,
			Argv:           copyStrings(s.Argv),
			Cwd:            s.Cwd,
			Mutates:        copyStrings(s.Mutates),
			Network:        s.Network,
			TimeoutS:       s.TimeoutS,
			OutputCapBytes: s.OutputCapBytes,
			Env:            copyStringMap(s.Env),
		}
	}
	return out
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
