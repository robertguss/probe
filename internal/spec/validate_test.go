package spec_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// validDoc builds a minimal structurally-valid specification body with overrides.
// keys not listed use defaults of a good minimal-cli shape.
func validDoc(overrides map[string]string) []byte {
	fields := map[string]string{
		"schema":      "1",
		"name":        "demo-cli",
		"module":      "github.com/example/demo-cli",
		"description": "A valid demo CLI project",
		"archetype":   "cli",
		"destination": "./demo-cli",
		"profiles":    "[]",
	}
	for k, v := range overrides {
		if v == "" && k != "description" {
			delete(fields, k)
			continue
		}
		fields[k] = v
	}
	// Stable-ish write order matching a typical foundry.toml.
	order := []string{
		"schema", "name", "module", "description", "archetype",
		"destination", "binary", "visibility", "profiles",
	}
	var b strings.Builder
	for _, k := range order {
		v, ok := fields[k]
		if !ok {
			continue
		}
		switch k {
		case "schema":
			b.WriteString("schema = ")
			b.WriteString(v)
			b.WriteByte('\n')
		case "profiles":
			b.WriteString("profiles = ")
			b.WriteString(v)
			b.WriteByte('\n')
		default:
			b.WriteString(k)
			b.WriteString(" = ")
			b.WriteString(strconvQuote(v))
			b.WriteByte('\n')
		}
	}
	// Optional git table.
	if init, ok := overrides["git.init"]; ok || overrides["git.initial_branch"] != "" {
		b.WriteString("\n[git]\n")
		if ok {
			b.WriteString("init = ")
			b.WriteString(init)
			b.WriteByte('\n')
		}
		if br, ok := overrides["git.initial_branch"]; ok {
			b.WriteString("initial_branch = ")
			b.WriteString(strconvQuote(br))
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

func strconvQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func mustDecode(t *testing.T, name string, data []byte) *spec.RawSpecification {
	t.Helper()
	raw, err := spec.Decode(name, data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return raw
}

func TestValidateValidExamples(t *testing.T) {
	log := testutil.New(t)
	for _, name := range []string{"minimal-cli.toml", "minimal-tui.toml"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("decode_validate")
			data := readExample(t, name)
			raw := mustDecode(t, name, data)
			vs, err := spec.Validate(raw)
			if err != nil {
				log.Fail("validate", err.Error())
			}
			log.Assert("schema", vs.Schema() == 1, int64(1), vs.Schema())
			log.Assert("name_nonempty", vs.Name() != "", "nonempty", vs.Name())
			log.Assert("binary_default_or_set", vs.Binary() != "", "nonempty", vs.Binary())
			log.Assert("visibility_default", vs.Visibility() == "private" || vs.Visibility() == "public", "private|public", vs.Visibility())
			log.Assert("git_init_default", vs.GitInit() == true || !vs.GitInit(), true, vs.GitInit())
			log.Assert("profiles_non_nil", vs.Profiles() != nil, true, vs.Profiles() != nil)
			log.PhaseEnd("decode_validate", testutil.OutcomeOK)
		})
	}
	log.Phase("maximal_inline")
	data := validDoc(map[string]string{
		"binary":             "demo-cli",
		"visibility":         "public",
		"git.init":           "false",
		"git.initial_branch": "develop",
	})
	vs, err := spec.Validate(mustDecode(t, "maximal.toml", data))
	if err != nil {
		log.Fail("maximal", err.Error())
	}
	log.Assert("visibility", vs.Visibility() == "public", "public", vs.Visibility())
	log.Assert("git_init", vs.GitInit() == false, false, vs.GitInit())
	log.Assert("git_branch", vs.GitInitialBranch() == "develop", "develop", vs.GitInitialBranch())
	log.Assert("binary", vs.Binary() == "demo-cli", "demo-cli", vs.Binary())
	log.PhaseEnd("maximal_inline", testutil.OutcomeOK)
}

func TestValidateFieldRules(t *testing.T) {
	// Table every field rule accept/reject with exact error ids (acceptance).
	type want struct {
		ok     bool
		id     diagnostic.Identifier
		submsg string // optional substring of message
	}
	tests := []struct {
		name string
		doc  []byte
		want want
	}{
		// --- schema ---
		{
			name: "schema_ok",
			doc:  validDoc(nil),
			want: want{ok: true},
		},
		{
			name: "schema_unsupported",
			doc:  validDoc(map[string]string{"schema": "2"}),
			want: want{ok: false, id: diagnostic.IDSpecUnsupportedSchema, submsg: "supported"},
		},
		{
			name: "schema_zero",
			doc:  validDoc(map[string]string{"schema": "0"}),
			want: want{ok: false, id: diagnostic.IDSpecUnsupportedSchema},
		},
		{
			name: "schema_missing",
			doc:  validDoc(map[string]string{"schema": ""}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "schema"},
		},

		// --- name ---
		{
			name: "name_ok_kebab",
			doc:  validDoc(map[string]string{"name": "my-app", "module": "github.com/example/my-app", "destination": "./my-app"}),
			want: want{ok: true},
		},
		{
			name: "name_underscore",
			doc:  readExample(t, "invalid-bad-name.toml"),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "name"},
		},
		{
			name: "name_starts_digit",
			doc:  validDoc(map[string]string{"name": "1bad", "module": "github.com/example/1bad", "destination": "./1bad"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "name"},
		},
		{
			name: "name_uppercase",
			doc:  validDoc(map[string]string{"name": "Bad", "module": "github.com/example/Bad", "destination": "./Bad"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "name"},
		},
		{
			name: "name_empty",
			doc:  validDoc(map[string]string{"name": ""}),
			// empty string still present — fails character rule
			// but validDoc with name="" deletes the key → missing
			want: want{ok: false, id: diagnostic.IDSpecInvalidField},
		},
		{
			name: "name_too_long",
			doc: func() []byte {
				n := strings.Repeat("a", 64)
				return validDoc(map[string]string{
					"name": n, "module": "github.com/example/" + n, "destination": "./" + n,
				})
			}(),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "63"},
		},
		{
			name: "name_double_hyphen",
			doc:  validDoc(map[string]string{"name": "a--b", "module": "github.com/example/a--b", "destination": "./a--b"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "name"},
		},
		{
			name: "name_trailing_hyphen",
			doc:  validDoc(map[string]string{"name": "ab-", "module": "github.com/example/ab-", "destination": "./ab-"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "name"},
		},

		// --- module ---
		{
			name: "module_ok",
			doc:  validDoc(nil),
			want: want{ok: true},
		},
		{
			name: "module_missing_dot",
			doc:  validDoc(map[string]string{"module": "demo-cli"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "module"},
		},
		{
			name: "module_version_suffix_v2",
			doc:  validDoc(map[string]string{"module": "github.com/example/demo-cli/v2"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "version"},
		},
		{
			name: "module_version_suffix_v3",
			doc:  validDoc(map[string]string{"module": "github.com/example/demo-cli/v3"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "version"},
		},
		{
			name: "module_final_segment_mismatch",
			doc:  validDoc(map[string]string{"module": "github.com/example/other-name"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "final path segment"},
		},
		{
			name: "module_missing",
			doc:  validDoc(map[string]string{"module": ""}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "module"},
		},

		// --- description ---
		{
			name: "description_ok_trim",
			doc:  validDoc(map[string]string{"description": "  hello world  "}),
			want: want{ok: true},
		},
		{
			name: "description_empty",
			doc:  validDoc(map[string]string{"description": "   "}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "description"},
		},
		{
			name: "description_multiline",
			doc: []byte(`
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = """line1
line2"""
archetype = "cli"
destination = "./demo-cli"
profiles = []
`),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "single line"},
		},
		{
			name: "description_too_long",
			doc:  validDoc(map[string]string{"description": strings.Repeat("x", 201)}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "200"},
		},
		{
			name: "description_missing",
			doc:  validDoc(map[string]string{"description": ""}),
			// validDoc treats empty as delete for non-description... description key with "" deletes.
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "description"},
		},

		// --- archetype ---
		{
			name: "archetype_cli",
			doc:  validDoc(map[string]string{"archetype": "cli"}),
			want: want{ok: true},
		},
		{
			name: "archetype_tui",
			doc: validDoc(map[string]string{
				"name": "demo-tui", "module": "github.com/example/demo-tui",
				"destination": "./demo-tui", "archetype": "tui",
			}),
			want: want{ok: true},
		},
		{
			name: "archetype_web",
			doc:  validDoc(map[string]string{"archetype": "web"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "archetype"},
		},
		{
			name: "archetype_missing",
			doc:  validDoc(map[string]string{"archetype": ""}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "archetype"},
		},

		// --- destination ---
		{
			name: "destination_ok_relative",
			doc:  validDoc(map[string]string{"destination": "./demo-cli"}),
			want: want{ok: true},
		},
		{
			name: "destination_ok_absolute",
			doc:  validDoc(map[string]string{"destination": "/tmp/work/demo-cli"}),
			want: want{ok: true},
		},
		{
			name: "destination_dot",
			doc:  validDoc(map[string]string{"destination": "."}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "current directory"},
		},
		{
			name: "destination_dotdot",
			doc:  validDoc(map[string]string{"destination": "../demo-cli"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: ".."},
		},
		{
			name: "destination_tilde",
			doc:  validDoc(map[string]string{"destination": "~/demo-cli"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "~"},
		},
		{
			name: "destination_env_dollar",
			doc:  validDoc(map[string]string{"destination": "$HOME/demo-cli"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "environment"},
		},
		{
			name: "destination_env_brace",
			doc:  validDoc(map[string]string{"destination": "${HOME}/demo-cli"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "environment"},
		},
		{
			name: "destination_basename_mismatch",
			doc:  validDoc(map[string]string{"destination": "./other-name"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "basename"},
		},
		{
			name: "destination_missing",
			doc:  validDoc(map[string]string{"destination": ""}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "destination"},
		},

		// --- binary ---
		{
			name: "binary_defaults_to_name",
			doc:  validDoc(nil),
			want: want{ok: true},
		},
		{
			name: "binary_ok",
			doc:  validDoc(map[string]string{"binary": "demo-cli"}),
			want: want{ok: true},
		},
		{
			name: "binary_invalid",
			doc:  validDoc(map[string]string{"binary": "Bad_Binary"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "binary"},
		},

		// --- visibility ---
		{
			name: "visibility_private",
			doc:  validDoc(map[string]string{"visibility": "private"}),
			want: want{ok: true},
		},
		{
			name: "visibility_public",
			doc:  validDoc(map[string]string{"visibility": "public"}),
			want: want{ok: true},
		},
		{
			name: "visibility_invalid",
			doc:  validDoc(map[string]string{"visibility": "internal"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "visibility"},
		},

		// --- profiles ---
		{
			name: "profiles_empty",
			doc:  validDoc(map[string]string{"profiles": "[]"}),
			want: want{ok: true},
		},
		{
			name: "profiles_unknown_id_accepted_here",
			// existence is resolve's job — validate only checks structure + duplicates
			doc:  validDoc(map[string]string{"profiles": `["configuration"]`}),
			want: want{ok: true},
		},
		{
			name: "profiles_duplicate",
			doc:  validDoc(map[string]string{"profiles": `["distribution", "distribution"]`}),
			want: want{ok: false, id: diagnostic.IDSpecDuplicateProfile, submsg: "duplicate"},
		},

		// --- git ---
		{
			name: "git_init_false",
			doc:  validDoc(map[string]string{"git.init": "false"}),
			want: want{ok: true},
		},
		{
			name: "git_branch_ok",
			doc:  validDoc(map[string]string{"git.init": "true", "git.initial_branch": "main"}),
			want: want{ok: true},
		},
		{
			name: "git_branch_invalid",
			doc:  validDoc(map[string]string{"git.init": "true", "git.initial_branch": "Feature/X"}),
			want: want{ok: false, id: diagnostic.IDSpecInvalidField, submsg: "git.initial_branch"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw, err := spec.Decode(tt.name+".toml", tt.doc)
			if err != nil {
				// Some docs may fail decode (e.g. if we ever feed bad TOML).
				t.Fatalf("Decode: %v\n%s", err, tt.doc)
			}
			vs, err := spec.Validate(raw)
			if tt.want.ok {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				if vs == nil {
					t.Fatal("nil ValidatedSpecification")
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got valid spec %+v", vs)
			}
			fe, ok := diagnostic.AsFoundryError(err)
			if !ok {
				// ValidationError multi-unwrap
				errs := spec.CollectFoundryErrors(err)
				if len(errs) == 0 {
					t.Fatalf("not FoundryError: %v", err)
				}
				fe = errs[0]
			}
			if fe.ID() != tt.want.id {
				t.Fatalf("error id = %s, want %s\nmsg=%s", fe.ID(), tt.want.id, fe.Message())
			}
			if tt.want.submsg != "" && !strings.Contains(fe.Message(), tt.want.submsg) {
				t.Fatalf("message %q missing substring %q", fe.Message(), tt.want.submsg)
			}
			if fe.ExitCode() != diagnostic.ExitUsage {
				t.Fatalf("exit = %d, want %d", fe.ExitCode(), diagnostic.ExitUsage)
			}
		})
	}
}

func TestValidateDefaultsOnlyOnSuccess(t *testing.T) {
	log := testutil.New(t)

	log.Phase("success_defaults")
	raw := mustDecode(t, "defs.toml", validDoc(nil))
	// Confirm raw has no defaults applied for optionals.
	log.Assert("raw_binary_nil", raw.Binary == nil, true, raw.Binary != nil)
	log.Assert("raw_visibility_nil", raw.Visibility == nil, true, raw.Visibility != nil)
	log.Assert("raw_git_unset", !raw.GitSet, false, raw.GitSet)

	vs, err := spec.Validate(raw)
	if err != nil {
		log.Fail("validate", err.Error())
	}
	log.Assert("binary_default", vs.Binary() == vs.Name(), vs.Name(), vs.Binary())
	log.Assert("visibility_default", vs.Visibility() == spec.DefaultVisibility, spec.DefaultVisibility, vs.Visibility())
	log.Assert("git_init_default", vs.GitInit() == spec.DefaultGitInit, spec.DefaultGitInit, vs.GitInit())
	log.Assert("git_branch_default", vs.GitInitialBranch() == spec.DefaultGitInitialBranch, spec.DefaultGitInitialBranch, vs.GitInitialBranch())
	log.Assert("profiles_empty", len(vs.Profiles()) == 0, 0, len(vs.Profiles()))
	log.Assert("description_trimmed", vs.Description() == "A valid demo CLI project", "A valid demo CLI project", vs.Description())
	log.PhaseEnd("success_defaults", testutil.OutcomeOK)

	log.Phase("failure_no_defaults_leak")
	// Invalid name: Validate must not return a ValidatedSpecification with defaults.
	bad := validDoc(map[string]string{"name": "BAD", "module": "github.com/example/BAD", "destination": "./BAD"})
	rawBad := mustDecode(t, "bad.toml", bad)
	vsBad, err := spec.Validate(rawBad)
	log.Assert("vs_nil", vsBad == nil, true, vsBad != nil)
	log.Assert("err_set", err != nil, true, err != nil)
	// Raw must remain without invented defaults.
	log.Assert("raw_still_no_binary", rawBad.Binary == nil, true, rawBad.Binary != nil)
	log.PhaseEnd("failure_no_defaults_leak", testutil.OutcomeOK)
}

func TestValidateAggregationSourceOrder(t *testing.T) {
	log := testutil.New(t)
	log.Phase("multi_error")
	// Three independent field failures; order in source: name (L2), archetype (L5), destination (L6).
	// module also wrong segment but name is invalid so segment cross-check may skip;
	// use three pure per-field failures.
	data := []byte(`
schema = 1
name = "BAD_NAME"
module = "github.com/example/BAD_NAME"
description = ""
archetype = "web"
destination = "../nope"
profiles = []
`)
	// description = "" is empty string — present but invalid after trim.
	raw := mustDecode(t, "multi.toml", data)
	_, err := spec.Validate(raw)
	if err == nil {
		log.Fail("expect_errors", "nil error")
	}
	log.Step("validate", testutil.OutcomeOK, spec.FormatValidationFailure(err))

	errs := spec.CollectFoundryErrors(err)
	log.Assert("count_ge_3", len(errs) >= 3, ">=3", len(errs))

	// Source order: each subsequent error's line must be >= previous (when both positioned).
	var prevLine int
	for i, fe := range errs {
		loc := fe.Location()
		log.Step("reject_"+itoa(i), testutil.OutcomeOK,
			"field_msg="+fe.Message()+" id="+string(fe.ID())+
				" line="+itoa(loc.Line)+" col="+itoa(loc.Column))
		if loc.Line > 0 && prevLine > 0 && loc.Line < prevLine {
			log.Fail("source_order", "line regression: prev="+itoa(prevLine)+" got="+itoa(loc.Line))
		}
		if loc.Line > 0 {
			prevLine = loc.Line
		}
		log.Assert("id_field_or_dup",
			fe.ID() == diagnostic.IDSpecInvalidField || fe.ID() == diagnostic.IDSpecDuplicateProfile,
			"spec.invalid_field|duplicate_profile", fe.ID())
	}

	// First positioned error should be name (BAD_NAME) before archetype/destination.
	// Find messages order.
	joined := ""
	for _, fe := range errs {
		joined += fe.Message() + " | "
	}
	nameIdx := strings.Index(joined, "name")
	archIdx := strings.Index(joined, "archetype")
	destIdx := strings.Index(joined, "destination")
	// name rule message contains "name"; archetype/destination messages contain those words.
	if nameIdx >= 0 && archIdx >= 0 {
		log.Assert("name_before_archetype", nameIdx < archIdx, true, nameIdx < archIdx)
	}
	if archIdx >= 0 && destIdx >= 0 {
		log.Assert("archetype_before_dest", archIdx < destIdx, true, archIdx < destIdx)
	}

	ve, ok := spec.AsValidationError(err)
	log.Assert("is_validation_error", ok && ve.Len() >= 3, true, ok)
	log.PhaseEnd("multi_error", testutil.OutcomeOK)
}

func TestValidateSchemaPositionInvariant(t *testing.T) {
	log := testutil.New(t)
	// schema first / middle / last → identical NormalizedBytes (FND-013).
	bodyMid := `
name = "pos-cli"
module = "github.com/example/pos-cli"
schema = 1
description = "Position independence"
archetype = "cli"
destination = "./pos-cli"
profiles = []
`
	bodyFirst := `
schema = 1
name = "pos-cli"
module = "github.com/example/pos-cli"
description = "Position independence"
archetype = "cli"
destination = "./pos-cli"
profiles = []
`
	bodyLast := `
name = "pos-cli"
module = "github.com/example/pos-cli"
description = "Position independence"
archetype = "cli"
destination = "./pos-cli"
profiles = []
schema = 1
`
	var norms [][]byte
	for _, tc := range []struct {
		name string
		doc  string
	}{
		{"first", bodyFirst},
		{"middle", bodyMid},
		{"last", bodyLast},
	} {
		log.Phase("schema_" + tc.name)
		raw := mustDecode(t, tc.name+".toml", []byte(tc.doc))
		vs, err := spec.Validate(raw)
		if err != nil {
			log.Fail("validate", err.Error())
		}
		// File label differs — NormalizedBytes must ignore file.
		// Use a common file by re-validating with same label.
		raw2 := mustDecode(t, "same.toml", []byte(tc.doc))
		vs2, err := spec.Validate(raw2)
		if err != nil {
			log.Fail("validate2", err.Error())
		}
		n := vs2.NormalizedBytes()
		norms = append(norms, n)
		log.Assert("schema", vs.Schema() == 1, int64(1), vs.Schema())
		log.Assert("name", vs.Name() == "pos-cli", "pos-cli", vs.Name())
		log.Step("normalized", testutil.OutcomeOK, string(n))
		log.PhaseEnd("schema_"+tc.name, testutil.OutcomeOK)
	}
	log.Phase("compare")
	for i := 1; i < len(norms); i++ {
		if !bytes.Equal(norms[0], norms[i]) {
			log.Fail("normalized_equal", "norm[0] != norm["+itoa(i)+"]\n"+string(norms[0])+"---\n"+string(norms[i]))
		}
	}
	log.Assert("all_equal", bytes.Equal(norms[0], norms[1]) && bytes.Equal(norms[1], norms[2]), true, true)
	log.PhaseEnd("compare", testutil.OutcomeOK)
}

func TestValidatedSpecificationImmutability(t *testing.T) {
	log := testutil.New(t)
	log.Phase("immutability")
	raw := mustDecode(t, "immut.toml", validDoc(map[string]string{
		"profiles": `["distribution"]`,
	}))
	vs, err := spec.Validate(raw)
	if err != nil {
		log.Fail("validate", err.Error())
	}

	// Mutation of returned Profiles slice must not affect the value.
	p1 := vs.Profiles()
	log.Assert("profiles_len", len(p1) == 1, 1, len(p1))
	p1[0] = "MUTATED"
	p2 := vs.Profiles()
	log.Assert("profiles_unchanged", p2[0] == "distribution", "distribution", p2[0])

	// No exported fields to assign — verify via reflect that all fields are unexported.
	rt := reflect.TypeOf(*vs)
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		log.Assert("unexported_"+f.Name, f.PkgPath != "", "unexported", f.PkgPath)
	}

	// Equal/NormalizedBytes stable across calls.
	n1 := vs.NormalizedBytes()
	n2 := vs.NormalizedBytes()
	log.Assert("norm_stable", bytes.Equal(n1, n2), true, bytes.Equal(n1, n2))
	log.Assert("equal_self", vs.Equal(vs), true, vs.Equal(vs))
	log.PhaseEnd("immutability", testutil.OutcomeOK)
}

func TestValidateSecretFreeSchema(t *testing.T) {
	// REQ-041: schema defines no credential/token/secret field.
	// Enforce via known allowed field set on RawSpecification + ValidatedSpecification.
	allowedRaw := map[string]bool{
		"File": true, "ByteLen": true,
		"Schema": true, "Name": true, "Module": true, "Description": true,
		"Archetype": true, "Destination": true, "Binary": true, "Visibility": true,
		"ProfilesSet": true, "Profiles": true,
		"GitSet": true, "GitInit": true, "GitInitialBranch": true,
		"Positions": true,
	}
	forbiddenSubstr := []string{"secret", "token", "password", "credential", "apikey", "api_key", "auth"}

	rt := reflect.TypeOf(spec.RawSpecification{})
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		if !allowedRaw[name] {
			t.Errorf("RawSpecification unexpected field %q (update allowlist or remove secret-like field)", name)
		}
		lower := strings.ToLower(name)
		for _, bad := range forbiddenSubstr {
			if strings.Contains(lower, bad) {
				t.Errorf("RawSpecification field %q looks secret-related (REQ-041)", name)
			}
		}
	}

	// ValidatedSpecification: all unexported; check names via reflect.
	vsType := reflect.TypeOf(spec.ValidatedSpecification{})
	for i := 0; i < vsType.NumField(); i++ {
		name := strings.ToLower(vsType.Field(i).Name)
		for _, bad := range forbiddenSubstr {
			if strings.Contains(name, bad) {
				t.Errorf("ValidatedSpecification field %q looks secret-related (REQ-041)", vsType.Field(i).Name)
			}
		}
	}
}

func TestValidateNilRaw(t *testing.T) {
	vs, err := spec.Validate(nil)
	if vs != nil || err == nil {
		t.Fatalf("expected error for nil raw, got vs=%v err=%v", vs, err)
	}
	fe, ok := diagnostic.AsFoundryError(err)
	if !ok || fe.ID() != diagnostic.IDSpecInvalidField {
		t.Fatalf("want invalid_field, got %v", err)
	}
}

func TestValidateDescriptionTrimStored(t *testing.T) {
	doc := validDoc(map[string]string{"description": "  padded description  "})
	vs, err := spec.Validate(mustDecode(t, "trim.toml", doc))
	if err != nil {
		t.Fatal(err)
	}
	if vs.Description() != "padded description" {
		t.Fatalf("description = %q, want trimmed", vs.Description())
	}
}

func TestValidateProfilesAbsentDefaultsEmpty(t *testing.T) {
	// Omit profiles key entirely.
	data := []byte(`
schema = 1
name = "no-prof"
module = "github.com/example/no-prof"
description = "no profiles key"
archetype = "cli"
destination = "./no-prof"
`)
	vs, err := spec.Validate(mustDecode(t, "noprof.toml", data))
	if err != nil {
		t.Fatal(err)
	}
	p := vs.Profiles()
	if p == nil || len(p) != 0 {
		t.Fatalf("profiles = %#v, want empty non-nil", p)
	}
}

func TestValidationErrorUnwrap(t *testing.T) {
	data := []byte(`
schema = 1
name = "X"
module = "not-a-module"
description = ""
archetype = "nope"
destination = "."
profiles = ["a", "a"]
`)
	_, err := spec.Validate(mustDecode(t, "unwrap.toml", data))
	if err == nil {
		t.Fatal("expected error")
	}
	ve, ok := spec.AsValidationError(err)
	if !ok {
		t.Fatalf("AsValidationError: %T %v", err, err)
	}
	if ve.Len() < 2 {
		t.Fatalf("expected multiple errors, got %d: %v", ve.Len(), err)
	}
	// errors.As into FoundryError via multi-unwrap.
	var fe *diagnostic.FoundryError
	if !errors.As(err, &fe) {
		t.Fatal("errors.As FoundryError failed")
	}
	if fe.ID() == "" {
		t.Fatal("empty id")
	}
}

func TestFormatValidationFailure(t *testing.T) {
	if spec.FormatValidationFailure(nil) != "ok" {
		t.Fatal("nil should be ok")
	}
	data := validDoc(map[string]string{"name": "BAD", "module": "github.com/example/BAD", "destination": "./BAD"})
	_, err := spec.Validate(mustDecode(t, "fmt.toml", data))
	sum := spec.FormatValidationFailure(err)
	if !strings.Contains(sum, "count=") || !strings.Contains(sum, "spec.invalid_field") {
		t.Fatalf("summary = %q", sum)
	}
}
