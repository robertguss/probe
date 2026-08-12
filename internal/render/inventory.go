package render

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// Entry is one immutable render-inventory record (Section 43 RenderInventory).
//
// Fields:
//   - Path: destination-relative output path
//   - Mode: planned file mode string (v1.0 files "0644"; directories "0755")
//   - Mechanism: static | template | gomod
//   - Source: catalog source path / source id (or "typed" for gomod)
//   - Owner: stable owner label when known (core, archetype:<id>, profile:<id>)
//   - SourceDigest: SHA-256 of catalog source bytes (static: equals ContentDigest)
//   - ContentDigest: SHA-256 of rendered content bytes
type Entry struct {
	Path          string
	Mode          string
	Mechanism     Mechanism
	Source        string
	Owner         string
	SourceDigest  DigestHex
	ContentDigest DigestHex
}

// Inventory is the immutable per-file render inventory (path, mode, mechanism,
// digests) plus pure content buffers for Phase 1 plan digests.
//
// Construct only via package constructors (Render / RenderAll / Merge,
// RenderStatic / RenderStaticAll, RenderTemplate / RenderTemplateAll,
// RenderGomod). Safe for concurrent read after construction.
type Inventory struct {
	entries []Entry
	// content maps cleaned path → defensive copy of rendered bytes.
	content map[string][]byte
}

// Entries returns a copy of inventory entries sorted by path.
func (inv *Inventory) Entries() []Entry {
	if inv == nil {
		return nil
	}
	out := make([]Entry, len(inv.entries))
	copy(out, inv.entries)
	return out
}

// Len returns the number of inventory entries.
func (inv *Inventory) Len() int {
	if inv == nil {
		return 0
	}
	return len(inv.entries)
}

// Content returns a defensive copy of rendered bytes for path.
func (inv *Inventory) Content(path string) ([]byte, bool) {
	if inv == nil || inv.content == nil {
		return nil, false
	}
	b, ok := inv.content[path]
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp, true
}

// EntryByPath returns the inventory entry for path, if present.
func (inv *Inventory) EntryByPath(path string) (Entry, bool) {
	if inv == nil {
		return Entry{}, false
	}
	for _, e := range inv.entries {
		if e.Path == path {
			return e, true
		}
	}
	return Entry{}, false
}

// Paths returns sorted output paths.
func (inv *Inventory) Paths() []string {
	if inv == nil {
		return nil
	}
	out := make([]string, len(inv.entries))
	for i, e := range inv.entries {
		out[i] = e.Path
	}
	return out
}

// Equal reports whether two inventories have identical entries and content
// (including ordered digests). Used by determinism tests.
func (inv *Inventory) Equal(o *Inventory) bool {
	if inv == nil || o == nil {
		return inv == o
	}
	if len(inv.entries) != len(o.entries) {
		return false
	}
	for i := range inv.entries {
		if inv.entries[i] != o.entries[i] {
			return false
		}
		a, aok := inv.content[inv.entries[i].Path]
		b, bok := o.content[o.entries[i].Path]
		if aok != bok {
			return false
		}
		if !bytes.Equal(a, b) {
			return false
		}
	}
	return true
}

// String returns a short debug summary (not a serialization format).
func (inv *Inventory) String() string {
	if inv == nil {
		return "Inventory(nil)"
	}
	parts := make([]string, 0, len(inv.entries))
	for _, e := range inv.entries {
		parts = append(parts, fmt.Sprintf("%s=%s", e.Path, e.ContentDigest))
	}
	return fmt.Sprintf("Inventory{n=%d %s}", len(inv.entries), strings.Join(parts, ","))
}

// newInventory builds a sorted inventory from entries and content maps.
// content keys must match entry paths; values are taken without re-copy
// (callers must already defensive-copy).
func newInventory(entries []Entry, content map[string][]byte) *Inventory {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	if content == nil {
		content = make(map[string][]byte, len(entries))
	}
	return &Inventory{entries: entries, content: content}
}
