package generate

import (
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/render"
)

// JobsFromPlan rebuilds render jobs from an immutable plan + catalog so
// generate renders the same files plan digests described (REQ-096 / Section 26).
// Pure construction — no filesystem I/O.
func JobsFromPlan(p *plan.Plan, cat *catalog.Catalog) ([]render.Job, error) {
	if p == nil {
		return nil, diagnostic.New(
			diagnostic.IDInternalBug,
			"JobsFromPlan: plan is nil",
			diagnostic.Location{},
		)
	}
	if cat == nil {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"JobsFromPlan: catalog is nil",
			diagnostic.PathLocation("catalog"),
		)
	}

	proj := p.Project()
	data := render.TemplateData{
		Name:                proj.Name,
		Binary:              proj.Binary,
		Module:              proj.Module,
		Description:         proj.Description,
		Archetype:           proj.Archetype,
		Visibility:          proj.Visibility,
		FoundryVersion:      p.Foundry().Version,
		DistributionEnabled: hasProfile(p.Profiles(), "distribution"),
	}
	if data.FoundryVersion == "" {
		data.FoundryVersion = plan.DefaultFoundryVersion
	}

	files := p.Files()
	jobs := make([]render.Job, 0, len(files))
	var gomodSeen bool
	for _, f := range files {
		switch f.Render {
		case string(render.MechanismStatic):
			jobs = append(jobs, render.Job{
				Mechanism: render.MechanismStatic,
				Static: &render.StaticJob{
					Path:   f.Path,
					Mode:   f.Mode,
					Source: f.Source,
					Owner:  f.Owner,
				},
			})
		case string(render.MechanismTemplate):
			jobs = append(jobs, render.Job{
				Mechanism: render.MechanismTemplate,
				Template: &render.TemplateJob{
					Path:   f.Path,
					Mode:   f.Mode,
					Source: f.Source,
					Owner:  f.Owner,
					Data:   data,
				},
			})
		case string(render.MechanismGomod):
			if gomodSeen {
				continue
			}
			gomodSeen = true
			in, err := gomodFromPlan(p, cat)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, render.Job{
				Mechanism: render.MechanismGomod,
				Gomod:     &in,
			})
		default:
			return nil, diagnostic.Newf(
				diagnostic.IDRenderFailed,
				diagnostic.PathLocation(f.Path),
				"unknown plan file render mechanism %q",
				f.Render,
			)
		}
	}
	if !gomodSeen {
		// Plans always include go.mod; rebuild if missing from files list.
		in, err := gomodFromPlan(p, cat)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, render.Job{
			Mechanism: render.MechanismGomod,
			Gomod:     &in,
		})
	}
	return jobs, nil
}

func gomodFromPlan(p *plan.Plan, cat *catalog.Catalog) (render.GomodInput, error) {
	lock := cat.Lock()
	goVer := render.CatalogGoVersion
	toolchain := toolrunDefaultToolchain
	if lock != nil {
		if lock.Toolchain.Go != "" {
			goVer = lock.Toolchain.Go
		}
		if lock.Toolchain.GoTag != "" {
			toolchain = lock.Toolchain.GoTag
		} else if lock.Toolchain.Go != "" {
			toolchain = "go" + lock.Toolchain.Go
		}
	}
	// Prefer plan foundry.go when present (go1.26.5 style → keep as toolchain).
	if fg := strings.TrimSpace(p.Foundry().Go); fg != "" {
		toolchain = fg
	}

	requires := make([]render.GomodRequire, 0, len(p.Dependencies()))
	for _, d := range p.Dependencies() {
		requires = append(requires, render.GomodRequire{
			Path:    d.Module,
			Version: d.Version,
		})
	}
	tools := make([]render.GomodTool, 0, len(p.Tools()))
	for _, t := range p.Tools() {
		path := t.Module
		if t.Name == "staticcheck" {
			path = "honnef.co/go/tools/cmd/staticcheck"
		} else if t.Name == "govulncheck" {
			path = "golang.org/x/vuln/cmd/govulncheck"
		}
		tools = append(tools, render.GomodTool{
			Path:    path,
			Module:  t.Module,
			Version: t.Version,
		})
	}
	return render.GomodInput{
		Module:    p.Project().Module,
		GoVersion: goVer,
		Toolchain: toolchain,
		Requires:  requires,
		Tools:     tools,
		Owner:     render.GomodOwnerDefault,
	}, nil
}

// toolrunDefaultToolchain matches catalog pin when lock fields are sparse.
const toolrunDefaultToolchain = "go1.26.5"

func hasProfile(profiles []string, id string) bool {
	for _, p := range profiles {
		if p == id {
			return true
		}
	}
	return false
}
