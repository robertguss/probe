package render_test

import (
	"strings"
	"testing"
	"unicode"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"github.com/robertguss/go-foundry-cli/internal/render"
)

// FuzzGomodValidModulePaths ensures random but valid module paths still
// produce parseable go.mod with no replace/exclude/retract (P1.5.c fuzz).
func FuzzGomodValidModulePaths(f *testing.F) {
	seeds := []string{
		"github.com/example/foundry-smoke-cli",
		"github.com/acme/tool",
		"example.com/a",
		"golang.org/x/mod",
		"charm.land/bubbletea/v2",
		"gopkg.in/yaml.v3",
		"std", // invalid — should reject
		"",
		"github.com/UPPER/case", // may reject
		"../escape",
		"github.com/acme/tool.git",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, modPath string) {
		// Bound size to keep CheckPath and formatting cheap.
		if len(modPath) > 256 {
			return
		}
		// Skip paths with control characters / non-printables that are never
		// valid module paths and only thrash error paths.
		for _, r := range modPath {
			if r == 0 || !unicode.IsPrint(r) {
				return
			}
		}

		in := render.GomodInput{
			Module:    modPath,
			GoVersion: render.CatalogGoVersion,
			Toolchain: "go1.26.5",
			Requires: []render.GomodRequire{
				{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
			},
		}
		content, err := render.GenerateGoMod(in)

		// If CheckPath rejects the path, generator must fail closed with
		// render.failed and produce no content.
		if checkErr := module.CheckPath(strings.TrimSpace(modPath)); checkErr != nil || strings.TrimSpace(modPath) == "" {
			if err == nil {
				t.Fatalf("expected error for invalid module path %q", modPath)
			}
			if content != nil {
				t.Fatalf("expected nil content on error for %q", modPath)
			}
			return
		}

		if err != nil {
			t.Fatalf("GenerateGoMod(%q): %v", modPath, err)
		}
		parsed, perr := modfile.Parse("go.mod", content, nil)
		if perr != nil {
			t.Fatalf("generated go.mod not parseable for %q: %v\n%s", modPath, perr, content)
		}
		if parsed.Module == nil || parsed.Module.Mod.Path != strings.TrimSpace(modPath) {
			t.Fatalf("module path mismatch: got %#v want %q", parsed.Module, modPath)
		}
		if len(parsed.Replace) != 0 || len(parsed.Exclude) != 0 || len(parsed.Retract) != 0 {
			t.Fatalf("forbidden directives present for %q: replace=%d exclude=%d retract=%d",
				modPath, len(parsed.Replace), len(parsed.Exclude), len(parsed.Retract))
		}
		if parsed.Go == nil || parsed.Go.Version != render.CatalogGoVersion {
			t.Fatalf("go directive missing/wrong for %q", modPath)
		}
		if parsed.Toolchain == nil || parsed.Toolchain.Name != "go1.26.5" {
			t.Fatalf("toolchain missing/wrong for %q", modPath)
		}
	})
}
