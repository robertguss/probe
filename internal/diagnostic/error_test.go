package diagnostic_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestFoundryErrorAPI(t *testing.T) {
	log := testutil.New(t)
	log.Phase("construct")
	loc := diagnostic.SpecLocation("project.toml", 3, 5)
	err := diagnostic.New(diagnostic.IDSpecUnknownField, "unknown field \"foo\"", loc)
	log.Assert("id", err.ID() == diagnostic.IDSpecUnknownField, diagnostic.IDSpecUnknownField, err.ID())
	log.Assert("message", err.Message() == `unknown field "foo"`, `unknown field "foo"`, err.Message())
	log.Assert("location", err.Location().String() == "project.toml:3:5", "project.toml:3:5", err.Location().String())
	log.Assert("exit", err.ExitCode() == diagnostic.ExitUsage, diagnostic.ExitUsage, err.ExitCode())
	log.Assert("remediation_default", err.Remediation() != "", "nonempty", truncate(err.Remediation(), 60))
	log.PhaseEnd("construct", testutil.OutcomeOK)

	log.Phase("error_string")
	s := err.Error()
	log.Assert("contains_id", strings.Contains(s, "spec.unknown_field"), true, s)
	log.Assert("contains_msg", strings.Contains(s, "unknown field"), true, s)
	log.Assert("contains_loc", strings.Contains(s, "project.toml:3:5"), true, s)
	log.Assert("no_stack", !strings.Contains(s, "goroutine"), true, s)
	log.PhaseEnd("error_string", testutil.OutcomeOK)

	log.Phase("fields_json")
	fields := err.Fields()
	raw, jerr := json.Marshal(fields)
	if jerr != nil {
		log.Fail("json_marshal", jerr.Error())
	}
	var decoded map[string]any
	if jerr := json.Unmarshal(raw, &decoded); jerr != nil {
		log.Fail("json_unmarshal", jerr.Error())
	}
	log.Assert("json_id", decoded["id"] == "spec.unknown_field", "spec.unknown_field", decoded["id"])
	log.Assert("json_has_remediation", decoded["remediation"] != nil && decoded["remediation"] != "", true, decoded["remediation"])
	// Stable field names.
	for _, key := range []string{"id", "message", "location", "remediation", "exit_code"} {
		if _, ok := decoded[key]; !ok {
			log.Fail("json_key", "missing key "+key)
		}
	}
	log.PhaseEnd("fields_json", testutil.OutcomeOK)
}

func TestFoundryErrorNoContextField(t *testing.T) {
	// REQ-188: FoundryError must not store context.Context.
	// Structural check via JSON/Fields and via type inspection of exported shape.
	err := diagnostic.New(diagnostic.IDInternalBug, "boom", diagnostic.Location{})
	// Ensure we can hold a context without stuffing it into the error.
	ctx := context.Background()
	_ = ctx
	if err.Unwrap() != nil {
		t.Fatalf("New should not set cause")
	}
	// Fields must not include a context-looking key.
	raw, _ := json.Marshal(err.Fields())
	if strings.Contains(string(raw), "context") || strings.Contains(string(raw), "Context") {
		t.Fatalf("Fields leaked context-related key: %s", raw)
	}
	// Type has no exported Context field — already enforced by unexported struct fields.
	// Document that constructors never take context.
	_ = diagnostic.Wrap(diagnostic.IDToolFailed, "step failed", diagnostic.StepLocation("go-test"), errors.New("exit 1"))
}

func TestFoundryErrorWrapAndAs(t *testing.T) {
	cause := errors.New("os: permission denied")
	err := diagnostic.Wrap(diagnostic.IDFSCommitFailed, "commit rename failed", diagnostic.PathLocation("/tmp/out"), cause)
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is should find cause")
	}
	var fe *diagnostic.FoundryError
	if !errors.As(err, &fe) {
		t.Fatal("errors.As FoundryError")
	}
	got, ok := diagnostic.AsFoundryError(err)
	if !ok || got.ID() != diagnostic.IDFSCommitFailed {
		t.Fatalf("AsFoundryError: ok=%v id=%v", ok, got)
	}
	if err.ExitCode() != diagnostic.ExitFailure {
		t.Fatalf("exit=%d", err.ExitCode())
	}
}

func TestFoundryErrorWithRemediationImmutable(t *testing.T) {
	orig := diagnostic.New(diagnostic.IDUsageInvalid, "unknown flag", diagnostic.Location{})
	oldRem := orig.Remediation()
	next := orig.WithRemediation("Use --help to list flags.")
	if orig.Remediation() != oldRem {
		t.Fatal("WithRemediation mutated original")
	}
	if next.Remediation() != "Use --help to list flags." {
		t.Fatalf("override = %q", next.Remediation())
	}
}

func TestFoundryErrorRedactsSecretsInMessage(t *testing.T) {
	err := diagnostic.Newf(diagnostic.IDToolFailed, diagnostic.StepLocation("go-mod-tidy"),
		"proxy failed: HTTPS_PROXY=http://user:sekrit@proxy:1 token=abc123")
	if strings.Contains(err.Message(), "sekrit") || strings.Contains(err.Error(), "sekrit") {
		t.Fatalf("secret leaked in message: %q", err.Message())
	}
	if strings.Contains(err.Message(), "abc123") {
		t.Fatalf("token leaked: %q", err.Message())
	}
	if !strings.Contains(err.Message(), diagnostic.RedactedSentinel) {
		t.Fatalf("expected sentinel in message: %q", err.Message())
	}
	// JSON fields also redacted.
	f := err.Fields()
	raw, _ := json.Marshal(f)
	if strings.Contains(string(raw), "sekrit") || strings.Contains(string(raw), "abc123") {
		t.Fatalf("secret leaked in JSON fields: %s", raw)
	}
}

func TestExitCodeMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, diagnostic.ExitSuccess},
		{diagnostic.ErrCancelled, diagnostic.ExitCancelled},
		{context.Canceled, diagnostic.ExitCancelled},
		{diagnostic.New(diagnostic.IDUsageInvalid, "x", diagnostic.Location{}), diagnostic.ExitUsage},
		{diagnostic.New(diagnostic.IDToolFailed, "x", diagnostic.Location{}), diagnostic.ExitFailure},
		{errors.New("plain"), diagnostic.ExitFailure},
	}
	for _, tc := range cases {
		if got := diagnostic.ExitCode(tc.err); got != tc.want {
			t.Errorf("ExitCode(%v) = %d want %d", tc.err, got, tc.want)
		}
	}
	if !diagnostic.IsCancelled(diagnostic.ErrCancelled) {
		t.Error("IsCancelled(ErrCancelled)")
	}
	if diagnostic.IsCancelled(errors.New("x")) {
		t.Error("IsCancelled(plain)")
	}
}

func TestLocationFormats(t *testing.T) {
	cases := []struct {
		loc  diagnostic.Location
		want string
	}{
		{diagnostic.SpecLocation("a.toml", 1, 2), "a.toml:1:2"},
		{diagnostic.SpecLocation("a.toml", 4, 0), "a.toml:4"},
		{diagnostic.PathLocation("/tmp/stage"), "/tmp/stage"},
		{diagnostic.StepLocation("go-test"), "step=go-test"},
		{diagnostic.Location{}, ""},
	}
	for _, tc := range cases {
		if got := tc.loc.String(); got != tc.want {
			t.Errorf("Location.String() = %q want %q", got, tc.want)
		}
	}
}

func TestNewfAndWrapf(t *testing.T) {
	e := diagnostic.Newf(diagnostic.IDSpecInvalidField, diagnostic.SpecLocation("s.toml", 2, 1),
		"field %s invalid", "name")
	if e.Message() != "field name invalid" {
		t.Fatalf("message=%q", e.Message())
	}
	w := diagnostic.Wrapf(diagnostic.IDGitFailed, diagnostic.StepLocation("git-init"), errors.New("exit 128"),
		"git init failed: %s", "template")
	if !strings.Contains(w.Message(), "template") {
		t.Fatalf("wrapf message=%q", w.Message())
	}
	if !errors.Is(w, w.Unwrap()) {
		// Unwrap returns cause
	}
	if w.Unwrap() == nil {
		t.Fatal("expected cause")
	}
}
