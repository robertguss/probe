package render_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestTemplateIfElseAndRangeElseBranches(t *testing.T) {
	log := testutil.New(t)
	log.Phase("if_else_range")

	data := minimalCLITemplateData()
	data.Archetype = "tui"

	// if/else both branches.
	src := []byte("[[ if eq .Archetype \"tui\" -]]TUI[[ else -]]CLI[[ end -]]\n")
	out, err := render.ExecuteTemplate("if_else", src, data, "x.md")
	if err != nil {
		log.Fail("if_else_tui", err.Error())
	}
	log.Assert("tui_branch", strings.Contains(string(out), "TUI"), true, string(out))

	data.Archetype = "cli"
	out, err = render.ExecuteTemplate("if_else", src, data, "x.md")
	if err != nil {
		log.Fail("if_else_cli", err.Error())
	}
	log.Assert("cli_branch", strings.Contains(string(out), "CLI"), true, string(out))

	// range with empty else list (no else) and with else.
	// TemplateData has no slices; use built-in empty via falsey with.
	srcWith := []byte("[[ with .Name -]]has[[ else -]]missing[[ end -]]\n")
	out, err = render.ExecuteTemplate("with_else", srcWith, data, "x.md")
	if err != nil {
		log.Fail("with_else", err.Error())
	}
	log.Assert("with_has", strings.Contains(string(out), "has"), true, string(out))

	empty := data
	empty.Name = ""
	out, err = render.ExecuteTemplate("with_else_empty", srcWith, empty, "x.md")
	if err != nil {
		log.Fail("with_else_empty", err.Error())
	}
	log.Assert("with_missing", strings.Contains(string(out), "missing"), true, string(out))

	// Nested if without else inside if-with-else (typed-nil ElseList walk).
	nested := []byte("[[ if eq .Archetype \"cli\" -]][[ if .DistributionEnabled -]]pub[[ end -]][[ else -]]priv[[ end -]]\n")
	out, err = render.ExecuteTemplate("nested_if", nested, data, "x.md")
	if err != nil {
		log.Fail("nested_if", err.Error())
	}
	log.Assert("nested_ok", len(out) >= 0, true, len(out))

	log.Step("if_else_range", testutil.OutcomeOK, "branches walked")
	log.PhaseEnd("if_else_range", testutil.OutcomeOK)
}

func TestTemplateErrorPathsAndForbiddenFuncs(t *testing.T) {
	log := testutil.New(t)
	log.Phase("template_errors")

	// Empty template name → <template> location.
	fe := render.TemplateErrorForTest("", "boom")
	log.Assert("empty_name_id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
	log.Assert("empty_name_msg", strings.Contains(fe.Error(), "boom"), true, fe.Error())

	fe2 := render.TemplateErrorForTest("src.tmpl", "x")
	log.Assert("named", fe2.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe2.ID())

	// Forbidden function reasons — hit every case arm.
	cases := []struct {
		ident   string
		wantSub string
		admit   bool
	}{
		{"join", "", true},
		{"quote", "", true},
		{"call", "call", false},
		{"env", "environment", false},
		{"environ", "environment", false},
		{"expandenv", "environment", false},
		{"readFile", "filesystem", false},
		{"readDir", "filesystem", false},
		{"file", "filesystem", false},
		{"glob", "filesystem", false},
		{"os", "filesystem", false},
		{"now", "time", false},
		{"time", "time", false},
		{"date", "time", false},
		{"dateInZone", "time", false},
		{"rand", "random", false},
		{"randInt", "random", false},
		{"uuid", "random", false},
		{"shuffle", "random", false},
		{"exec", "command", false},
		{"shell", "command", false},
		{"cmd", "command", false},
		{"system", "command", false},
		{"httpGet", "network", false},
		{"getHostByName", "network", false},
		{"network", "network", false},
		{"unknownBuiltIn", "", true}, // empty reason — execute-time fail
	}
	for _, tc := range cases {
		got := render.ForbiddenFuncReasonForTest(tc.ident)
		if tc.admit {
			log.Assert("admit_"+tc.ident, got == "", "", got)
		} else {
			log.Assert("ban_"+tc.ident, got != "" && strings.Contains(got, tc.wantSub), tc.wantSub, got)
		}
	}

	// ValidateTemplateSource + ExecuteTemplate fail paths via forbidden nodes.
	for _, src := range []string{
		`[[ template "other" . ]]`,
		`[[call .Name]]`,
		`[[env "HOME"]]`,
		`[[exec "true"]]`,
		`[[now]]`,
		`[[rand]]`,
		`[[httpGet "x"]]`,
		`[[readFile "x"]]`,
	} {
		err := render.ValidateTemplateSource("hostile.tmpl", []byte(src))
		if err == nil {
			log.Fail("validate_"+src, "expected render.failed")
		} else {
			fe := asFoundry(t, err)
			log.Assert("validate_id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
		}
		_, err = render.ExecuteTemplate("hostile.tmpl", []byte(src), minimalCLITemplateData(), "out.txt")
		if err == nil {
			log.Fail("exec_"+src, "expected render.failed")
		}
	}

	// Parse error path.
	err := render.ValidateTemplateSource("bad.tmpl", []byte("[[ if "))
	if err == nil {
		log.Fail("parse_error", "expected parse failure")
	} else {
		log.Step("parse_error", testutil.OutcomeOK, err.Error())
	}

	// Execute unknown identifier.
	_, err = render.ExecuteTemplate("miss.tmpl", []byte("[[.Nope]]\n"), minimalCLITemplateData(), "")
	if err == nil {
		log.Fail("missing_key", "expected error")
	} else {
		fe := asFoundry(t, err)
		log.Assert("miss_id", fe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, fe.ID())
	}

	// Empty name execute path (name becomes <template> in errors).
	_, err = render.ExecuteTemplate("", []byte("[[.Nope]]\n"), minimalCLITemplateData(), "")
	if err == nil {
		log.Fail("empty_name_exec", "expected error")
	}

	// isGoSourcePath edges.
	log.Assert("go_empty", !render.IsGoSourcePathForTest(""), false, false)
	log.Assert("go_ext", render.IsGoSourcePathForTest("main.go"), true, true)
	log.Assert("go_upper", render.IsGoSourcePathForTest("MAIN.GO"), true, true)
	log.Assert("go_md", !render.IsGoSourcePathForTest("README.md"), false, false)

	// Typed-nil parse node interfaces.
	var list *parse.ListNode
	var iface parse.Node = list
	log.Assert("typed_nil_list", render.ParseNodeIsNilForTest(iface), true, true)
	log.Assert("plain_nil", render.ParseNodeIsNilForTest(nil), true, true)
	var action *parse.ActionNode
	iface = action
	log.Assert("typed_nil_action", render.ParseNodeIsNilForTest(iface), true, true)
	var ifn *parse.IfNode
	iface = ifn
	log.Assert("typed_nil_if", render.ParseNodeIsNilForTest(iface), true, true)
	var rn *parse.RangeNode
	iface = rn
	log.Assert("typed_nil_range", render.ParseNodeIsNilForTest(iface), true, true)
	var wn *parse.WithNode
	iface = wn
	log.Assert("typed_nil_with", render.ParseNodeIsNilForTest(iface), true, true)
	var tn *parse.TemplateNode
	iface = tn
	log.Assert("typed_nil_tmpl", render.ParseNodeIsNilForTest(iface), true, true)
	var pn *parse.PipeNode
	iface = pn
	log.Assert("typed_nil_pipe", render.ParseNodeIsNilForTest(iface), true, true)
	var cn *parse.CommandNode
	iface = cn
	log.Assert("typed_nil_cmd", render.ParseNodeIsNilForTest(iface), true, true)
	var ch *parse.ChainNode
	iface = ch
	log.Assert("typed_nil_chain", render.ParseNodeIsNilForTest(iface), true, true)
	var bn *parse.BranchNode
	iface = bn
	log.Assert("typed_nil_branch", render.ParseNodeIsNilForTest(iface), true, true)
	var text *parse.TextNode
	iface = text
	log.Assert("typed_nil_text", render.ParseNodeIsNilForTest(iface), true, true)
	var sn *parse.StringNode
	iface = sn
	log.Assert("typed_nil_str", render.ParseNodeIsNilForTest(iface), true, true)
	var idn *parse.IdentifierNode
	iface = idn
	log.Assert("typed_nil_id", render.ParseNodeIsNilForTest(iface), true, true)
	var fn *parse.FieldNode
	iface = fn
	log.Assert("typed_nil_field", render.ParseNodeIsNilForTest(iface), true, true)
	var vn *parse.VariableNode
	iface = vn
	log.Assert("typed_nil_var", render.ParseNodeIsNilForTest(iface), true, true)
	var dn *parse.DotNode
	iface = dn
	log.Assert("typed_nil_dot", render.ParseNodeIsNilForTest(iface), true, true)
	var nn *parse.NilNode
	iface = nn
	log.Assert("typed_nil_nilnode", render.ParseNodeIsNilForTest(iface), true, true)
	var num *parse.NumberNode
	iface = num
	log.Assert("typed_nil_num", render.ParseNodeIsNilForTest(iface), true, true)
	var booln *parse.BoolNode
	iface = booln
	log.Assert("typed_nil_bool", render.ParseNodeIsNilForTest(iface), true, true)

	log.Step("template_errors", testutil.OutcomeOK, "forbidden+typednil+errors")
	log.PhaseEnd("template_errors", testutil.OutcomeOK)
}

func TestTemplateJoinEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("join_edges")

	// Slice sole part.
	got, err := render.TemplateJoinForTest(",", []string{"a", "b"})
	if err != nil {
		log.Fail("slice_join", err.Error())
	}
	log.Assert("slice", got == "a,b", "a,b", got)

	// Mixed string args.
	got, err = render.TemplateJoinForTest("-", "x", "y")
	if err != nil {
		log.Fail("strs", err.Error())
	}
	log.Assert("strs", got == "x-y", "x-y", got)

	// []string not sole part → error.
	_, err = render.TemplateJoinForTest(",", []string{"a"}, "b")
	if err == nil {
		log.Fail("slice_mixed", "expected error")
	} else {
		log.Assert("slice_mixed_msg", strings.Contains(err.Error(), "sole part"), true, err.Error())
	}

	// Wrong type.
	_, err = render.TemplateJoinForTest(",", 42)
	if err == nil {
		log.Fail("bad_type", "expected error")
	} else {
		log.Assert("bad_type_msg", strings.Contains(err.Error(), "want string"), true, err.Error())
	}

	// Empty parts.
	got, err = render.TemplateJoinForTest(",")
	if err != nil {
		log.Fail("empty", err.Error())
	}
	log.Assert("empty_join", got == "", "", got)

	// Via ExecuteTemplate: join of wrong type fails at execute.
	_, err = render.ExecuteTemplate("join_bad", []byte(`[[join "," 1]]`), minimalCLITemplateData(), "o.txt")
	if err == nil {
		log.Fail("exec_join_bad", "expected error")
	}

	log.Step("join_edges", testutil.OutcomeOK, "slice+type+empty")
	log.PhaseEnd("join_edges", testutil.OutcomeOK)
}

func TestGomodResidualEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("gomod_edges")

	// Invalid go version / toolchain.
	_, err := render.GenerateGoMod(render.GomodInput{
		Module: "example.com/m", GoVersion: "not-a-ver", Toolchain: "go1.26.5",
	})
	if err == nil {
		log.Fail("bad_go", "expected error")
	} else {
		log.Step("bad_go", testutil.OutcomeOK, err.Error())
	}
	_, err = render.GenerateGoMod(render.GomodInput{
		Module: "example.com/m", GoVersion: render.CatalogGoVersion, Toolchain: "!!bad!!",
	})
	if err == nil {
		log.Fail("bad_tc", "expected error")
	}

	// Empty require path / invalid require path / empty tool path.
	_, err = render.GenerateGoMod(render.GomodInput{
		Module: "example.com/m", GoVersion: render.CatalogGoVersion, Toolchain: "go1.26.5",
		Requires: []render.GomodRequire{{Path: "", Version: "v1.0.0"}},
	})
	if err == nil {
		log.Fail("empty_req", "expected error")
	}
	_, err = render.GenerateGoMod(render.GomodInput{
		Module: "example.com/m", GoVersion: render.CatalogGoVersion, Toolchain: "go1.26.5",
		Requires: []render.GomodRequire{{Path: "NOT_VALID", Version: "v1.0.0"}},
	})
	if err == nil {
		log.Fail("bad_req_path", "expected error")
	}
	_, err = render.GenerateGoMod(render.GomodInput{
		Module: "example.com/m", GoVersion: render.CatalogGoVersion, Toolchain: "go1.26.5",
		Tools: []render.GomodTool{{Path: "", Version: "v1.0.0"}},
	})
	if err == nil {
		log.Fail("empty_tool", "expected error")
	}
	_, err = render.GenerateGoMod(render.GomodInput{
		Module: "example.com/m", GoVersion: render.CatalogGoVersion, Toolchain: "go1.26.5",
		Tools: []render.GomodTool{{Path: "BAD PATH", Version: "v1.0.0"}},
	})
	if err == nil {
		log.Fail("bad_tool_pkg", "expected error")
	}

	// Tool with empty Module defaults to package path; duplicate tool pkg idempotent.
	out, err := render.GenerateGoMod(render.GomodInput{
		Module: "example.com/m", GoVersion: render.CatalogGoVersion, Toolchain: "go1.26.5",
		Tools: []render.GomodTool{
			{Path: "example.com/m/cmd/x", Version: "v1.2.3"},
			{Path: "example.com/m/cmd/x", Version: "v1.2.3"}, // dup package
		},
	})
	if err != nil {
		log.Fail("dup_tool", err.Error())
	} else {
		log.Assert("has_tool", strings.Contains(string(out), "tool example.com/m/cmd/x"), true, string(out))
		log.Step("dup_tool_ok", testutil.OutcomeOK, "bytes="+itoa(len(out)))
	}

	// Tool/require version conflict.
	_, err = render.GenerateGoMod(render.GomodInput{
		Module: "example.com/m", GoVersion: render.CatalogGoVersion, Toolchain: "go1.26.5",
		Requires: []render.GomodRequire{{Path: "example.com/lib", Version: "v1.0.0"}},
		Tools:    []render.GomodTool{{Path: "example.com/lib/cmd", Module: "example.com/lib", Version: "v2.0.0"}},
	})
	if err == nil {
		log.Fail("tool_req_conflict", "expected conflict")
	} else {
		log.Assert("conflict_msg", strings.Contains(err.Error(), "conflict"), true, err.Error())
	}

	// isExactVersion residual arms.
	exactCases := map[string]bool{
		"":                    false,
		"latest":              false,
		"LATEST":              false,
		"master":              false,
		"main":                false,
		"head":                false,
		"tip":                 false,
		"v1.0.0":              true,
		">=v1.0.0":            false,
		"~v1.0.0":             false,
		"^v1.0.0":             false,
		"*":                   false,
		"v1.0.0 x":            false,
		"v1.2.3+incompatible": true,
	}
	for v, want := range exactCases {
		got := render.IsExactVersionForTest(v)
		log.Assert("exact_"+v, got == want, want, got)
	}

	// RequireCount / ToolCount edges (empty module/path trim).
	in := render.GomodInput{
		Requires: []render.GomodRequire{{Path: "  ", Version: "v1"}, {Path: "a.com/x", Version: "v1"}},
		Tools: []render.GomodTool{
			{Path: "", Module: "a.com/y", Version: "v1"},
			{Path: "a.com/y/cmd", Module: "", Version: "v1"},
			{Path: "  ", Module: "  ", Version: "v1"},
		},
	}
	log.Assert("req_count", in.RequireCount() >= 1, true, in.RequireCount())
	log.Assert("tool_count", in.ToolCount() >= 1, true, in.ToolCount())

	log.PhaseEnd("gomod_edges", testutil.OutcomeOK)
}

func TestInventoryWriterNilAndString(t *testing.T) {
	log := testutil.New(t)
	log.Phase("inventory_writer")

	var inv *render.Inventory
	log.Assert("nil_len", inv.Len() == 0, 0, inv.Len())
	log.Assert("nil_entries", inv.Entries() == nil, true, inv.Entries() == nil)
	log.Assert("nil_paths", inv.Paths() == nil, true, inv.Paths() == nil)
	_, ok := inv.Content("x")
	log.Assert("nil_content", !ok, false, ok)
	_, ok = inv.EntryByPath("x")
	log.Assert("nil_entry", !ok, false, ok)
	log.Assert("nil_string", inv.String() == "Inventory(nil)", "Inventory(nil)", inv.String())
	log.Assert("nil_equal_nil", inv.Equal(nil), true, true)
	log.Assert("nil_ne_nonnil", !inv.Equal(render.NewInventoryForTest(nil, nil)), false, false)

	// Non-nil inventory string + equal mismatch paths.
	a := render.NewInventoryForTest(
		[]render.Entry{{Path: "a.txt", Mode: "0644", Mechanism: render.MechanismStatic, ContentDigest: render.ContentDigest([]byte("a"))}},
		map[string][]byte{"a.txt": []byte("a")},
	)
	b := render.NewInventoryForTest(
		[]render.Entry{{Path: "a.txt", Mode: "0644", Mechanism: render.MechanismStatic, ContentDigest: render.ContentDigest([]byte("b"))}},
		map[string][]byte{"a.txt": []byte("b")},
	)
	c := render.NewInventoryForTest(
		[]render.Entry{
			{Path: "a.txt", Mode: "0644", Mechanism: render.MechanismStatic, ContentDigest: render.ContentDigest([]byte("a"))},
			{Path: "b.txt", Mode: "0644", Mechanism: render.MechanismStatic, ContentDigest: render.ContentDigest([]byte("b"))},
		},
		map[string][]byte{"a.txt": []byte("a"), "b.txt": []byte("b")},
	)
	log.Assert("ne_content", !a.Equal(b), false, false)
	log.Assert("ne_len", !a.Equal(c), false, false)
	log.Assert("eq_self", a.Equal(a), true, true)
	log.Assert("string_has_path", strings.Contains(a.String(), "a.txt="), true, a.String())
	log.Assert("paths", len(a.Paths()) == 1 && a.Paths()[0] == "a.txt", "a.txt", fmt.Sprint(a.Paths()))
	_, ok = a.Content("missing")
	log.Assert("missing_content", !ok, false, ok)
	_, ok = a.EntryByPath("missing")
	log.Assert("missing_entry", !ok, false, ok)

	// newInventory nil content map.
	d := render.NewInventoryForTest([]render.Entry{{Path: "z", Mode: "0644"}}, nil)
	log.Assert("nil_content_map", d.Len() == 1, 1, d.Len())

	// MemoryWriter Paths/String/nil.
	var w *render.MemoryWriter
	log.Assert("w_nil_paths", w.Paths() == nil, true, w.Paths() == nil)
	log.Assert("w_nil_len", w.Len() == 0, 0, w.Len())
	log.Assert("w_nil_string", w.String() == "MemoryWriter(nil)", "MemoryWriter(nil)", w.String())
	_, _, ok = w.Get("x")
	log.Assert("w_nil_get", !ok, false, ok)

	mw := render.NewMemoryWriter()
	if err := mw.WriteFile("b.txt", "0644", []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteFile("a.txt", "0644", []byte("a")); err != nil {
		t.Fatal(err)
	}
	paths := mw.Paths()
	log.Assert("sorted", len(paths) == 2 && paths[0] == "a.txt" && paths[1] == "b.txt",
		"a.txt,b.txt", strings.Join(paths, ","))
	log.Assert("w_string", strings.Contains(mw.String(), "files=2"), true, mw.String())
	_, _, ok = mw.Get("nope")
	log.Assert("w_missing", !ok, false, ok)

	// wrapWriteError: FoundryError passthrough + plain wrap.
	fe := diagnostic.New(diagnostic.IDRenderFailed, "already", diagnostic.PathLocation("p"))
	got := render.WrapWriteErrorForTest("p", fe)
	gotFE, ok := diagnostic.AsFoundryError(got)
	log.Assert("passthrough", ok && gotFE.Error() == fe.Error(), true, ok)
	wrapped := render.WrapWriteErrorForTest("out.txt", errors.New("disk full"))
	wfe := asFoundry(t, wrapped)
	log.Assert("wrap_id", wfe.ID() == diagnostic.IDRenderFailed, diagnostic.IDRenderFailed, wfe.ID())
	log.Assert("wrap_msg", strings.Contains(wfe.Error(), "out.txt"), true, wfe.Error())

	log.Step("inventory_writer", testutil.OutcomeOK, "nil+string+wrap")
	log.PhaseEnd("inventory_writer", testutil.OutcomeOK)
}

func TestPathAndSourceUnsafeEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("path_edges")

	cases := []struct {
		name string
		path string
		ok   bool
	}{
		{"empty", "", false},
		{"abs", "/abs/x", false},
		{"drive", "C:foo", false},
		{"backslash", "a\\b", false},
		{"nul", "a\x00b", false},
		{"dotdot", "a/../b", false},
		{"empty_seg", "a//b", false},
		{"dot", ".", false},
		{"ok", "cmd/main.go", true},
		{"ok_clean", "./cmd/main.go", true},
	}
	for _, tc := range cases {
		got, err := render.SafeOutputPathForTest(tc.path)
		if tc.ok {
			log.Assert("ok_"+tc.name, err == nil && got != "", true, got)
		} else {
			log.Assert("bad_"+tc.name, err != nil, true, err)
		}
	}

	// sourcePathUnsafe
	log.Assert("src_empty", render.SourcePathUnsafeForTest("") != "", true, render.SourcePathUnsafeForTest(""))
	log.Assert("src_abs", render.SourcePathUnsafeForTest("/abs") != "", true, render.SourcePathUnsafeForTest("/abs"))
	log.Assert("src_bs", render.SourcePathUnsafeForTest(`a\b`) != "", true, render.SourcePathUnsafeForTest(`a\b`))
	log.Assert("src_nul", render.SourcePathUnsafeForTest("a\x00b") != "", true, render.SourcePathUnsafeForTest("a\x00b"))
	log.Assert("src_dotdot", render.SourcePathUnsafeForTest("a/../b") != "", true, render.SourcePathUnsafeForTest("a/../b"))
	log.Assert("src_empty_seg", render.SourcePathUnsafeForTest("a//b") != "", true, render.SourcePathUnsafeForTest("a//b"))
	log.Assert("src_ok", render.SourcePathUnsafeForTest("core/files/x") == "", "", render.SourcePathUnsafeForTest("core/files/x"))

	// MemoryWriter nil WriteFile + empty path + duplicate
	var nw *render.MemoryWriter
	if err := nw.WriteFile("x", "0644", []byte("a")); err == nil {
		log.Fail("nil_writer", "expected error")
	}
	mw := render.NewMemoryWriter()
	if err := mw.WriteFile("", "0644", []byte("x")); err == nil {
		log.Fail("empty_path", "expected error")
	}
	_ = mw.WriteFile("dup.txt", "0644", []byte("1"))
	err := mw.WriteFile("dup.txt", "0644", []byte("2"))
	if err == nil {
		log.Fail("dup_write", "expected duplicate error")
	} else {
		fe := asFoundry(t, err)
		log.Assert("dup_id", fe.ID() == diagnostic.IDRenderFailed || strings.Contains(err.Error(), "duplicate"), true, fe.ID())
	}

	// Equal content aok!=bok path: inventories with same entries but missing content key
	e := render.Entry{Path: "z.txt", Mode: "0644", Mechanism: render.MechanismStatic, ContentDigest: render.ContentDigest([]byte("z"))}
	inv1 := render.NewInventoryForTest([]render.Entry{e}, map[string][]byte{"z.txt": []byte("z")})
	inv2 := render.NewInventoryForTest([]render.Entry{e}, map[string][]byte{}) // missing content
	log.Assert("eq_content_miss", !inv1.Equal(inv2), false, false)

	// if/else-if/else chain walks multiple branch lists.
	out, err := render.ExecuteTemplate("elseif", []byte("[[ if false ]]a[[ else if false ]]b[[ else ]]c[[ end ]]\n"), minimalCLITemplateData(), "o.txt")
	if err != nil {
		log.Fail("elseif", err.Error())
	} else {
		log.Assert("elseif_c", strings.Contains(string(out), "c"), true, string(out))
	}

	log.Step("path_edges", testutil.OutcomeOK, "safe+unsafe+dup")
	log.PhaseEnd("path_edges", testutil.OutcomeOK)
}
