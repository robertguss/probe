package resolve_test

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestCatalogToResolveManifestShapeContract codifies the catalog invariants
// that resolve assumes (REQ-092 / Section 24.3). Every catalog unit consumed by
// Resolve must have a non-empty ID, a valid Kind, and profile units must not
// be recipe-only IDs.
func TestCatalogToResolveManifestShapeContract(t *testing.T) {
	log := testutil.New(t)
	log.Phase("load_catalog")
	cat := mustLoadCatalog(t)
	archetypes := make(map[string]bool)
	for _, id := range cat.ArchetypeIDs() {
		archetypes[id] = true
	}
	profiles := make(map[string]bool)
	for _, id := range cat.ProfileIDs() {
		profiles[id] = true
	}
	log.PhaseEnd("load_catalog", testutil.OutcomeOK)

	log.Phase("manifest_shape_table")
	cases := []struct {
		name   string
		id     string
		want   catalog.Kind
		checks func(t *testing.T, m *catalog.Manifest)
	}{
		{
			name: "core",
			id:   "core",
			want: catalog.KindCore,
			checks: func(t *testing.T, m *catalog.Manifest) {
				if m.ID != "core" {
					t.Fatalf("core manifest ID = %q, want %q", m.ID, "core")
				}
			},
		},
		{
			name: "archetype_cli",
			id:   "cli",
			want: catalog.KindArchetype,
			checks: func(t *testing.T, m *catalog.Manifest) {
				if !archetypes[m.ID] {
					t.Fatalf("archetype manifest %q not in ArchetypeIDs", m.ID)
				}
			},
		},
		{
			name: "archetype_tui",
			id:   "tui",
			want: catalog.KindArchetype,
			checks: func(t *testing.T, m *catalog.Manifest) {
				if !archetypes[m.ID] {
					t.Fatalf("archetype manifest %q not in ArchetypeIDs", m.ID)
				}
			},
		},
		{
			name: "profile_distribution",
			id:   "distribution",
			want: catalog.KindProfile,
			checks: func(t *testing.T, m *catalog.Manifest) {
				if !profiles[m.ID] {
					t.Fatalf("profile manifest %q not in ProfileIDs", m.ID)
				}
				if catalog.IsRecipeOnlyProfile(m.ID) {
					t.Fatalf("profile manifest %q is recipe-only and must not be selectable", m.ID)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase(tc.name)
			m, ok := cat.Manifest(tc.id)
			if !ok {
				t.Fatalf("manifest %q missing from catalog", tc.id)
			}
			if m == nil {
				t.Fatalf("manifest %q is nil", tc.id)
			}
			if m.ID == "" {
				t.Fatalf("manifest %q has empty ID", tc.id)
			}
			if m.Kind != tc.want {
				t.Fatalf("manifest %q kind = %q, want %q", tc.id, m.Kind, tc.want)
			}
			if tc.checks != nil {
				tc.checks(t, m)
			}
			sub.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("manifest_shape_table", testutil.OutcomeOK)
}

// TestResolveToPlanResolvedProjectContract codifies the invariants Resolve
// promises to plan.Construct (Section 27 / REQ-098). A successful resolve must
// yield a non-nil project with sorted unique profiles, clean relative file
// paths, sorted dependencies, and a valid module path.
func TestResolveToPlanResolvedProjectContract(t *testing.T) {
	log := testutil.New(t)
	log.Phase("resolve_contract")
	cat := mustLoadCatalog(t)

	cases := []struct {
		name string
		vs   *spec.ValidatedSpecification
	}{
		{
			name: "minimal_cli",
			vs:   minimalCLI(t),
		},
		{
			name: "minimal_tui",
			vs:   minimalTUI(t),
		},
		{
			name: "cli_with_distribution",
			vs:   specWith(t, map[string]string{"visibility": "public", "profiles": `["distribution"]`}),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase("resolve")
			rp, err := resolve.Resolve(tc.vs, cat)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if rp == nil {
				t.Fatal("Resolve returned nil project")
			}

			sub.Phase("invariants")
			if rp.Name() == "" {
				t.Fatal("ResolvedProject.Name is empty")
			}
			if rp.Module() == "" {
				t.Fatal("ResolvedProject.Module is empty")
			}
			if rp.Destination() == "" {
				t.Fatal("ResolvedProject.Destination is empty")
			}
			if rp.Binary() == "" {
				t.Fatal("ResolvedProject.Binary is empty")
			}

			profs := rp.Profiles()
			if profs == nil {
				t.Fatal("ResolvedProject.Profiles is nil after success")
			}
			seenProf := make(map[string]bool, len(profs))
			for i, id := range profs {
				if seenProf[id] {
					t.Fatalf("duplicate profile ID: %s", id)
				}
				seenProf[id] = true
				if i > 0 && id <= profs[i-1] {
					t.Fatalf("profiles not sorted: %v", profs)
				}
			}

			files := rp.Files()
			seen := make(map[string]bool, len(files))
			for _, f := range files {
				if f.Path == "" {
					t.Fatal("file contribution has empty path")
				}
				if seen[f.Path] {
					t.Fatalf("duplicate file path after resolve: %s", f.Path)
				}
				seen[f.Path] = true
				if f.Owner == "" {
					t.Fatalf("file %q has empty owner", f.Path)
				}
			}

			deps := rp.Dependencies()
			for i := 1; i < len(deps); i++ {
				if deps[i].Module < deps[i-1].Module {
					t.Fatalf("dependencies not sorted by module: %v", deps)
				}
			}
			sub.PhaseEnd("invariants", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("resolve_contract", testutil.OutcomeOK)
}
