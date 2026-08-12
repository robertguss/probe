package resolve

import (
	"path"
	"sort"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
)

// collectContributions gathers file and dependency contributions from core,
// the selected archetype, and selected profile manifests. Output is unsorted;
// callers sort and run collision detection (Section 27 steps 4–6).
func collectContributions(units []*catalog.Manifest) (files []FileContribution, deps []DependencyContribution) {
	for _, m := range units {
		if m == nil {
			continue
		}
		owner := OwnerLabel(m.Kind, m.ID)
		for _, f := range m.Files {
			src := f.Source
			if m.UnitDir != "" {
				src = path.Join(m.UnitDir, f.Source)
			}
			files = append(files, FileContribution{
				Path:      f.Path,
				Owner:     owner,
				OwnerKind: m.Kind,
				OwnerID:   m.ID,
				Render:    f.Render,
				Source:    src,
				Mode:      f.Mode,
			})
		}
		for _, d := range m.Dependencies {
			deps = append(deps, DependencyContribution{
				Module:  d.Module,
				Version: d.Version,
				Scope:   d.Scope,
				Owner:   owner,
			})
		}
	}
	return files, deps
}

// sortFiles sorts file contributions by Path, then Owner (stable, REQ-100).
func sortFiles(files []FileContribution) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Path != files[j].Path {
			return files[i].Path < files[j].Path
		}
		return files[i].Owner < files[j].Owner
	})
}

// sortDeps sorts dependency contributions by Module, then Owner, then Scope.
func sortDeps(deps []DependencyContribution) {
	sort.SliceStable(deps, func(i, j int) bool {
		if deps[i].Module != deps[j].Module {
			return deps[i].Module < deps[j].Module
		}
		if deps[i].Owner != deps[j].Owner {
			return deps[i].Owner < deps[j].Owner
		}
		return deps[i].Scope < deps[j].Scope
	})
}

// unitOrder builds the contribution unit list: core, archetype, then
// selected profiles in sorted ID order (selection order is irrelevant).
func unitOrder(core, archetype *catalog.Manifest, profiles []*catalog.Manifest) []*catalog.Manifest {
	n := 0
	if core != nil {
		n++
	}
	if archetype != nil {
		n++
	}
	n += len(profiles)
	out := make([]*catalog.Manifest, 0, n)
	if core != nil {
		out = append(out, core)
	}
	if archetype != nil {
		out = append(out, archetype)
	}
	// profiles already sorted by ID at selection time.
	out = append(out, profiles...)
	return out
}
