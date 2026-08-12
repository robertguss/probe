package plan

import (
	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/spec"
)

// Inputs is the pure input bag for Construct (Section 28 / REQ-120).
//
// All host-dependent observations (destination, tool binaries, host env
// captures) are supplied by the caller so Construct itself stays free of
// FS/env/subprocess side effects (Phase 1 purity / REQ-183).
type Inputs struct {
	// Resolved is the collision-free resolve result (required).
	Resolved *resolve.ResolvedProject
	// Catalog is the loaded catalog used for render digests (required).
	Catalog *catalog.Catalog

	// FoundryVersion is reported in plan.foundry.version (default DefaultFoundryVersion).
	FoundryVersion string
	// FoundryCommit is optional embedded VCS commit (omit when empty).
	FoundryCommit string
	// GoVersion is the Foundry's Go version string (e.g. "go1.26.5").
	GoVersion string

	// SpecSource is "path" or "stdin".
	SpecSource SpecSourceKind
	// SpecPath is the authored path when SpecSource == path (basename-safe for logs).
	SpecPath string

	// Destination is the absolute normalized destination path and observation.
	Destination DestinationInfo

	// Verify selects default or strict external step / check lists.
	Verify VerifyMode

	// GoBinary is the absolute path of the go tool (recorded in external_steps).
	GoBinary string
	// GitBinary is the absolute path of git (required when Resolved.GitInit()).
	GitBinary string
	// GitTemplateDir is the Foundry-owned empty scratch template path for git init env.
	GitTemplateDir string

	// Host is the Section 34.2 host-captured allowlist values for go steps.
	Host HostEnv
}

// HostEnv holds host-effective values reused deliberately in go-step env
// construction (Section 34.2). Injected for purity; CLI captures once at startup.
type HostEnv struct {
	PATH       string
	HOME       string
	TMPDIR     string // may be empty — key omitted when empty
	GOMODCACHE string
	GOCACHE    string
	GOPATH     string
	GOPROXY    string
	GOSUMDB    string
}

// PipelineOptions configures the shared validate/plan/generate pure pipeline.
type PipelineOptions struct {
	// Foundry identity
	FoundryVersion string
	FoundryCommit  string
	GoVersion      string

	// Spec source
	SpecSource SpecSourceKind
	SpecPath   string

	// Destination (pre-observed)
	Destination DestinationInfo

	// Verify mode
	Verify VerifyMode

	// Tool binaries + host env
	GoBinary       string
	GitBinary      string
	GitTemplateDir string
	Host           HostEnv
}

// Pipeline is the sole pure planning entrypoint shared by validate, plan, and
// generate (REQ-032/033/120). It resolves the specification against the catalog
// then Constructs the Generation Plan.
//
// validate discards the returned plan; plan prints it; generate executes it.
// Identical Inputs → byte-identical plan JSON (REQ-122).
func Pipeline(vs *spec.ValidatedSpecification, cat *catalog.Catalog, opts PipelineOptions) (*Plan, error) {
	rp, err := resolve.Resolve(vs, cat)
	if err != nil {
		return nil, err
	}
	return Construct(Inputs{
		Resolved:       rp,
		Catalog:        cat,
		FoundryVersion: opts.FoundryVersion,
		FoundryCommit:  opts.FoundryCommit,
		GoVersion:      opts.GoVersion,
		SpecSource:     opts.SpecSource,
		SpecPath:       opts.SpecPath,
		Destination:    opts.Destination,
		Verify:         opts.Verify,
		GoBinary:       opts.GoBinary,
		GitBinary:      opts.GitBinary,
		GitTemplateDir: opts.GitTemplateDir,
		Host:           opts.Host,
	})
}
