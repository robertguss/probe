package render_test

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/render"
)

// mapCatalog is an in-memory CatalogReader for unit tests.
type mapCatalog map[string][]byte

func (m mapCatalog) Read(path string) ([]byte, error) {
	b, ok := m[path]
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

func mustLoadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return c
}

func fixtureCatalog() mapCatalog {
	return mapCatalog{
		"core/files/LICENSE":                       []byte("MIT License\nCopyright 2026\n"),
		"core/files/AGENTS.md":                     []byte("# Agents\n\nFollow the project AGENTS.md.\n"),
		"core/files/scripts/run.sh":                []byte("#!/bin/sh\necho ok\n"),
		"profiles/distribution/files/releasing.md": []byte("# Releasing\n\nPublic release scaffolding.\n"),
	}
}

func asFoundry(t *testing.T, err error) *diagnostic.FoundryError {
	t.Helper()
	fe, ok := diagnostic.AsFoundryError(err)
	if !ok {
		t.Fatalf("expected FoundryError, got %T: %v", err, err)
	}
	return fe
}

// digestOf is a test helper matching render.ContentDigest.
func digestOf(b []byte) render.DigestHex {
	return render.ContentDigest(b)
}
