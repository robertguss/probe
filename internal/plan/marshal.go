package plan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// wirePlan is the exact JSON schema 1 encoding surface (Section 28.2).
// Field order is the canonical serialization order (REQ-122).
//
// plan_sha256 is the Appendix C / acceptance name for the plan digest
// (Section 28.2 table also calls this plan_digest — JSON uses plan_sha256).
type wirePlan struct {
	Schema            int               `json:"schema"`
	Foundry           FoundryMeta       `json:"foundry"`
	Specification     SpecificationRef  `json:"specification"`
	Project           ProjectInfo       `json:"project"`
	Destination       DestinationInfo   `json:"destination"`
	Profiles          []string          `json:"profiles"`
	Files             []FileEntry       `json:"files"`
	Dependencies      []DependencyEntry `json:"dependencies"`
	Tools             []ToolEntry       `json:"tools"`
	ExternalSteps     []ExternalStep    `json:"external_steps"`
	ToolOutputs       []ToolOutput      `json:"tool_outputs"`
	Verification      Verification      `json:"verification"`
	Network           NetworkDisclosure `json:"network"`
	Git               GitInfo           `json:"git"`
	CommitResultModel string            `json:"commit_result_model"`
	PlanSHA256        string            `json:"plan_sha256"`
	Warnings          []string          `json:"warnings"`
}

// wirePlanNoDigest is identical to wirePlan without plan_sha256 — used to
// compute the digest over the canonical body (REQ-121/122).
type wirePlanNoDigest struct {
	Schema            int               `json:"schema"`
	Foundry           FoundryMeta       `json:"foundry"`
	Specification     SpecificationRef  `json:"specification"`
	Project           ProjectInfo       `json:"project"`
	Destination       DestinationInfo   `json:"destination"`
	Profiles          []string          `json:"profiles"`
	Files             []FileEntry       `json:"files"`
	Dependencies      []DependencyEntry `json:"dependencies"`
	Tools             []ToolEntry       `json:"tools"`
	ExternalSteps     []ExternalStep    `json:"external_steps"`
	ToolOutputs       []ToolOutput      `json:"tool_outputs"`
	Verification      Verification      `json:"verification"`
	Network           NetworkDisclosure `json:"network"`
	Git               GitInfo           `json:"git"`
	CommitResultModel string            `json:"commit_result_model"`
	Warnings          []string          `json:"warnings"`
}

// seal computes plan_sha256 and caches canonical JSON. Called once from Construct.
func (p *Plan) seal() error {
	if p == nil {
		return diagnostic.New(diagnostic.IDInternalBug, "plan: seal nil", diagnostic.Location{})
	}

	// Ensure non-nil slices for stable "[]" encoding (not null).
	if p.profiles == nil {
		p.profiles = []string{}
	}
	if p.files == nil {
		p.files = []FileEntry{}
	}
	if p.dependencies == nil {
		p.dependencies = []DependencyEntry{}
	}
	if p.tools == nil {
		p.tools = []ToolEntry{}
	}
	if p.externalSteps == nil {
		p.externalSteps = []ExternalStep{}
	}
	if p.toolOutputs == nil {
		p.toolOutputs = []ToolOutput{}
	}
	if p.warnings == nil {
		p.warnings = []string{}
	}
	// Ensure mutates is non-nil on every step.
	for i := range p.externalSteps {
		if p.externalSteps[i].Mutates == nil {
			p.externalSteps[i].Mutates = []string{}
		}
		if p.externalSteps[i].Argv == nil {
			p.externalSteps[i].Argv = []string{}
		}
		if p.externalSteps[i].Env == nil {
			p.externalSteps[i].Env = map[string]string{}
		}
	}

	body := p.toWireNoDigest()
	bodyJSON, err := marshalCanonical(body)
	if err != nil {
		return diagnostic.Wrap(
			diagnostic.IDInternalBug,
			"plan: marshal body for digest",
			diagnostic.Location{},
			err,
		)
	}
	sum := sha256.Sum256(bodyJSON)
	p.planSHA256 = hex.EncodeToString(sum[:])

	full := p.toWire()
	fullJSON, err := marshalCanonical(full)
	if err != nil {
		return diagnostic.Wrap(
			diagnostic.IDInternalBug,
			"plan: marshal full plan",
			diagnostic.Location{},
			err,
		)
	}
	// Trailing newline for POSIX text / golden stability.
	if len(fullJSON) == 0 || fullJSON[len(fullJSON)-1] != '\n' {
		fullJSON = append(fullJSON, '\n')
	}
	p.canonicalJSON = fullJSON
	return nil
}

func (p *Plan) toWire() wirePlan {
	return wirePlan{
		Schema:            p.schema,
		Foundry:           p.foundry,
		Specification:     p.specification,
		Project:           p.project,
		Destination:       p.destination,
		Profiles:          p.profiles,
		Files:             p.files,
		Dependencies:      p.dependencies,
		Tools:             p.tools,
		ExternalSteps:     p.externalSteps,
		ToolOutputs:       p.toolOutputs,
		Verification:      p.verification,
		Network:           p.network,
		Git:               p.git,
		CommitResultModel: p.commitResultModel,
		PlanSHA256:        p.planSHA256,
		Warnings:          p.warnings,
	}
}

func (p *Plan) toWireNoDigest() wirePlanNoDigest {
	return wirePlanNoDigest{
		Schema:            p.schema,
		Foundry:           p.foundry,
		Specification:     p.specification,
		Project:           p.project,
		Destination:       p.destination,
		Profiles:          p.profiles,
		Files:             p.files,
		Dependencies:      p.dependencies,
		Tools:             p.tools,
		ExternalSteps:     p.externalSteps,
		ToolOutputs:       p.toolOutputs,
		Verification:      p.verification,
		Network:           p.network,
		Git:               p.git,
		CommitResultModel: p.commitResultModel,
		Warnings:          p.warnings,
	}
}

// marshalCanonical produces stable JSON: encoding/json with no HTML escape,
// compact (no indent), map keys sorted by the encoder.
func marshalCanonical(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encode appends a newline; strip it so seal controls trailing newline
	// for the full plan only. Body digest excludes trailing newline for
	// stability of the hash over pure content.
	b := buf.Bytes()
	b = bytes.TrimSuffix(b, []byte("\n"))
	return b, nil
}

// MarshalJSON implements json.Marshaler using the sealed canonical bytes.
func (p *Plan) MarshalJSON() ([]byte, error) {
	if p == nil {
		return []byte("null"), nil
	}
	if len(p.canonicalJSON) == 0 {
		return nil, fmt.Errorf("plan: not sealed")
	}
	// Strip trailing newline for MarshalJSON (Encoder will re-add if used).
	b := p.canonicalJSON
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

// BannedJSONKeys must never appear in marshaled plan JSON (FND-007/009, Section 58).
var BannedJSONKeys = []string{
	"offline",
	"capability",
	"capabilities",
	"provenance",
	"provenance_closure",
	"helper_binary",
	"helper_binaries",
	"requires_graph",
	"conflicts_graph",
	"profile_closure",
}
