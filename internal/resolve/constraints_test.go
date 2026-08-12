package resolve_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestCheckProfilePredicatesTable is the flat table for every
// archetype/visibility/module-host predicate that applies (acceptance).
func TestCheckProfilePredicatesTable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("predicate_table")

	// Synthetic distribution-shaped profile (matches Section 20).
	dist := &catalog.Manifest{
		ID:                   "distribution",
		Kind:                 catalog.KindProfile,
		CompatibleArchetypes: []string{"cli", "tui"},
		RequiresVisibility:   "public",
	}
	// Synthetic profile with no visibility predicate (archetype-only).
	cliOnly := &catalog.Manifest{
		ID:                   "cli-only-fixture",
		Kind:                 catalog.KindProfile,
		CompatibleArchetypes: []string{"cli"},
	}
	// Synthetic tui+private fixture.
	tuiPrivate := &catalog.Manifest{
		ID:                   "tui-private-fixture",
		Kind:                 catalog.KindProfile,
		CompatibleArchetypes: []string{"tui"},
		RequiresVisibility:   "private",
	}

	type row struct {
		name       string
		m          *catalog.Manifest
		archetype  string
		visibility string
		module     string
		wantOK     bool
		predicate  string // expected named predicate on failure
	}
	rows := []row{
		// distribution happy paths
		{"dist_cli_public_gh", dist, "cli", "public", "github.com/acme/demo-cli", true, ""},
		{"dist_tui_public_gh", dist, "tui", "public", "github.com/acme/demo-tui", true, ""},
		// archetype failures
		{"dist_bad_archetype", dist, "web", "public", "github.com/acme/demo-cli", false, resolve.PredicateCompatibleArchetype},
		{"cli_only_rejects_tui", cliOnly, "tui", "private", "github.com/acme/x", false, resolve.PredicateCompatibleArchetype},
		{"cli_only_accepts_cli", cliOnly, "cli", "private", "github.com/acme/x", true, ""},
		// visibility failures
		{"dist_private", dist, "cli", "private", "github.com/acme/demo-cli", false, resolve.PredicateRequiresVisibility},
		{"tui_private_ok", tuiPrivate, "tui", "private", "github.com/acme/x", true, ""},
		{"tui_private_rejects_public", tuiPrivate, "tui", "public", "github.com/acme/x", false, resolve.PredicateRequiresVisibility},
		// module-host (distribution only)
		{"dist_gitlab", dist, "cli", "public", "gitlab.com/acme/demo-cli", false, resolve.PredicateModuleHost},
		{"dist_nested", dist, "cli", "public", "github.com/acme/org/demo-cli", false, resolve.PredicateModuleHost},
		{"dist_host_only", dist, "cli", "public", "github.com", false, resolve.PredicateModuleHost},
		{"dist_empty_module", dist, "cli", "public", "", false, resolve.PredicateModuleHost},
		{"dist_extra_segment", dist, "cli", "public", "github.com/acme/demo-cli/v2", false, resolve.PredicateModuleHost},
		// non-distribution ignores module host
		{"cli_only_non_gh_ok", cliOnly, "cli", "private", "example.com/acme/x", true, ""},
	}

	for _, tc := range rows {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("predicate")
			log.Inputs(map[string]string{
				"profile":    tc.m.ID,
				"archetype":  tc.archetype,
				"visibility": tc.visibility,
				"module":     tc.module,
			})
			err := resolve.CheckProfilePredicates(tc.m, tc.archetype, tc.visibility, tc.module)
			if tc.wantOK {
				log.Assert("ok", err == nil, true, err == nil)
				if err != nil {
					log.Fail("unexpected", err.Error())
				}
			} else {
				log.Assert("err", err != nil, true, err != nil)
				fe, ok := diagnostic.AsFoundryError(err)
				log.Assert("foundry", ok, true, ok)
				if ok {
					log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
					log.Assert("id_constraint", fe.ID() == diagnostic.IDResolveProfileConstraint,
						string(diagnostic.IDResolveProfileConstraint), string(fe.ID()))
					log.Assert("names_predicate", strings.Contains(fe.Message(), tc.predicate) ||
						strings.Contains(fe.Remediation(), tc.predicate),
						tc.predicate, fe.Message())
					log.Step("failed_predicate", testutil.OutcomeOK, "predicate="+tc.predicate)
					log.Assert("exit_2", fe.ExitCode() == diagnostic.ExitUsage, diagnostic.ExitUsage, fe.ExitCode())
				}
			}
			log.PhaseEnd("predicate", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("predicate_table", testutil.OutcomeOK)
}

// TestIsGitHubHostedModule table (Section 20 shape).
func TestIsGitHubHostedModule(t *testing.T) {
	log := testutil.New(t)
	log.Phase("module_host")
	cases := []struct {
		in   string
		want bool
	}{
		{"github.com/acme/tool", true},
		{"github.com/robertguss/repo-map", true},
		{"github.com/a/b", true},
		{"", false},
		{"github.com", false},
		{"github.com/", false},
		{"github.com/acme", false},
		{"github.com/acme/", false},
		{"github.com/acme/tool/extra", false},
		{"github.com/acme/tool/v2", false},
		{"gitlab.com/acme/tool", false},
		{"example.com/acme/tool", false},
		{"github.com/../tool", false},
		{"GitHub.com/acme/tool", false}, // case-sensitive host
	}
	for _, tc := range cases {
		got := resolve.IsGitHubHostedModule(tc.in)
		log.Assert("mod_"+tc.in, got == tc.want, tc.want, got)
	}
	log.PhaseEnd("module_host", testutil.OutcomeOK)
}

// TestDistributionPredicatesViaCatalogManifest uses the real embedded
// distribution unit for predicate evaluation (still not selectable in MVP).
func TestDistributionPredicatesViaCatalogManifest(t *testing.T) {
	log := testutil.New(t)
	log.Phase("catalog_dist_predicates")
	cat := mustLoadCatalog(t)
	m, ok := cat.Manifest("distribution")
	log.Assert("present", ok, true, ok)
	if !ok {
		return
	}
	log.Assert("kind", m.Kind == catalog.KindProfile, catalog.KindProfile, m.Kind)
	log.Assert("visibility", m.RequiresVisibility == "public", "public", m.RequiresVisibility)

	// Public + GH + cli → OK at predicate layer.
	err := resolve.CheckProfilePredicates(m, "cli", "public", "github.com/acme/demo-cli")
	log.Assert("ok_public", err == nil, true, err == nil)

	// Private fails visibility even with GH module.
	err = resolve.CheckProfilePredicates(m, "cli", "private", "github.com/acme/demo-cli")
	log.Assert("fail_private", err != nil, true, err != nil)
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
		log.Assert("predicate_vis", strings.Contains(fe.Message(), resolve.PredicateRequiresVisibility) ||
			strings.Contains(fe.Message(), "visibility"), true, false)
	}
	log.PhaseEnd("catalog_dist_predicates", testutil.OutcomeOK)
}
