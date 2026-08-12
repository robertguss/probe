package catalog

import (
	"fmt"
	"io/fs"
	"sort"
	"sync"

	rootcatalog "github.com/robertguss/go-foundry-cli/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Catalog is an immutable loaded catalog: file map, digest, lock, roots, and
// validated flat unit manifests. Safe for concurrent read after Load returns.
type Catalog struct {
	files     map[string][]byte
	paths     []string // sorted
	roots     []string // sorted top-level names
	digest    DigestHex
	lock      *Lock
	manifests []*Manifest // sorted by ID
	byID      map[string]*Manifest
}

// Load loads and validates the embedded production catalog.
func Load() (*Catalog, error) {
	return LoadFS(rootcatalog.FS)
}

// LoadFS loads and validates a catalog from any fs.FS (tests, foundrydev).
//
// Steps: read all files → layout validation → parse versions.toml →
// flat unit-manifest schema validation (REQ-092) → digest.
//
// Dependency module+version pins must match the lock. Full lock-entry
// exactly-once consumption is not required at load (foundry-only pins are
// consumed outside unit manifests); use ValidateConsumption with
// LockConsumptionFromManifests for partial or full checks.
func LoadFS(fsys fs.FS) (*Catalog, error) {
	files, err := readAllFiles(fsys)
	if err != nil {
		return nil, diagnostic.Wrap(
			diagnostic.IDCatalogInvalid,
			"failed to read catalog",
			diagnostic.PathLocation("catalog"),
			err,
		)
	}
	if err := validateLayoutFiles(files); err != nil {
		return nil, err
	}

	lockBytes, ok := files["versions.toml"]
	if !ok {
		// validateLayoutFiles already requires this; defensive.
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"versions.toml missing",
			diagnostic.PathLocation("versions.toml"),
		)
	}
	lock, err := ParseLock(lockBytes)
	if err != nil {
		return nil, err
	}

	manifests, err := ValidateUnitManifests(files, lock)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*Manifest, len(manifests))
	for _, m := range manifests {
		byID[m.ID] = m
	}

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// Defensive copy of contents so callers cannot mutate the catalog.
	copied := make(map[string][]byte, len(files))
	for p, b := range files {
		cp := make([]byte, len(b))
		copy(cp, b)
		copied[p] = cp
	}

	return &Catalog{
		files:     copied,
		paths:     paths,
		roots:     listRoots(copied),
		digest:    DigestMap(copied),
		lock:      lock,
		manifests: manifests,
		byID:      byID,
	}, nil
}

// Digest returns the deterministic SHA-256 hex digest (REQ-090).
func (c *Catalog) Digest() DigestHex {
	if c == nil {
		return ""
	}
	return c.digest
}

// FileCount returns the number of embedded regular files.
func (c *Catalog) FileCount() int {
	if c == nil {
		return 0
	}
	return len(c.paths)
}

// Paths returns a copy of sorted relative file paths.
func (c *Catalog) Paths() []string {
	if c == nil {
		return nil
	}
	out := make([]string, len(c.paths))
	copy(out, c.paths)
	return out
}

// Roots returns sorted top-level catalog names (files and directories).
func (c *Catalog) Roots() []string {
	if c == nil {
		return nil
	}
	out := make([]string, len(c.roots))
	copy(out, c.roots)
	return out
}

// Lock returns the parsed versions.toml lock (never nil after successful Load).
func (c *Catalog) Lock() *Lock {
	if c == nil {
		return nil
	}
	return c.lock
}

// Manifests returns validated unit manifests sorted by ID (immutable copies
// of the slice header; Manifest values must not be mutated).
func (c *Catalog) Manifests() []*Manifest {
	if c == nil {
		return nil
	}
	out := make([]*Manifest, len(c.manifests))
	copy(out, c.manifests)
	return out
}

// Manifest returns the validated unit manifest for id, if present.
func (c *Catalog) Manifest(id string) (*Manifest, bool) {
	if c == nil || c.byID == nil {
		return nil, false
	}
	m, ok := c.byID[id]
	return m, ok
}

// Read returns a copy of the file bytes at path, or catalog.invalid if missing.
func (c *Catalog) Read(path string) ([]byte, error) {
	if c == nil {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"catalog is nil",
			diagnostic.PathLocation(path),
		)
	}
	b, ok := c.files[path]
	if !ok {
		return nil, diagnostic.Newf(
			diagnostic.IDCatalogInvalid,
			diagnostic.PathLocation(path),
			"catalog file not found: %s",
			path,
		)
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

// Files returns a deep copy of the path→content map (for mutation tests).
func (c *Catalog) Files() map[string][]byte {
	if c == nil {
		return nil
	}
	out := make(map[string][]byte, len(c.files))
	for p, b := range c.files {
		cp := make([]byte, len(b))
		copy(cp, b)
		out[p] = cp
	}
	return out
}

// String summarizes file count and digest for logging.
func (c *Catalog) String() string {
	if c == nil {
		return "catalog<nil>"
	}
	return fmt.Sprintf("catalog{files=%d digest=%s}", c.FileCount(), c.digest)
}

// defaultCatalog caches the embedded Load result for process-wide reuse.
// Built once; concurrent readers only (REQ-188 — no reassignment after success).
var (
	defaultOnce    sync.Once
	defaultCatalog *Catalog
	defaultErr     error
)

// Default returns the process-wide embedded catalog, loading once.
func Default() (*Catalog, error) {
	defaultOnce.Do(func() {
		defaultCatalog, defaultErr = Load()
	})
	return defaultCatalog, defaultErr
}
