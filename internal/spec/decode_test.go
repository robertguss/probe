package spec_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestDecodeValidMinimalMaximal(t *testing.T) {
	log := testutil.New(t)

	log.Phase("minimal")
	minimal := readExample(t, "minimal-cli.toml")
	log.Fixture("minimal-cli.toml", "bytes="+itoa(len(minimal)))
	raw, err := spec.Decode("minimal-cli.toml", minimal)
	if err != nil {
		log.Fail("decode_minimal", err.Error())
	}
	log.Assert("schema", raw.Schema != nil && *raw.Schema == 1, int64(1), ptrInt(raw.Schema))
	log.Assert("name", raw.Name != nil && *raw.Name == "minimal-cli", "minimal-cli", ptrStr(raw.Name))
	log.Assert("module", raw.Module != nil && strings.HasSuffix(*raw.Module, "minimal-cli"), true, ptrStr(raw.Module))
	log.Assert("archetype", raw.Archetype != nil && *raw.Archetype == "cli", "cli", ptrStr(raw.Archetype))
	log.Assert("profiles_set", raw.ProfilesSet, true, raw.ProfilesSet)
	log.Assert("profiles_empty", raw.ProfilesSet && len(raw.Profiles) == 0, 0, len(raw.Profiles))
	// No defaults for optional fields when absent.
	log.Assert("binary_absent", raw.Binary == nil, true, raw.Binary != nil)
	log.Assert("visibility_absent", raw.Visibility == nil, true, raw.Visibility != nil)
	log.Assert("git_absent", !raw.GitSet && raw.GitInit == nil, true, raw.GitSet)
	// Positions for required keys.
	for _, key := range []string{"schema", "name", "module", "description", "archetype", "destination", "profiles"} {
		loc := raw.Position(key)
		log.Assert("pos_"+key+"_line", loc.Line > 0, ">0", loc.Line)
		log.Assert("pos_"+key+"_file", loc.File == "minimal-cli.toml", "minimal-cli.toml", loc.File)
	}
	log.PhaseEnd("minimal", testutil.OutcomeOK)

	log.Phase("maximal")
	maximal := []byte(`
schema = 1
name = "maximal-cli"
module = "github.com/example/maximal-cli"
description = "All optional fields present; no defaults applied by decode"
archetype = "cli"
destination = "./maximal-cli"
binary = "maximal-cli"
visibility = "private"
profiles = []

[git]
init = true
initial_branch = "main"
`)
	log.Fixture("maximal", "bytes="+itoa(len(maximal)))
	raw2, err := spec.Decode("maximal.toml", maximal)
	if err != nil {
		log.Fail("decode_maximal", err.Error())
	}
	log.Assert("binary", raw2.Binary != nil && *raw2.Binary == "maximal-cli", "maximal-cli", ptrStr(raw2.Binary))
	log.Assert("visibility", raw2.Visibility != nil && *raw2.Visibility == "private", "private", ptrStr(raw2.Visibility))
	log.Assert("git_set", raw2.GitSet, true, raw2.GitSet)
	log.Assert("git_init", raw2.GitInit != nil && *raw2.GitInit == true, true, raw2.GitInit)
	log.Assert("git_branch", raw2.GitInitialBranch != nil && *raw2.GitInitialBranch == "main", "main", ptrStr(raw2.GitInitialBranch))
	log.Assert("pos_git_init", raw2.Position("git.init").Line > 0, ">0", raw2.Position("git.init").Line)
	log.Assert("pos_git_branch", raw2.Position("git.initial_branch").Line > 0, ">0", raw2.Position("git.initial_branch").Line)
	// Still no defaults invented for unset fields — all optionals were set.
	log.PhaseEnd("maximal", testutil.OutcomeOK)

	log.Phase("no_defaults_empty_doc")
	// Completely empty document decodes with everything absent (not defaulted).
	empty, err := spec.Decode("empty.toml", []byte(""))
	if err != nil {
		log.Fail("decode_empty", err.Error())
	}
	log.Assert("empty_schema_nil", empty.Schema == nil, true, empty.Schema != nil)
	log.Assert("empty_name_nil", empty.Name == nil, true, empty.Name != nil)
	log.Assert("empty_profiles_unset", !empty.ProfilesSet, false, empty.ProfilesSet)
	log.Assert("empty_git_unset", !empty.GitSet, false, empty.GitSet)
	log.PhaseEnd("no_defaults_empty_doc", testutil.OutcomeOK)
}

func TestDecodeParseFailures(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantID  diagnostic.Identifier
		wantPos bool // expect line > 0 when available
	}{
		{
			name:    "syntax_unclosed_string",
			data:    []byte("schema = \"unclosed\n"),
			wantID:  diagnostic.IDSpecParseError,
			wantPos: true,
		},
		{
			name:    "syntax_bare_garbage",
			data:    []byte("@@@ not toml\n"),
			wantID:  diagnostic.IDSpecParseError,
			wantPos: true,
		},
		{
			name:    "duplicate_key_top_level",
			data:    []byte("schema = 1\nschema = 1\nname = \"x\"\n"),
			wantID:  diagnostic.IDSpecDuplicateKey,
			wantPos: true,
		},
		{
			name:    "duplicate_table",
			data:    []byte("schema = 1\n[git]\ninit = true\n[git]\ninit = false\n"),
			wantID:  diagnostic.IDSpecDuplicateKey,
			wantPos: true,
		},
		{
			name:    "unknown_field_top_level",
			data:    readExample(t, "invalid-unknown-field.toml"),
			wantID:  diagnostic.IDSpecUnknownField,
			wantPos: true,
		},
		{
			name:    "unknown_nested_field",
			data:    []byte("schema = 1\n[git]\ninit = true\nextra = true\n"),
			wantID:  diagnostic.IDSpecUnknownField,
			wantPos: true,
		},
		{
			name:    "unknown_table",
			data:    []byte("schema = 1\n[meta]\nauthor = \"x\"\n"),
			wantID:  diagnostic.IDSpecUnknownField,
			wantPos: true,
		},
		{
			name:    "invalid_encoding_latin1",
			data:    []byte("schema = \"caf\xffe\"\n"),
			wantID:  diagnostic.IDSpecInvalidEncoding,
			wantPos: true,
		},
		{
			name:    "invalid_encoding_bom",
			data:    append([]byte{0xEF, 0xBB, 0xBF}, []byte("schema = 1\n")...),
			wantID:  diagnostic.IDSpecInvalidEncoding,
			wantPos: true,
		},
		{
			name:    "type_mismatch_schema_string",
			data:    []byte("schema = \"1\"\n"),
			wantID:  diagnostic.IDSpecParseError,
			wantPos: true,
		},
		{
			name:    "control_null_byte",
			data:    []byte("schema = 1\x00\n"),
			wantID:  diagnostic.IDSpecParseError,
			wantPos: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("open")
			log.Inputs(map[string]string{
				"case":    tt.name,
				"bytes":   itoa(len(tt.data)),
				"want_id": string(tt.wantID),
			})
			log.PhaseEnd("open", testutil.OutcomeOK)

			log.Phase("decode")
			raw, err := spec.Decode(tt.name+".toml", tt.data)
			log.Step("decode", testutil.OutcomeOK, spec.FormatDecodeFailure(len(tt.data), err))
			log.PhaseEnd("decode", testutil.OutcomeOK)

			log.Phase("map_errors")
			if raw != nil {
				log.Fail("expected_error", "got raw spec")
			}
			fe, ok := diagnostic.AsFoundryError(err)
			if !ok {
				log.Fail("foundry_error", "not a FoundryError: "+errString(err))
			}
			log.NoteID(string(fe.ID()))
			log.Assert("error_id", fe.ID() == tt.wantID, tt.wantID, fe.ID())
			log.Assert("exit_code", fe.ExitCode() == diagnostic.ExitUsage, diagnostic.ExitUsage, fe.ExitCode())
			if tt.wantPos {
				log.Assert("has_line", fe.Location().Line > 0, ">0", fe.Location().Line)
			}
			log.Assert("has_file", fe.Location().File != "", "nonempty", fe.Location().File)
			log.Assert("remediation", fe.Remediation() != "", "nonempty", truncate(fe.Remediation(), 60))
			// Failure log contract: summary includes byte length + id.
			sum := spec.FormatDecodeFailure(len(tt.data), err)
			log.Assert("summary_bytes", strings.Contains(sum, "byte_length="), true, sum)
			log.Assert("summary_id", strings.Contains(sum, string(tt.wantID)), true, sum)
			log.PhaseEnd("map_errors", testutil.OutcomeOK)
		})
	}
}

func TestDecodeSizeBoundary(t *testing.T) {
	log := testutil.New(t)
	log.Phase("exactly_1mib")
	// Build a valid document of exactly MaxSpecBytes.
	// Header + padding comments + trailing newline.
	header := []byte("schema = 1\nname = \"size-boundary\"\n")
	// Rest is `#` comment padding to exact size.
	if len(header) >= spec.MaxSpecBytes {
		t.Fatal("header too large")
	}
	exact := make([]byte, spec.MaxSpecBytes)
	copy(exact, header)
	for i := len(header); i < len(exact); i++ {
		exact[i] = '#'
	}
	// Last byte must be newline for a clean line; replace end with \n.
	exact[len(exact)-1] = '\n'
	log.Assert("exact_len", len(exact) == spec.MaxSpecBytes, spec.MaxSpecBytes, len(exact))
	raw, err := spec.Decode("exact.toml", exact)
	if err != nil {
		log.Fail("exact_accept", err.Error())
	}
	log.Assert("exact_schema", raw.Schema != nil && *raw.Schema == 1, int64(1), ptrInt(raw.Schema))
	log.Assert("exact_byte_len", raw.ByteLen == spec.MaxSpecBytes, spec.MaxSpecBytes, raw.ByteLen)
	log.PhaseEnd("exactly_1mib", testutil.OutcomeOK)

	log.Phase("one_over")
	over := append(append([]byte{}, exact...), '#')
	log.Assert("over_len", len(over) == spec.MaxSpecBytes+1, spec.MaxSpecBytes+1, len(over))
	raw2, err := spec.Decode("over.toml", over)
	if raw2 != nil {
		log.Fail("over_nil_raw", "expected nil raw")
	}
	fe, ok := diagnostic.AsFoundryError(err)
	if !ok {
		log.Fail("over_err", errString(err))
	}
	log.Assert("over_id", fe.ID() == diagnostic.IDSpecTooLarge, diagnostic.IDSpecTooLarge, fe.ID())
	log.Assert("over_exit", fe.ExitCode() == diagnostic.ExitUsage, diagnostic.ExitUsage, fe.ExitCode())
	log.PhaseEnd("one_over", testutil.OutcomeOK)
}

func TestDecodeHostilePlainData(t *testing.T) {
	// Includes/env-like strings are plain data — not evaluated — and must decode
	// when otherwise valid. Field-rule rejection is out of scope (P1.2.b).
	cases := []struct {
		name string
		data string
	}{
		{
			name: "env_like_in_description",
			data: `
schema = 1
name = "env-like"
module = "github.com/example/env-like"
description = "Uses ${HOME} and $PATH as plain text not env expansion"
archetype = "cli"
destination = "./env-like"
profiles = []
`,
		},
		{
			name: "include_like_in_description",
			data: `
schema = 1
name = "include-like"
module = "github.com/example/include-like"
description = "include other.toml and #include are plain data"
archetype = "cli"
destination = "./include-like"
profiles = []
`,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := spec.Decode(tc.name+".toml", []byte(tc.data))
			if err != nil {
				t.Fatalf("expected plain-data decode success: %v", err)
			}
			if raw.Description == nil || !strings.Contains(*raw.Description, "plain") {
				t.Fatalf("description not preserved: %v", raw.Description)
			}
			// Ensure we did not invent defaults for absent optionals.
			if raw.Binary != nil || raw.Visibility != nil {
				t.Fatalf("defaults applied: binary=%v visibility=%v", raw.Binary, raw.Visibility)
			}
		})
	}
}

func TestDecodeStepLoggerPipeline(t *testing.T) {
	// Multi-step open→decode→map errors case required by the bead.
	log := testutil.New(t)
	log.Phase("open")
	data := []byte("schema = 1\nauthor = \"nope\"\n")
	log.Fixture("inline", "bytes="+itoa(len(data)))
	log.Inputs(map[string]string{"spec": "pipeline.toml"})
	log.PhaseEnd("open", testutil.OutcomeOK)

	log.Phase("decode")
	_, err := spec.Decode("pipeline.toml", data)
	log.Step("decode_call", testutil.OutcomeOK, "err="+errString(err))
	log.PhaseEnd("decode", testutil.OutcomeOK)

	log.Phase("map_errors")
	fe, ok := diagnostic.AsFoundryError(err)
	log.Assert("is_foundry", ok, true, ok)
	log.Assert("id", fe.ID() == diagnostic.IDSpecUnknownField, diagnostic.IDSpecUnknownField, fe.ID())
	log.Assert("pos_line", fe.Location().Line > 0, ">0", fe.Location().Line)
	log.Step("summary", testutil.OutcomeOK, spec.FormatDecodeFailure(len(data), err))
	log.PhaseEnd("map_errors", testutil.OutcomeOK)
}

func TestDecodeDeterminism(t *testing.T) {
	// -count=2 style: same input → identical error id + location.
	data := []byte("a = 1\na = 2\n")
	var first string
	for i := 0; i < 2; i++ {
		_, err := spec.Decode("det.toml", data)
		fe, ok := diagnostic.AsFoundryError(err)
		if !ok {
			t.Fatalf("iter %d: not FoundryError: %v", i, err)
		}
		sig := string(fe.ID()) + "|" + fe.Location().String() + "|" + fe.Message()
		if i == 0 {
			first = sig
		} else if sig != first {
			t.Fatalf("nondeterministic: %q vs %q", first, sig)
		}
	}
}

func TestMaxSpecBytesConstant(t *testing.T) {
	if spec.MaxSpecBytes != 1<<20 {
		t.Fatalf("MaxSpecBytes = %d, want 1048576", spec.MaxSpecBytes)
	}
	// Sanity: valid UTF-8 empty is size 0.
	if !utf8.Valid(bytes.Repeat([]byte("a"), 0)) {
		t.Fatal("empty should be valid utf8")
	}
}

func readExample(t *testing.T, name string) []byte {
	t.Helper()
	// Walk up to module root from this test file's package dir.
	root := findModuleRoot(t)
	p := filepath.Join(root, "examples", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read example %s: %v", p, err)
	}
	return b
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func ptrStr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func ptrInt(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
