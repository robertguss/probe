package report_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// Fixed fixtures for success goldens (host-independent).
var (
	fixtureVersion = map[string]string{
		"version":        "0.1.0-test",
		"commit":         "abc1234",
		"go":             "go1.26.5",
		"catalog_digest": "deadbeefcafebabe000000000000000000000000000000000000000000000000",
	}
	fixtureCatalogList = map[string]any{
		"catalog_digest": fixtureVersion["catalog_digest"],
		"units": []map[string]string{
			{"id": "cli", "kind": "archetype", "description": "CLI archetype", "manifest_path": "archetypes/cli/manifest.toml"},
			{"id": "core", "kind": "core", "description": "Core unit", "manifest_path": "core/manifest.toml"},
		},
	}
	fixtureValidate = map[string]any{
		"status": "ok",
		"spec":   "foundry.toml",
	}
	fixturePlan = map[string]any{
		"schema":  1,
		"project": map[string]string{"name": "demo-cli", "module": "example.com/demo-cli"},
		"files":   []string{"go.mod", "cmd/demo-cli/main.go"},
	}
)

func TestGoldenValidateSuccess(t *testing.T) {
	log := testutil.New(t)
	log.Phase("validate_success")

	// Text
	enc, cap := newEnc(t, textOpts())
	if err := enc.Success("validate", fixtureValidate, report.FormatValidateText("foundry.toml")); err != nil {
		log.Fail("text_write", err.Error())
	}
	log.Assert("text_stderr_empty", cap.Stderr.String() == "", "", cap.Stderr.String())
	compareGolden(t, log, "validate_success_text", cap.Stdout.String())

	// JSON
	enc, cap = newEnc(t, jsonOpts())
	if err := enc.Success("validate", fixtureValidate, ""); err != nil {
		log.Fail("json_write", err.Error())
	}
	log.Assert("json_stderr_empty", cap.Stderr.String() == "", "", cap.Stderr.String())
	// Raw encoder bytes preserve struct field order (Section 37 determinism).
	compareGolden(t, log, "validate_success_json", cap.Stdout.String())

	log.PhaseEnd("validate_success", testutil.OutcomeOK)
}

func TestGoldenValidateFailure(t *testing.T) {
	log := testutil.New(t)
	log.Phase("validate_failure")

	err := diagnostic.New(
		diagnostic.IDSpecUnknownField,
		`unknown field "offline"`,
		diagnostic.SpecLocation("foundry.toml", 4, 1),
	)

	// Text
	enc, cap := newEnc(t, textOpts())
	code, werr := enc.Failure("validate", err, "")
	log.Assert("text_write_nil", werr == nil, true, werr)
	log.Assert("text_exit", code == diagnostic.ExitUsage, diagnostic.ExitUsage, code)
	log.Assert("text_stdout_empty", cap.Stdout.String() == "", "", cap.Stdout.String())
	compareGolden(t, log, "validate_failure_text", cap.Stderr.String())

	// JSON
	enc, cap = newEnc(t, jsonOpts())
	code, werr = enc.Failure("validate", err, "")
	log.Assert("json_write_nil", werr == nil, true, werr)
	log.Assert("json_exit", code == diagnostic.ExitUsage, diagnostic.ExitUsage, code)
	log.Assert("json_stderr_empty", cap.Stderr.String() == "", "", cap.Stderr.String())
	compareGolden(t, log, "validate_failure_json", cap.Stdout.String())

	env := parseEnvelope(t, cap.Stdout.String())
	obj := errorObject(t, env)
	log.Assert("error_id", obj["error_id"] == "spec.unknown_field", "spec.unknown_field", obj["error_id"])
	log.Assert("remediation", obj["remediation"] != nil && obj["remediation"] != "", true, obj["remediation"])
	log.Assert("path", obj["path"] == "foundry.toml", "foundry.toml", obj["path"])
	log.Assert("line", obj["line"] == float64(4), 4, obj["line"])
	log.Assert("col", obj["col"] == float64(1), 1, obj["col"])
	// Struct key order: schema before command before ok (not map-alpha).
	raw := cap.Stdout.String()
	log.Assert("key_order_schema_first", strings.HasPrefix(strings.TrimSpace(raw), `{"schema":`), true, raw)

	log.PhaseEnd("validate_failure", testutil.OutcomeOK)
}

func TestGoldenPlanSuccess(t *testing.T) {
	log := testutil.New(t)
	log.Phase("plan_success")

	text := report.FormatPlanText("demo-cli", "./demo-cli", 2)
	enc, cap := newEnc(t, textOpts())
	_ = enc.Success("plan", fixturePlan, text)
	compareGolden(t, log, "plan_success_text", cap.Stdout.String())

	enc, cap = newEnc(t, jsonOpts())
	_ = enc.Success("plan", fixturePlan, "")
	compareGolden(t, log, "plan_success_json", cap.Stdout.String())

	log.PhaseEnd("plan_success", testutil.OutcomeOK)
}

func TestGoldenPlanFailure(t *testing.T) {
	log := testutil.New(t)
	log.Phase("plan_failure")

	err := diagnostic.New(
		diagnostic.IDPlanFileCollision,
		`file "go.mod" claimed by core and cli`,
		diagnostic.PathLocation("go.mod"),
	)

	enc, cap := newEnc(t, textOpts())
	code, _ := enc.Failure("plan", err, "")
	log.Assert("exit", code == diagnostic.ExitFailure, diagnostic.ExitFailure, code)
	compareGolden(t, log, "plan_failure_text", cap.Stderr.String())

	enc, cap = newEnc(t, jsonOpts())
	_, _ = enc.Failure("plan", err, "")
	compareGolden(t, log, "plan_failure_json", cap.Stdout.String())

	log.PhaseEnd("plan_failure", testutil.OutcomeOK)
}

func TestGoldenVersionSuccess(t *testing.T) {
	log := testutil.New(t)
	log.Phase("version_success")

	text := report.FormatVersionText(
		fixtureVersion["version"],
		fixtureVersion["commit"],
		fixtureVersion["go"],
		fixtureVersion["catalog_digest"],
	)
	enc, cap := newEnc(t, textOpts())
	_ = enc.Success("version", fixtureVersion, text)
	compareGolden(t, log, "version_success_text", cap.Stdout.String())

	enc, cap = newEnc(t, jsonOpts())
	_ = enc.Success("version", fixtureVersion, "")
	compareGolden(t, log, "version_success_json", cap.Stdout.String())

	log.PhaseEnd("version_success", testutil.OutcomeOK)
}

func TestGoldenCatalogSuccess(t *testing.T) {
	log := testutil.New(t)
	log.Phase("catalog_success")

	text := "cli\tarchetype\tCLI archetype\ncore\tcore\tCore unit"
	enc, cap := newEnc(t, textOpts())
	_ = enc.Success("catalog list", fixtureCatalogList, text)
	compareGolden(t, log, "catalog_list_success_text", cap.Stdout.String())

	enc, cap = newEnc(t, jsonOpts())
	_ = enc.Success("catalog list", fixtureCatalogList, "")
	compareGolden(t, log, "catalog_list_success_json", cap.Stdout.String())

	log.PhaseEnd("catalog_success", testutil.OutcomeOK)
}

func TestGoldenCatalogFailure(t *testing.T) {
	log := testutil.New(t)
	log.Phase("catalog_failure")

	err := diagnostic.New(
		diagnostic.IDCatalogInvalid,
		`catalog unit "nope" not found; available units: [cli, core]`,
		diagnostic.PathLocation("nope"),
	).WithRemediation(
		"Use an exact unit id from the available set [cli, core]. " +
			"Run `foundry catalog list` to inspect embedded catalog units.",
	)

	enc, cap := newEnc(t, textOpts())
	code, _ := enc.Failure("catalog show", err, "")
	log.Assert("exit", code == diagnostic.ExitFailure, diagnostic.ExitFailure, code)
	compareGolden(t, log, "catalog_failure_text", cap.Stderr.String())

	enc, cap = newEnc(t, jsonOpts())
	_, _ = enc.Failure("catalog show", err, "")
	compareGolden(t, log, "catalog_failure_json", cap.Stdout.String())

	env := parseEnvelope(t, cap.Stdout.String())
	obj := errorObject(t, env)
	log.Assert("error_id", obj["error_id"] == "catalog.invalid", "catalog.invalid", obj["error_id"])
	log.Assert("remediation", obj["remediation"] != "", true, obj["remediation"])

	log.PhaseEnd("catalog_failure", testutil.OutcomeOK)
}
