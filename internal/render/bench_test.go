package render_test

import (
	"sort"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/render"
)

// BenchmarkRenderAll measures rendering a small mixed batch of planned jobs
// (static + template) against the embedded catalog.
func BenchmarkRenderAll(b *testing.B) {
	cat, err := catalog.Load()
	if err != nil {
		b.Fatalf("catalog.Load: %v", err)
	}

	// Pick deterministic catalog sources for the static job.
	files := cat.Files()
	if len(files) == 0 {
		b.Fatal("catalog has no files")
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	jobs := []render.Job{
		{
			Mechanism: render.MechanismStatic,
			Static: &render.StaticJob{
				Path:   "internal/version/version.go",
				Mode:   "0644",
				Source: "core/files/internal/version/version.go.static",
				Owner:  "core",
			},
		},
		{
			Mechanism: render.MechanismTemplate,
			Template: &render.TemplateJob{
				Path:   "README.md",
				Mode:   "0644",
				Source: "core/files/README.md.tmpl",
				Owner:  "core",
				Data: render.TemplateData{
					Name:        "foundry-bench",
					Module:      "github.com/example/foundry-bench",
					Description: "benchmark fixture",
					Archetype:   "cli",
					Visibility:  "private",
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inv, err := render.RenderAll(cat, jobs, nil)
		if err != nil {
			b.Fatalf("render: %v", err)
		}
		if inv == nil {
			b.Fatal("nil inventory")
		}
	}
}
