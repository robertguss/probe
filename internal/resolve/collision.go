package resolve

import (
	"path"
	"sort"
	"strings"
)

// pathOwners maps an output path to the ordered list of owner labels that claim it.
type pathOwners map[string][]string

// detectFileCollisions reports the first exact-path multi-owner collision or
// parent-file/child-path conflict (REQ-093, Section 25.1). Paths are compared
// as cleaned slash-separated relative strings (template tokens preserved).
//
// Deterministic: walks sorted paths so -count=2 reports the same first error.
func detectFileCollisions(files []FileContribution) error {
	if len(files) == 0 {
		return nil
	}

	// Exact path collisions.
	byPath := make(pathOwners, len(files))
	for _, f := range files {
		p := cleanOutputPath(f.Path)
		byPath[p] = append(byPath[p], f.Owner)
	}

	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		// Multiple claims — including byte-identical content from two owners —
		// are fatal (REQ-093). Same owner listed twice is still multi-claim.
		if len(byPath[p]) > 1 {
			return fileCollisionError(p, byPath[p])
		}
	}

	// Parent-file versus child-path: if path A is a strict path-prefix of B
	// as a full segment boundary, both cannot be ordinary owned files.
	// Example: "docs" vs "docs/releasing.md".
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			a, b := paths[i], paths[j]
			if isPathPrefix(a, b) {
				return parentChildCollisionError(a, b, byPath[a][0], byPath[b][0])
			}
			if isPathPrefix(b, a) {
				return parentChildCollisionError(b, a, byPath[b][0], byPath[a][0])
			}
		}
	}
	return nil
}

// detectDependencyConflicts reports the first same-module version conflict
// across owners (Section 23.2 / 25.2). Same module+version from multiple
// owners is allowed (aggregated); differing versions are fatal.
//
// Deterministic: modules sorted lexicographically.
func detectDependencyConflicts(deps []DependencyContribution) error {
	type pin struct {
		version string
		owner   string
	}
	// module → first pin, then compare subsequent.
	first := make(map[string]pin, len(deps))

	// Process in stable owner/module order for first-error determinism.
	ordered := append([]DependencyContribution(nil), deps...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Module != ordered[j].Module {
			return ordered[i].Module < ordered[j].Module
		}
		return ordered[i].Owner < ordered[j].Owner
	})

	for _, d := range ordered {
		prev, ok := first[d.Module]
		if !ok {
			first[d.Module] = pin{version: d.Version, owner: d.Owner}
			continue
		}
		if prev.version != d.Version {
			return dependencyConflictError(d.Module, prev.version, prev.owner, d.Version, d.Owner)
		}
	}
	return nil
}

// cleanOutputPath normalizes a relative output path for comparison without
// resolving ".." (already rejected at catalog load) and without expanding
// template tokens.
func cleanOutputPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	// path.Clean collapses "a/./b" → "a/b" but also "." → "."; keep non-empty.
	c := path.Clean(p)
	if c == "." {
		return p
	}
	// path.Clean on Windows-style is fine; we forced /.
	return c
}

// isPathPrefix reports whether parent is a strict directory-prefix of child
// (segment-boundary). "foo" prefixes "foo/bar"; "foo" does not prefix "foobar".
func isPathPrefix(parent, child string) bool {
	if parent == "" || child == "" || parent == child {
		return false
	}
	if !strings.HasPrefix(child, parent) {
		return false
	}
	// Next rune after parent must be '/'.
	if len(child) <= len(parent) {
		return false
	}
	return child[len(parent)] == '/'
}
