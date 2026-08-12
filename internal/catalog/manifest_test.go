package catalog_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestValidManifestsEmbedded loads the production catalog and asserts every
// unit manifest is flat-valid (core, cli, tui, distribution).
func TestValidManifestsEmbedded(t *testing.T) {
	log := testutil.New(t)
	log.Phase("load")
	log.Fixture("catalog", "embedded")

	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	ms := c.Manifests()
	log.Step("manifest_count", testutil.OutcomeOK, "n="+itoa(len(ms)))
	for _, m := range ms {
		log.Step("manifest_"+m.ID, testutil.OutcomeOK, m.String())
		log.NoteID(m.ID)
	}
	log.PhaseEnd("load", testutil.OutcomeOK)

	log.Phase("assert_units")
	wantIDs := []string{"cli", "core", "distribution", "tui"}
	gotIDs := make([]string, len(ms))
	for i, m := range ms {
		gotIDs[i] = m.ID
	}
	log.Assert("ids_sorted", equalStrings(gotIDs, wantIDs), strings.Join(wantIDs, ","), strings.Join(gotIDs, ","))

	core, ok := c.Manifest("core")
	log.Assert("core_present", ok, true, ok)
	if ok {
		log.Assert("core_kind", core.Kind == catalog.KindCore, catalog.KindCore, core.Kind)
		log.Assert("core_schema", core.Schema == 1, 1, core.Schema)
		log.Assert("core_desc_nonempty", core.Description != "", true, core.Description != "")
		log.Assert("core_files", len(core.Files) == 11, 11, len(core.Files))
		log.Assert("core_no_compat", len(core.CompatibleArchetypes) == 0, 0, len(core.CompatibleArchetypes))
		// Core mixes static + template (Section 16.2); every file is mode 0644.
		hasStatic, hasTemplate := false, false
		for _, f := range core.Files {
			log.Assert("core_mode_"+f.Path, f.Mode == catalog.FileModeV1, catalog.FileModeV1, f.Mode)
			switch f.Render {
			case catalog.RenderStatic:
				hasStatic = true
			case catalog.RenderTemplate:
				hasTemplate = true
			}
		}
		log.Assert("core_has_static", hasStatic, true, hasStatic)
		log.Assert("core_has_template", hasTemplate, true, hasTemplate)
		log.Assert("core_has_go_cmp", hasDepModule(core, "github.com/google/go-cmp"), true, false)
		log.Assert("core_cmp_scope", depScope(core, "github.com/google/go-cmp") == catalog.ScopeTest,
			catalog.ScopeTest, depScope(core, "github.com/google/go-cmp"))
	}

	cli, ok := c.Manifest("cli")
	log.Assert("cli_present", ok, true, ok)
	if ok {
		log.Assert("cli_kind", cli.Kind == catalog.KindArchetype, catalog.KindArchetype, cli.Kind)
		log.Assert("cli_has_cobra", hasDepModule(cli, "github.com/spf13/cobra"), true, false)
		log.Assert("cli_dep_scope", depScope(cli, "github.com/spf13/cobra") == catalog.ScopeRuntime,
			catalog.ScopeRuntime, depScope(cli, "github.com/spf13/cobra"))
	}

	tui, ok := c.Manifest("tui")
	log.Assert("tui_present", ok, true, ok)
	if ok {
		log.Assert("tui_kind", tui.Kind == catalog.KindArchetype, catalog.KindArchetype, tui.Kind)
		log.Assert("tui_has_bubbletea", hasDepModule(tui, "charm.land/bubbletea/v2"), true, false)
	}

	dist, ok := c.Manifest("distribution")
	log.Assert("dist_present", ok, true, ok)
	if ok {
		log.Assert("dist_kind", dist.Kind == catalog.KindProfile, catalog.KindProfile, dist.Kind)
		log.Assert("dist_compat_cli", contains(dist.CompatibleArchetypes, "cli"), true, false)
		log.Assert("dist_compat_tui", contains(dist.CompatibleArchetypes, "tui"), true, false)
		log.Assert("dist_visibility", dist.RequiresVisibility == "public", "public", dist.RequiresVisibility)
		log.Assert("dist_static_file", len(dist.Files) >= 1 && dist.Files[0].Render == catalog.RenderStatic,
			catalog.RenderStatic, dist.Files[0].Render)
	}
	log.PhaseEnd("assert_units", testutil.OutcomeOK)

	log.Phase("lock_pins")
	// Declared deps must match lock pins (already enforced at Load).
	consumed := catalog.LockConsumptionFromManifests(c.Lock(), ms)
	log.Step("consumed_modules", testutil.OutcomeOK, "keys="+itoa(len(consumed)))
	log.Assert("cobra_consumed", consumed["module:cobra"] >= 1, 1, consumed["module:cobra"])
	log.Assert("bubbletea_consumed", consumed["module:bubbletea"] >= 1, 1, consumed["module:bubbletea"])
	log.PhaseEnd("lock_pins", testutil.OutcomeOK)
}

// TestValidManifestTable covers additional synthetic valid manifests via
// ParseManifest (without full catalog layout).
func TestValidManifestTable(t *testing.T) {
	log := testutil.New(t)
	cases := []struct {
		name string
		path string
		toml string
	}{
		{
			name: "core_static",
			path: "core/manifest.toml",
			toml: `
schema = 1
id = "core"
kind = "core"
description = "core unit"

[[files]]
path = "LICENSE"
render = "static"
source = "files/LICENSE"
mode = "0644"
`,
		},
		{
			name: "archetype_cli_min",
			path: "archetypes/cli/manifest.toml",
			toml: `
schema = 1
id = "cli"
kind = "archetype"
description = "cli archetype"

[[files]]
path = "cmd/{{binary}}/main.go"
render = "template"
source = "files/main.go.tmpl"
mode = "0644"
`,
		},
		{
			name: "profile_flat",
			path: "profiles/distribution/manifest.toml",
			toml: `
schema = 1
id = "distribution"
kind = "profile"
description = "distribution profile"
compatible_archetypes = ["tui", "cli"]
requires_visibility = "public"

[[files]]
path = "docs/releasing.md"
render = "static"
source = "files/releasing.md"
mode = "0644"
`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("parse_" + tc.name)
			log.Inputs(map[string]string{"manifest_id": tc.name, "path": tc.path})
			// Provide matching source bytes so source-existence check passes.
			files := map[string][]byte{}
			// Parse without files map first for schema-only, then with sources.
			m, err := catalog.ParseManifest(tc.path, []byte(tc.toml), nil)
			log.Assert("err_nil", err == nil, true, errString(err))
			if err != nil {
				log.PhaseEnd("parse_"+tc.name, testutil.OutcomeFail)
				return
			}
			log.Step("manifest_id", testutil.OutcomeOK, "id="+m.ID)
			log.Assert("schema_1", m.Schema == 1, 1, m.Schema)
			log.NoteID(m.ID)
			// Profile compat is sorted.
			if m.Kind == catalog.KindProfile {
				log.Assert("compat_sorted", equalStrings(m.CompatibleArchetypes, []string{"cli", "tui"}),
					"cli,tui", strings.Join(m.CompatibleArchetypes, ","))
			}
			_ = files
			log.PhaseEnd("parse_"+tc.name, testutil.OutcomeOK)
		})
	}
	_ = log
}

// TestInvalidManifestTable covers unknown fields, bad mechanisms, empty id,
// and DAG-like keys. Every invalid class must yield catalog.invalid.
func TestInvalidManifestTable(t *testing.T) {
	type tc struct {
		name       string
		path       string
		toml       string
		wantSubstr string // message or field-path hint
		class      string // for step logs
	}
	cases := []tc{
		{
			name:       "unknown_field",
			path:       "core/manifest.toml",
			class:      "unknown_field",
			wantSubstr: "unknown field",
			toml: `
schema = 1
id = "core"
kind = "core"
description = "x"
extra_field = true
`,
		},
		{
			name:       "dag_requires",
			path:       "profiles/distribution/manifest.toml",
			class:      "dag_key",
			wantSubstr: "requires",
			toml: `
schema = 1
id = "distribution"
kind = "profile"
description = "x"
compatible_archetypes = ["cli"]
requires = ["other"]
`,
		},
		{
			name:       "dag_conflicts",
			path:       "profiles/distribution/manifest.toml",
			class:      "dag_key",
			wantSubstr: "conflicts",
			toml: `
schema = 1
id = "distribution"
kind = "profile"
description = "x"
compatible_archetypes = ["cli"]
conflicts = ["other"]
`,
		},
		{
			name:       "dag_capabilities",
			path:       "profiles/distribution/manifest.toml",
			class:      "dag_key",
			wantSubstr: "capabilities",
			toml: `
schema = 1
id = "distribution"
kind = "profile"
description = "x"
compatible_archetypes = ["cli"]
[capabilities]
foo = true
`,
		},
		{
			name:       "dag_provides",
			path:       "profiles/distribution/manifest.toml",
			class:      "dag_key",
			wantSubstr: "provides",
			toml: `
schema = 1
id = "distribution"
kind = "profile"
description = "x"
compatible_archetypes = ["cli"]
provides = ["cap"]
`,
		},
		{
			name:       "dag_helper_binary",
			path:       "profiles/distribution/manifest.toml",
			class:      "dag_key",
			wantSubstr: "helper_binary",
			toml: `
schema = 1
id = "distribution"
kind = "profile"
description = "x"
compatible_archetypes = ["cli"]
helper_binary = "foo"
`,
		},
		{
			name:       "bad_mechanism_yaml",
			path:       "core/manifest.toml",
			class:      "bad_mechanism",
			wantSubstr: "render",
			toml: `
schema = 1
id = "core"
kind = "core"
description = "x"

[[files]]
path = "a.yml"
render = "yaml"
source = "files/a.yml"
mode = "0644"
`,
		},
		{
			name:       "bad_mechanism_gomod_on_file",
			path:       "core/manifest.toml",
			class:      "bad_mechanism",
			wantSubstr: "gomod",
			toml: `
schema = 1
id = "core"
kind = "core"
description = "x"

[[files]]
path = "go.mod"
render = "gomod"
source = "files/go.mod"
mode = "0644"
`,
		},
		{
			name:       "empty_id",
			path:       "core/manifest.toml",
			class:      "empty_id",
			wantSubstr: "id",
			toml: `
schema = 1
id = ""
kind = "core"
description = "x"
`,
		},
		{
			name:       "missing_description",
			path:       "core/manifest.toml",
			class:      "missing_required",
			wantSubstr: "description",
			toml: `
schema = 1
id = "core"
kind = "core"
`,
		},
		{
			name:       "bad_kind",
			path:       "core/manifest.toml",
			class:      "bad_kind",
			wantSubstr: "kind",
			toml: `
schema = 1
id = "core"
kind = "plugin"
description = "x"
`,
		},
		{
			name:       "bad_schema",
			path:       "core/manifest.toml",
			class:      "bad_schema",
			wantSubstr: "schema",
			toml: `
schema = 99
id = "core"
kind = "core"
description = "x"
`,
		},
		{
			name:       "compat_on_core",
			path:       "core/manifest.toml",
			class:      "profile_field_on_core",
			wantSubstr: "compatible_archetypes",
			toml: `
schema = 1
id = "core"
kind = "core"
description = "x"
compatible_archetypes = ["cli"]
`,
		},
		{
			name:       "profile_missing_compat",
			path:       "profiles/distribution/manifest.toml",
			class:      "profile_missing_compat",
			wantSubstr: "compatible_archetypes",
			toml: `
schema = 1
id = "distribution"
kind = "profile"
description = "x"
`,
		},
		{
			name:       "unknown_archetype_id",
			path:       "profiles/distribution/manifest.toml",
			class:      "unknown_archetype",
			wantSubstr: "compatible_archetypes",
			toml: `
schema = 1
id = "distribution"
kind = "profile"
description = "x"
compatible_archetypes = ["web"]
`,
		},
		{
			name:       "unsafe_path_dotdot",
			path:       "core/manifest.toml",
			class:      "unsafe_path",
			wantSubstr: "path",
			toml: `
schema = 1
id = "core"
kind = "core"
description = "x"

[[files]]
path = "../escape"
render = "static"
source = "files/x"
mode = "0644"
`,
		},
		{
			name:       "absolute_path",
			path:       "core/manifest.toml",
			class:      "unsafe_path",
			wantSubstr: "path",
			toml: `
schema = 1
id = "core"
kind = "core"
description = "x"

[[files]]
path = "/etc/passwd"
render = "static"
source = "files/x"
mode = "0644"
`,
		},
		{
			name:       "bad_scope",
			path:       "archetypes/cli/manifest.toml",
			class:      "bad_scope",
			wantSubstr: "scope",
			toml: `
schema = 1
id = "cli"
kind = "archetype"
description = "x"

[[dependencies]]
module = "github.com/spf13/cobra"
version = "v1.10.2"
scope = "optional"
`,
		},
		{
			name:       "version_no_v",
			path:       "archetypes/cli/manifest.toml",
			class:      "bad_version",
			wantSubstr: "version",
			toml: `
schema = 1
id = "cli"
kind = "archetype"
description = "x"

[[dependencies]]
module = "github.com/spf13/cobra"
version = "1.10.2"
scope = "runtime"
`,
		},
		{
			name:       "id_mismatch_dir",
			path:       "archetypes/cli/manifest.toml",
			class:      "id_mismatch",
			wantSubstr: "id",
			toml: `
schema = 1
id = "notcli"
kind = "archetype"
description = "x"
`,
		},
		{
			name:       "missing_source_file",
			path:       "core/manifest.toml",
			class:      "missing_source",
			wantSubstr: "source",
			toml: `
schema = 1
id = "core"
kind = "core"
description = "x"

[[files]]
path = "README.md"
render = "template"
source = "files/does-not-exist.tmpl"
mode = "0644"
`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("invalid_" + tc.class)
			log.Inputs(map[string]string{
				"case":  tc.name,
				"class": tc.class,
				"path":  tc.path,
			})

			var files map[string][]byte
			if tc.class == "missing_source" {
				// Non-nil map without the source path.
				files = map[string][]byte{}
			}
			m, err := catalog.ParseManifest(tc.path, []byte(tc.toml), files)
			log.Assert("err_non_nil", err != nil, true, err == nil)
			log.Assert("manifest_nil", m == nil, true, m != nil)

			fe, ok := diagnostic.AsFoundryError(err)
			log.Assert("is_foundry_error", ok, true, ok)
			if ok {
				// Golden error id for every invalid class.
				log.Assert("error_id", fe.ID() == diagnostic.IDCatalogInvalid,
					string(diagnostic.IDCatalogInvalid), string(fe.ID()))
				log.Step("error_id", testutil.OutcomeOK, "id="+string(fe.ID()))
				log.Step("field_path", testutil.OutcomeOK, "loc="+fe.Location().String())
				log.NoteID(string(fe.ID()))
				msg := err.Error()
				log.Assert("msg_mentions_class",
					strings.Contains(msg, tc.wantSubstr) || strings.Contains(fe.Location().String(), tc.wantSubstr),
					tc.wantSubstr, msg)
			}
			log.PhaseEnd("invalid_"+tc.class, testutil.OutcomeOK)
		})
	}
}

// TestManifestValidationDeterminism re-parses embedded manifests twice and
// asserts stable IDs/kinds (also intended for go test -count=2).
func TestManifestValidationDeterminism(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")

	c1, err := catalog.Load()
	if err != nil {
		log.Fail("load1", err.Error())
	}
	c2, err := catalog.Load()
	if err != nil {
		log.Fail("load2", err.Error())
	}
	m1, m2 := c1.Manifests(), c2.Manifests()
	log.Assert("count_equal", len(m1) == len(m2), len(m1), len(m2))
	for i := range m1 {
		log.Assert("id_"+m1[i].ID, m1[i].ID == m2[i].ID, m1[i].ID, m2[i].ID)
		log.Assert("kind_"+m1[i].ID, m1[i].Kind == m2[i].Kind, m1[i].Kind, m2[i].Kind)
		log.Assert("files_"+m1[i].ID, len(m1[i].Files) == len(m2[i].Files), len(m1[i].Files), len(m2[i].Files))
	}
	log.Step("manifest_ids", testutil.OutcomeOK, idsCSV(m1))
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

// TestLoadFSRejectsBadManifest ensures LoadFS fails closed on invalid units.
func TestLoadFSRejectsBadManifest(t *testing.T) {
	log := testutil.New(t)
	log.Phase("inject_dag")
	base := mustEmbeddedMap(t)
	// Reintroduce a deleted capability key on the distribution profile.
	base["profiles/distribution/manifest.toml"] = []byte(`
schema = 1
id = "distribution"
kind = "profile"
description = "broken"
compatible_archetypes = ["cli", "tui"]
requires_visibility = "public"
capabilities = ["release"]
`)
	_, err := catalog.LoadFS(toMapFS(base))
	log.Assert("err", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "profiles/distribution/manifest.toml")
	if err != nil {
		log.Step("error_ids", testutil.OutcomeOK, "msg="+err.Error())
		log.Assert("mentions_capabilities", strings.Contains(err.Error(), "capabilities"), true, false)
	}
	log.PhaseEnd("inject_dag", testutil.OutcomeOK)

	log.Phase("inject_unknown_render")
	base2 := mustEmbeddedMap(t)
	base2["core/manifest.toml"] = []byte(`
schema = 1
id = "core"
kind = "core"
description = "broken"

[[files]]
path = "README.md"
render = "mustache"
source = "files/README.md.tmpl"
mode = "0644"
`)
	_, err = catalog.LoadFS(toMapFS(base2))
	log.Assert("err", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "core/manifest.toml")
	log.PhaseEnd("inject_unknown_render", testutil.OutcomeOK)

	log.Phase("unpinned_dependency")
	base3 := mustEmbeddedMap(t)
	base3["archetypes/cli/manifest.toml"] = []byte(`
schema = 1
id = "cli"
kind = "archetype"
description = "cli"

[[files]]
path = "cmd/{{binary}}/main.go"
render = "template"
source = "files/main.go.tmpl"
mode = "0644"

[[dependencies]]
module = "example.com/not-in-lock"
version = "v0.0.1"
scope = "runtime"
`)
	_, err = catalog.LoadFS(toMapFS(base3))
	log.Assert("err", err != nil, true, err != nil)
	assertCatalogInvalid(t, log, err, "dependencies")
	log.PhaseEnd("unpinned_dependency", testutil.OutcomeOK)
}

// --- manifest helpers ---

func hasDepModule(m *catalog.Manifest, mod string) bool {
	if m == nil {
		return false
	}
	for _, d := range m.Dependencies {
		if d.Module == mod {
			return true
		}
	}
	return false
}

func depScope(m *catalog.Manifest, mod string) catalog.DependencyScope {
	if m == nil {
		return ""
	}
	for _, d := range m.Dependencies {
		if d.Module == mod {
			return d.Scope
		}
	}
	return ""
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func idsCSV(ms []*catalog.Manifest) string {
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = m.ID
	}
	return strings.Join(parts, ",")
}
