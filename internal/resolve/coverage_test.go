package resolve

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestErrorConstructorsAndHelpers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("error_ctors")

	ua := unknownArchetypeError("widget", []string{"tui", "cli"})
	log.Assert("ua_id", ua.ID() == diagnostic.IDResolveUnknownArchetype, true, ua.ID())
	log.Assert("ua_sorted", strings.Contains(ua.Message(), "[cli, tui]"), true, ua.Message())
	log.Assert("ua_rem", strings.Contains(ua.Remediation(), "cli"), true, ua.Remediation())

	dup := duplicateProfileError("distribution", 0, 2, []string{"distribution"})
	log.Assert("dup_id", dup.ID() == diagnostic.IDSpecDuplicateProfile, true, dup.ID())
	log.Assert("dup_idx", strings.Contains(dup.Message(), "indexes 0 and 2"), true, dup.Message())

	miss := missingImplementedProfileInCatalog("ghost", []string{"distribution"})
	log.Assert("miss_id", miss.ID() == diagnostic.IDCatalogInvalid, true, miss.ID())
	log.Assert("miss_path", miss.Location().Path == "profiles/ghost", true, miss.Location().Path)

	core := missingCoreError()
	log.Assert("core_id", core.ID() == diagnostic.IDCatalogInvalid, true, core.ID())
	log.Assert("core_path", core.Location().Path == "core", true, core.Location().Path)

	mau := missingArchetypeUnitError("missing", []string{"cli"})
	log.Assert("mau_id", mau.ID() == diagnostic.IDResolveUnknownArchetype, true, mau.ID())

	fc := fileCollisionError("README.md", []string{"profile:b", "core"})
	log.Assert("fc_id", fc.ID() == diagnostic.IDPlanFileCollision, true, fc.ID())
	log.Assert("fc_owners", strings.Contains(fc.Message(), "core") && strings.Contains(fc.Message(), "profile:b"), true, fc.Message())

	pc := parentChildCollisionError("docs", "docs/a.md", "core", "archetype:cli")
	log.Assert("pc_id", pc.ID() == diagnostic.IDPlanFileCollision, true, pc.ID())
	log.Assert("pc_msg", strings.Contains(pc.Message(), "docs/a.md"), true, pc.Message())

	dc := dependencyConflictError("github.com/x/y", "v1.0.0", "core", "v2.0.0", "archetype:cli")
	log.Assert("dc_id", dc.ID() == diagnostic.IDPlanFileCollision, true, dc.ID())
	log.Assert("dc_vers", strings.Contains(dc.Message(), "v1.0.0") && strings.Contains(dc.Message(), "v2.0.0"), true, dc.Message())

	rej := rejectProfileSelection("configuration", []string{"distribution"})
	fe, ok := diagnostic.AsFoundryError(rej)
	log.Assert("rej_foundry", ok, true, ok)
	if ok {
		log.Assert("rej_id", fe.ID() == diagnostic.IDResolveUnknownProfile, true, fe.ID())
	}

	log.PhaseEnd("error_ctors", testutil.OutcomeOK)
}

func TestResolvedProjectAccessorsEqualString(t *testing.T) {
	log := testutil.New(t)
	log.Phase("project_accessors")

	var nilRP *ResolvedProject
	log.Assert("nil_name", nilRP.Name() == "", true, nilRP.Name())
	log.Assert("nil_module", nilRP.Module() == "", true, nilRP.Module())
	log.Assert("nil_desc", nilRP.Description() == "", true, nilRP.Description())
	log.Assert("nil_arch", nilRP.Archetype() == "", true, nilRP.Archetype())
	log.Assert("nil_dest", nilRP.Destination() == "", true, nilRP.Destination())
	log.Assert("nil_bin", nilRP.Binary() == "", true, nilRP.Binary())
	log.Assert("nil_vis", nilRP.Visibility() == "", true, nilRP.Visibility())
	log.Assert("nil_git", !nilRP.GitInit(), true, nilRP.GitInit())
	log.Assert("nil_branch", nilRP.GitInitialBranch() == "", true, nilRP.GitInitialBranch())
	log.Assert("nil_profiles", nilRP.Profiles() == nil, true, nilRP.Profiles() != nil)
	log.Assert("nil_digest", nilRP.CatalogDigest() == "", true, string(nilRP.CatalogDigest()))
	log.Assert("nil_files", nilRP.Files() == nil, true, nilRP.Files() != nil)
	log.Assert("nil_deps", nilRP.Dependencies() == nil, true, nilRP.Dependencies() != nil)
	log.Assert("nil_fc", nilRP.FileCount() == 0, true, nilRP.FileCount())
	log.Assert("nil_dc", nilRP.DependencyCount() == 0, true, nilRP.DependencyCount())
	log.Assert("nil_string", nilRP.String() == "ResolvedProject(nil)", true, nilRP.String())
	log.Assert("nil_equal_nil", nilRP.Equal(nil), true, false)
	log.Assert("nil_ne_empty", !nilRP.Equal(&ResolvedProject{}), true, false)
	log.Assert("summary_nil", Summary(nil) == "resolve: <nil>", true, Summary(nil))

	rp := &ResolvedProject{
		name:             "demo",
		module:           "github.com/ex/demo",
		description:      "d",
		archetype:        "cli",
		destination:      "./demo",
		binary:           "demo",
		visibility:       "private",
		gitInit:          true,
		gitInitialBranch: "main",
		profiles:         []string{"distribution"},
		catalogDigest:    "abc",
		files: []FileContribution{
			{Path: "a.go", Owner: "core"},
		},
		dependencies: []DependencyContribution{
			{Module: "m", Version: "v1", Owner: "core"},
		},
	}
	log.Assert("name", rp.Name() == "demo", true, rp.Name())
	log.Assert("module", rp.Module() == "github.com/ex/demo", true, rp.Module())
	log.Assert("desc", rp.Description() == "d", true, rp.Description())
	log.Assert("arch", rp.Archetype() == "cli", true, rp.Archetype())
	log.Assert("dest", rp.Destination() == "./demo", true, rp.Destination())
	log.Assert("bin", rp.Binary() == "demo", true, rp.Binary())
	log.Assert("vis", rp.Visibility() == "private", true, rp.Visibility())
	log.Assert("git", rp.GitInit(), true, rp.GitInit())
	log.Assert("branch", rp.GitInitialBranch() == "main", true, rp.GitInitialBranch())
	log.Assert("profiles", len(rp.Profiles()) == 1 && rp.Profiles()[0] == "distribution", true, rp.Profiles())
	log.Assert("digest", string(rp.CatalogDigest()) == "abc", true, string(rp.CatalogDigest()))
	log.Assert("files", len(rp.Files()) == 1, true, len(rp.Files()))
	log.Assert("deps", len(rp.Dependencies()) == 1, true, len(rp.Dependencies()))
	log.Assert("fc", rp.FileCount() == 1, true, rp.FileCount())
	log.Assert("dc", rp.DependencyCount() == 1, true, rp.DependencyCount())
	s := rp.String()
	log.Assert("string", strings.Contains(s, "demo") && strings.Contains(s, "cli"), true, s)
	sum := Summary(rp)
	log.Assert("summary", strings.Contains(sum, "archetype=cli") && strings.Contains(sum, "files=1"), true, sum)

	other := *rp
	other.files = append([]FileContribution(nil), rp.files...)
	other.dependencies = append([]DependencyContribution(nil), rp.dependencies...)
	other.profiles = append([]string(nil), rp.profiles...)
	log.Assert("equal_self", rp.Equal(&other), true, false)

	diffName := other
	diffName.name = "x"
	log.Assert("ne_name", !rp.Equal(&diffName), true, false)

	diffProfLen := other
	diffProfLen.profiles = []string{"a", "b"}
	log.Assert("ne_prof_len", !rp.Equal(&diffProfLen), true, false)

	diffProfVal := other
	diffProfVal.profiles = []string{"other"}
	log.Assert("ne_prof_val", !rp.Equal(&diffProfVal), true, false)

	diffFilesLen := other
	diffFilesLen.files = nil
	log.Assert("ne_files_len", !rp.Equal(&diffFilesLen), true, false)

	diffFile := other
	diffFile.files = []FileContribution{{Path: "b.go", Owner: "core"}}
	log.Assert("ne_file", !rp.Equal(&diffFile), true, false)

	diffDepsLen := other
	diffDepsLen.dependencies = nil
	log.Assert("ne_deps_len", !rp.Equal(&diffDepsLen), true, false)

	diffDep := other
	diffDep.dependencies = []DependencyContribution{{Module: "m", Version: "v2", Owner: "core"}}
	log.Assert("ne_dep", !rp.Equal(&diffDep), true, false)

	log.PhaseEnd("project_accessors", testutil.OutcomeOK)
}

func TestOwnerLabelAndFormatIDSet(t *testing.T) {
	log := testutil.New(t)
	log.Phase("owner_label")
	log.Assert("core", OwnerLabel(catalog.KindCore, "core") == "core", true, OwnerLabel(catalog.KindCore, "core"))
	log.Assert("arch", OwnerLabel(catalog.KindArchetype, "cli") == "archetype:cli", true, OwnerLabel(catalog.KindArchetype, "cli"))
	log.Assert("prof", OwnerLabel(catalog.KindProfile, "distribution") == "profile:distribution", true, OwnerLabel(catalog.KindProfile, "distribution"))
	log.Assert("unknown_empty_id", OwnerLabel(catalog.Kind("weird"), "") == "weird", true, OwnerLabel(catalog.Kind("weird"), ""))
	log.Assert("unknown_with_id", OwnerLabel(catalog.Kind("weird"), "x") == "weird:x", true, OwnerLabel(catalog.Kind("weird"), "x"))
	log.Assert("empty_set", formatIDSet(nil) == "[]", true, formatIDSet(nil))
	log.Assert("set", formatIDSet([]string{"a", "b"}) == "[a, b]", true, formatIDSet([]string{"a", "b"}))
	log.PhaseEnd("owner_label", testutil.OutcomeOK)
}

func TestSortedCopyImplementedSetAndContribute(t *testing.T) {
	log := testutil.New(t)
	log.Phase("helpers")

	sc := sortedCopy([]string{"b", "", "a", "b", "a"})
	log.Assert("sorted_unique", len(sc) == 2 && sc[0] == "a" && sc[1] == "b", true, sc)
	log.Assert("sorted_empty", len(sortedCopy(nil)) == 0, true, len(sortedCopy(nil)))

	m := implementedSetFrom([]string{"distribution", "", "x"})
	_, ok := m["distribution"]
	log.Assert("impl_set", ok && len(m) == 2, true, len(m))
	_, empty := m[""]
	log.Assert("no_empty_key", !empty, true, empty)

	files, deps := collectContributions([]*catalog.Manifest{
		nil,
		{
			Kind:    catalog.KindCore,
			ID:      "core",
			UnitDir: "core",
			Files: []catalog.FileEntry{
				{Path: "README.md", Source: "README.md", Render: catalog.RenderStatic, Mode: "0644"},
			},
			Dependencies: []catalog.Dependency{
				{Module: "example.com/a", Version: "v1.0.0", Scope: catalog.ScopeRuntime},
			},
		},
		{
			Kind: catalog.KindArchetype,
			ID:   "cli",
			Files: []catalog.FileEntry{
				{Path: "cmd/{{binary}}/main.go", Source: "main.go.tmpl", Render: catalog.RenderTemplate, Mode: "0644"},
			},
			Dependencies: []catalog.Dependency{
				{Module: "example.com/b", Version: "v2.0.0", Scope: catalog.ScopeRuntime},
				{Module: "example.com/a", Version: "v1.0.0", Scope: catalog.ScopeTest},
			},
		},
	})
	log.Assert("files_n", len(files) == 2, true, len(files))
	log.Assert("src_joined", files[0].Source == "core/README.md", true, files[0].Source)
	log.Assert("deps_n", len(deps) == 3, true, len(deps))

	sf := []FileContribution{
		{Path: "a", Owner: "z"},
		{Path: "a", Owner: "a"},
		{Path: "b", Owner: "b"},
	}
	sortFiles(sf)
	log.Assert("files_owner_tie", sf[0].Owner == "a" && sf[1].Owner == "z", true, sf[0].Owner+","+sf[1].Owner)

	sd := []DependencyContribution{
		{Module: "m", Owner: "b", Scope: catalog.ScopeTool},
		{Module: "m", Owner: "a", Scope: catalog.ScopeTest},
		{Module: "m", Owner: "a", Scope: catalog.ScopeRuntime},
		{Module: "a", Owner: "x", Scope: catalog.ScopeRuntime},
	}
	sortDeps(sd)
	log.Assert("deps_mod", sd[0].Module == "a", true, sd[0].Module)
	log.Assert("deps_owner_scope", sd[1].Owner == "a" && sd[1].Scope == catalog.ScopeRuntime, true, string(sd[1].Scope))
	log.Assert("deps_scope2", sd[2].Owner == "a" && sd[2].Scope == catalog.ScopeTest, true, string(sd[2].Scope))

	u := unitOrder(nil, nil, nil)
	log.Assert("unit_empty", len(u) == 0, true, len(u))
	coreM := &catalog.Manifest{ID: "core", Kind: catalog.KindCore}
	archM := &catalog.Manifest{ID: "cli", Kind: catalog.KindArchetype}
	u2 := unitOrder(coreM, archM, []*catalog.Manifest{{ID: "distribution", Kind: catalog.KindProfile}})
	log.Assert("unit_order", len(u2) == 3 && u2[0].ID == "core" && u2[1].ID == "cli", true, len(u2))

	log.Assert("clean_slash", cleanOutputPath(`a\b`) == "a/b", true, cleanOutputPath(`a\b`))
	log.Assert("clean_space", cleanOutputPath("  x/y  ") == "x/y", true, cleanOutputPath("  x/y  "))
	_ = cleanOutputPath(".")
	log.Assert("prefix_empty", !isPathPrefix("", "a"), true, false)
	log.Assert("prefix_same", !isPathPrefix("a", "a"), true, false)
	log.Assert("prefix_ok", isPathPrefix("docs", "docs/x"), true, false)
	log.Assert("prefix_no", !isPathPrefix("doc", "docs/x"), true, false)
	log.Assert("prefix_foobar", !isPathPrefix("foo", "foobar"), true, false)

	if err := detectFileCollisions(nil); err != nil {
		t.Fatalf("empty files: %v", err)
	}
	err := detectFileCollisions([]FileContribution{
		{Path: "docs/x.md", Owner: "profile:d"},
		{Path: "docs", Owner: "core"},
	})
	log.Assert("parent_child", err != nil, true, err != nil)

	log.PhaseEnd("helpers", testutil.OutcomeOK)
}

func TestResolveFlatErrorBranches(t *testing.T) {
	log := testutil.New(t)
	log.Phase("resolve_flat_errors")

	cat, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}

	_, err = resolveFlat(nil, cat, nil)
	log.Assert("nil_spec", err != nil, true, err != nil)
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		log.Assert("nil_spec_id", fe.ID() == diagnostic.IDSpecInvalidField, true, fe.ID())
	}

	vs := mustValidateInternal(t, `
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "d"
archetype = "cli"
destination = "./demo-cli"
profiles = []
`)
	_, err = resolveFlat(vs, nil, nil)
	log.Assert("nil_cat", err != nil, true, err != nil)
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		log.Assert("nil_cat_id", fe.ID() == diagnostic.IDCatalogInvalid, true, fe.ID())
	}

	_, err = profileManifestsForSelection(cat, []string{"does-not-exist-as-profile-unit-xyz"})
	log.Assert("missing_impl_profile", err != nil, true, err != nil)
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		log.Assert("missing_impl_id", fe.ID() == diagnostic.IDCatalogInvalid, true, fe.ID())
	}

	_, err = profileManifestsForSelection(cat, []string{"cli"})
	log.Assert("kind_mismatch", err != nil, true, err != nil)

	err = CheckProfilePredicates(nil, "cli", "private", "m")
	log.Assert("pred_nil", err != nil, true, err != nil)

	coreMan, ok := cat.Manifest("core")
	if !ok {
		t.Fatal("core missing")
	}
	err = CheckProfilePredicates(coreMan, "cli", "private", "m")
	log.Assert("pred_non_profile", err != nil, true, err != nil)

	// Recipe-only ID listed as implemented still fails (RejectUnknownProfiles / fence).
	vsRecipe := mustValidateInternal(t, `
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "d"
archetype = "cli"
destination = "./demo-cli"
profiles = ["configuration"]
`)
	_, err = resolveFlat(vsRecipe, cat, []string{"configuration"})
	log.Assert("recipe_fence", err != nil, true, err != nil)

	// Happy path with empty implemented (no profiles).
	rp, err := resolveFlat(vs, cat, nil)
	if err != nil {
		log.Fail("empty_impl_ok", err.Error())
	}
	log.Assert("empty_profiles", len(rp.Profiles()) == 0, true, len(rp.Profiles()))
	log.Assert("equal_copy", rp.Equal(rp), true, false)

	log.PhaseEnd("resolve_flat_errors", testutil.OutcomeOK)
}
