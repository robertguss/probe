package catalog_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// treeUnit captures per-unit assertions for the consolidated catalog tree
// tests. It replaces the previous cli_tree_test.go, core_tree_test.go,
// tui_tree_test.go, and distribution_tree_test.go files.
type treeUnit struct {
	id                  string
	kind                catalog.Kind
	pathGolden          string
	inventoryGolden     string
	wantStatic          int
	wantTemplate        int
	wantFileCount       int
	wantDepCount        int
	deps                []depExpect
	forbiddenPaths      []string
	forbiddenContent    []string
	requiredPaths       []string
	requiredMainContent []string
	extra               func(t *testing.T, log *testutil.Logger, c *catalog.Catalog, m *catalog.Manifest, byPath map[string]catalog.FileEntry)
}

type depExpect struct {
	module string
	scope  catalog.DependencyScope
	// version is optional; empty means do not assert version.
	version string
}

var treeUnits = []treeUnit{
	{
		id:              "core",
		kind:            catalog.KindCore,
		pathGolden:      "core_section_16_2_paths.txt",
		inventoryGolden: "core_section_16_2_inventory.txt",
		wantStatic:      4,
		wantTemplate:    7,
		wantFileCount:   11,
		wantDepCount:    1,
		deps: []depExpect{
			{module: "github.com/google/go-cmp", scope: catalog.ScopeTest, version: "v0.7.0"},
		},
		forbiddenPaths: []string{
			"LICENSE", "license", "COPYING", "NOTICE",
			"provenance", "Makefile", "Taskfile", ".golangci",
			"pkg/", "util/", "common/", "scripts/",
			"greet", "demo",
		},
		forbiddenContent: []string{
			"internal/greet",
			"package greet",
			"package demo",
			"foundry-provenance",
			".foundry/provenance",
			"include taskfile",
			"include makefile",
			"golangci-lint run",
		},
		extra: coreExtraChecks,
	},
	{
		id:              "cli",
		kind:            catalog.KindArchetype,
		pathGolden:      "cli_section_17_2_paths.txt",
		inventoryGolden: "cli_section_17_2_inventory.txt",
		wantStatic:      1,
		wantTemplate:    8,
		wantFileCount:   9,
		wantDepCount:    2,
		deps: []depExpect{
			{module: "github.com/spf13/cobra", scope: catalog.ScopeRuntime, version: "v1.10.2"},
			{module: "github.com/rogpeppe/go-internal", scope: catalog.ScopeTest, version: "v1.15.0"},
		},
		forbiddenPaths: []string{
			"greet", "demo", "viper",
			"internal/app", "internal/service", "internal/domain", "internal/hello",
			"pkg/", "util/",
		},
		forbiddenContent: []string{
			"internal/greet",
			"package greet",
			"package demo",
			"github.com/spf13/viper",
			"\"viper\"",
			"func newgreet",
			"func greetcmd",
		},
		extra: archetypeExtraChecks,
	},
	{
		id:              "tui",
		kind:            catalog.KindArchetype,
		pathGolden:      "tui_section_18_2_paths.txt",
		inventoryGolden: "tui_section_18_2_inventory.txt",
		wantStatic:      5,
		wantTemplate:    6,
		wantFileCount:   11,
		wantDepCount:    3,
		deps: []depExpect{
			{module: "charm.land/bubbletea/v2", scope: catalog.ScopeRuntime, version: "v2.0.8"},
			{module: "charm.land/bubbles/v2", scope: catalog.ScopeRuntime, version: "v2.1.1"},
			{module: "charm.land/lipgloss/v2", scope: catalog.ScopeRuntime, version: "v2.0.5"},
		},
		forbiddenPaths: []string{
			"greet", "demo", "effects.go", "messages.go",
			"internal/app", "internal/service", "internal/domain", "internal/hello",
			"pkg/", "util/",
		},
		forbiddenContent: []string{
			"package greet",
			"package demo",
			"github.com/robertguss/go-foundry-cli",
			"internal/foundry",
			"func newgreet",
			"fake loading",
			"synthetic init",
		},
		requiredPaths: []string{
			"cmd/{{binary}}/main.go",
			"docs/ui-architecture.md",
			"internal/tui/app.go",
			"internal/tui/state.go",
			"internal/tui/update.go",
			"internal/tui/view.go",
			"internal/tui/keymap.go",
			"internal/tui/theme.go",
			"internal/tui/lifecycle_test.go",
		},
		extra: tuiExtraChecks,
	},
}

// TestCatalogTreeInventory is the consolidated inventory test for core, CLI,
// and TUI catalog units. It preserves the golden path-set and inventory
// assertions from the previous per-unit tree tests and removes duplicated
// scanning logic.
func TestCatalogTreeInventory(t *testing.T) {
	c := mustLoadCatalog(t)

	for _, u := range treeUnits {
		t.Run(u.id, func(t *testing.T) {
			log := testutil.New(t)
			m, ok := c.Manifest(u.id)
			if !ok || m == nil {
				log.Fail("manifest", u.id+" missing")
				return
			}
			log.Assert("kind", m.Kind == u.kind, u.kind, m.Kind)
			log.Assert("id", m.ID == u.id, u.id, m.ID)

			gotPaths := make([]string, 0, len(m.Files))
			for _, f := range m.Files {
				gotPaths = append(gotPaths, f.Path)
			}
			sort.Strings(gotPaths)

			wantPaths := loadLines(t, u.pathGolden)
			log.Assert("path_count", len(gotPaths) == len(wantPaths), len(wantPaths), len(gotPaths))
			pathsOK := equalStrings(gotPaths, wantPaths)
			log.Assert("paths_equal", pathsOK, true, pathsOK)
			if !pathsOK {
				t.Logf("got:\n%s", strings.Join(gotPaths, "\n"))
				t.Logf("want:\n%s", strings.Join(wantPaths, "\n"))
				t.Logf("got only: %v", diffLeft(gotPaths, wantPaths))
				t.Logf("want only: %v", diffLeft(wantPaths, gotPaths))
			}

			byPath := make(map[string]catalog.FileEntry, len(m.Files))
			for _, f := range m.Files {
				byPath[f.Path] = f
			}

			wantInv := loadInventoryRows(t, u.inventoryGolden)
			log.Assert("inventory_row_count", len(wantInv) == len(m.Files), len(m.Files), len(wantInv))
			hist := map[string]int{}
			for _, row := range wantInv {
				f, ok := byPath[row.path]
				if !ok {
					log.Fail("missing_path", row.path)
					continue
				}
				log.Assert("mode_"+row.path, f.Mode == row.mode, row.mode, f.Mode)
				log.Assert("render_"+row.path, string(f.Render) == row.render, row.render, string(f.Render))
				log.Assert("source_"+row.path, f.Source == row.source, row.source, f.Source)
				log.Assert("mode_v1_"+row.path, f.Mode == catalog.FileModeV1, catalog.FileModeV1, f.Mode)
				srcPath := path.Join(m.UnitDir, f.Source)
				raw, err := c.Read(srcPath)
				if err != nil {
					log.Fail("source_missing_"+row.path, srcPath+": "+err.Error())
				}
				if u.id == "core" {
					// Core logs source digests for release traceability.
					digest := sha256.Sum256(raw)
					log.Step("file", testutil.OutcomeOK,
						"owner=core path="+f.Path+" render="+string(f.Render)+" source="+srcPath+
							" mode="+f.Mode+" bytes="+itoa(len(raw))+" source_sha256="+hex.EncodeToString(digest[:]))
				} else {
					log.Step("file", testutil.OutcomeOK,
						"path="+f.Path+" render="+string(f.Render)+" source="+srcPath+" mode="+f.Mode)
				}
				hist[string(f.Render)]++
			}
			log.Step("mechanism_histogram", testutil.OutcomeOK,
				"static="+itoa(hist[string(catalog.RenderStatic)])+
					" template="+itoa(hist[string(catalog.RenderTemplate)]))
			log.Assert("has_static", hist[string(catalog.RenderStatic)] == u.wantStatic, u.wantStatic, hist[string(catalog.RenderStatic)])
			log.Assert("has_template", hist[string(catalog.RenderTemplate)] == u.wantTemplate, u.wantTemplate, hist[string(catalog.RenderTemplate)])
			log.Assert("no_gomod_file_mode", hist["gomod"] == 0, 0, hist["gomod"])
			sum := hist[string(catalog.RenderStatic)] + hist[string(catalog.RenderTemplate)]
			log.Assert("histogram_sum", sum == len(m.Files), len(m.Files), sum)

			for _, p := range u.requiredPaths {
				log.Assert("has_"+sanitizeAssertName(p), contains(gotPaths, p), true, false)
			}

			checkForbiddenPathsAndContent(t, log, c, m, u)
			checkDependencies(t, log, m, u)
			if u.extra != nil {
				u.extra(t, log, c, m, byPath)
			}
		})
	}

	t.Run("distribution", func(t *testing.T) {
		log := testutil.New(t)
		dist, ok := c.Manifest("distribution")
		if !ok || dist == nil {
			log.Fail("manifest", "distribution missing")
			return
		}
		log.Assert("kind", dist.Kind == catalog.KindProfile, catalog.KindProfile, dist.Kind)
		want := []string{
			".github/workflows/dependency-review.yml",
			".github/workflows/release.yml",
			".goreleaser.yaml",
			"CONTRIBUTING.md",
			"SECURITY.md",
			"docs/releasing.md",
		}
		sort.Strings(want)
		got := make([]string, 0, len(dist.Files))
		for _, f := range dist.Files {
			got = append(got, f.Path)
			src := path.Join(dist.UnitDir, f.Source)
			if _, err := c.Read(src); err != nil {
				log.Fail("source", src+": "+err.Error())
			}
		}
		sort.Strings(got)
		log.Assert("paths_equal", equalStrings(got, want), true, equalStrings(got, want))
		log.Assert("no_runtime_deps", len(dist.Dependencies) == 0, 0, len(dist.Dependencies))
		raw, _ := c.Read(path.Join(dist.UnitDir, "files/releasing.md"))
		log.Assert("license_block", strings.Contains(string(raw), "LICENSE"), true, false)
		log.Assert("compat_cli_tui", len(dist.CompatibleArchetypes) == 2, 2, len(dist.CompatibleArchetypes))
		log.Assert("requires_public", dist.RequiresVisibility == "public", "public", dist.RequiresVisibility)
	})
}

func checkForbiddenPathsAndContent(t *testing.T, log *testutil.Logger, c *catalog.Catalog, m *catalog.Manifest, u treeUnit) {
	t.Helper()
	var body strings.Builder
	for _, f := range m.Files {
		base := path.Base(f.Path)
		lowerPath := strings.ToLower(f.Path)
		for _, bad := range u.forbiddenPaths {
			if strings.Contains(lowerPath, strings.ToLower(bad)) || base == bad {
				log.Fail("forbidden_path", f.Path+" matches "+bad)
			}
		}
		srcPath := path.Join(m.UnitDir, f.Source)
		raw, err := c.Read(srcPath)
		if err != nil {
			log.Fail("read_"+srcPath, err.Error())
			continue
		}
		body.Write(raw)
		body.WriteByte('\n')
	}

	if u.id == "core" {
		// Core additionally forbids Claude-specific authority files as paths.
		for _, f := range m.Files {
			base := path.Base(f.Path)
			if base == "CLAUDE.md" || strings.HasPrefix(f.Path, ".claude/") {
				log.Fail("claude_specific", f.Path)
			}
		}
	}

	content := strings.ToLower(body.String())
	for _, tok := range u.forbiddenContent {
		present := strings.Contains(content, strings.ToLower(tok))
		if present {
			log.Fail("forbidden_content", tok)
		}
		log.Assert("absent_"+sanitizeAssertName(tok), !present, true, !present)
	}

	if u.id == "tui" {
		plain := body.String()
		if strings.Contains(plain, "tea.WithoutCatchPanics(") || strings.Contains(plain, "WithoutCatchPanics()") {
			log.Fail("without_catch_panics_call", "WithoutCatchPanics must not be called")
		}
		// Only internal/tui packages under internal/.
		for _, f := range m.Files {
			if strings.HasPrefix(f.Path, "internal/") && !strings.HasPrefix(f.Path, "internal/tui/") {
				log.Fail("extra_internal_package", f.Path)
			}
		}
	}
	if u.id == "cli" {
		for _, f := range m.Files {
			if strings.HasPrefix(f.Path, "internal/") && !strings.HasPrefix(f.Path, "internal/cli/") {
				log.Fail("extra_internal_package", f.Path)
			}
		}
	}
}

func checkDependencies(t *testing.T, log *testutil.Logger, m *catalog.Manifest, u treeUnit) {
	t.Helper()
	log.Step("dep_count", testutil.OutcomeOK, "n="+itoa(len(m.Dependencies)))
	log.Assert("dep_count", len(m.Dependencies) == u.wantDepCount, u.wantDepCount, len(m.Dependencies))
	found := map[string]bool{}
	runtimeDeps := 0
	for _, d := range m.Dependencies {
		if d.Scope == catalog.ScopeRuntime {
			runtimeDeps++
		}
		for i, exp := range u.deps {
			if d.Module == exp.module {
				found[exp.module] = true
				if exp.version != "" {
					log.Assert("version_"+exp.module, d.Version == exp.version, exp.version, d.Version)
				}
				log.Assert("scope_"+exp.module, d.Scope == exp.scope, exp.scope, d.Scope)
				_ = i
			}
		}
		if u.id == "core" {
			if strings.Contains(d.Module, "go-foundry-cli") || strings.Contains(d.Module, "foundry") {
				log.Fail("foundry_runtime_dep", d.Module)
			}
		}
	}
	if u.id == "core" {
		log.Assert("zero_runtime_deps", runtimeDeps == 0, 0, runtimeDeps)
	}
	for _, exp := range u.deps {
		log.Assert("has_"+exp.module, found[exp.module], true, found[exp.module])
	}
}

// TestCatalogSourcesShape consolidates the source-shape assertions from the
// former per-archetype tree tests.
func TestCatalogSourcesShape(t *testing.T) {
	c := mustLoadCatalog(t)
	cases := []struct {
		id             string
		wantStatic     int
		wantTemplate   int
		extraForbidden []string
	}{
		{
			id:           "core",
			wantStatic:   4,
			wantTemplate: 7,
		},
		{
			id:           "cli",
			wantStatic:   1,
			wantTemplate: 8,
			extraForbidden: []string{
				"newGreet",
				"GreetCmd",
			},
		},
		{
			id:           "tui",
			wantStatic:   5,
			wantTemplate: 6,
			extraForbidden: []string{
				"github.com/robertguss/go-foundry-cli",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			log := testutil.New(t)
			m, ok := c.Manifest(tc.id)
			if !ok || m == nil {
				log.Fail("manifest", tc.id+" missing")
				return
			}
			templates, statics := 0, 0
			for _, f := range m.Files {
				srcPath := path.Join(m.UnitDir, f.Source)
				raw, err := c.Read(srcPath)
				if err != nil {
					log.Fail("read", srcPath)
					continue
				}
				log.Assert("non_empty_"+f.Path, len(raw) > 0, true, len(raw) > 0)
				if bytesContainsCR(raw) {
					log.Fail("cr_in_source", srcPath)
				}
				s := string(raw)
				for _, tok := range tc.extraForbidden {
					if strings.Contains(s, tok) {
						log.Fail("forbidden_"+sanitizeAssertName(tok), srcPath)
					}
				}
				switch f.Render {
				case catalog.RenderTemplate:
					if !strings.Contains(s, "[[") {
						log.Fail("template_missing_delims", srcPath)
					}
					templates++
				case catalog.RenderStatic:
					if strings.Contains(s, "[[") || strings.Contains(s, "]]") {
						log.Fail("static_has_template_token", srcPath)
					}
					statics++
				default:
					log.Fail("unknown_render", f.Path+" render="+string(f.Render))
				}
			}
			log.Assert("template_count", templates == tc.wantTemplate, tc.wantTemplate, templates)
			log.Assert("static_count", statics == tc.wantStatic, tc.wantStatic, statics)
			log.Assert("all_files_covered", templates+statics == len(m.Files), len(m.Files), templates+statics)
		})
	}
}

// TestCatalogManifestValidationRoundTrip consolidates the ValidateUnitManifests
// round-trip assertions from the former per-archetype tree tests.
func TestCatalogManifestValidationRoundTrip(t *testing.T) {
	c := mustLoadCatalog(t)
	cases := []struct {
		id        string
		wantFiles int
		wantDeps  int
	}{
		{"core", 11, 1},
		{"cli", 9, 2},
		{"tui", 11, 3},
	}
	ms, err := catalog.ValidateUnitManifests(c.Files(), c.Lock())
	if err != nil {
		t.Fatalf("ValidateUnitManifests: %v", err)
	}
	byID := make(map[string]*catalog.Manifest, len(ms))
	for _, m := range ms {
		byID[m.ID] = m
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			log := testutil.New(t)
			m, ok := byID[tc.id]
			log.Assert("present", ok, true, ok)
			if !ok {
				return
			}
			log.Assert("file_count", len(m.Files) == tc.wantFiles, tc.wantFiles, len(m.Files))
			log.Assert("dep_count", len(m.Dependencies) == tc.wantDeps, tc.wantDeps, len(m.Dependencies))
		})
	}
}

// coreExtraChecks preserves the Core-specific workflow structure, AGENTS.md,
// and docs/commands.md assertions.
func coreExtraChecks(t *testing.T, log *testutil.Logger, c *catalog.Catalog, core *catalog.Manifest, byPath map[string]catalog.FileEntry) {
	t.Helper()
	ciRaw := readSource(t, log, c, core, byPath, ".github/workflows/ci.yml")
	ci := string(ciRaw)
	jobKeys := yamlTopLevelJobKeys(ci)
	log.Step("ci_job_keys", testutil.OutcomeOK, "keys="+strings.Join(jobKeys, ","))
	log.Assert("ci_single_job", len(jobKeys) == 1, 1, len(jobKeys))
	if len(jobKeys) == 1 {
		log.Assert("ci_job_check", jobKeys[0] == "check", "check", jobKeys[0])
	}
	for _, cmd := range []string{
		"gofmt -l .",
		"go mod verify",
		"go test -count=1 ./...",
		"go vet ./...",
		"go tool staticcheck ./...",
	} {
		log.Assert("ci_has_"+sanitizeAssertName(cmd), strings.Contains(ci, cmd), true, strings.Contains(ci, cmd))
	}
	log.Assert("ci_concurrency", strings.Contains(ci, "concurrency:"), true, false)
	log.Assert("ci_cancel", strings.Contains(ci, "cancel-in-progress: true"), true, false)
	log.Assert("ci_ubuntu", strings.Contains(ci, "ubuntu-latest"), true, false)
	checkout, ok := actionByID(c, "checkout")
	log.Assert("lock_checkout", ok, true, ok)
	setupGo, ok := actionByID(c, "setup_go")
	log.Assert("lock_setup_go", ok, true, ok)
	if ok {
		log.Assert("ci_checkout_sha", strings.Contains(ci, checkout.SHA), true, false)
		log.Assert("ci_setup_go_sha", strings.Contains(ci, setupGo.SHA), true, false)
	}

	strictRaw := readSource(t, log, c, core, byPath, ".github/workflows/strict.yml")
	strict := string(strictRaw)
	log.Assert("strict_schedule", strings.Contains(strict, "schedule:"), true, false)
	log.Assert("strict_dispatch", strings.Contains(strict, "workflow_dispatch:"), true, false)
	log.Assert("strict_govulncheck", strings.Contains(strict, "go tool govulncheck ./..."), true, false)
	log.Assert("strict_race", strings.Contains(strict, "CGO_ENABLED"), true, false)
	log.Assert("strict_macos", strings.Contains(strict, "macos-latest"), true, false)
	log.Assert("strict_fuzz", strings.Contains(strict, "Fuzz"), true, false)
	log.Assert("strict_no_pull_request", !strings.Contains(strict, "pull_request:"), true, false)
	if ok {
		log.Assert("strict_checkout_sha", strings.Contains(strict, checkout.SHA), true, false)
		log.Assert("strict_setup_go_sha", strings.Contains(strict, setupGo.SHA), true, false)
	}

	agentsRaw := readSource(t, log, c, core, byPath, "AGENTS.md")
	agents := string(agentsRaw)
	for _, section := range []string{
		"Canonical commands",
		"Architecture map",
		"Change-completion report",
		"gofmt -l .",
		"go test -count=1 ./...",
		"go tool staticcheck ./...",
		"go tool govulncheck ./...",
		"Grok Build",
		"Codex",
		"Cursor",
		"Files",
		"Commands",
		"Results",
		"Risks",
	} {
		log.Assert("agents_"+sanitizeAssertName(section), strings.Contains(agents, section), true, false)
	}
	log.Assert("agents_foundry_version_token", strings.Contains(agents, "[[.FoundryVersion]]"), true, false)
	log.Assert("agents_forbids_claude_authority",
		strings.Contains(agents, "CLAUDE.md") || strings.Contains(agents, "vendor-specific"),
		true, false)

	cmdsRaw := readSource(t, log, c, core, byPath, "docs/commands.md")
	cmds := string(cmdsRaw)
	for _, cmd := range []string{"gofmt -l .", "go test -count=1 ./...", "go vet ./...", "go tool staticcheck ./..."} {
		log.Assert("commands_doc_"+sanitizeAssertName(cmd), strings.Contains(cmds, cmd), true, false)
		log.Assert("ci_cmd_"+sanitizeAssertName(cmd), strings.Contains(ci, cmd), true, false)
	}

	consumed := catalog.LockConsumptionFromManifests(c.Lock(), c.Manifests())
	log.Assert("go_cmp_consumed", consumed["module:go_cmp"] >= 1, 1, consumed["module:go_cmp"])
}

// archetypeExtraChecks asserts the single-primary-binary contract shared by
// CLI and TUI archetypes.
func archetypeExtraChecks(t *testing.T, log *testutil.Logger, c *catalog.Catalog, m *catalog.Manifest, byPath map[string]catalog.FileEntry) {
	t.Helper()
	mainCount := 0
	for _, f := range m.Files {
		if strings.HasPrefix(f.Path, "cmd/") && strings.HasSuffix(f.Path, "/main.go") {
			mainCount++
			log.Assert("main_uses_binary_token", f.Path == "cmd/{{binary}}/main.go", "cmd/{{binary}}/main.go", f.Path)
		}
	}
	log.Assert("single_main", mainCount == 1, 1, mainCount)
}

// tuiExtraChecks preserves the TUI-specific lifecycle and debug-log contract
// assertions.
func tuiExtraChecks(t *testing.T, log *testutil.Logger, c *catalog.Catalog, tui *catalog.Manifest, byPath map[string]catalog.FileEntry) {
	t.Helper()
	archetypeExtraChecks(t, log, c, tui, byPath)
	mainSrc := readSource(t, log, c, tui, byPath, "cmd/{{binary}}/main.go")
	mainBody := string(mainSrc)
	for _, want := range []string{
		"signal.NotifyContext",
		"os.Interrupt",
		"syscall.SIGTERM",
		"--version",
		"--no-color",
		"--debug-log",
		"O_CREATE",
		"O_EXCL",
		"0o600",
		"validateDebugLogBasename",
	} {
		log.Assert("main_"+sanitizeAssertName(want), strings.Contains(mainBody, want), true, false)
	}
	appSrc := readSource(t, log, c, tui, byPath, "internal/tui/app.go")
	appBody := string(appSrc)
	for _, want := range []string{
		"tea.WithContext",
		"tea.WithoutSignalHandler",
		"MapExitCode",
		"ExitSignal",
	} {
		log.Assert("app_"+sanitizeAssertName(want), strings.Contains(appBody, want), true, false)
	}
	uiSrc := readSource(t, log, c, tui, byPath, "docs/ui-architecture.md")
	uiLower := strings.ToLower(string(uiSrc))
	for _, want := range []string{
		"single lifecycle owner",
		"withoutsignalhandler",
		"o_create|o_excl|o_wronly",
		"0600",
		"cooperative",
		"safe basename",
		"effects.go",
		"messages.go",
	} {
		log.Assert("ui_arch_"+sanitizeAssertName(want), strings.Contains(uiLower, strings.ToLower(want)), true, false)
	}
	core, ok := c.Manifest("core")
	if !ok || core == nil {
		log.Fail("core_missing", "core unit missing")
		return
	}
	agentsPath := ""
	for _, f := range core.Files {
		if f.Path == "AGENTS.md" {
			agentsPath = path.Join(core.UnitDir, f.Source)
			break
		}
	}
	if agentsPath == "" {
		log.Fail("agents_not_found", "AGENTS.md missing in core")
		return
	}
	agentsRaw, err := c.Read(agentsPath)
	if err != nil {
		log.Fail("read_agents", err.Error())
		return
	}
	agents := string(agentsRaw)
	log.Assert("agents_tui_lifecycle", strings.Contains(agents, "Single lifecycle owner"), true, false)
	log.Assert("agents_tui_debug_log", strings.Contains(agents, "--debug-log"), true, false)
	log.Assert("agents_tui_ui_arch", strings.Contains(agents, "ui-architecture.md"), true, false)

	consumed := catalog.LockConsumptionFromManifests(c.Lock(), c.Manifests())
	log.Assert("bubbletea_consumed", consumed["module:bubbletea"] >= 1, 1, consumed["module:bubbletea"])
	log.Assert("bubbles_consumed", consumed["module:bubbles"] >= 1, 1, consumed["module:bubbles"])
	log.Assert("lipgloss_consumed", consumed["module:lipgloss"] >= 1, 1, consumed["module:lipgloss"])
}

func readSource(t *testing.T, log *testutil.Logger, c *catalog.Catalog, m *catalog.Manifest, byPath map[string]catalog.FileEntry, p string) []byte {
	t.Helper()
	f, ok := byPath[p]
	if !ok {
		log.Fail("missing_"+sanitizeAssertName(p), "path not in manifest: "+p)
		return nil
	}
	src := path.Join(m.UnitDir, f.Source)
	b, err := c.Read(src)
	if err != nil {
		log.Fail("read_"+sanitizeAssertName(p), err.Error())
	}
	return b
}

func mustLoadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return c
}

func loadLines(t *testing.T, name string) []string {
	t.Helper()
	p := filepath.Join(testdataDir(t), name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

type invRow struct {
	path   string
	render string
	mode   string
	source string
}

func loadInventoryRows(t *testing.T, name string) []invRow {
	t.Helper()
	lines := loadLines(t, name)
	out := make([]invRow, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			t.Fatalf("inventory golden %s: want 4 tab fields, got %d in %q", name, len(parts), line)
		}
		out = append(out, invRow{path: parts[0], render: parts[1], mode: parts[2], source: parts[3]})
	}
	return out
}

func sanitizeAssertName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, s)
	if len(s) > 40 {
		return s[:40]
	}
	return s
}

func bytesContainsCR(b []byte) bool {
	for _, v := range b {
		if v == '\r' {
			return true
		}
	}
	return false
}

func actionByID(c *catalog.Catalog, id string) (catalog.LockAction, bool) {
	lock := c.Lock()
	if lock == nil {
		return catalog.LockAction{}, false
	}
	for _, a := range lock.Actions {
		if a.ID == id {
			return a, true
		}
	}
	return catalog.LockAction{}, false
}

// yamlTopLevelJobKeys returns job IDs under the top-level "jobs:" mapping.
func yamlTopLevelJobKeys(body string) []string {
	lines := strings.Split(body, "\n")
	inJobs := false
	var keys []string
	for _, line := range lines {
		trim := strings.TrimRight(line, " \t")
		if !inJobs {
			if strings.TrimSpace(trim) == "jobs:" {
				inJobs = true
			}
			continue
		}
		if len(trim) > 0 && trim[0] != ' ' && trim[0] != '\t' && !strings.HasPrefix(strings.TrimSpace(trim), "#") {
			break
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			rest := strings.TrimSpace(line)
			if strings.HasPrefix(rest, "#") {
				continue
			}
			if i := strings.Index(rest, ":"); i > 0 {
				key := rest[:i]
				if !strings.Contains(key, " ") && !strings.ContainsAny(key, "[]*?") {
					keys = append(keys, key)
				}
			}
		}
	}
	return keys
}
