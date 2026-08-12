//go:build foundrydev

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// LoadDir loads and fully validates a catalog tree from a filesystem directory.
//
// Available ONLY under the foundrydev build tag (REQ-091 / Section 24.4).
// Release binaries compiled without -tags=foundrydev do not contain this
// entrypoint — development filesystem loading is not a production extension
// surface.
//
// Behavior:
//   - Empty path, missing path, or non-directory → catalog.invalid (fail closed)
//   - Reads only the production surface (EmbeddedTopLevel): versions.toml,
//     core/, archetypes/, profiles/, schemas/ — matching go:embed roots
//   - Skips package .go sources and testdata/ (not part of the production
//     catalog; Section 24.2)
//   - Same validation path as LoadFS / Load (layout, lock, flat manifests)
//   - No network, no writes, no subprocesses
func LoadDir(dir string) (*Catalog, error) {
	if dir == "" {
		return nil, diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"catalog directory path is empty",
			diagnostic.PathLocation("catalog"),
		).WithRemediation(
			"Pass a non-empty path to a Git-visible catalog/ tree " +
				"(foundrydev builds only; release binaries use the embedded catalog).",
		)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, diagnostic.Wrap(
			diagnostic.IDCatalogInvalid,
			fmt.Sprintf("cannot resolve catalog directory %q", dir),
			diagnostic.PathLocation(dir),
			err,
		)
	}

	st, err := os.Stat(abs)
	if err != nil {
		return nil, diagnostic.Wrap(
			diagnostic.IDCatalogInvalid,
			fmt.Sprintf("catalog directory not found or unreadable: %s", abs),
			diagnostic.PathLocation(abs),
			err,
		).WithRemediation(
			"Ensure the path exists, is a directory, and is readable. " +
				"Dev filesystem loading is fail-closed (REQ-091).",
		)
	}
	if !st.IsDir() {
		return nil, diagnostic.Newf(
			diagnostic.IDCatalogInvalid,
			diagnostic.PathLocation(abs),
			"catalog path is not a directory: %s",
			abs,
		).WithRemediation(
			"Point foundrydev LoadDir at a catalog/ directory tree, not a file.",
		)
	}

	files, err := readProductionDir(abs)
	if err != nil {
		return nil, diagnostic.Wrap(
			diagnostic.IDCatalogInvalid,
			fmt.Sprintf("failed to read catalog directory %s", abs),
			diagnostic.PathLocation(abs),
			err,
		)
	}
	return LoadFS(toMemFS(files))
}

// readProductionDir walks only EmbeddedTopLevel under root, skipping .go
// sources so the in-memory map matches the go:embed production surface.
func readProductionDir(root string) (map[string][]byte, error) {
	out := make(map[string][]byte)
	for _, top := range EmbeddedTopLevel {
		p := filepath.Join(root, top)
		st, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				// Missing top-level is reported later by ValidateLayout/LoadFS.
				continue
			}
			return nil, err
		}
		if !st.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			out[filepath.ToSlash(top)] = b
			continue
		}
		err = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".go") {
				return nil
			}
			info, infoErr := d.Info()
			if infoErr == nil && !info.Mode().IsRegular() {
				return fmt.Errorf("catalog: non-regular file %s", path)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = b
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// toMemFS adapts a path→bytes map to fs.FS for LoadFS.
func toMemFS(files map[string][]byte) fstest.MapFS {
	m := make(fstest.MapFS, len(files))
	for p, b := range files {
		// Defensive copy so later mutation of the map cannot affect the FS.
		cp := make([]byte, len(b))
		copy(cp, b)
		m[p] = &fstest.MapFile{Data: cp}
	}
	return m
}
