package render_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// smokeCLIGomodInput is the representative foundry-smoke-cli typed BOM for
// golden tests (Section 44.2 CLI + Core tools; REQ-002 language/toolchain).
func smokeCLIGomodInput(t *testing.T) render.GomodInput {
	t.Helper()
	lock := mustLoadLock(t)
	cobra, ok := lock.ModuleByID("cobra")
	if !ok {
		t.Fatal("lock missing module:cobra")
	}
	cmp, ok := lock.ModuleByID("go_cmp")
	if !ok {
		t.Fatal("lock missing module:go_cmp")
	}
	ts, ok := lock.ModuleByID("testscript")
	if !ok {
		t.Fatal("lock missing module:testscript")
	}
	sc, ok := toolByID(lock, "staticcheck")
	if !ok {
		t.Fatal("lock missing tool:staticcheck")
	}
	gv, ok := toolByID(lock, "govulncheck")
	if !ok {
		t.Fatal("lock missing tool:govulncheck")
	}
	return render.GomodInput{
		Module:    "github.com/example/foundry-smoke-cli",
		GoVersion: render.CatalogGoVersion,
		Toolchain: toolchainFromLock(lock),
		Requires: []render.GomodRequire{
			{Path: cobra.Path, Version: cobra.Version},
			{Path: cmp.Path, Version: cmp.Version},
			{Path: ts.Path, Version: ts.Version},
		},
		Tools: []render.GomodTool{
			{Path: "honnef.co/go/tools/cmd/staticcheck", Module: sc.Module, Version: sc.Version},
			{Path: "golang.org/x/vuln/cmd/govulncheck", Module: gv.Module, Version: gv.Version},
		},
	}
}

func mustLoadLock(t *testing.T) *catalog.Lock {
	t.Helper()
	c := mustLoadCatalog(t)
	lock := c.Lock()
	if lock == nil {
		t.Fatal("catalog lock is nil")
	}
	return lock
}

func toolByID(lock *catalog.Lock, id string) (catalog.LockTool, bool) {
	for _, t := range lock.Tools {
		if t.ID == id {
			return t, true
		}
	}
	return catalog.LockTool{}, false
}

func toolchainFromLock(lock *catalog.Lock) string {
	if lock.Toolchain.GoTag != "" {
		return lock.Toolchain.GoTag
	}
	return "go" + lock.Toolchain.Go
}

// TestGomodSmokeCLIGolden locks the golden go.mod for smoke-cli fixture inputs.
func TestGomodSmokeCLIGolden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("arrange")
	in := smokeCLIGomodInput(t)
	log.Inputs(map[string]string{
		"module":    in.Module,
		"go":        in.GoVersion,
		"toolchain": in.Toolchain,
		"requires":  fmt.Sprintf("%d", len(in.Requires)),
		"tools":     fmt.Sprintf("%d", len(in.Tools)),
		"mechanism": "gomod",
	})
	log.Fixture("fixture", "foundry-smoke-cli")
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	buf := render.NewMemoryWriter()
	inv, err := render.RenderGomod(in, buf)
	if err != nil {
		log.Fail("render_gomod", err.Error())
	}
	content, ok := inv.Content(render.GomodPath)
	log.Assert("content_present", ok, true, ok)
	log.Step("generate", testutil.OutcomeOK, fmt.Sprintf(
		"module=%s require_count=%d tool_count=%d digest=%s",
		in.Module, in.RequireCount(), in.ToolCount(), inv.Entries()[0].ContentDigest,
	))
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	e, found := inv.EntryByPath(render.GomodPath)
	log.Assert("entry_found", found, true, found)
	log.Assert("path", e.Path == render.GomodPath, render.GomodPath, e.Path)
	log.Assert("mode", e.Mode == render.GomodMode, render.GomodMode, e.Mode)
	log.Assert("mechanism", e.Mechanism == render.MechanismGomod, render.MechanismGomod, e.Mechanism)
	log.Assert("source", e.Source == render.GomodSourceTyped, render.GomodSourceTyped, e.Source)
	log.Assert("owner", e.Owner == render.GomodOwnerDefault, render.GomodOwnerDefault, e.Owner)
	log.Assert("source_digest_empty", e.SourceDigest == "", "", string(e.SourceDigest))
	log.Assert("content_digest", e.ContentDigest == digestOf(content), digestOf(content), e.ContentDigest)

	wContent, wMode, wok := buf.Get(render.GomodPath)
	log.Assert("writer_present", wok, true, wok)
	log.Assert("writer_mode", wMode == render.GomodMode, render.GomodMode, wMode)
	log.Assert("writer_bytes", bytes.Equal(content, wContent), true, bytes.Equal(content, wContent))

	golden := testutil.GoldenPath(filepath.Join(packageDir(t), "testdata"), "smoke_cli_gomod")
	testutil.CompareGolden(t, golden, content)
	log.Step("golden", testutil.OutcomeOK, "path=testdata/smoke_cli_gomod.golden")
	log.NotePath(render.GomodPath)
	log.NoteID(string(e.ContentDigest))
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestGomodPinsMatchLock ensures every emitted require version matches the
// catalog lock (no float versions; pin set from versions.toml).
func TestGomodPinsMatchLock(t *testing.T) {
	log := testutil.New(t)
	log.Phase("pins")
	lock := mustLoadLock(t)
	in := smokeCLIGomodInput(t)

	content, err := render.GenerateGoMod(in)
	if err != nil {
		log.Fail("generate", err.Error())
	}
	parsed, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		log.Fail("parse", err.Error())
	}

	// Index lock modules by path.
	byPath := make(map[string]string, len(lock.Modules)+len(lock.Tools))
	for _, m := range lock.Modules {
		byPath[m.Path] = m.Version
	}
	for _, tl := range lock.Tools {
		if tl.Module != "" {
			byPath[tl.Module] = tl.Version
		}
	}

	log.Assert("has_requires", len(parsed.Require) > 0, true, len(parsed.Require) > 0)
	for _, r := range parsed.Require {
		want, ok := byPath[r.Mod.Path]
		log.Assert("lock_has_"+r.Mod.Path, ok, true, ok)
		log.Assert("pin_"+r.Mod.Path, r.Mod.Version == want, want, r.Mod.Version)
		log.Assert("exact_"+r.Mod.Path, strings.HasPrefix(r.Mod.Version, "v"), true, r.Mod.Version)
		log.Step("pin", testutil.OutcomeOK, fmt.Sprintf(
			"module=%s version=%s lock_match=ok", r.Mod.Path, r.Mod.Version,
		))
	}

	// Language / toolchain from lock + REQ-002.
	log.Assert("go_directive", parsed.Go != nil && parsed.Go.Version == render.CatalogGoVersion,
		render.CatalogGoVersion, goVersionOf(parsed))
	wantTC := toolchainFromLock(lock)
	log.Assert("toolchain", parsed.Toolchain != nil && parsed.Toolchain.Name == wantTC,
		wantTC, toolchainOf(parsed))
	log.Assert("lock_go", lock.Toolchain.Go == "1.26.5", "1.26.5", lock.Toolchain.Go)
	log.PhaseEnd("pins", testutil.OutcomeOK)
}

// TestGomodForbiddenDirectives ensures replace/exclude/retract never appear.
func TestGomodForbiddenDirectives(t *testing.T) {
	log := testutil.New(t)
	log.Phase("forbidden")
	in := smokeCLIGomodInput(t)
	content, err := render.GenerateGoMod(in)
	if err != nil {
		log.Fail("generate", err.Error())
	}
	parsed, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		log.Fail("parse", err.Error())
	}
	log.Assert("replace_zero", len(parsed.Replace) == 0, 0, len(parsed.Replace))
	log.Assert("exclude_zero", len(parsed.Exclude) == 0, 0, len(parsed.Exclude))
	log.Assert("retract_zero", len(parsed.Retract) == 0, 0, len(parsed.Retract))

	text := string(content)
	for _, bad := range []string{"\nreplace ", "\nexclude ", "\nretract "} {
		log.Assert("no_line_"+strings.TrimSpace(bad), !strings.Contains(text, bad), false, strings.Contains(text, bad))
	}
	// Also bare start-of-file / after newline forms without trailing space.
	for _, verb := range []string{"replace", "exclude", "retract"} {
		for _, line := range strings.Split(text, "\n") {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, verb+" ") || trim == verb || strings.HasPrefix(trim, verb+"(") {
				log.Fail("forbidden_verb", "found line: "+line)
			}
		}
	}
	log.Step("forbidden", testutil.OutcomeOK, "replace=0 exclude=0 retract=0")
	log.PhaseEnd("forbidden", testutil.OutcomeOK)
}

// TestGomodInvalidModulePath rejects bad module paths before emit.
func TestGomodInvalidModulePath(t *testing.T) {
	log := testutil.New(t)
	log.Phase("invalid_module")
	cases := []struct {
		name   string
		module string
	}{
		{name: "empty", module: ""},
		{name: "space", module: "github.com/acme/bad path"},
		{name: "leading_slash", module: "/github.com/acme/x"},
		{name: "dotdot", module: "github.com/../x"},
		{name: "uppercase_host_segment_issue", module: "GitHub.com/Acme/X"}, // CheckPath rejects capitals in path
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase("reject")
			sub.Inputs(map[string]string{"module": tc.module})
			inv, err := render.RenderGomod(render.GomodInput{
				Module:    tc.module,
				GoVersion: render.CatalogGoVersion,
				Toolchain: "go1.26.5",
			}, nil)
			sub.Assert("nil_inventory", inv == nil, true, inv == nil)
			if err == nil {
				sub.Fail("expected_error", "want render.failed for invalid module")
			}
			fe := asFoundry(t, err)
			sub.Assert("id", fe.ID() == diagnostic.IDRenderFailed,
				diagnostic.IDRenderFailed, fe.ID())
			sub.Step("reject", testutil.OutcomeOK, fmt.Sprintf(
				"module=%q id=%s outcome=refused", tc.module, fe.ID(),
			))
			sub.PhaseEnd("reject", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("invalid_module", testutil.OutcomeOK)
}

// TestGomodRejectFloatVersions rejects non-exact version pins.
func TestGomodRejectFloatVersions(t *testing.T) {
	log := testutil.New(t)
	log.Phase("float_versions")
	cases := []struct {
		name    string
		version string
	}{
		{name: "latest", version: "latest"},
		{name: "main", version: "main"},
		{name: "no_v_prefix", version: "1.10.2"},
		{name: "range_ge", version: ">=v1.0.0"},
		{name: "tilde", version: "~v1.2.0"},
		{name: "empty", version: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase("reject")
			sub.Inputs(map[string]string{"version": tc.version})
			_, err := render.GenerateGoMod(render.GomodInput{
				Module:    "github.com/example/foundry-smoke-cli",
				GoVersion: render.CatalogGoVersion,
				Toolchain: "go1.26.5",
				Requires: []render.GomodRequire{
					{Path: "github.com/spf13/cobra", Version: tc.version},
				},
			})
			if err == nil {
				sub.Fail("expected_error", "want render.failed for float version")
			}
			fe := asFoundry(t, err)
			sub.Assert("id", fe.ID() == diagnostic.IDRenderFailed,
				diagnostic.IDRenderFailed, fe.ID())
			sub.Step("reject", testutil.OutcomeOK, fmt.Sprintf(
				"version=%q id=%s outcome=refused", tc.version, fe.ID(),
			))
			sub.PhaseEnd("reject", testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("float_versions", testutil.OutcomeOK)
}

// TestGomodVersionConflict fails closed on same-module different versions.
func TestGomodVersionConflict(t *testing.T) {
	log := testutil.New(t)
	log.Phase("conflict")
	_, err := render.GenerateGoMod(render.GomodInput{
		Module:    "github.com/example/foundry-smoke-cli",
		GoVersion: render.CatalogGoVersion,
		Toolchain: "go1.26.5",
		Requires: []render.GomodRequire{
			{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
			{Path: "github.com/spf13/cobra", Version: "v1.9.0"},
		},
	})
	if err == nil {
		log.Fail("expected_error", "want version conflict")
	}
	fe := asFoundry(t, err)
	log.Assert("id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
	log.Assert("mentions_conflict", strings.Contains(fe.Error(), "conflict") || strings.Contains(fe.Error(), "version"),
		true, fe.Error())
	log.Step("conflict", testutil.OutcomeOK, "id="+string(fe.ID()))
	log.PhaseEnd("conflict", testutil.OutcomeOK)
}

// TestGomodDeterminism ensures identical inputs produce identical digests and
// bytes under repeated invocation (pair with go test -count=2).
func TestGomodDeterminism(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")
	in := smokeCLIGomodInput(t)

	a, err := render.RenderGomod(in, render.NewMemoryWriter())
	if err != nil {
		log.Fail("render_a", err.Error())
	}
	b, err := render.RenderGomod(in, render.NewMemoryWriter())
	if err != nil {
		log.Fail("render_b", err.Error())
	}
	log.Assert("inventory_equal", a.Equal(b), true, a.Equal(b))
	ca, _ := a.Content(render.GomodPath)
	cb, _ := b.Content(render.GomodPath)
	log.Assert("bytes_equal", bytes.Equal(ca, cb), true, bytes.Equal(ca, cb))
	log.Assert("digest_equal", a.Entries()[0].ContentDigest == b.Entries()[0].ContentDigest,
		a.Entries()[0].ContentDigest, b.Entries()[0].ContentDigest)

	// Shuffled require order must still yield identical output.
	shuffled := in
	shuffled.Requires = []render.GomodRequire{
		in.Requires[2], in.Requires[0], in.Requires[1],
	}
	shuffled.Tools = []render.GomodTool{in.Tools[1], in.Tools[0]}
	c, err := render.GenerateGoMod(shuffled)
	if err != nil {
		log.Fail("render_shuffled", err.Error())
	}
	log.Assert("order_independent", bytes.Equal(ca, c), true, bytes.Equal(ca, c))
	log.Step("digest", testutil.OutcomeOK, fmt.Sprintf(
		"module=%s require_count=%d digest=%s",
		in.Module, in.RequireCount(), a.Entries()[0].ContentDigest,
	))
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

// TestGomodStepLogger records module path, require count, and content digest.
func TestGomodStepLogger(t *testing.T) {
	log := testutil.New(t)
	log.Phase("step_logger")
	in := smokeCLIGomodInput(t)
	inv, err := render.RenderGomod(in, nil)
	if err != nil {
		log.Fail("render", err.Error())
	}
	e := inv.Entries()[0]
	detail := fmt.Sprintf(
		"module=%s require_count=%d tool_count=%d content_digest=%s outcome=ok",
		in.Module, in.RequireCount(), in.ToolCount(), e.ContentDigest,
	)
	log.Step("gomod", testutil.OutcomeOK, detail)
	log.Assert("require_count", in.RequireCount() == 5, 5, in.RequireCount()) // 3 requires + 2 tool modules
	log.Assert("tool_count", in.ToolCount() == 2, 2, in.ToolCount())
	log.Assert("digest_nonempty", e.ContentDigest != "", true, e.ContentDigest != "")
	log.NotePath(in.Module)
	log.NoteID(string(e.ContentDigest))
	log.PhaseEnd("step_logger", testutil.OutcomeOK)
}

// TestGomodParseableAlways re-parses generated content with modfile.Parse.
func TestGomodParseableAlways(t *testing.T) {
	log := testutil.New(t)
	log.Phase("parseable")
	in := smokeCLIGomodInput(t)
	content, err := render.GenerateGoMod(in)
	if err != nil {
		log.Fail("generate", err.Error())
	}
	parsed, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		log.Fail("parse", err.Error())
	}
	log.Assert("module", parsed.Module != nil && parsed.Module.Mod.Path == in.Module,
		in.Module, moduleOf(parsed))
	log.Assert("require_n", len(parsed.Require) == in.RequireCount(), in.RequireCount(), len(parsed.Require))
	log.Assert("tool_n", len(parsed.Tool) == in.ToolCount(), in.ToolCount(), len(parsed.Tool))
	log.Step("parse", testutil.OutcomeOK, fmt.Sprintf(
		"module=%s requires=%d tools=%d", in.Module, len(parsed.Require), len(parsed.Tool),
	))
	log.PhaseEnd("parseable", testutil.OutcomeOK)
}

// TestGomodMinimalModuleOnly allows a module with zero requires/tools.
func TestGomodMinimalModuleOnly(t *testing.T) {
	log := testutil.New(t)
	log.Phase("minimal")
	content, err := render.GenerateGoMod(render.GomodInput{
		Module:    "example.com/minimal",
		GoVersion: render.CatalogGoVersion,
		Toolchain: "go1.26.5",
	})
	if err != nil {
		log.Fail("generate", err.Error())
	}
	parsed, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		log.Fail("parse", err.Error())
	}
	log.Assert("module", parsed.Module.Mod.Path == "example.com/minimal",
		"example.com/minimal", parsed.Module.Mod.Path)
	log.Assert("no_requires", len(parsed.Require) == 0, 0, len(parsed.Require))
	log.Assert("no_tools", len(parsed.Tool) == 0, 0, len(parsed.Tool))
	log.Assert("has_go", parsed.Go != nil, true, parsed.Go != nil)
	log.Assert("has_toolchain", parsed.Toolchain != nil, true, parsed.Toolchain != nil)
	log.Step("minimal", testutil.OutcomeOK, "module=example.com/minimal requires=0")
	log.PhaseEnd("minimal", testutil.OutcomeOK)
}

// TestGomodMissingGoOrToolchain rejects incomplete pins.
func TestGomodMissingGoOrToolchain(t *testing.T) {
	log := testutil.New(t)
	log.Phase("missing_pins")
	_, err := render.GenerateGoMod(render.GomodInput{
		Module:    "github.com/example/x",
		Toolchain: "go1.26.5",
	})
	if err == nil {
		log.Fail("expected_go_error", "want missing go version")
	}
	log.Assert("go_id", asFoundry(t, err).ID() == diagnostic.IDRenderFailed,
		diagnostic.IDRenderFailed, asFoundry(t, err).ID())

	_, err = render.GenerateGoMod(render.GomodInput{
		Module:    "github.com/example/x",
		GoVersion: render.CatalogGoVersion,
	})
	if err == nil {
		log.Fail("expected_toolchain_error", "want missing toolchain")
	}
	log.Assert("tc_id", asFoundry(t, err).ID() == diagnostic.IDRenderFailed,
		diagnostic.IDRenderFailed, asFoundry(t, err).ID())
	log.PhaseEnd("missing_pins", testutil.OutcomeOK)
}

// TestGomodNilWriterInventoryOnly still returns pure content without a writer.
func TestGomodNilWriterInventoryOnly(t *testing.T) {
	log := testutil.New(t)
	log.Phase("nil_writer")
	in := smokeCLIGomodInput(t)
	inv, err := render.RenderGomod(in, nil)
	if err != nil {
		log.Fail("render", err.Error())
	}
	out, ok := inv.Content(render.GomodPath)
	log.Assert("present", ok, true, ok)
	log.Assert("nonempty", len(out) > 0, true, len(out) > 0)
	log.Step("file", testutil.OutcomeOK, fmt.Sprintf(
		"path=go.mod source=typed mode=0644 digest=%s outcome=ok",
		inv.Entries()[0].ContentDigest,
	))
	log.PhaseEnd("nil_writer", testutil.OutcomeOK)
}

// TestGomodIdenticalPinsDedup merges identical module pins from require+tool.
func TestGomodIdenticalPinsDedup(t *testing.T) {
	log := testutil.New(t)
	log.Phase("dedup")
	content, err := render.GenerateGoMod(render.GomodInput{
		Module:    "github.com/example/x",
		GoVersion: render.CatalogGoVersion,
		Toolchain: "go1.26.5",
		Requires: []render.GomodRequire{
			{Path: "honnef.co/go/tools", Version: "v0.7.0"},
		},
		Tools: []render.GomodTool{
			{Path: "honnef.co/go/tools/cmd/staticcheck", Module: "honnef.co/go/tools", Version: "v0.7.0"},
		},
	})
	if err != nil {
		log.Fail("generate", err.Error())
	}
	parsed, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		log.Fail("parse", err.Error())
	}
	n := 0
	for _, r := range parsed.Require {
		if r.Mod.Path == "honnef.co/go/tools" {
			n++
		}
	}
	log.Assert("single_require", n == 1, 1, n)
	log.Assert("one_tool", len(parsed.Tool) == 1, 1, len(parsed.Tool))
	log.Step("dedup", testutil.OutcomeOK, "module=honnef.co/go/tools n=1")
	log.PhaseEnd("dedup", testutil.OutcomeOK)
}

func goVersionOf(f *modfile.File) string {
	if f == nil || f.Go == nil {
		return ""
	}
	return f.Go.Version
}

func toolchainOf(f *modfile.File) string {
	if f == nil || f.Toolchain == nil {
		return ""
	}
	return f.Toolchain.Name
}

func moduleOf(f *modfile.File) string {
	if f == nil || f.Module == nil {
		return ""
	}
	return f.Module.Mod.Path
}
