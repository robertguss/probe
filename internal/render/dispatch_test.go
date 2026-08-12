package render_test

import (
	"bytes"
	"fmt"
	"go/format"
	"path"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestMechanismEnumExhaustiveness locks the three product mechanisms (REQ-096).
// Strategy enum tests: Valid, Mechanisms closed set, rejected deleted emitters.
func TestMechanismEnumExhaustiveness(t *testing.T) {
	log := testutil.New(t)
	log.Phase("mechanism_enum")

	all := render.Mechanisms()
	log.Assert("count", len(all) == 3, 3, len(all))
	log.Assert("order_0", all[0] == render.MechanismStatic, render.MechanismStatic, all[0])
	log.Assert("order_1", all[1] == render.MechanismTemplate, render.MechanismTemplate, all[1])
	log.Assert("order_2", all[2] == render.MechanismGomod, render.MechanismGomod, all[2])

	seen := make(map[render.Mechanism]struct{}, len(all))
	for _, m := range all {
		log.Assert("valid_"+string(m), m.Valid(), true, m.Valid())
		log.Assert("string_"+string(m), m.String() == string(m), string(m), m.String())
		if _, dup := seen[m]; dup {
			log.Fail("duplicate_mechanism", string(m))
		}
		seen[m] = struct{}{}
	}

	// Deleted Stage-4 / FND-015 strategies must never be valid mechanisms.
	banned := []string{
		"token", "token-subst", "@{}", "yaml", "goreleaser",
		"gitignore", "dependabot", "workflow", "compose", "structured",
		"", "STATIC", "Template", "GoMod",
	}
	for _, b := range banned {
		m := render.Mechanism(b)
		log.Assert("banned_"+b, !m.Valid(), false, m.Valid())
	}

	// Exhaustive switch coverage: every Mechanisms() entry has a dispatch arm.
	for _, m := range all {
		switch m {
		case render.MechanismStatic, render.MechanismTemplate, render.MechanismGomod:
			log.Step("switch_arm", testutil.OutcomeOK, "mechanism="+string(m))
		default:
			log.Fail("missing_switch_arm", string(m))
		}
	}

	log.PhaseEnd("mechanism_enum", testutil.OutcomeOK)
}

// TestDispatchAllThreeMechanisms exercises pure multi-mechanism inventory
// (root acceptance: three mechanisms only; unit goldens per mechanism; pure no FS).
func TestDispatchAllThreeMechanisms(t *testing.T) {
	log := testutil.New(t)
	log.Phase("dispatch_three")

	cat := mapCatalog{
		"core/files/LICENSE": []byte("MIT License\n"),
		"core/files/README.md.tmpl": []byte(
			"# [[.Name]]\n\n[[.Description]]\n",
		),
	}
	data := minimalCLITemplateData()
	gomodIn := smokeCLIGomodInput(t)

	jobs := []render.Job{
		{
			Mechanism: render.MechanismStatic,
			Static: &render.StaticJob{
				Path:   "LICENSE",
				Mode:   "0644",
				Source: "core/files/LICENSE",
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
				Data:   data,
			},
		},
		{
			Mechanism: render.MechanismGomod,
			Gomod:     &gomodIn,
		},
	}

	buf := render.NewMemoryWriter()
	inv, err := render.RenderAll(cat, jobs, buf)
	if err != nil {
		log.Fail("render_all", err.Error())
	}

	log.Assert("len", inv.Len() == 3, 3, inv.Len())
	log.Assert("writer_len", buf.Len() == 3, 3, buf.Len())

	// Sorted inventory paths.
	paths := inv.Paths()
	log.Assert("sorted_0", paths[0] == "LICENSE", "LICENSE", paths[0])
	log.Assert("sorted_1", paths[1] == "README.md", "README.md", paths[1])
	log.Assert("sorted_2", paths[2] == "go.mod", "go.mod", paths[2])

	// Mechanism tags per path.
	wantMech := map[string]render.Mechanism{
		"LICENSE":   render.MechanismStatic,
		"README.md": render.MechanismTemplate,
		"go.mod":    render.MechanismGomod,
	}
	for p, want := range wantMech {
		e, ok := inv.EntryByPath(p)
		if !ok {
			log.Fail("missing_entry", p)
		}
		log.Assert("mech_"+p, e.Mechanism == want, want, e.Mechanism)
		log.Step("entry", testutil.OutcomeOK, fmt.Sprintf(
			"path=%s mechanism=%s source=%s content_digest=%s",
			e.Path, e.Mechanism, e.Source, e.ContentDigest,
		))
	}

	// Static: source digest == content digest; byte-identical.
	staticEnt, _ := inv.EntryByPath("LICENSE")
	log.Assert("static_digest_eq", staticEnt.SourceDigest == staticEnt.ContentDigest,
		staticEnt.SourceDigest, staticEnt.ContentDigest)
	staticBytes, ok := inv.Content("LICENSE")
	if !ok {
		log.Fail("static_content", "missing")
	}
	log.Assert("static_bytes", string(staticBytes) == "MIT License\n", "MIT License\n", string(staticBytes))

	// Template golden (minimal README shape).
	readme, ok := inv.Content("README.md")
	if !ok {
		log.Fail("readme_content", "missing")
	}
	if !strings.Contains(string(readme), "# minimal-cli") {
		log.Fail("readme_name", string(readme))
	}
	log.Step("template_content", testutil.OutcomeOK, "digest="+string(inv.Entries()[1].ContentDigest))

	// Gomod golden path (smoke-cli fixture).
	gomodBytes, ok := inv.Content("go.mod")
	if !ok {
		log.Fail("gomod_content", "missing")
	}
	testutil.CompareGolden(t, filepathJoin("testdata", "smoke_cli_gomod.golden"), gomodBytes)
	if strings.Contains(string(gomodBytes), "replace ") ||
		strings.Contains(string(gomodBytes), "exclude ") ||
		strings.Contains(string(gomodBytes), "retract ") {
		log.Fail("gomod_forbidden", "replace/exclude/retract present")
	}
	log.Step("gomod_content", testutil.OutcomeOK, "digest="+string(staticEnt.ContentDigest))

	// Writer agrees with inventory.
	for _, p := range paths {
		want, _ := inv.Content(p)
		got, mode, ok := buf.Get(p)
		if !ok {
			log.Fail("writer_missing", p)
		}
		e, _ := inv.EntryByPath(p)
		log.Assert("writer_mode_"+p, mode == e.Mode, e.Mode, mode)
		log.Assert("writer_bytes_"+p, bytes.Equal(want, got), true, bytes.Equal(want, got))
	}

	log.PhaseEnd("dispatch_three", testutil.OutcomeOK)
}

// TestDispatchUnknownMechanismFailClosed rejects non-product mechanisms.
func TestDispatchUnknownMechanismFailClosed(t *testing.T) {
	log := testutil.New(t)
	log.Phase("unknown_mechanism")

	for _, mech := range []render.Mechanism{"token", "yaml", "workflow", "@@"} {
		inv, err := render.Render(nil, render.Job{Mechanism: mech}, nil)
		if inv != nil {
			log.Fail("inv_not_nil", string(mech))
		}
		fe := asFoundry(t, err)
		log.Assert("id_"+string(mech), fe.ID() == diagnostic.IDRenderFailed,
			diagnostic.IDRenderFailed, fe.ID())
		log.Step("reject", testutil.OutcomeOK, "mechanism="+string(mech)+" id="+string(fe.ID()))
	}

	log.PhaseEnd("unknown_mechanism", testutil.OutcomeOK)
}

// TestDispatchPayloadMismatch fails when Job payload does not match Mechanism.
func TestDispatchPayloadMismatch(t *testing.T) {
	log := testutil.New(t)
	log.Phase("payload_mismatch")

	cases := []struct {
		name string
		job  render.Job
	}{
		{name: "static_nil", job: render.Job{Mechanism: render.MechanismStatic}},
		{name: "template_nil", job: render.Job{Mechanism: render.MechanismTemplate}},
		{name: "gomod_nil", job: render.Job{Mechanism: render.MechanismGomod}},
		{
			name: "static_extra_template",
			job: render.Job{
				Mechanism: render.MechanismStatic,
				Static:    &render.StaticJob{Path: "a", Mode: "0644", Source: "s"},
				Template:  &render.TemplateJob{Path: "b", Mode: "0644", Source: "t"},
			},
		},
		{
			name: "gomod_extra_static",
			job: render.Job{
				Mechanism: render.MechanismGomod,
				Gomod:     &render.GomodInput{Module: "m.example/x", GoVersion: "1.26.0", Toolchain: "go1.26.5"},
				Static:    &render.StaticJob{Path: "a", Mode: "0644", Source: "s"},
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			_, err := render.Render(fixtureCatalog(), tc.job, nil)
			fe := asFoundry(t, err)
			sub.Assert("id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
			sub.Step("reject", testutil.OutcomeOK, "case="+tc.name)
		})
	}

	log.PhaseEnd("payload_mismatch", testutil.OutcomeOK)
}

// TestDispatchDuplicatePathFailClosed rejects colliding outputs across mechanisms.
func TestDispatchDuplicatePathFailClosed(t *testing.T) {
	log := testutil.New(t)
	log.Phase("dup_path")

	cat := mapCatalog{"a": []byte("one\n"), "b": []byte("two\n")}
	jobs := []render.Job{
		{Mechanism: render.MechanismStatic, Static: &render.StaticJob{Path: "out", Mode: "0644", Source: "a"}},
		{Mechanism: render.MechanismStatic, Static: &render.StaticJob{Path: "out", Mode: "0644", Source: "b"}},
	}
	_, err := render.RenderAll(cat, jobs, nil)
	fe := asFoundry(t, err)
	log.Assert("id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
	log.Step("dup", testutil.OutcomeOK, "id="+string(fe.ID()))
	log.PhaseEnd("dup_path", testutil.OutcomeOK)
}

// TestMergeInventories combines pure inventories and refuses collisions.
func TestMergeInventories(t *testing.T) {
	log := testutil.New(t)
	log.Phase("merge")

	cat := fixtureCatalog()
	staticInv, err := render.RenderStatic(cat, render.StaticJob{
		Path: "LICENSE", Mode: "0644", Source: "core/files/LICENSE", Owner: "core",
	}, nil)
	if err != nil {
		log.Fail("static", err.Error())
	}
	tplInv, err := render.RenderTemplate(templateFixtureCatalog(), render.TemplateJob{
		Path: "README.md", Mode: "0644", Source: "core/files/README.md.tmpl",
		Owner: "core", Data: minimalCLITemplateData(),
	}, nil)
	if err != nil {
		log.Fail("template", err.Error())
	}
	in := smokeCLIGomodInput(t)
	gomodInv, err := render.RenderGomod(in, nil)
	if err != nil {
		log.Fail("gomod", err.Error())
	}

	merged, err := render.Merge(staticInv, tplInv, gomodInv)
	if err != nil {
		log.Fail("merge", err.Error())
	}
	log.Assert("len", merged.Len() == 3, 3, merged.Len())

	// Collision fails closed.
	_, err = render.Merge(staticInv, staticInv)
	fe := asFoundry(t, err)
	log.Assert("collision_id", fe.ID() == diagnostic.IDRenderFailed,
		diagnostic.IDRenderFailed, fe.ID())

	// Nil inventories ignored.
	again, err := render.Merge(nil, merged, nil)
	if err != nil {
		log.Fail("merge_nil", err.Error())
	}
	log.Assert("equal_after_nil", merged.Equal(again), true, merged.Equal(again))

	log.PhaseEnd("merge", testutil.OutcomeOK)
}

// TestDispatchDeterminism ensures RenderAll is stable under -count=2 inputs.
func TestDispatchDeterminism(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")

	cat := mapCatalog{
		"core/files/LICENSE":        []byte("MIT\n"),
		"core/files/README.md.tmpl": []byte("Name=[[.Name]]\n"),
	}
	data := minimalCLITemplateData()
	in := smokeCLIGomodInput(t)
	jobs := []render.Job{
		{Mechanism: render.MechanismStatic, Static: &render.StaticJob{Path: "LICENSE", Mode: "0644", Source: "core/files/LICENSE"}},
		{Mechanism: render.MechanismTemplate, Template: &render.TemplateJob{Path: "README.md", Mode: "0644", Source: "core/files/README.md.tmpl", Data: data}},
		{Mechanism: render.MechanismGomod, Gomod: &in},
	}

	a, err := render.RenderAll(cat, jobs, render.NewMemoryWriter())
	if err != nil {
		log.Fail("a", err.Error())
	}
	b, err := render.RenderAll(cat, jobs, render.NewMemoryWriter())
	if err != nil {
		log.Fail("b", err.Error())
	}
	log.Assert("equal", a.Equal(b), true, a.Equal(b))
	for i, e := range a.Entries() {
		be := b.Entries()[i]
		log.Assert("digest_"+e.Path, e.ContentDigest == be.ContentDigest, e.ContentDigest, be.ContentDigest)
	}
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

// TestHostileTemplateCatalogValidation validates real catalog templates and a
// hostile catalog of rejected shapes (REQ-097 / description acceptance).
func TestHostileTemplateCatalogValidation(t *testing.T) {
	log := testutil.New(t)
	log.Phase("hostile_catalog")

	// --- Product catalog templates must validate and render ---
	c := mustLoadCatalog(t)
	var templateSources int
	for _, m := range c.Manifests() {
		for _, f := range m.Files {
			if f.Render != catalog.RenderTemplate {
				continue
			}
			srcPath := f.Source
			if m.UnitDir != "" {
				srcPath = path.Join(m.UnitDir, f.Source)
			}
			raw, err := c.Read(srcPath)
			if err != nil {
				log.Fail("read_"+srcPath, err.Error())
			}
			if err := render.ValidateTemplateSource(srcPath, raw); err != nil {
				log.Fail("validate_"+srcPath, err.Error())
			}
			// Execute with representative data; expand {{binary}} path tokens for .go outPath.
			outPath := f.Path
			outPath = strings.ReplaceAll(outPath, "{{binary}}", "minimal-cli")
			outPath = strings.ReplaceAll(outPath, "[[.Binary]]", "minimal-cli")
			content, err := render.ExecuteTemplate(srcPath, raw, minimalCLITemplateData(), outPath)
			if err != nil {
				log.Fail("execute_"+srcPath, err.Error())
			}
			templateSources++
			log.Step("catalog_template", testutil.OutcomeOK, fmt.Sprintf(
				"source=%s out=%s bytes=%d digest=%s",
				srcPath, outPath, len(content), render.ContentDigest(content),
			))
		}
	}
	log.Assert("has_templates", templateSources >= 2, true, templateSources >= 2)

	// --- Hostile catalog: each must fail ValidateTemplateSource or Execute ---
	hostile := []struct {
		name   string
		source string
		// validateOnly: fail at ValidateTemplateSource; else may need execute
		stage string // "validate" or "execute"
	}{
		{name: "partial_include", source: `[[template "other"]]`, stage: "validate"},
		{name: "nested_define", source: `[[define "x"]]y[[end]]`, stage: "validate"},
		{name: "call_func", source: `[[call .Name]]`, stage: "validate"},
		{name: "exec_func", source: `[[exec "rm" "-rf" "/"]]`, stage: "validate"},
		{name: "env_func", source: `[[env "HOME"]]`, stage: "validate"},
		{name: "readFile_func", source: `[[readFile "/etc/passwd"]]`, stage: "validate"},
		{name: "now_func", source: `[[now]]`, stage: "validate"},
		{name: "rand_func", source: `[[rand]]`, stage: "validate"},
		{name: "legacy_token_delims", source: `Hello @{.Name}`, stage: "execute"}, // parses as text; unknown @{ not action — execute OK but we assert no expansion of @{
		{name: "unknown_field", source: `[[.NotAField]]`, stage: "execute"},
		{name: "wrong_delims_mustache", source: `{{.Name}}`, stage: "execute"}, // literal text under [[ ]] delims
	}

	for _, h := range hostile {
		h := h
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			name := "hostile/" + h.name
			switch h.stage {
			case "validate":
				err := render.ValidateTemplateSource(name, []byte(h.source))
				if err == nil {
					sub.Fail("expected_validate_error", h.name)
				}
				fe := asFoundry(t, err)
				sub.Assert("id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
				sub.Step("hostile_validate", testutil.OutcomeOK, "name="+h.name+" id="+string(fe.ID()))
			case "execute":
				// Validate may pass for pure text; execute must not expand banned forms
				// or must fail on unknown fields.
				if h.name == "unknown_field" {
					_, err := render.ExecuteTemplate(name, []byte(h.source), minimalCLITemplateData(), "out.txt")
					if err == nil {
						sub.Fail("expected_execute_error", h.name)
					}
					fe := asFoundry(t, err)
					sub.Assert("id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
					sub.Step("hostile_execute", testutil.OutcomeOK, "name="+h.name)
					return
				}
				// Legacy @{...} and {{...}} must not be interpreted as template actions
				// under product delimiters — they remain literal text.
				out, err := render.ExecuteTemplate(name, []byte(h.source), minimalCLITemplateData(), "out.txt")
				if err != nil {
					sub.Fail("unexpected_err", err.Error())
				}
				s := string(out)
				if h.name == "legacy_token_delims" {
					if !strings.Contains(s, "@{.Name}") {
						sub.Fail("token_expanded", s)
					}
					if strings.Contains(s, "minimal-cli") {
						sub.Fail("token_substituted", s)
					}
				}
				if h.name == "wrong_delims_mustache" {
					if !strings.Contains(s, "{{.Name}}") {
						sub.Fail("mustache_expanded", s)
					}
				}
				sub.Step("hostile_literal", testutil.OutcomeOK, "name="+h.name)
			}
		})
	}

	log.PhaseEnd("hostile_catalog", testutil.OutcomeOK)
}

// TestGoFormatIdempotence asserts Section 26.3: go/format twice is stable.
func TestGoFormatIdempotence(t *testing.T) {
	log := testutil.New(t)
	log.Phase("gofmt_idempotent")

	cat := mustLoadCatalog(t)
	src := "archetypes/cli/files/main.go.tmpl"
	raw, err := cat.Read(src)
	if err != nil {
		log.Fail("read", err.Error())
	}
	outPath := "cmd/minimal-cli/main.go"
	once, err := render.ExecuteTemplate(src, raw, minimalCLITemplateData(), outPath)
	if err != nil {
		log.Fail("execute", err.Error())
	}
	// First format already applied by ExecuteTemplate; re-format must be equal.
	again, err := format.Source(once)
	if err != nil {
		log.Fail("format_again", err.Error())
	}
	log.Assert("idempotent", bytes.Equal(once, again), true, bytes.Equal(once, again))
	log.Assert("digest_stable", render.ContentDigest(once) == render.ContentDigest(again),
		render.ContentDigest(once), render.ContentDigest(again))

	// Goldens for CLI main.go remain format-stable.
	testutil.CompareGolden(t, filepathJoin("testdata", "minimal_cli_main_go.golden"), once)

	// Messy whitespace input becomes stable after one format pass.
	messy := []byte("package main\nfunc main(  )  {\n}\n")
	f1, err := format.Source(messy)
	if err != nil {
		log.Fail("messy_f1", err.Error())
	}
	f2, err := format.Source(f1)
	if err != nil {
		log.Fail("messy_f2", err.Error())
	}
	log.Assert("messy_idempotent", bytes.Equal(f1, f2), true, bytes.Equal(f1, f2))
	log.Step("gofmt", testutil.OutcomeOK, "digest="+string(render.ContentDigest(once)))
	log.PhaseEnd("gofmt_idempotent", testutil.OutcomeOK)
}

// TestGomodConflictsFatalViaDispatch ensures version conflicts remain fatal
// through the dispatch path (REQ-095; acceptance: gomod conflicts fatal).
func TestGomodConflictsFatalViaDispatch(t *testing.T) {
	log := testutil.New(t)
	log.Phase("gomod_conflict")

	in := render.GomodInput{
		Module:    "example.com/app",
		GoVersion: render.CatalogGoVersion,
		Toolchain: "go1.26.5",
		Requires: []render.GomodRequire{
			{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
			{Path: "github.com/spf13/cobra", Version: "v1.9.1"},
		},
	}
	_, err := render.Render(nil, render.Job{Mechanism: render.MechanismGomod, Gomod: &in}, nil)
	fe := asFoundry(t, err)
	log.Assert("id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
	if !strings.Contains(fe.Error(), "conflict") && !strings.Contains(fe.Error(), "version") {
		log.Fail("msg", fe.Error())
	}
	log.Step("conflict", testutil.OutcomeOK, "id="+string(fe.ID()))
	log.PhaseEnd("gomod_conflict", testutil.OutcomeOK)
}

// TestDispatchNilWriterInventoryOnly: pure plan-digest path (no Writer).
func TestDispatchNilWriterInventoryOnly(t *testing.T) {
	log := testutil.New(t)
	log.Phase("nil_writer")

	cat := fixtureCatalog()
	inv, err := render.Render(cat, render.Job{
		Mechanism: render.MechanismStatic,
		Static: &render.StaticJob{
			Path: "LICENSE", Mode: "0644", Source: "core/files/LICENSE",
		},
	}, nil)
	if err != nil {
		log.Fail("render", err.Error())
	}
	log.Assert("len", inv.Len() == 1, 1, inv.Len())
	b, ok := inv.Content("LICENSE")
	log.Assert("has_content", ok && len(b) > 0, true, ok && len(b) > 0)
	log.PhaseEnd("nil_writer", testutil.OutcomeOK)
}

// filepathJoin is a tiny pure helper so tests avoid importing path/filepath
// solely for golden paths (testdata is package-relative).
func filepathJoin(elem ...string) string {
	return path.Join(elem...)
}
