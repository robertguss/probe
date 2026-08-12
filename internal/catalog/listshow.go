package catalog

import (
	"fmt"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// UnitSummary is stable list-row metadata for `foundry catalog list`
// (REQ-035). Sorted by ID when returned from List.
//
// JSON field names are the contract surface for --output json goldens
// (internal/cli catalog list); keep stable across releases.
type UnitSummary struct {
	ID           string `json:"id"`
	Kind         Kind   `json:"kind"`
	Description  string `json:"description"`
	ManifestPath string `json:"manifest_path"`
}

// List returns every validated unit in the catalog as UnitSummary rows,
// sorted by ID (deterministic; -count=2 safe). Includes core, archetypes,
// and any catalogued profiles (e.g. distribution post-MVP content).
//
// Pure: no I/O. Safe for concurrent use after Load.
func (c *Catalog) List() []UnitSummary {
	if c == nil {
		return nil
	}
	out := make([]UnitSummary, 0, len(c.manifests))
	for _, m := range c.manifests {
		if m == nil {
			continue
		}
		out = append(out, UnitSummary{
			ID:           m.ID,
			Kind:         m.Kind,
			Description:  m.Description,
			ManifestPath: m.Path,
		})
	}
	return out
}

// ListByKind returns List rows filtered to kind, still sorted by ID.
func (c *Catalog) ListByKind(kind Kind) []UnitSummary {
	all := c.List()
	if all == nil {
		return nil
	}
	out := make([]UnitSummary, 0, len(all))
	for _, u := range all {
		if u.Kind == kind {
			out = append(out, u)
		}
	}
	return out
}

// ArchetypeIDs returns sorted archetype unit IDs present in the catalog.
func (c *Catalog) ArchetypeIDs() []string {
	if c == nil {
		return nil
	}
	var out []string
	for _, m := range c.manifests {
		if m != nil && m.Kind == KindArchetype {
			out = append(out, m.ID)
		}
	}
	return out
}

// UnitIDs returns every unit ID sorted (core + archetypes + profiles).
func (c *Catalog) UnitIDs() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.manifests))
	for _, m := range c.manifests {
		if m != nil {
			out = append(out, m.ID)
		}
	}
	return out
}

// Show returns the validated Manifest for id (catalog list/show data model).
// Unknown IDs fail closed with catalog.invalid, naming the rejected id and
// the sorted available unit set so agents can self-correct without guessing
// (REQ-035 / REQ-101 — exact IDs only, no did-you-mean).
func (c *Catalog) Show(id string) (*Manifest, error) {
	if c == nil {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"catalog is nil",
			diagnostic.PathLocation(id),
		)
	}
	if id == "" {
		return nil, unknownUnitError(id, c.UnitIDs())
	}
	m, ok := c.byID[id]
	if !ok || m == nil {
		return nil, unknownUnitError(id, c.UnitIDs())
	}
	return m, nil
}

// unknownUnitError builds catalog.invalid for a missing catalog unit id.
func unknownUnitError(requested string, available []string) *diagnostic.FoundryError {
	avail := append([]string(nil), available...)
	sort.Strings(avail)
	availText := "[]"
	if len(avail) > 0 {
		availText = "[" + strings.Join(avail, ", ") + "]"
	}
	msg := fmt.Sprintf(
		"catalog unit %q not found; available units: %s",
		requested, availText,
	)
	fe := diagnostic.New(diagnostic.IDCatalogInvalid, msg, diagnostic.PathLocation(requested))
	return fe.WithRemediation(fmt.Sprintf(
		"Use an exact unit id from the available set %s. "+
			"Run `foundry catalog list` to inspect embedded catalog units. "+
			"Foundry does not suggest nearby spellings (exact IDs only).",
		availText,
	))
}
