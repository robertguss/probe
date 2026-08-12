package plan

// SchemaVersion is the only admitted plan JSON schema (Section 28.2).
const SchemaVersion = 1

// CommitResultModelRef is the Section 31.9 result-matrix reference recorded
// in every plan (REQ-121).
const CommitResultModelRef = "section-31.9/v1"

// StageDescriptorCWD is the external_steps cwd token: transactional children
// bind cwd to the retained stage descriptor, never a host pathname (Section 34.4).
const StageDescriptorCWD = "stage-descriptor"

// OutputCapBytes is the per-stream capture cap (Section 34.3 / Appendix C).
const OutputCapBytes = 4 << 20 // 4 MiB

// DefaultFoundryVersion is used when Inputs.FoundryVersion is empty.
// CLI embed/version wiring may override.
const DefaultFoundryVersion = "0.1.0"

// VerifyMode selects the verification step list (REQ-033 / REQ-152).
type VerifyMode string

const (
	// VerifyDefault is gofmt + module-mutation + verify + test + vet + final-conformance.
	VerifyDefault VerifyMode = "default"
	// VerifyStrict adds staticcheck and govulncheck (Section 35.2).
	VerifyStrict VerifyMode = "strict"
)

// Valid reports whether m is default or strict.
func (m VerifyMode) Valid() bool {
	switch m {
	case VerifyDefault, VerifyStrict, "":
		return true
	default:
		return false
	}
}

// Normalize returns VerifyDefault when m is empty.
func (m VerifyMode) Normalize() VerifyMode {
	if m == "" {
		return VerifyDefault
	}
	return m
}

// SpecSourceKind is how the Project Specification was supplied.
type SpecSourceKind string

const (
	// SpecSourcePath means the specification was read from a filesystem path.
	SpecSourcePath SpecSourceKind = "path"
	// SpecSourceStdin means the specification was read from stdin ("-").
	SpecSourceStdin SpecSourceKind = "stdin"
)

// DestinationObservation is the non-binding read-only observation at plan time
// (Section 28.2). Plan identity only; never generated content. generate
// re-establishes destination state inside the transaction.
type DestinationObservation string

const (
	// ObservationAbsent — destination path does not exist.
	ObservationAbsent DestinationObservation = "absent"
	// ObservationExists — destination path exists in any form.
	ObservationExists DestinationObservation = "exists"
	// ObservationParentMissing — destination parent is absent or not a directory.
	ObservationParentMissing DestinationObservation = "parent-missing"
)

// NetworkMode is an external step's declared network disclosure.
type NetworkMode string

const (
	NetworkNo  NetworkMode = "no"
	NetworkMay NetworkMode = "may"
)

// FoundryMeta is plan.foundry (Section 28.2).
type FoundryMeta struct {
	Version       string `json:"version"`
	Commit        string `json:"commit,omitempty"`
	Go            string `json:"go"`
	CatalogDigest string `json:"catalog_digest"`
}

// SpecificationRef is plan.specification — source of the input specification.
type SpecificationRef struct {
	// Source is "path" or "stdin".
	Source SpecSourceKind `json:"source"`
	// Path is set when Source == path (may be relative as authored; not host-secret).
	Path string `json:"path,omitempty"`
}

// ProjectInfo is plan.project.
type ProjectInfo struct {
	Name        string `json:"name"`
	Binary      string `json:"binary"`
	Module      string `json:"module"`
	Description string `json:"description"`
	Archetype   string `json:"archetype"`
	Visibility  string `json:"visibility"`
}

// DestinationInfo is plan.destination (absolute path + observation).
type DestinationInfo struct {
	Path        string                 `json:"path"`
	Parent      string                 `json:"parent"`
	Basename    string                 `json:"basename"`
	Observation DestinationObservation `json:"observation"`
}

// FileEntry is one planned output file (Section 28.2 files[]).
//
// SourceSHA256 is the catalog source digest when the mechanism has catalog
// bytes (static/template). Empty for typed gomod (source="typed").
// ContentSHA256 is always the planned rendered-content digest.
type FileEntry struct {
	Path          string `json:"path"`
	Owner         string `json:"owner"`
	Mode          string `json:"mode"`
	Render        string `json:"render"` // static | template | gomod
	Source        string `json:"source"`
	SourceSHA256  string `json:"source_sha256,omitempty"`
	ContentSHA256 string `json:"content_sha256"`
}

// DependencyEntry is one aggregated go.mod module pin.
type DependencyEntry struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Scope   string `json:"scope"`
	Owner   string `json:"owner"`
}

// ToolEntry is one declared tool dependency with exact version and owner.
type ToolEntry struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Owner   string `json:"owner"`
	// Name is the human tool id (staticcheck, govulncheck) when known.
	Name string `json:"name,omitempty"`
}

// ExternalStep is one planned subprocess step (Section 28.2 / Appendix E).
//
// Cwd is always StageDescriptorCWD for transactional steps — never a host path.
type ExternalStep struct {
	ID             string            `json:"id"`
	Binary         string            `json:"binary"`
	Argv           []string          `json:"argv"`
	Cwd            string            `json:"cwd"`
	Mutates        []string          `json:"mutates"`
	Network        NetworkMode       `json:"network"`
	TimeoutS       int               `json:"timeout_s"`
	OutputCapBytes int               `json:"output_cap_bytes"`
	Env            map[string]string `json:"env"`
}

// ToolOutput declares a file external steps may create or modify.
type ToolOutput struct {
	Path  string   `json:"path"`
	Steps []string `json:"steps"`
}

// Verification is plan.verification (mode + ordered checks).
type Verification struct {
	Mode   VerifyMode `json:"mode"`
	Checks []string   `json:"checks"`
}

// NetworkDisclosure is plan.network (FND-007 truthful disclosure).
type NetworkDisclosure struct {
	MayBeRequired bool     `json:"may_be_required"`
	Reasons       []string `json:"reasons"`
}

// GitInfo is plan.git.
type GitInfo struct {
	Init          bool   `json:"init"`
	InitialBranch string `json:"initial_branch"`
	Isolated      bool   `json:"isolated"`
}
