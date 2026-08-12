package catalog

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// ValidateLayout checks that fsys satisfies Section 24.2 required roots/files
// and rejects forbidden recipe-profile trees and dual lock files.
//
// Returns *diagnostic.FoundryError with id catalog.invalid on failure.
func ValidateLayout(fsys fs.FS) error {
	files, err := readAllFiles(fsys)
	if err != nil {
		return diagnostic.Wrap(diagnostic.IDCatalogInvalid,
			"failed to read catalog filesystem",
			diagnostic.PathLocation("catalog"),
			err,
		)
	}
	return validateLayoutFiles(files)
}

func validateLayoutFiles(files map[string][]byte) error {
	// Forbidden paths first (clearer agent remediation).
	var forbiddenHits []string
	for p := range files {
		if isForbiddenPath(p) {
			forbiddenHits = append(forbiddenHits, p)
		}
	}
	if len(forbiddenHits) > 0 {
		sort.Strings(forbiddenHits)
		fe := diagnostic.Newf(
			diagnostic.IDCatalogInvalid,
			diagnostic.PathLocation(forbiddenHits[0]),
			"catalog contains forbidden path(s): %s (configuration and local-persistence are recipe-only; dual lock files are rejected)",
			strings.Join(forbiddenHits, ", "),
		)
		return fe.WithRemediation(recipeOnlyLayoutRemediation(forbiddenHits))
	}

	var missing []string
	for _, req := range RequiredFiles {
		if _, ok := files[req]; !ok {
			missing = append(missing, req)
		}
	}
	if len(missing) > 0 {
		return diagnostic.Newf(
			diagnostic.IDCatalogInvalid,
			diagnostic.PathLocation(missing[0]),
			"catalog missing required file(s): %s",
			strings.Join(missing, ", "),
		)
	}

	// Required roots must contribute at least one file (empty dirs are not embedded).
	rootHit := map[string]bool{}
	for p := range files {
		for _, root := range RequiredRoots {
			if p == root || strings.HasPrefix(p, root+"/") {
				rootHit[root] = true
			}
		}
	}
	var missingRoots []string
	for _, root := range RequiredRoots {
		if !rootHit[root] {
			missingRoots = append(missingRoots, root)
		}
	}
	if len(missingRoots) > 0 {
		return diagnostic.Newf(
			diagnostic.IDCatalogInvalid,
			diagnostic.PathLocation(missingRoots[0]),
			"catalog missing required root(s): %s",
			strings.Join(missingRoots, ", "),
		)
	}

	return nil
}

func isForbiddenPath(p string) bool {
	p = path.Clean(p)
	// Dual lock filename anywhere.
	if p == "versions.lock" || strings.HasSuffix(p, "/versions.lock") {
		return true
	}
	// Recipe-only profile trees (REQ-073/075).
	for _, base := range []string{"profiles/configuration", "profiles/local-persistence"} {
		if p == base || strings.HasPrefix(p, base+"/") {
			return true
		}
	}
	return false
}

// listRoots returns sorted unique top-level names present in files
// (directories implied by first path component, plus top-level files).
func listRoots(files map[string][]byte) []string {
	set := map[string]struct{}{}
	for p := range files {
		p = path.Clean(p)
		if p == "." || p == "" {
			continue
		}
		if i := strings.IndexByte(p, '/'); i >= 0 {
			set[p[:i]] = struct{}{}
		} else {
			set[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// readAllFiles walks fsys and returns path→content for every regular file.
// Paths use forward slashes relative to the FS root.
func readAllFiles(fsys fs.FS) (map[string][]byte, error) {
	out := make(map[string][]byte)
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip non-regular (embed.FS is regular-only; be defensive for mapFS tests).
		if !d.Type().IsRegular() && d.Type() != 0 {
			// embed.FS DirEntry Type may be 0; still try read.
			info, infoErr := d.Info()
			if infoErr == nil && !info.Mode().IsRegular() {
				return fmt.Errorf("catalog: non-regular file %s", p)
			}
		}
		p = path.Clean(p)
		if p == "." {
			return nil
		}
		b, readErr := fs.ReadFile(fsys, p)
		if readErr != nil {
			return readErr
		}
		out[p] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
