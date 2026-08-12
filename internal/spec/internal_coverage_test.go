package spec

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// White-box tests for unexported helpers. This file must stay package spec and
// must not import internal/testutil: testutil/propfixture.go imports internal/spec,
// which would create an import cycle not allowed in test (go-foundry-cli-ipk.1).

func TestSplitDottedKeyAndPositions(t *testing.T) {
	if splitDottedKey("") != nil {
		t.Errorf("empty: want nil, got %v", splitDottedKey(""))
	}
	if splitDottedKey("   ") != nil {
		t.Errorf("space: want nil, got %v", splitDottedKey("   "))
	}
	parts := splitDottedKey("a.b.c")
	if len(parts) != 3 || parts[0] != "a" || parts[2] != "c" {
		t.Errorf("bare: want [a b c], got %v", parts)
	}
	parts = splitDottedKey(`"a.b".c`)
	if len(parts) != 2 || parts[0] != "a.b" || parts[1] != "c" {
		t.Errorf("quoted: want [a.b c], got %v", parts)
	}
	parts = splitDottedKey(`'x'.y`)
	if len(parts) != 2 || parts[0] != "x" {
		t.Errorf("single_quote: want first part x, got %v", parts)
	}
	// unclosed quote takes rest
	parts = splitDottedKey(`"noend`)
	if len(parts) != 1 || !strings.HasPrefix(parts[0], `"`) {
		t.Errorf("unclosed: want single quoted part, got %v", parts)
	}
	// escape inside double quotes
	parts = splitDottedKey(`"a\"b".c`)
	if len(parts) < 1 {
		t.Errorf("escape: want at least one part, got %v", parts)
	}

	if got := indexUnquotedEquals("a = 1"); got != 2 {
		t.Errorf("eq_plain: want 2, got %d", got)
	}
	if got := indexUnquotedEquals(`a = "x=y"`); got != 2 {
		t.Errorf("eq_in_dq: want 2, got %d", got)
	}
	if got := indexUnquotedEquals("# a = 1"); got != -1 {
		t.Errorf("eq_comment: want -1, got %d", got)
	}
	if got := indexUnquotedEquals("nope"); got != -1 {
		t.Errorf("eq_none: want -1, got %d", got)
	}
	if got := indexUnquotedEquals(`"a\"=" = 1`); got < 0 {
		t.Errorf("eq_escaped: want >= 0, got %d", got)
	}

	if got := columnOf("abc", -1); got != 1 {
		t.Errorf("col_neg: want 1, got %d", got)
	}
	if got := columnOf("abc", 2); got != 3 {
		t.Errorf("col_mid: want 3, got %d", got)
	}

	line, col := offsetToLineCol(nil, 0)
	if line != 1 || col != 1 {
		t.Errorf("off_empty: want 1,1 got %d,%d", line, col)
	}
	line, col = offsetToLineCol([]byte("ab\nc"), -5)
	if line != 1 {
		t.Errorf("off_neg: want line 1, got %d", line)
	}
	line, col = offsetToLineCol([]byte("ab\nc"), 100)
	if line != 2 {
		t.Errorf("off_past: want line 2, got %d", line)
	}
	data := []byte("a\nβb") // multi-byte rune
	line, col = offsetToLineCol(data, len(data))
	if line < 1 || col < 1 {
		t.Errorf("off_utf8: want positive line/col, got %d,%d", line, col)
	}
	// invalid utf8 byte
	bad := []byte{'a', 0xff, 'b'}
	_, col = offsetToLineCol(bad, 2)
	if col < 1 {
		t.Errorf("off_invalid: want col >= 1, got %d", col)
	}

	if got := snippetAround(nil, 0, 10); got != "" {
		t.Errorf("snip_empty: want empty, got %q", got)
	}
	if len(snippetAround([]byte("hello world"), 0, 0)) == 0 {
		t.Error("snip_max0: want non-empty snippet")
	}
	if got := snippetAround([]byte("abc"), -1, 10); got != "abc" {
		t.Errorf("snip_neg_off: want abc, got %q", got)
	}
	if got := snippetAround([]byte("abc"), 99, 10); got != "abc" {
		t.Errorf("snip_past: want abc, got %q", got)
	}
	long := []byte(strings.Repeat("x", 200))
	sn := snippetAround(long, 100, 40)
	if len(sn) > 40+10 {
		t.Errorf("snip_window: want len <= 50, got %d", len(sn))
	}
	if utf8.RuneCountInString(sn) > 40 && len(sn) > 40 {
		t.Errorf("snip_window_len: want bounded window, got runes=%d len=%d", utf8.RuneCountInString(sn), len(sn))
	}

	// control chars sanitized
	ctrl := snippetAround([]byte("a\x00b\x01c"), 0, 80)
	if strings.Contains(ctrl, "\x00") {
		t.Errorf("sanitize_ctrl: want no NUL, got %q", ctrl)
	}

	// indexKeyPositions with tables, comments, quoted keys
	src := []byte(`
# comment
schema = 1
[git]
init = true
[[profiles]]
"dotted.key" = "v"
a.b = 1
`)
	pos := indexKeyPositions("f.toml", src)
	if pos["schema"].Line <= 0 {
		t.Errorf("has_schema: want positive line, got %+v", pos["schema"])
	}
	if pos["git"].Line <= 0 && pos["git.init"].Line <= 0 {
		t.Errorf("has_git: want git or git.init position, got %+v", pos)
	}
}

func TestNameRuleMessageAndDescription(t *testing.T) {
	if msg := nameRuleMessage("name", ""); !strings.Contains(msg, "non-empty") {
		t.Errorf("empty_name: want non-empty message, got %q", msg)
	}
	long := strings.Repeat("a", 64)
	if msg := nameRuleMessage("name", long); !strings.Contains(msg, "63") {
		t.Errorf("long_name: want 63 in message, got %q", msg)
	}
	if msg := nameRuleMessage("binary", "BAD"); !strings.Contains(msg, "kebab-case") {
		t.Errorf("bad_pat: want kebab-case message, got %q", msg)
	}
	// valid name still returns an "is invalid" style default message when forced
	if msg := nameRuleMessage("name", "ok-name"); !strings.Contains(msg, "invalid") {
		t.Errorf("default_branch: want invalid message, got %q", msg)
	}

	_, msg := validateDescription("a\nb")
	if !strings.Contains(msg, "single line") {
		t.Errorf("desc_nl: want single line message, got %q", msg)
	}
	_, msg = validateDescription("   ")
	if !strings.Contains(msg, "non-empty") {
		t.Errorf("desc_empty: want non-empty message, got %q", msg)
	}
	_, msg = validateDescription(strings.Repeat("x", 201))
	if !strings.Contains(msg, "200") {
		t.Errorf("desc_long: want 200 in message, got %q", msg)
	}
	// invalid utf8
	_, msg = validateDescription(string([]byte{0xff, 0xfe}))
	if !strings.Contains(msg, "UTF-8") {
		t.Errorf("desc_utf8: want UTF-8 message, got %q", msg)
	}
	tr, msg := validateDescription("  hello  ")
	if tr != "hello" || msg != "" {
		t.Errorf("desc_ok: want trimmed hello and empty msg, got tr=%q msg=%q", tr, msg)
	}

	if hasSemanticImportVersionSuffix("") {
		t.Error("semver_empty: want false")
	}
	if !hasSemanticImportVersionSuffix("github.com/x/y/v2") {
		t.Error("semver_v2: want true")
	}
	if hasSemanticImportVersionSuffix("github.com/x/y") {
		t.Error("semver_no: want false")
	}
	if !hasSemanticImportVersionSuffix("github.com/x/y/v1") {
		t.Error("semver_v1: want true")
	}
	if hasSemanticImportVersionSuffix("github.com/x/y/v2beta") {
		t.Error("semver_v2beta: want false")
	}
}

func TestAsValidationErrorMultiAndCollect(t *testing.T) {
	fe := diagnostic.New(diagnostic.IDSpecInvalidField, "x", diagnostic.Location{})
	ve := &ValidationError{Errs: []*diagnostic.FoundryError{fe}}

	var target *ValidationError
	ok := asValidationError(ve, &target)
	if !ok || target != ve {
		t.Errorf("direct: want ok with target=ve, got ok=%v target=%v", ok, target)
	}

	// unwrapper chain
	w := errWrap{ve}
	target = nil
	ok = asValidationError(w, &target)
	if !ok || target != ve {
		t.Errorf("unwrap_chain: want ok with target=ve, got ok=%v target=%v", ok, target)
	}

	// multi bag
	m := errMulti{[]error{errWrap{errorsNew("x")}, ve}}
	target = nil
	ok = asValidationError(m, &target)
	if !ok || target != ve {
		t.Errorf("multi_bag: want ok with target=ve, got ok=%v target=%v", ok, target)
	}

	// collect multi
	got := CollectFoundryErrors(m)
	if len(got) < 1 {
		t.Errorf("collect_multi: want >= 1 errors, got %d", len(got))
	}
}

type errWrap struct{ e error }

func (w errWrap) Error() string { return "w" }
func (w errWrap) Unwrap() error { return w.e }

type errMulti struct{ es []error }

func (m errMulti) Error() string   { return "m" }
func (m errMulti) Unwrap() []error { return m.es }

func errorsNew(s string) error {
	return &plainErr{s}
}

type plainErr struct{ s string }

func (p *plainErr) Error() string { return p.s }
