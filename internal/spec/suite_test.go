package spec_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// All Appendix D spec.* identifiers that apply at the internal/spec layer
// (REQ-210 / acceptance for go-foundry-cli-d0i).
var appendixDSpecIDs = []diagnostic.Identifier{
	diagnostic.IDSpecParseError,
	diagnostic.IDSpecUnsupportedSchema,
	diagnostic.IDSpecUnknownField,
	diagnostic.IDSpecDuplicateKey,
	diagnostic.IDSpecTooLarge,
	diagnostic.IDSpecInvalidEncoding,
	diagnostic.IDSpecInvalidField,
	diagnostic.IDSpecDuplicateProfile,
}

// TestAppendixDSpecIDCompleteness asserts the package-local list matches the
// registry inventory filtered to the spec domain (append-only contract).
func TestAppendixDSpecIDCompleteness(t *testing.T) {
	log := testutil.New(t)
	log.Phase("inventory")
	var registry []diagnostic.Identifier
	for _, id := range diagnostic.AllIdentifiers() {
		if strings.HasPrefix(string(id), "spec.") {
			registry = append(registry, id)
		}
	}
	log.Assert("registry_count", len(registry) == len(appendixDSpecIDs), len(appendixDSpecIDs), len(registry))
	want := make(map[diagnostic.Identifier]bool, len(appendixDSpecIDs))
	for _, id := range appendixDSpecIDs {
		want[id] = true
	}
	for _, id := range registry {
		if !want[id] {
			log.Fail("extra_registry_id", string(id))
		}
		delete(want, id)
	}
	for id := range want {
		log.Fail("missing_registry_id", string(id))
	}
	log.PhaseEnd("inventory", testutil.OutcomeOK)
}

// TestSpecErrorIDTable is the exhaustive table: every Appendix D spec.*
// identifier is produced by a fixture (file or programmatic boundary case).
// Failure dumps include error id list + fixture name via the step logger.
func TestSpecErrorIDTable(t *testing.T) {
	type row struct {
		fixture string // basename under testdata/errors, or programmatic key
		wantID  diagnostic.Identifier
		// programmatic builds input when set; otherwise load fixture file.
		programmatic func(t *testing.T) []byte
	}
	rows := []row{
		{fixture: "parse_error.toml", wantID: diagnostic.IDSpecParseError},
		{fixture: "unsupported_schema.toml", wantID: diagnostic.IDSpecUnsupportedSchema},
		{fixture: "unknown_field.toml", wantID: diagnostic.IDSpecUnknownField},
		{fixture: "duplicate_key.toml", wantID: diagnostic.IDSpecDuplicateKey},
		{fixture: "invalid_field.toml", wantID: diagnostic.IDSpecInvalidField},
		{fixture: "duplicate_profile.toml", wantID: diagnostic.IDSpecDuplicateProfile},
		{fixture: "nested_unknown_table.toml", wantID: diagnostic.IDSpecUnknownField},
		{fixture: "hostile_env_destination.toml", wantID: diagnostic.IDSpecInvalidField},
		{
			fixture: "too_large.programmatic",
			wantID:  diagnostic.IDSpecTooLarge,
			programmatic: func(t *testing.T) []byte {
				t.Helper()
				// Exactly MaxSpecBytes+1 of comment padding after a tiny header.
				header := []byte("schema = 1\n")
				over := make([]byte, spec.MaxSpecBytes+1)
				copy(over, header)
				for i := len(header); i < len(over); i++ {
					over[i] = '#'
				}
				over[len(over)-1] = '\n'
				return over
			},
		},
		{
			fixture: "invalid_encoding_bom.programmatic",
			wantID:  diagnostic.IDSpecInvalidEncoding,
			programmatic: func(t *testing.T) []byte {
				t.Helper()
				return append([]byte{0xEF, 0xBB, 0xBF}, []byte("schema = 1\n")...)
			},
		},
		{
			fixture: "invalid_encoding_latin1.programmatic",
			wantID:  diagnostic.IDSpecInvalidEncoding,
			programmatic: func(t *testing.T) []byte {
				t.Helper()
				return []byte("schema = \"caf\xffe\"\n")
			},
		},
	}

	seen := make(map[diagnostic.Identifier]bool)
	var seenMu sync.Mutex
	for _, tt := range rows {
		tt := tt
		t.Run(tt.fixture, func(t *testing.T) {
			// Not parallel: subtests populate a shared coverage map that the
			// coverage_complete subtest inspects after this loop.
			log := testutil.New(t)
			log.Phase("open")
			var data []byte
			if tt.programmatic != nil {
				data = tt.programmatic(t)
				log.Fixture(tt.fixture, "programmatic bytes="+itoa(len(data)))
			} else {
				data = readSpecTestdata(t, filepath.Join("errors", tt.fixture))
				log.Fixture(tt.fixture, "bytes="+itoa(len(data)))
			}
			log.Inputs(map[string]string{
				"fixture": tt.fixture,
				"want_id": string(tt.wantID),
			})
			log.PhaseEnd("open", testutil.OutcomeOK)

			log.Phase("decode_validate")
			raw, err := spec.Decode(tt.fixture, data)
			var ve error
			if err != nil {
				ve = err
			} else {
				_, ve = spec.Validate(raw)
			}
			if ve == nil {
				log.Fail("expected_error", "fixture="+tt.fixture+" produced no error")
			}
			errs := spec.CollectFoundryErrors(ve)
			ids := make([]string, 0, len(errs))
			for _, fe := range errs {
				ids = append(ids, string(fe.ID()))
				log.NoteID(string(fe.ID()))
			}
			log.Step("error_ids", testutil.OutcomeOK, "fixture="+tt.fixture+" ids="+strings.Join(ids, ","))
			if len(errs) == 0 {
				log.Fail("no_foundry_errors", ve.Error())
			}
			got := errs[0].ID()
			log.Assert("error_id", got == tt.wantID, tt.wantID, got)
			log.Assert("exit_usage", errs[0].ExitCode() == diagnostic.ExitUsage, diagnostic.ExitUsage, errs[0].ExitCode())
			log.PhaseEnd("decode_validate", testutil.OutcomeOK)
			seenMu.Lock()
			seen[tt.wantID] = true
			seenMu.Unlock()
		})
	}

	// Completeness: every Appendix D spec.* ID appeared at least once.
	t.Run("coverage_complete", func(t *testing.T) {
		log := testutil.New(t)
		log.Phase("coverage")
		for _, id := range appendixDSpecIDs {
			log.Assert("covered_"+string(id), seen[id], true, seen[id])
			if !seen[id] {
				log.Fail("missing_id_coverage", string(id))
			}
		}
		log.PhaseEnd("coverage", testutil.OutcomeOK)
	})
}

// TestAppendixBGoldens decodes+validates each Appendix B canonical example
// and compares ValidatedSpecification.NormalizedBytes to checked-in goldens.
func TestAppendixBGoldens(t *testing.T) {
	cases := []struct {
		name   string
		file   string
		golden string
	}{
		{"private_cli", "appendix_b/private-cli.toml", "appendix_b_private_cli"},
		{"private_tui", "appendix_b/private-tui.toml", "appendix_b_private_tui"},
		{"public_cli_distribution", "appendix_b/public-cli-distribution.toml", "appendix_b_public_cli_distribution"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("decode_validate")
			data := readSpecTestdata(t, tc.file)
			log.Fixture(tc.file, "bytes="+itoa(len(data)))
			// Stable file label so NormalizedBytes is independent of absolute path.
			raw, err := spec.Decode("foundry.toml", data)
			if err != nil {
				log.Fail("decode", err.Error())
			}
			vs, err := spec.Validate(raw)
			if err != nil {
				log.Fail("validate", "ids="+errorIDs(err)+" err="+err.Error())
			}
			log.Assert("schema", vs.Schema() == 1, int64(1), vs.Schema())
			norm := vs.NormalizedBytes()
			log.Step("normalized", testutil.OutcomeOK, truncate(string(norm), 200))
			log.PhaseEnd("decode_validate", testutil.OutcomeOK)

			log.Phase("golden")
			gpath := testutil.GoldenPath(filepath.Join("testdata", "goldens"), tc.golden)
			testutil.CompareGolden(t, gpath, norm)
			log.PhaseEnd("golden", testutil.OutcomeOK)
		})
	}
}

// TestSchemaPositionPermutations places schema in ≥3 positions and asserts
// identical NormalizedBytes (FND-013 / REQ-039).
func TestSchemaPositionPermutations(t *testing.T) {
	log := testutil.New(t)
	// Five permutations: first, early, middle, late, last.
	bodies := []struct {
		name string
		doc  string
	}{
		{
			name: "schema_first",
			doc: `schema = 1
name = "pos-cli"
module = "github.com/example/pos-cli"
description = "Position independence"
archetype = "cli"
destination = "./pos-cli"
profiles = []
`,
		},
		{
			name: "schema_after_name",
			doc: `name = "pos-cli"
schema = 1
module = "github.com/example/pos-cli"
description = "Position independence"
archetype = "cli"
destination = "./pos-cli"
profiles = []
`,
		},
		{
			name: "schema_middle",
			doc: `name = "pos-cli"
module = "github.com/example/pos-cli"
schema = 1
description = "Position independence"
archetype = "cli"
destination = "./pos-cli"
profiles = []
`,
		},
		{
			name: "schema_before_profiles",
			doc: `name = "pos-cli"
module = "github.com/example/pos-cli"
description = "Position independence"
archetype = "cli"
destination = "./pos-cli"
schema = 1
profiles = []
`,
		},
		{
			name: "schema_last",
			doc: `name = "pos-cli"
module = "github.com/example/pos-cli"
description = "Position independence"
archetype = "cli"
destination = "./pos-cli"
profiles = []
schema = 1
`,
		},
	}
	if len(bodies) < 3 {
		t.Fatal("need ≥3 schema positions")
	}
	var norms [][]byte
	for _, tc := range bodies {
		log.Phase(tc.name)
		raw := mustDecode(t, "same.toml", []byte(tc.doc))
		vs, err := spec.Validate(raw)
		if err != nil {
			log.Fail("validate", err.Error())
		}
		n := vs.NormalizedBytes()
		norms = append(norms, n)
		log.Step("normalized", testutil.OutcomeOK, string(n))
		log.PhaseEnd(tc.name, testutil.OutcomeOK)
	}
	log.Phase("compare")
	allEqual := true
	for i := 1; i < len(norms); i++ {
		if !bytes.Equal(norms[0], norms[i]) {
			allEqual = false
			log.Fail("normalized_equal", "norm[0] != norm["+itoa(i)+"]")
		}
	}
	log.Assert("all_equal", allEqual && len(norms) >= 3, true, len(norms))
	log.PhaseEnd("compare", testutil.OutcomeOK)
}

// TestHostileInputs covers include/env/expression plain-data acceptance and
// nested unknown table rejection (Section 14.4).
func TestHostileInputs(t *testing.T) {
	accept := []string{
		"hostile_include_like.toml",
		"hostile_env_like.toml",
		"hostile_expression_like.toml",
	}
	for _, name := range accept {
		name := name
		t.Run("accept_"+name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("decode_validate")
			data := readSpecTestdata(t, filepath.Join("errors", name))
			log.Fixture(name, "bytes="+itoa(len(data)))
			raw, err := spec.Decode(name, data)
			if err != nil {
				log.Fail("decode", err.Error())
			}
			vs, err := spec.Validate(raw)
			if err != nil {
				log.Fail("validate", "ids="+errorIDs(err)+" err="+err.Error())
			}
			log.Assert("description_preserved", strings.Contains(vs.Description(), "plain") || strings.Contains(vs.Description(), "expression"),
				"contains plain|expression", vs.Description())
			// No defaults invented for absent optionals when not set.
			log.Assert("binary_defaulted", vs.Binary() == vs.Name(), vs.Name(), vs.Binary())
			log.PhaseEnd("decode_validate", testutil.OutcomeOK)
		})
	}

	t.Run("reject_nested_unknown_table", func(t *testing.T) {
		t.Parallel()
		log := testutil.New(t)
		data := readSpecTestdata(t, "errors/nested_unknown_table.toml")
		log.Fixture("nested_unknown_table.toml", "bytes="+itoa(len(data)))
		_, err := spec.Decode("nested.toml", data)
		if err == nil {
			log.Fail("expected_error", "nested unknown table accepted")
		}
		fe, ok := diagnostic.AsFoundryError(err)
		if !ok {
			log.Fail("foundry_error", errString(err))
		}
		log.NoteID(string(fe.ID()))
		log.Assert("id", fe.ID() == diagnostic.IDSpecUnknownField, diagnostic.IDSpecUnknownField, fe.ID())
	})

	t.Run("reject_env_destination", func(t *testing.T) {
		t.Parallel()
		log := testutil.New(t)
		data := readSpecTestdata(t, "errors/hostile_env_destination.toml")
		raw, err := spec.Decode("envdest.toml", data)
		if err != nil {
			log.Fail("decode", err.Error())
		}
		_, err = spec.Validate(raw)
		if err == nil {
			log.Fail("expected_error", "env destination accepted")
		}
		errs := spec.CollectFoundryErrors(err)
		log.Step("error_ids", testutil.OutcomeOK, "ids="+errorIDs(err))
		if len(errs) == 0 || errs[0].ID() != diagnostic.IDSpecInvalidField {
			log.Fail("want_invalid_field", errorIDs(err))
		}
	})
}

// TestPipelineDeterminism ensures identical error signatures across two runs
// (supports go test -count=2 stability for both decode and validate paths).
func TestPipelineDeterminism(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		// when true, run Validate after successful Decode
		validate bool
	}{
		{
			name: "decode_duplicate_key",
			data: []byte("schema = 1\nschema = 1\n"),
		},
		{
			name: "decode_unknown_field",
			data: []byte("schema = 1\nauthor = \"x\"\n"),
		},
		{
			name:     "validate_invalid_name",
			data:     validDoc(map[string]string{"name": "BAD", "module": "github.com/example/BAD", "destination": "./BAD"}),
			validate: true,
		},
		{
			name:     "validate_multi_field",
			data:     []byte("schema = 1\nname = \"X\"\nmodule = \"not-a-module\"\ndescription = \"\"\narchetype = \"web\"\ndestination = \".\"\nprofiles = [\"a\", \"a\"]\n"),
			validate: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var first string
			for i := 0; i < 2; i++ {
				sig := pipelineErrorSig(t, tc.data, tc.validate)
				if i == 0 {
					first = sig
				} else if sig != first {
					t.Fatalf("nondeterministic: %q vs %q", first, sig)
				}
			}
		})
	}
}

// TestFieldRulesPositiveBoundary covers accept edges not only reject paths
// (REQ-210: every 14.3 field rule positive + negative — negatives live in
// validate_test; this suite adds positive boundaries + full contract walk).
func TestFieldRulesPositiveBoundary(t *testing.T) {
	log := testutil.New(t)
	// name length exactly 63
	n63 := "a" + strings.Repeat("b", 62)
	doc := validDoc(map[string]string{
		"name":               n63,
		"module":             "github.com/example/" + n63,
		"destination":        "./" + n63,
		"binary":             n63,
		"visibility":         "private",
		"git.init":           "true",
		"git.initial_branch": "main",
	})
	log.Phase("name_63")
	vs, err := spec.Validate(mustDecode(t, "n63.toml", doc))
	if err != nil {
		log.Fail("validate", err.Error())
	}
	log.Assert("name_len", len(vs.Name()) == 63, 63, len(vs.Name()))
	log.Assert("binary", vs.Binary() == n63, n63, vs.Binary())
	log.PhaseEnd("name_63", testutil.OutcomeOK)

	// description exactly 200 after trim
	log.Phase("description_200")
	d200 := strings.Repeat("d", 200)
	vs2, err := spec.Validate(mustDecode(t, "d200.toml", validDoc(map[string]string{"description": "  " + d200 + "  "})))
	if err != nil {
		log.Fail("validate", err.Error())
	}
	log.Assert("desc_len", len(vs2.Description()) == 200, 200, len(vs2.Description()))
	log.PhaseEnd("description_200", testutil.OutcomeOK)

	// absolute destination basename match
	log.Phase("absolute_dest")
	vs3, err := spec.Validate(mustDecode(t, "abs.toml", validDoc(map[string]string{
		"destination": "/var/tmp/demo-cli",
	})))
	if err != nil {
		log.Fail("validate", err.Error())
	}
	log.Assert("dest", vs3.Destination() == "/var/tmp/demo-cli", "/var/tmp/demo-cli", vs3.Destination())
	log.PhaseEnd("absolute_dest", testutil.OutcomeOK)
}

// TestExportedE2EFixturesPresent asserts fixtures exported for write-free e2e
// (P1.8.a / 5an.1) exist under cmd/foundry/testdata/specs and examples/.
func TestExportedE2EFixturesPresent(t *testing.T) {
	root := findModuleRoot(t)
	required := []string{
		// Appendix B copies for e2e + examples surface
		"examples/appendix-b-private-cli.toml",
		"examples/appendix-b-private-tui.toml",
		"examples/appendix-b-public-cli.toml",
		// Existing product fixtures
		"examples/minimal-cli.toml",
		"examples/minimal-tui.toml",
		"examples/invalid-unknown-field.toml",
		"examples/invalid-bad-name.toml",
		// e2e-oriented export tree
		"cmd/foundry/testdata/specs/valid/appendix-b-private-cli.toml",
		"cmd/foundry/testdata/specs/valid/appendix-b-private-tui.toml",
		"cmd/foundry/testdata/specs/valid/appendix-b-public-cli.toml",
		"cmd/foundry/testdata/specs/valid/minimal-cli.toml",
		"cmd/foundry/testdata/specs/invalid/parse_error.toml",
		"cmd/foundry/testdata/specs/invalid/unsupported_schema.toml",
		"cmd/foundry/testdata/specs/invalid/unknown_field.toml",
		"cmd/foundry/testdata/specs/invalid/duplicate_key.toml",
		"cmd/foundry/testdata/specs/invalid/invalid_field.toml",
		"cmd/foundry/testdata/specs/invalid/duplicate_profile.toml",
		"cmd/foundry/testdata/specs/README.md",
	}
	log := testutil.New(t)
	log.Phase("export_layout")
	for _, rel := range required {
		p := filepath.Join(root, rel)
		_, err := os.Stat(p)
		log.Assert("exists_"+filepath.Base(rel), err == nil, "exists", err)
		if err != nil {
			t.Errorf("missing exported fixture %s: %v", rel, err)
		}
	}
	log.PhaseEnd("export_layout", testutil.OutcomeOK)
}

func pipelineErrorSig(t *testing.T, data []byte, doValidate bool) string {
	t.Helper()
	raw, err := spec.Decode("det.toml", data)
	if err != nil {
		return errorSig(err)
	}
	if !doValidate {
		t.Fatalf("expected decode error for non-validate case")
	}
	_, err = spec.Validate(raw)
	if err == nil {
		t.Fatal("expected validate error")
	}
	return errorSig(err)
}

func errorSig(err error) string {
	errs := spec.CollectFoundryErrors(err)
	if len(errs) == 0 {
		return "raw:" + errString(err)
	}
	var b strings.Builder
	for i, fe := range errs {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(string(fe.ID()))
		b.WriteByte('|')
		b.WriteString(fe.Location().String())
		b.WriteByte('|')
		b.WriteString(fe.Message())
	}
	return b.String()
}

func errorIDs(err error) string {
	errs := spec.CollectFoundryErrors(err)
	ids := make([]string, len(errs))
	for i, fe := range errs {
		ids[i] = string(fe.ID())
	}
	return strings.Join(ids, ",")
}

func readSpecTestdata(t *testing.T, rel string) []byte {
	t.Helper()
	p := filepath.Join("testdata", rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read testdata %s: %v", p, err)
	}
	return b
}
