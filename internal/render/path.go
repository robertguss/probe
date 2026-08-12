package render

import (
	"path"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// safeOutputPath validates a destination-relative output path at the render
// planning layer (REQ-094). Rejects absolute paths, backslashes, empty
// segments, NUL, and ".." escape components.
//
// On success returns the cleaned forward-slash path. On failure returns
// fs.unsafe_path (stable escape/unsafe-path identifier).
func safeOutputPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", unsafePathError(p, "output path must be non-empty")
	}
	if path.IsAbs(p) || strings.HasPrefix(p, "/") {
		return "", unsafePathError(p, "output path must be relative (not absolute)")
	}
	// Windows drive / UNC shapes (defensive; Foundry paths are POSIX-style).
	if len(p) >= 2 && p[1] == ':' {
		return "", unsafePathError(p, "output path must be relative (not absolute)")
	}
	if strings.Contains(p, "\\") {
		return "", unsafePathError(p, "output path must use forward slashes only")
	}
	if strings.Contains(p, "\x00") {
		return "", unsafePathError(p, "output path must not contain NUL")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", unsafePathError(p, `output path must not contain ".." segments`)
		}
		if seg == "" {
			return "", unsafePathError(p, "output path must not contain empty segments")
		}
	}
	// path.Clean collapses "./a" → "a"; re-check that clean does not escape.
	cleaned := path.Clean(p)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", unsafePathError(p, `output path must not contain ".." segments`)
	}
	if path.IsAbs(cleaned) || strings.HasPrefix(cleaned, "/") {
		return "", unsafePathError(p, "output path must be relative (not absolute)")
	}
	if cleaned == "." {
		return "", unsafePathError(p, "output path must name a file, not the current directory")
	}
	return cleaned, nil
}

func unsafePathError(p, msg string) *diagnostic.FoundryError {
	loc := diagnostic.PathLocation(p)
	if p == "" {
		loc = diagnostic.PathLocation("<empty>")
	}
	return diagnostic.New(diagnostic.IDFSUnsafePath, msg, loc).WithRemediation(
		"Use a relative output path with no \"..\" segments, no leading slash, and forward slashes only. " +
			"Catalog and plan paths are destination-relative (REQ-094).",
	)
}
