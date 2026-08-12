package render_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/render"
)

// FuzzSafeOutputPathNeverPanics feeds arbitrary strings through safeOutputPath.
// Every input must either return a cleaned relative path or fs.unsafe_path —
// never panic (bead go-foundry-cli-n0y.7).
func FuzzSafeOutputPathNeverPanics(f *testing.F) {
	seeds := []string{
		"",
		"main.go",
		"cmd/foo/main.go",
		"./rel.go",
		"/abs/path",
		"../escape",
		"a/../b",
		"a//b",
		"a\\b",
		"C:\\windows",
		"a\x00b",
		"..",
		".",
		"...",
		strings.Repeat("a/", 64) + "x.go",
		"docs/README.md",
		".github/workflows/ci.yml",
		"profiles/distribution/files/releasing.md",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, p string) {
		if len(p) > 8192 {
			return
		}
		// Skip pure invalid UTF-8 thrash; still must not panic.
		if !utf8.ValidString(p) && len(p) > 256 {
			return
		}
		out, err := render.SafeOutputPathForTest(p)
		if err == nil {
			if out == "" {
				t.Fatalf("empty cleaned path for %q", p)
			}
			// Reject only true parent-escape segments, not names like "...".
			for _, seg := range strings.Split(out, "/") {
				if seg == ".." {
					t.Fatalf("cleaned path still has .. segment: %q from %q", out, p)
				}
			}
			if strings.HasPrefix(out, "/") {
				t.Fatalf("cleaned path absolute: %q from %q", out, p)
			}
			return
		}
		fe, ok := diagnostic.AsFoundryError(err)
		if !ok {
			t.Fatalf("non-FoundryError for %q: %v", p, err)
		}
		if fe.ID() != diagnostic.IDFSUnsafePath {
			t.Fatalf("unexpected id %s for %q: %v", fe.ID(), p, err)
		}
	})
}
