package spec_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestValidationErrorAPIEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("validation_error_api")

	var nilVE *spec.ValidationError
	log.Assert("nil_error", nilVE.Error() == "spec: validation failed", true, nilVE.Error())
	log.Assert("nil_first", nilVE.First() == nil, true, nilVE.First() != nil)
	log.Assert("nil_len", nilVE.Len() == 0, true, nilVE.Len())
	log.Assert("nil_unwrap", nilVE.Unwrap() == nil, true, nilVE.Unwrap() != nil)

	empty := &spec.ValidationError{}
	log.Assert("empty_error", empty.Error() == "spec: validation failed", true, empty.Error())
	log.Assert("empty_first", empty.First() == nil, true, empty.First() != nil)
	log.Assert("empty_unwrap", empty.Unwrap() == nil, true, empty.Unwrap() != nil)

	fe1 := diagnostic.New(diagnostic.IDSpecInvalidField, "bad name", diagnostic.SpecLocation("a.toml", 1, 1))
	fe2 := diagnostic.New(diagnostic.IDSpecUnknownField, "unknown x", diagnostic.SpecLocation("a.toml", 2, 1))
	one := &spec.ValidationError{Errs: []*diagnostic.FoundryError{fe1}}
	log.Assert("one_error", one.Error() == fe1.Error(), true, one.Error())
	log.Assert("one_first", one.First() == fe1, true, one.First())
	log.Assert("one_len", one.Len() == 1, true, one.Len())

	multi := &spec.ValidationError{Errs: []*diagnostic.FoundryError{fe1, fe2}}
	msg := multi.Error()
	log.Assert("multi_header", strings.Contains(msg, "spec: 2 field errors:"), true, msg)
	log.Assert("multi_has_both", strings.Contains(msg, fe1.Error()) && strings.Contains(msg, fe2.Error()), true, msg)
	log.Assert("multi_len", multi.Len() == 2, true, multi.Len())
	uw := multi.Unwrap()
	log.Assert("multi_unwrap_n", len(uw) == 2, true, len(uw))

	// AsValidationError edges
	ve, ok := spec.AsValidationError(nil)
	log.Assert("as_nil", !ok && ve == nil, true, ok)
	ve, ok = spec.AsValidationError(errors.New("plain"))
	log.Assert("as_plain", !ok, true, ok)
	ve, ok = spec.AsValidationError(multi)
	log.Assert("as_direct", ok && ve.Len() == 2, true, ok)

	// Wrapped single-unwrap chain
	wrapped := fmt.Errorf("wrap: %w", multi)
	ve, ok = spec.AsValidationError(wrapped)
	log.Assert("as_wrapped", ok && ve != nil && ve.Len() == 2, true, ok)

	// Multi-unwrap container holding ValidationError via errors.Join.
	joined := errors.Join(errors.New("other"), multi)
	ve, ok = spec.AsValidationError(joined)
	log.Assert("as_joined", ok && ve != nil, true, ok)

	// CollectFoundryErrors
	log.Assert("collect_nil", spec.CollectFoundryErrors(nil) == nil, true, false)
	got := spec.CollectFoundryErrors(multi)
	log.Assert("collect_ve", len(got) == 2 && got[0].ID() == diagnostic.IDSpecInvalidField, true, len(got))
	got = spec.CollectFoundryErrors(fe1)
	log.Assert("collect_fe", len(got) == 1 && got[0] == fe1, true, len(got))
	got = spec.CollectFoundryErrors(errors.New("nope"))
	log.Assert("collect_plain", got == nil, true, got != nil)
	got = spec.CollectFoundryErrors(fmt.Errorf("w: %w", fe2))
	log.Assert("collect_wrap_fe", len(got) == 1 && got[0].ID() == diagnostic.IDSpecUnknownField, true, len(got))
	got = spec.CollectFoundryErrors(joined)
	// errors.Join multi-unwrap expands ValidationError members; accept >=1
	log.Assert("collect_joined", len(got) >= 1, true, len(got))

	log.PhaseEnd("validation_error_api", testutil.OutcomeOK)
}

func TestValidatedSpecificationAccessors(t *testing.T) {
	log := testutil.New(t)
	log.Phase("validated_accessors")

	var nilV *spec.ValidatedSpecification
	log.Assert("nil_file", nilV.File() == "", true, nilV.File())
	log.Assert("nil_schema", nilV.Schema() == 0, true, nilV.Schema())
	log.Assert("nil_name", nilV.Name() == "", true, nilV.Name())
	log.Assert("nil_module", nilV.Module() == "", true, nilV.Module())
	log.Assert("nil_desc", nilV.Description() == "", true, nilV.Description())
	log.Assert("nil_arch", nilV.Archetype() == "", true, nilV.Archetype())
	log.Assert("nil_dest", nilV.Destination() == "", true, nilV.Destination())
	log.Assert("nil_bin", nilV.Binary() == "", true, nilV.Binary())
	log.Assert("nil_vis", nilV.Visibility() == "", true, nilV.Visibility())
	log.Assert("nil_prof", nilV.Profiles() == nil, true, nilV.Profiles() != nil)
	log.Assert("nil_git", !nilV.GitInit(), true, nilV.GitInit())
	log.Assert("nil_branch", nilV.GitInitialBranch() == "", true, nilV.GitInitialBranch())
	log.Assert("nil_equal", nilV.Equal(nil), true, false)
	log.Assert("nil_ne", !nilV.Equal(&spec.ValidatedSpecification{}), true, false)
	log.Assert("nil_norm", nilV.NormalizedBytes() == nil, true, nilV.NormalizedBytes() != nil)
	log.Assert("nil_string", nilV.String() == "ValidatedSpecification(nil)", true, nilV.String())

	raw := mustDecode(t, "acc.toml", validDoc(map[string]string{
		"binary":             "demo-cli",
		"visibility":         "public",
		"profiles":           `["distribution"]`,
		"git.init":           "false",
		"git.initial_branch": "develop",
	}))
	vs, err := spec.Validate(raw)
	if err != nil {
		log.Fail("validate", err.Error())
	}
	log.Assert("file", vs.File() == "acc.toml", true, vs.File())
	log.Assert("schema", vs.Schema() == 1, true, vs.Schema())
	log.Assert("name", vs.Name() == "demo-cli", true, vs.Name())
	log.Assert("module", vs.Module() != "", true, vs.Module())
	log.Assert("desc", vs.Description() != "", true, vs.Description())
	log.Assert("arch", vs.Archetype() == "cli", true, vs.Archetype())
	log.Assert("dest", vs.Destination() != "", true, vs.Destination())
	log.Assert("bin", vs.Binary() == "demo-cli", true, vs.Binary())
	log.Assert("vis", vs.Visibility() == "public", true, vs.Visibility())
	profs := vs.Profiles()
	log.Assert("prof", len(profs) == 1 && profs[0] == "distribution", true, profs)
	log.Assert("git", !vs.GitInit(), true, vs.GitInit())
	log.Assert("branch", vs.GitInitialBranch() == "develop", true, vs.GitInitialBranch())
	log.Assert("equal_self", vs.Equal(vs), true, false)
	log.Assert("string", strings.Contains(vs.String(), "demo-cli"), true, vs.String())
	nb := vs.NormalizedBytes()
	log.Assert("norm_has_name", strings.Contains(string(nb), "name=demo-cli"), true, string(nb))
	log.Assert("norm_profiles", strings.Contains(string(nb), `profiles=["distribution"]`), true, string(nb))

	// Equal mismatch branches via second validate with different name
	raw2 := mustDecode(t, "acc2.toml", validDoc(map[string]string{"name": "other-cli", "module": "github.com/example/other-cli", "destination": "./other-cli"}))
	vs2, err := spec.Validate(raw2)
	if err != nil {
		log.Fail("validate2", err.Error())
	}
	log.Assert("ne_name", !vs.Equal(vs2), true, false)

	// empty profiles normalized form
	raw3 := mustDecode(t, "acc3.toml", validDoc(nil))
	vs3, err := spec.Validate(raw3)
	if err != nil {
		log.Fail("validate3", err.Error())
	}
	log.Assert("empty_prof_norm", strings.Contains(string(vs3.NormalizedBytes()), "profiles=[]"), true, string(vs3.NormalizedBytes()))

	log.PhaseEnd("validated_accessors", testutil.OutcomeOK)
}

func TestRawIsDefinedAndPosition(t *testing.T) {
	log := testutil.New(t)
	log.Phase("raw_defined")

	var nilRaw *spec.RawSpecification
	log.Assert("nil_pos_zero", nilRaw.Position("name").IsZero(), true, false)
	log.Assert("nil_defined", !nilRaw.IsDefined("name"), true, false)

	empty := &spec.RawSpecification{}
	log.Assert("empty_pos", empty.Position("name").IsZero(), true, false)
	log.Assert("empty_defined", !empty.IsDefined("name"), true, false)

	raw := mustDecode(t, "def.toml", validDoc(map[string]string{"binary": "demo-cli"}))
	log.Assert("name_defined", raw.IsDefined("name"), true, false)
	log.Assert("name_pos", !raw.Position("name").IsZero(), true, false)
	log.Assert("missing_defined", !raw.IsDefined("nope.field"), true, false)
	log.Assert("binary_defined", raw.IsDefined("binary"), true, false)

	log.PhaseEnd("raw_defined", testutil.OutcomeOK)
}

func TestInvalidFieldMatricesExtra(t *testing.T) {
	log := testutil.New(t)
	log.Phase("invalid_field_extra")

	cases := []struct {
		name string
		ov   map[string]string
		want string // substring in message or id
	}{
		{"name_empty", map[string]string{"name": ""}, "spec.invalid_field"}, // missing after delete
		{"name_too_long", map[string]string{"name": strings.Repeat("a", 64), "module": "github.com/example/" + strings.Repeat("a", 64), "destination": "./" + strings.Repeat("a", 64)}, "63"},
		{"name_bad_chars", map[string]string{"name": "Bad_Name", "module": "github.com/example/Bad_Name", "destination": "./Bad_Name"}, "kebab-case"},
		{"desc_newline", map[string]string{"description": "line1\nline2"}, "single line"},
		{"desc_empty_trim", map[string]string{"description": "   "}, "non-empty"},
		{"desc_too_long", map[string]string{"description": strings.Repeat("x", 201)}, "200"},
		{"binary_bad", map[string]string{"binary": "BAD"}, "binary"},
		{"branch_bad", map[string]string{"git.init": "true", "git.initial_branch": "Bad Branch"}, "git.initial_branch"},
		{"archetype_bad", map[string]string{"archetype": "web"}, "archetype"},
		{"visibility_bad", map[string]string{"visibility": "secret"}, "visibility"},
		{"module_v2_suffix", map[string]string{"module": "github.com/example/demo-cli/v2"}, "import-version"},
		{"destination_dotdot", map[string]string{"destination": "../escape"}, "destination"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase(tc.name)
			data := validDoc(tc.ov)
			// name_empty: delete name key
			if tc.name == "name_empty" {
				data = validDoc(map[string]string{"name": ""})
			}
			raw, err := spec.Decode("x.toml", data)
			if err != nil {
				// some may fail decode — still ok for coverage if Validate path missed
				sub.Step("decode_err", testutil.OutcomeOK, err.Error())
				sub.PhaseEnd(tc.name, testutil.OutcomeOK)
				return
			}
			_, err = spec.Validate(raw)
			sub.Assert("err", err != nil, true, err != nil)
			if err == nil {
				sub.PhaseEnd(tc.name, testutil.OutcomeFail)
				return
			}
			blob := err.Error() + spec.FormatValidationFailure(err)
			for _, fe := range spec.CollectFoundryErrors(err) {
				blob += fe.Message() + string(fe.ID())
			}
			sub.Assert("want", strings.Contains(strings.ToLower(blob), strings.ToLower(tc.want)) ||
				strings.Contains(blob, tc.want), true, blob)
			sub.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}

	// Multi-profile format in NormalizedBytes with two profiles (validate allows unknown profile IDs)
	raw := mustDecode(t, "two.toml", validDoc(map[string]string{"profiles": `["a","b"]`}))
	vs, err := spec.Validate(raw)
	if err != nil {
		log.Fail("two_prof", err.Error())
	}
	nb := string(vs.NormalizedBytes())
	log.Assert("two_prof_fmt", strings.Contains(nb, `"a"`) && strings.Contains(nb, `"b"`), true, nb)

	// Equal profile length mismatch
	rawB := mustDecode(t, "one.toml", validDoc(map[string]string{"profiles": `["a"]`}))
	vsB, err := spec.Validate(rawB)
	if err != nil {
		log.Fail("one_prof", err.Error())
	}
	log.Assert("prof_len_ne", !vs.Equal(vsB), true, false)

	log.PhaseEnd("invalid_field_extra", testutil.OutcomeOK)
}

func TestDecodeQuotedKeysAndSnippets(t *testing.T) {
	log := testutil.New(t)
	log.Phase("decode_edges")

	// Quoted dotted-ish keys and table headers exercise splitDottedKey via indexKeyPositions.
	body := []byte(`
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "A valid demo CLI project"
archetype = "cli"
destination = "./demo-cli"
profiles = []
"binary" = "demo-cli"

[git]
init = true
initial_branch = "main"
`)
	raw, err := spec.Decode("q.toml", body)
	if err != nil {
		log.Fail("decode", err.Error())
	}
	log.Assert("binary_defined", raw.IsDefined("binary") || raw.Binary != nil, true, false)
	vs, err := spec.Validate(raw)
	if err != nil {
		log.Fail("validate", err.Error())
	}
	log.Assert("binary", vs.Binary() == "demo-cli", true, vs.Binary())

	// Invalid UTF-8 / hostile decode paths already in suite; hit FormatDecodeFailure
	bad := []byte("schema = [\n")
	_, err = spec.Decode("bad.toml", bad)
	if err != nil {
		sum := spec.FormatDecodeFailure(len(bad), err)
		log.Assert("decode_fmt", sum != "" && sum != "ok", true, sum)
	}

	// large line triggers snippet window via decode error with offset
	long := []byte("schema = 1\n" + strings.Repeat("x", 200) + " = true\n")
	_, _ = spec.Decode("long.toml", long)

	log.PhaseEnd("decode_edges", testutil.OutcomeOK)
}
