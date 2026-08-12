package plan_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestResolveToPlanContract codifies the invariants plan.Construct expects from
// a successfully resolved project (Section 28 / REQ-120). The resulting Plan
// must expose a deterministic, complete generation plan: valid schema, non-empty
// metadata, sorted unique files and dependencies, valid external steps, and a
// canonical JSON representation with a non-empty digest.
func TestResolveToPlanContract(t *testing.T) {
	log := testutil.New(t)
	log.Phase("resolve_to_plan_contract")
	cat := mustLoadCatalog(t)

	cases := []struct {
		name string
		vs   *spec.ValidatedSpecification
	}{
		{name: "minimal_cli", vs: minimalCLI(t)},
		{name: "minimal_tui", vs: minimalTUI(t)},
		{
			name: "cli_with_distribution",
			vs: mustValidate(t, "distribution-cli.toml", `
schema = 1
name = "dist-cli"
module = "github.com/example/dist-cli"
description = "A public CLI with distribution"
archetype = "cli"
destination = "./dist-cli"
visibility = "public"
profiles = ["distribution"]
`),
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

			sub.Phase("construct")
			dest := fixedDestination(tc.vs.Destination(), plan.ObservationParentMissing)
			in := defaultInputs(rp, cat, dest)
			p, err := plan.Construct(in)
			if err != nil {
				t.Fatalf("Construct: %v", err)
			}
			if p == nil {
				t.Fatal("Construct returned nil plan")
			}

			sub.Phase("schema_and_meta")
			if p.Schema() != 1 {
				t.Fatalf("schema = %d, want 1", p.Schema())
			}
			foundry := p.Foundry()
			if foundry.Version == "" || foundry.Go == "" || foundry.CatalogDigest == "" {
				t.Fatalf("incomplete foundry meta: %+v", foundry)
			}

			sub.Phase("project_identity")
			proj := p.Project()
			if proj.Name == "" || proj.Module == "" || proj.Binary == "" || proj.Archetype == "" {
				t.Fatalf("incomplete project identity: %+v", proj)
			}

			sub.Phase("profiles_sorted_unique")
			profs := p.Profiles()
			if profs == nil {
				t.Fatal("Profiles is nil")
			}
			seenProf := make(map[string]bool, len(profs))
			for i, id := range profs {
				if seenProf[id] {
					t.Fatalf("duplicate profile: %s", id)
				}
				seenProf[id] = true
				if i > 0 && id < profs[i-1] {
					t.Fatalf("profiles not sorted: %v", profs)
				}
			}

			sub.Phase("files_sorted_unique_with_content_digest")
			files := p.Files()
			seenPath := make(map[string]bool, len(files))
			for i, f := range files {
				if f.Path == "" {
					t.Fatalf("file %d has empty path", i)
				}
				if seenPath[f.Path] {
					t.Fatalf("duplicate planned file: %s", f.Path)
				}
				seenPath[f.Path] = true
				if i > 0 && f.Path < files[i-1].Path {
					t.Fatalf("files not sorted by path: %s before %s", files[i-1].Path, f.Path)
				}
				if f.Mode == "" {
					t.Fatalf("file %q has empty mode", f.Path)
				}
				if f.Render == "" {
					t.Fatalf("file %q has empty render mechanism", f.Path)
				}
				if f.ContentSHA256 == "" {
					t.Fatalf("file %q has empty content digest", f.Path)
				}
				if f.Owner == "" {
					t.Fatalf("file %q has empty owner", f.Path)
				}
			}

			sub.Phase("dependencies_sorted_by_module")
			deps := p.Dependencies()
			for i := 1; i < len(deps); i++ {
				if deps[i].Module < deps[i-1].Module {
					t.Fatalf("dependencies not sorted by module: %v", deps)
				}
				if deps[i].Module == deps[i-1].Module && deps[i].Scope < deps[i-1].Scope {
					t.Fatalf("dependencies with same module not sorted by scope: %v", deps)
				}
			}

			sub.Phase("tools_non_empty_when_known")
			tools := p.Tools()
			for i, tl := range tools {
				if tl.Module == "" || tl.Version == "" || tl.Owner == "" {
					t.Fatalf("tool %d incomplete: %+v", i, tl)
				}
			}

			sub.Phase("external_steps")
			steps := p.ExternalSteps()
			if len(steps) == 0 {
				t.Fatal("Plan has no external steps")
			}
			for i, s := range steps {
				if s.ID == "" {
					t.Fatalf("external step %d has empty ID", i)
				}
				if s.Binary == "" {
					t.Fatalf("external step %d has empty Binary", i)
				}
				if len(s.Argv) == 0 {
					t.Fatalf("external step %d has empty Argv", i)
				}
				if s.Cwd == "" {
					t.Fatalf("external step %d has empty Cwd", i)
				}
				if s.TimeoutS <= 0 {
					t.Fatalf("external step %d has non-positive timeout", i)
				}
			}

			sub.Phase("verification")
			v := p.Verification()
			if !v.Mode.Valid() {
				t.Fatalf("unknown verification mode %q", v.Mode)
			}
			if len(v.Checks) == 0 {
				t.Fatal("verification has no ordered checks")
			}

			sub.Phase("canonical_json_and_digest")
			jsonBytes := p.JSON()
			if len(jsonBytes) == 0 {
				t.Fatal("Plan.JSON is empty")
			}
			if !strings.HasSuffix(string(jsonBytes), "\n") {
				t.Fatal("Plan.JSON does not end with newline")
			}
			digest := p.PlanSHA256()
			if digest == "" {
				t.Fatal("Plan.PlanSHA256 is empty")
			}
			if len(digest) != 64 {
				t.Fatalf("Plan.PlanSHA256 length = %d, want 64", len(digest))
			}
			sub.PhaseEnd("canonical_json_and_digest", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("resolve_to_plan_contract", testutil.OutcomeOK)
}
