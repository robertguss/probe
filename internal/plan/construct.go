package plan

import (
	"path"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
)

// Construct builds an immutable Generation Plan from pure Inputs (Section 28).
//
// Fail-closed on missing required fields, render failures, and invalid verify
// mode. Does not touch the filesystem, environment, network, or clock.
//
// ResolvedProject input invariants and the resulting Plan contract are enforced
// by TestResolveToPlanContract in contract_test.go.
func Construct(in Inputs) (*Plan, error) {
	if in.Resolved == nil {
		return nil, diagnostic.New(
			diagnostic.IDInternalBug,
			"plan: ResolvedProject is nil",
			diagnostic.Location{},
		)
	}
	if in.Catalog == nil {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"plan: catalog is nil",
			diagnostic.Location{},
		)
	}
	verify := in.Verify.Normalize()
	if !verify.Valid() {
		return nil, diagnostic.Newf(
			diagnostic.IDUsageInvalid,
			diagnostic.Location{},
			"plan: invalid verify mode %q (allowed: default, strict)",
			in.Verify,
		)
	}

	rp := in.Resolved
	if err := validateDestination(in.Destination); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.GoBinary) == "" {
		return nil, diagnostic.New(
			diagnostic.IDToolMissing,
			"plan: go binary path is required",
			diagnostic.PathLocation("go"),
		).WithRemediation("Locate go on PATH at startup and pass the absolute path into plan.Inputs.GoBinary.")
	}
	if rp.GitInit() && strings.TrimSpace(in.GitBinary) == "" {
		return nil, diagnostic.New(
			diagnostic.IDToolMissing,
			"plan: git binary path is required when git.init is true",
			diagnostic.PathLocation("git"),
		).WithRemediation("Locate git on PATH at startup and pass the absolute path into plan.Inputs.GitBinary.")
	}

	specRef, err := normalizeSpecSource(in.SpecSource, in.SpecPath)
	if err != nil {
		return nil, err
	}

	foundry := FoundryMeta{
		Version:       strings.TrimSpace(in.FoundryVersion),
		Commit:        strings.TrimSpace(in.FoundryCommit),
		Go:            strings.TrimSpace(in.GoVersion),
		CatalogDigest: string(rp.CatalogDigest()),
	}
	if foundry.Version == "" {
		foundry.Version = DefaultFoundryVersion
	}
	if foundry.Go == "" {
		// Prefer catalog toolchain tag when caller omitted GoVersion.
		lock := in.Catalog.Lock()
		if lock != nil && lock.Toolchain.GoTag != "" {
			foundry.Go = lock.Toolchain.GoTag
		} else if lock != nil && lock.Toolchain.Go != "" {
			foundry.Go = "go" + lock.Toolchain.Go
		} else {
			foundry.Go = "go" + render.CatalogGoVersion
		}
	}
	if foundry.CatalogDigest == "" {
		foundry.CatalogDigest = string(in.Catalog.Digest())
	}

	project := ProjectInfo{
		Name:        rp.Name(),
		Binary:      rp.Binary(),
		Module:      rp.Module(),
		Description: rp.Description(),
		Archetype:   rp.Archetype(),
		Visibility:  rp.Visibility(),
	}

	profiles := rp.Profiles()
	if profiles == nil {
		profiles = []string{}
	}

	files, deps, tools, err := buildFilesDepsTools(rp, in.Catalog, foundry.Version)
	if err != nil {
		return nil, err
	}

	steps := buildExternalSteps(in, rp, verify)
	toolOutputs := buildToolOutputs()
	verification := buildVerification(verify)
	network := buildNetwork(verify)
	git := GitInfo{
		Init:          rp.GitInit(),
		InitialBranch: rp.GitInitialBranch(),
		Isolated:      true,
	}
	warnings := []string{} // non-nil empty for stable JSON

	p := &Plan{
		schema:            SchemaVersion,
		foundry:           foundry,
		specification:     specRef,
		project:           project,
		destination:       in.Destination,
		profiles:          profiles,
		files:             files,
		dependencies:      deps,
		tools:             tools,
		externalSteps:     steps,
		toolOutputs:       toolOutputs,
		verification:      verification,
		network:           network,
		git:               git,
		commitResultModel: CommitResultModelRef,
		warnings:          warnings,
	}
	if err := p.seal(); err != nil {
		return nil, err
	}
	return p, nil
}

func validateDestination(d DestinationInfo) error {
	if strings.TrimSpace(d.Path) == "" {
		return diagnostic.New(
			diagnostic.IDSpecInvalidField,
			"plan: destination.path is required",
			diagnostic.PathLocation("destination"),
		)
	}
	if strings.TrimSpace(d.Basename) == "" {
		return diagnostic.New(
			diagnostic.IDSpecInvalidField,
			"plan: destination.basename is required",
			diagnostic.PathLocation("destination"),
		)
	}
	if strings.TrimSpace(d.Parent) == "" {
		return diagnostic.New(
			diagnostic.IDSpecInvalidField,
			"plan: destination.parent is required",
			diagnostic.PathLocation("destination"),
		)
	}
	switch d.Observation {
	case ObservationAbsent, ObservationExists, ObservationParentMissing:
		// ok
	default:
		return diagnostic.Newf(
			diagnostic.IDInternalBug,
			diagnostic.PathLocation("destination"),
			"plan: invalid destination observation %q",
			d.Observation,
		)
	}
	return nil
}

func normalizeSpecSource(kind SpecSourceKind, p string) (SpecificationRef, error) {
	switch kind {
	case SpecSourcePath, "":
		if kind == "" {
			kind = SpecSourcePath
		}
		if strings.TrimSpace(p) == "" {
			return SpecificationRef{}, diagnostic.New(
				diagnostic.IDSpecInvalidField,
				"plan: specification path is required when source is path",
				diagnostic.PathLocation("specification"),
			)
		}
		return SpecificationRef{Source: SpecSourcePath, Path: p}, nil
	case SpecSourceStdin:
		return SpecificationRef{Source: SpecSourceStdin}, nil
	default:
		return SpecificationRef{}, diagnostic.Newf(
			diagnostic.IDInternalBug,
			diagnostic.PathLocation("specification"),
			"plan: invalid specification source %q",
			kind,
		)
	}
}

// buildFilesDepsTools renders inventory digests and assembles sorted plan
// collections for files, dependencies, and tools.
func buildFilesDepsTools(rp *resolve.ResolvedProject, cat *catalog.Catalog, foundryVersion string) ([]FileEntry, []DependencyEntry, []ToolEntry, error) {
	contribFiles := rp.Files()
	jobs := make([]render.Job, 0, len(contribFiles)+1)
	data := templateData(rp, foundryVersion)

	for _, f := range contribFiles {
		outPath := expandPathTokens(f.Path, rp.Binary(), rp.Name())
		switch f.Render {
		case catalog.RenderStatic:
			jobs = append(jobs, render.Job{
				Mechanism: render.MechanismStatic,
				Static: &render.StaticJob{
					Path:   outPath,
					Mode:   f.Mode,
					Source: f.Source,
					Owner:  f.Owner,
				},
			})
		case catalog.RenderTemplate:
			jobs = append(jobs, render.Job{
				Mechanism: render.MechanismTemplate,
				Template: &render.TemplateJob{
					Path:   outPath,
					Mode:   f.Mode,
					Source: f.Source,
					Owner:  f.Owner,
					Data:   data,
				},
			})
		default:
			return nil, nil, nil, diagnostic.Newf(
				diagnostic.IDRenderFailed,
				diagnostic.PathLocation(outPath),
				"plan: unknown file render mechanism %q for %s",
				f.Render, outPath,
			)
		}
	}

	// Typed go.mod (always; not a catalog file contribution).
	gomodIn, tools, err := gomodInputAndTools(rp, cat)
	if err != nil {
		return nil, nil, nil, err
	}
	jobs = append(jobs, render.Job{
		Mechanism: render.MechanismGomod,
		Gomod:     &gomodIn,
	})

	inv, err := render.RenderAll(cat, jobs, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	files := make([]FileEntry, 0, inv.Len())
	for _, e := range inv.Entries() {
		files = append(files, FileEntry{
			Path:          e.Path,
			Owner:         e.Owner,
			Mode:          e.Mode,
			Render:        string(e.Mechanism),
			Source:        e.Source,
			SourceSHA256:  string(e.SourceDigest),
			ContentSHA256: string(e.ContentDigest),
		})
	}
	// Inventory is already path-sorted; keep explicit sort for defense.
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	deps := make([]DependencyEntry, 0, rp.DependencyCount())
	for _, d := range rp.Dependencies() {
		deps = append(deps, DependencyEntry{
			Module:  d.Module,
			Version: d.Version,
			Scope:   string(d.Scope),
			Owner:   d.Owner,
		})
	}
	sort.SliceStable(deps, func(i, j int) bool {
		if deps[i].Module != deps[j].Module {
			return deps[i].Module < deps[j].Module
		}
		if deps[i].Owner != deps[j].Owner {
			return deps[i].Owner < deps[j].Owner
		}
		return deps[i].Scope < deps[j].Scope
	})

	sort.SliceStable(tools, func(i, j int) bool {
		if tools[i].Module != tools[j].Module {
			return tools[i].Module < tools[j].Module
		}
		return tools[i].Name < tools[j].Name
	})

	return files, deps, tools, nil
}

func templateData(rp *resolve.ResolvedProject, foundryVersion string) render.TemplateData {
	dist := false
	for _, id := range rp.Profiles() {
		if id == "distribution" {
			dist = true
			break
		}
	}
	fv := strings.TrimSpace(foundryVersion)
	if fv == "" {
		fv = DefaultFoundryVersion
	}
	return render.TemplateData{
		Name:                rp.Name(),
		Binary:              rp.Binary(),
		Module:              rp.Module(),
		Description:         rp.Description(),
		Archetype:           rp.Archetype(),
		Visibility:          rp.Visibility(),
		FoundryVersion:      fv,
		DistributionEnabled: dist,
	}
}

// expandPathTokens substitutes catalog path tokens ({{binary}}, {{name}}).
func expandPathTokens(p, binary, name string) string {
	p = strings.ReplaceAll(p, "{{binary}}", binary)
	p = strings.ReplaceAll(p, "{{name}}", name)
	// Normalize to clean relative form for inventory (no leading ./).
	return path.Clean(p)
}

// gomodInputAndTools builds typed go.mod input from resolve deps + core tools
// from the catalog lock (staticcheck, govulncheck — Section 12 / 35).
func gomodInputAndTools(rp *resolve.ResolvedProject, cat *catalog.Catalog) (render.GomodInput, []ToolEntry, error) {
	lock := cat.Lock()
	if lock == nil {
		return render.GomodInput{}, nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"plan: catalog lock is nil",
			diagnostic.PathLocation("versions.toml"),
		)
	}
	toolchain := lock.Toolchain.GoTag
	if toolchain == "" {
		toolchain = "go" + lock.Toolchain.Go
	}

	var requires []render.GomodRequire
	for _, d := range rp.Dependencies() {
		// tool-scope deps are not ordinary requires; core tools come from lock.
		if d.Scope == catalog.ScopeTool {
			continue
		}
		requires = append(requires, render.GomodRequire{
			Path:    d.Module,
			Version: d.Version,
		})
	}

	coreTools, toolEntries, err := coreToolPins(lock)
	if err != nil {
		return render.GomodInput{}, nil, err
	}

	return render.GomodInput{
		Module:    rp.Module(),
		GoVersion: render.CatalogGoVersion,
		Toolchain: toolchain,
		Requires:  requires,
		Tools:     coreTools,
		Owner:     render.GomodOwnerDefault,
	}, toolEntries, nil
}

// coreToolPins returns the always-on generated-project tool set from the lock.
func coreToolPins(lock *catalog.Lock) ([]render.GomodTool, []ToolEntry, error) {
	type toolSpec struct {
		id      string
		pkgPath string
		owner   string
	}
	// Order is sorted by module path later; declaration order is free.
	wanted := []toolSpec{
		{id: "staticcheck", pkgPath: "honnef.co/go/tools/cmd/staticcheck", owner: "core"},
		{id: "govulncheck", pkgPath: "golang.org/x/vuln/cmd/govulncheck", owner: "core"},
	}
	var gomodTools []render.GomodTool
	var entries []ToolEntry
	for _, w := range wanted {
		t, ok := lockToolByID(lock, w.id)
		if !ok {
			return nil, nil, diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation("versions.toml"),
				"plan: lock missing tool %q",
				w.id,
			)
		}
		if t.Module == "" {
			return nil, nil, diagnostic.Newf(
				diagnostic.IDCatalogInvalid,
				diagnostic.PathLocation("versions.toml"),
				"plan: tool %q missing module path",
				w.id,
			)
		}
		gomodTools = append(gomodTools, render.GomodTool{
			Path:    w.pkgPath,
			Module:  t.Module,
			Version: t.Version,
		})
		entries = append(entries, ToolEntry{
			Module:  t.Module,
			Version: t.Version,
			Owner:   w.owner,
			Name:    t.Name,
		})
	}
	return gomodTools, entries, nil
}

func lockToolByID(lock *catalog.Lock, id string) (catalog.LockTool, bool) {
	for _, t := range lock.Tools {
		if t.ID == id {
			return t, true
		}
	}
	return catalog.LockTool{}, false
}

// FixedGoEnvKeys is the Section 34.2 closed allowlist for go steps (sorted for docs).
// Values come from FixedGoEnvValues + HostEnv captures.
var FixedGoEnvValues = map[string]string{
	"GOPRIVATE":   "",
	"GONOPROXY":   "",
	"GONOSUMDB":   "",
	"GOINSECURE":  "",
	"GOENV":       "off",
	"GOFLAGS":     "",
	"GOCACHEPROG": "",
	"GOTOOLCHAIN": "local",
	"GOWORK":      "off",
	"GOVCS":       "*:off",
	"GOAUTH":      "off",
	"CGO_ENABLED": "0",
	"LC_ALL":      "C",
	"LANG":        "C",
	"TERM":        "dumb",
}

func buildGoEnv(host HostEnv) map[string]string {
	m := make(map[string]string, len(FixedGoEnvValues)+8)
	for k, v := range FixedGoEnvValues {
		m[k] = v
	}
	m["PATH"] = host.PATH
	m["HOME"] = host.HOME
	if host.TMPDIR != "" {
		m["TMPDIR"] = host.TMPDIR
	}
	m["GOMODCACHE"] = host.GOMODCACHE
	m["GOCACHE"] = host.GOCACHE
	m["GOPATH"] = host.GOPATH
	m["GOPROXY"] = host.GOPROXY
	m["GOSUMDB"] = host.GOSUMDB
	return m
}

func buildGitEnv(path, templateDir string) map[string]string {
	return map[string]string{
		"PATH":                path,
		"LC_ALL":              "C",
		"LANG":                "C",
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_SYSTEM":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TEMPLATE_DIR":    templateDir,
	}
}

func buildExternalSteps(in Inputs, rp *resolve.ResolvedProject, verify VerifyMode) []ExternalStep {
	goEnv := buildGoEnv(in.Host)
	goBin := in.GoBinary

	steps := []ExternalStep{
		{
			ID: "go-mod-tidy", Binary: goBin,
			// Argv is `go mod tidy` only: Go 1.16+ rejects `-mod` on the tidy
			// subcommand (Appendix E historical `-mod=mod` is not a valid tidy flag).
			// Tidy always rewrites go.mod/go.sum; -mod=readonly is used on
			// verify/test/vet so those steps cannot mutate the module graph.
			Argv: []string{"go", "mod", "tidy"},
			Cwd:  StageDescriptorCWD, Mutates: []string{"go.mod", "go.sum"},
			Network: NetworkMay, TimeoutS: 600, OutputCapBytes: OutputCapBytes,
			Env: copyStringMap(goEnv),
		},
		{
			ID: "go-mod-verify", Binary: goBin,
			Argv: []string{"go", "mod", "verify"},
			Cwd:  StageDescriptorCWD, Mutates: []string{},
			Network: NetworkNo, TimeoutS: 120, OutputCapBytes: OutputCapBytes,
			Env: copyStringMap(goEnv),
		},
		{
			ID: "go-test", Binary: goBin,
			Argv: []string{"go", "test", "-count=1", "-buildvcs=false", "-mod=readonly", "./..."},
			Cwd:  StageDescriptorCWD, Mutates: []string{},
			Network: NetworkNo, TimeoutS: 300, OutputCapBytes: OutputCapBytes,
			Env: copyStringMap(goEnv),
		},
		{
			ID: "go-vet", Binary: goBin,
			Argv: []string{"go", "vet", "-buildvcs=false", "-mod=readonly", "./..."},
			Cwd:  StageDescriptorCWD, Mutates: []string{},
			Network: NetworkNo, TimeoutS: 300, OutputCapBytes: OutputCapBytes,
			Env: copyStringMap(goEnv),
		},
	}

	if verify == VerifyStrict {
		steps = append(steps,
			ExternalStep{
				ID: "go-staticcheck", Binary: goBin,
				Argv: []string{"go", "tool", "staticcheck", "./..."},
				Cwd:  StageDescriptorCWD, Mutates: []string{},
				Network: NetworkNo, TimeoutS: 300, OutputCapBytes: OutputCapBytes,
				Env: copyStringMap(goEnv),
			},
			ExternalStep{
				ID: "go-govulncheck", Binary: goBin,
				Argv: []string{"go", "tool", "govulncheck", "./..."},
				Cwd:  StageDescriptorCWD, Mutates: []string{},
				Network: NetworkMay, TimeoutS: 600, OutputCapBytes: OutputCapBytes,
				Env: copyStringMap(goEnv),
			},
		)
	}

	if rp.GitInit() {
		branch := rp.GitInitialBranch()
		if branch == "" {
			branch = "main"
		}
		tmpl := in.GitTemplateDir
		if tmpl == "" {
			// Stable plan-space token when CLI has not yet materialised the scratch.
			// toolrun fills the real absolute path at generate time only after
			// re-sealing is not allowed — CLI must inject the path before Construct.
			tmpl = "foundry-owned-git-template-scratch"
		}
		steps = append(steps, ExternalStep{
			ID:     "git-init",
			Binary: in.GitBinary,
			Argv: []string{
				"git", "init",
				"--initial-branch=" + branch,
				"--template=" + tmpl,
				".",
			},
			Cwd:            StageDescriptorCWD,
			Mutates:        []string{},
			Network:        NetworkNo,
			TimeoutS:       60,
			OutputCapBytes: OutputCapBytes,
			Env:            buildGitEnv(in.Host.PATH, tmpl),
		})
	}

	return steps
}

func buildToolOutputs() []ToolOutput {
	// Section 28.2: go.sum; go.mod/go.sum for tidy.
	return []ToolOutput{
		{Path: "go.mod", Steps: []string{"go-mod-tidy"}},
		{Path: "go.sum", Steps: []string{"go-mod-tidy"}},
	}
}

func buildVerification(mode VerifyMode) Verification {
	checks := []string{
		"gofmt",
		"module-mutation",
		"go-mod-verify",
		"go-test",
		"go-vet",
	}
	if mode == VerifyStrict {
		checks = append(checks, "go-staticcheck", "go-govulncheck")
	}
	checks = append(checks, "final-conformance")
	return Verification{Mode: mode, Checks: checks}
}

func buildNetwork(mode VerifyMode) NetworkDisclosure {
	reasons := []string{
		"module resolution during go mod tidy on cold caches",
	}
	if mode == VerifyStrict {
		reasons = append(reasons, "vulnerability database access during go tool govulncheck")
	}
	// Stable sort for determinism (already insertion-ordered; keep explicit).
	sort.Strings(reasons)
	return NetworkDisclosure{
		MayBeRequired: true,
		Reasons:       reasons,
	}
}
