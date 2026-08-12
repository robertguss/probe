package render_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// contractCatalog returns an in-memory catalog with the minimal source files
// needed to render a canonical CLI project surface.
func contractCatalog() mapCatalog {
	c := mapCatalog{}
	for k, v := range fixtureCatalog() {
		c[k] = v
	}
	for k, v := range templateFixtureCatalog() {
		c[k] = v
	}
	// Core static files required for the generated-project contract.
	c["core/files/gitignore.static"] = []byte("*.tmp\n")
	c["core/files/internal/version/version.go.static"] = []byte("package version\nconst Version = \"0.0.0\"\n")
	c["core/files/workflows/ci.yml.tmpl"] = []byte(
		"# CI for [[.Name]]\nname: ci\non:\n  push:\n    branches: [main]\n  pull_request:\n    branches: [main]\njobs:\n  check:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - run: go test ./...\n")
	return c
}

// TestRenderInventoryContract codifies the invariants verify (and later
// placement) assumes about a rendered Inventory (REQ-096 / Section 30).
// Inventory entries must be sorted, path-clean, relative, unique, and paired
// with accessible content and a valid mode.
func TestRenderInventoryContract(t *testing.T) {
	log := testutil.New(t)
	log.Phase("render_inventory_contract")
	cat := contractCatalog()

	cases := []struct {
		name string
		jobs []render.Job
	}{
		{
			name: "static_only",
			jobs: []render.Job{
				{Mechanism: render.MechanismStatic, Static: &render.StaticJob{Path: "LICENSE", Mode: "0644", Source: "core/files/LICENSE"}},
				{Mechanism: render.MechanismStatic, Static: &render.StaticJob{Path: "docs/NOTICE", Mode: "0644", Source: "core/files/LICENSE"}},
			},
		},
		{
			name: "template_and_static",
			jobs: []render.Job{
				{Mechanism: render.MechanismTemplate, Template: &render.TemplateJob{Path: "README.md", Mode: "0644", Source: "core/files/README.md.tmpl", Data: minimalCLITemplateData()}},
				{Mechanism: render.MechanismStatic, Static: &render.StaticJob{Path: "LICENSE", Mode: "0644", Source: "core/files/LICENSE"}},
			},
		},
		{
			name: "gomod_only",
			jobs: []render.Job{
				{Mechanism: render.MechanismGomod, Gomod: &render.GomodInput{Module: "github.com/example/demo", GoVersion: "1.26.0", Toolchain: "go1.26.5"}},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase(tc.name)
			inv, err := render.RenderAll(cat, tc.jobs, nil)
			if err != nil {
				t.Fatalf("RenderAll: %v", err)
			}
			if inv == nil {
				t.Fatal("RenderAll returned nil inventory")
			}

			entries := inv.Entries()
			paths := inv.Paths()
			if len(entries) != len(paths) {
				t.Fatalf("Entries/Paths length mismatch: %d vs %d", len(entries), len(paths))
			}

			seen := make(map[string]bool, len(entries))
			for i, e := range entries {
				if e.Path == "" {
					t.Fatalf("entry %d has empty path", i)
				}
				if filepath.IsAbs(e.Path) {
					t.Fatalf("entry path is absolute: %s", e.Path)
				}
				if strings.Contains(e.Path, "..") {
					t.Fatalf("entry path contains ..: %s", e.Path)
				}
				if e.Path != filepath.Clean(e.Path) {
					t.Fatalf("entry path not clean: %s != %s", e.Path, filepath.Clean(e.Path))
				}
				if seen[e.Path] {
					t.Fatalf("duplicate inventory path: %s", e.Path)
				}
				seen[e.Path] = true
				if i > 0 && e.Path <= entries[i-1].Path {
					t.Fatalf("entries not sorted by path at %d: %s <= %s", i, e.Path, entries[i-1].Path)
				}
				if e.Mode == "" {
					t.Fatalf("entry %q has empty mode", e.Path)
				}
				if _, ok := inv.Content(e.Path); !ok {
					t.Fatalf("content missing for path %q", e.Path)
				}
				if e.Mechanism != render.MechanismStatic && e.Mechanism != render.MechanismTemplate && e.Mechanism != render.MechanismGomod {
					t.Fatalf("entry %q has unknown mechanism %q", e.Path, e.Mechanism)
				}
			}
			sub.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("render_inventory_contract", testutil.OutcomeOK)
}

// TestGeneratedProjectRequiredFilesContract renders the full file set for the
// canonical minimal-cli spec and asserts that the generated project contains
// required files with valid modes and a syntactically parseable CI workflow
// (Section 15 / REQ-033 / REQ-120).
func TestGeneratedProjectRequiredFilesContract(t *testing.T) {
	log := testutil.New(t)
	log.Phase("generated_project_contract")
	cat := contractCatalog()

	jobs := []render.Job{
		{Mechanism: render.MechanismTemplate, Template: &render.TemplateJob{Path: "README.md", Mode: "0644", Source: "core/files/README.md.tmpl", Data: minimalCLITemplateData()}},
		{Mechanism: render.MechanismTemplate, Template: &render.TemplateJob{Path: ".github/workflows/ci.yml", Mode: "0644", Source: "core/files/workflows/ci.yml.tmpl", Data: minimalCLITemplateData()}},
		{Mechanism: render.MechanismStatic, Static: &render.StaticJob{Path: ".gitignore", Mode: "0644", Source: "core/files/gitignore.static"}},
		{Mechanism: render.MechanismStatic, Static: &render.StaticJob{Path: "internal/version/version.go", Mode: "0644", Source: "core/files/internal/version/version.go.static"}},
		{Mechanism: render.MechanismTemplate, Template: &render.TemplateJob{Path: "main.go", Mode: "0644", Source: "archetypes/cli/files/main.go.tmpl", Data: minimalCLITemplateData()}},
		{Mechanism: render.MechanismGomod, Gomod: &render.GomodInput{Module: "github.com/example/demo-cli", GoVersion: "1.26.0", Toolchain: "go1.26.5"}},
	}

	inv, err := render.RenderAll(cat, jobs, nil)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}

	required := map[string]bool{
		"go.mod":                      true,
		"main.go":                     true,
		"README.md":                   true,
		".gitignore":                  true,
		"internal/version/version.go": true,
		".github/workflows/ci.yml":    true,
	}

	for _, e := range inv.Entries() {
		if required[e.Path] {
			if e.Mode == "" {
				t.Fatalf("required file %q has empty mode", e.Path)
			}
			delete(required, e.Path)
		}
	}
	if len(required) > 0 {
		missing := make([]string, 0, len(required))
		for p := range required {
			missing = append(missing, p)
		}
		t.Fatalf("missing required generated files: %v", missing)
	}

	ciYAML, ok := inv.Content(".github/workflows/ci.yml")
	if !ok {
		t.Fatal("CI workflow content missing")
	}
	if len(ciYAML) == 0 {
		t.Fatal("CI workflow content is empty")
	}
	// Syntactic YAML check: rendered workflow must contain GitHub Actions markers.
	if !strings.Contains(string(ciYAML), "on:") && !strings.Contains(string(ciYAML), "jobs:") {
		sample := string(ciYAML[:minInt(len(ciYAML), 200)])
		t.Fatalf("CI workflow does not look like GitHub Actions YAML: %q", sample)
	}

	log.PhaseEnd("generated_project_contract", testutil.OutcomeOK)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
