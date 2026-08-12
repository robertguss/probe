package diagnostic_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestNilFoundryErrorAccessors(t *testing.T) {
	log := testutil.New(t)
	log.Phase("nil_accessors")

	var fe *diagnostic.FoundryError
	log.Assert("id", fe.ID() == "", true, fe.ID())
	log.Assert("msg", fe.Message() == "", true, fe.Message())
	log.Assert("loc_zero", fe.Location().IsZero(), true, false)
	log.Assert("rem", fe.Remediation() == "", true, fe.Remediation())
	log.Assert("exit", fe.ExitCode() == diagnostic.ExitFailure, true, fe.ExitCode())
	log.Assert("err_str", fe.Error() == "foundry: <nil>", true, fe.Error())
	log.Assert("unwrap", fe.Unwrap() == nil, true, fe.Unwrap() != nil)
	log.Assert("fields_empty", fe.Fields().ID == "", true, fe.Fields().ID)
	log.Assert("with_rem_nil", fe.WithRemediation("x") == nil, true, false)

	got, ok := diagnostic.AsFoundryError(nil)
	log.Assert("as_nil", !ok && got == nil, true, ok)
	got, ok = diagnostic.AsFoundryError(errors.New("plain"))
	log.Assert("as_plain", !ok, true, ok)

	// Remediation fallback when empty after construct with unknown id:
	// Remediation() returns RemediationFor(id), which is empty for unknown ids.
	unk := diagnostic.New(diagnostic.Identifier("not.a.real.id"), "msg", diagnostic.Location{})
	log.Assert("unk_exit", unk.ExitCode() == diagnostic.ExitFailure, true, unk.ExitCode())
	log.Assert("unk_rem_empty", unk.Remediation() == "", "", unk.Remediation())
	log.Assert("unk_rem_registry", diagnostic.RemediationFor(diagnostic.Identifier("not.a.real.id")) == "", "", diagnostic.RemediationFor(diagnostic.Identifier("not.a.real.id")))

	// Error() without location and without message branches
	onlyID := diagnostic.New(diagnostic.IDInternalBug, "", diagnostic.Location{})
	s := onlyID.Error()
	log.Assert("only_id", s == string(diagnostic.IDInternalBug), true, s)

	withMsg := diagnostic.New(diagnostic.IDInternalBug, "boom", diagnostic.Location{})
	log.Assert("id_msg", strings.Contains(withMsg.Error(), "boom"), true, withMsg.Error())

	log.PhaseEnd("nil_accessors", testutil.OutcomeOK)
}

func TestLocationIsZeroEqual(t *testing.T) {
	t.Parallel()
	log := testutil.New(t)
	log.Phase("location")

	z := diagnostic.Location{}
	log.Assert("zero", z.IsZero(), true, false)
	log.Assert("zero_str", z.String() == "", true, z.String())
	log.Assert("equal_zero", z.Equal(diagnostic.Location{}), true, false)

	f := diagnostic.SpecLocation("a.toml", 0, 0)
	log.Assert("file_only", f.String() == "a.toml", true, f.String())
	log.Assert("not_zero", !f.IsZero(), true, false)

	fl := diagnostic.SpecLocation("a.toml", 3, 0)
	log.Assert("file_line", fl.String() == "a.toml:3", true, fl.String())

	flc := diagnostic.SpecLocation("a.toml", 3, 4)
	log.Assert("file_line_col", flc.String() == "a.toml:3:4", true, flc.String())
	log.Assert("equal_self", flc.Equal(flc), true, false)
	log.Assert("ne", !flc.Equal(fl), true, false)

	p := diagnostic.PathLocation("/tmp/x")
	log.Assert("path", p.String() == "/tmp/x", true, p.String())
	st := diagnostic.StepLocation("go-test")
	log.Assert("step", st.String() == "step=go-test", true, st.String())

	log.PhaseEnd("location", testutil.OutcomeOK)
}

func TestRegistryEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("registry_edges")

	e := diagnostic.MustLookup(diagnostic.IDSpecInvalidField)
	log.Assert("must_id", e.ID == diagnostic.IDSpecInvalidField, true, e.ID)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustLookup unknown should panic")
		}
	}()
	_ = diagnostic.MustLookup(diagnostic.Identifier("totally.unknown.id.xyz"))
}

func TestRegistryExitRemediationUnknown(t *testing.T) {
	log := testutil.New(t)
	log.Phase("registry_unknown")

	unk := diagnostic.Identifier("no.such.id")
	log.Assert("exit_unk", diagnostic.ExitCodeFor(unk) == diagnostic.ExitFailure, true, diagnostic.ExitCodeFor(unk))
	log.Assert("rem_unk", diagnostic.RemediationFor(unk) == "", true, diagnostic.RemediationFor(unk))
	log.Assert("known_false", !diagnostic.Known(unk), true, false)
	_, ok := diagnostic.Lookup(unk)
	log.Assert("lookup_false", !ok, true, ok)

	// MissingIDs / ExtraIDs with partial want set
	want := []diagnostic.Identifier{diagnostic.IDSpecInvalidField, diagnostic.Identifier("missing.from.registry")}
	miss := diagnostic.MissingIDs(want)
	log.Assert("missing_one", len(miss) == 1 && miss[0] == "missing.from.registry", true, miss)
	// empty missing
	log.Assert("missing_none", len(diagnostic.MissingIDs([]diagnostic.Identifier{diagnostic.IDSpecInvalidField})) == 0, true, false)

	extra := diagnostic.ExtraIDs([]diagnostic.Identifier{diagnostic.IDSpecInvalidField})
	log.Assert("extra_many", len(extra) > 0, true, len(extra))
	// full set → no extras
	all := diagnostic.AllIdentifiers()
	log.Assert("extra_none", len(diagnostic.ExtraIDs(all)) == 0, true, len(diagnostic.ExtraIDs(all)))
	// MissingIDs empty want
	log.Assert("miss_empty_want", len(diagnostic.MissingIDs(nil)) == 0, true, false)

	log.PhaseEnd("registry_unknown", testutil.OutcomeOK)
}

func TestRedactEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("redact_edges")

	// LooksSecret edges
	log.Assert("empty_key", !diagnostic.LooksSecret(""), true, false)
	log.Assert("space_key", !diagnostic.LooksSecret("   "), true, false)
	log.Assert("password", diagnostic.LooksSecret("password"), true, false)
	log.Assert("my_token", diagnostic.LooksSecret("MY_TOKEN"), true, false)
	log.Assert("http_proxy", diagnostic.LooksSecret("HTTP_PROXY"), true, false)
	log.Assert("goproxy_not_secret_key", !diagnostic.LooksSecret("GOPROXY"), true, false)
	log.Assert("no_proxy_not", !diagnostic.LooksSecret("NO_PROXY"), true, false)
	log.Assert("home", !diagnostic.LooksSecret("HOME"), true, false)

	// RedactURL edges
	log.Assert("url_empty", diagnostic.RedactURL("") == "", true, false)
	log.Assert("url_non_url", strings.Contains(diagnostic.RedactURL("password=x"), diagnostic.RedactedSentinel) || diagnostic.RedactURL("password=x") != "password=x", true, diagnostic.RedactURL("password=x"))
	log.Assert("url_no_userinfo", diagnostic.RedactURL("https://example.com/a") == "https://example.com/a", true, diagnostic.RedactURL("https://example.com/a"))
	// user only
	u := diagnostic.RedactURL("https://user@host/path")
	log.Assert("url_user", strings.Contains(u, diagnostic.RedactedSentinel) && strings.Contains(u, "host"), true, u)
	// user+pass
	up := diagnostic.RedactURL("https://user:pass@host/path")
	log.Assert("url_userpass", strings.Contains(up, diagnostic.RedactedSentinel) && !strings.Contains(up, "pass"), true, up)

	// RedactEnv nil
	log.Assert("env_nil", diagnostic.RedactEnv(nil) == nil, true, false)

	// RedactEnvValue empty + non-secret content scrub
	log.Assert("env_empty", diagnostic.RedactEnvValue("HOME", "") == "", true, false)
	v := diagnostic.RedactEnvValue("NOTE", "Bearer supersecrettoken")
	log.Assert("env_bearer_scrub", strings.Contains(v, diagnostic.RedactedSentinel) && !strings.Contains(v, "supersecrettoken"), true, v)
	log.Assert("env_proxy", strings.Contains(diagnostic.RedactEnvValue("HTTPS_PROXY", "http://u:p@proxy:1"), diagnostic.RedactedSentinel), true, diagnostic.RedactEnvValue("HTTPS_PROXY", "http://u:p@proxy:1"))
	log.Assert("env_secret_key", diagnostic.RedactEnvValue("API_KEY", "xyz") == diagnostic.RedactedSentinel, true, diagnostic.RedactEnvValue("API_KEY", "xyz"))

	// Bearer / Basic in free text
	r := diagnostic.Redact("Authorization: Bearer abc.def Authorization: Basic dXNlcjpwYXNz")
	log.Assert("bearer_basic", strings.Contains(r, diagnostic.RedactedSentinel) && !strings.Contains(r, "abc.def"), true, r)

	// URL in free text
	r2 := diagnostic.Redact("clone https://user:tok@github.com/org/repo.git now")
	log.Assert("url_in_text", !strings.Contains(r2, "tok") && strings.Contains(r2, diagnostic.RedactedSentinel), true, r2)

	log.PhaseEnd("redact_edges", testutil.OutcomeOK)
}

func TestExitCodeAndCancelledEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("exit_edges")

	log.Assert("nil_ok", diagnostic.ExitCode(nil) == diagnostic.ExitSuccess, true, diagnostic.ExitCode(nil))
	log.Assert("cancel_direct", diagnostic.ExitCode(diagnostic.ErrCancelled) == diagnostic.ExitCancelled, true, false)
	log.Assert("cancel_ctx", diagnostic.ExitCode(context.Canceled) == diagnostic.ExitCancelled, true, false)
	log.Assert("cancel_wrap", diagnostic.ExitCode(fmt.Errorf("w: %w", diagnostic.ErrCancelled)) == diagnostic.ExitCancelled, true, false)
	log.Assert("is_cancel_nil", !diagnostic.IsCancelled(nil), true, false)
	log.Assert("is_cancel_ctx", diagnostic.IsCancelled(context.Canceled), true, false)
	log.Assert("is_cancel_wrap", diagnostic.IsCancelled(fmt.Errorf("x: %w", context.Canceled)), true, false)

	// FoundryError exit via ExitCode()
	fe := diagnostic.New(diagnostic.IDUsageInvalid, "x", diagnostic.Location{})
	log.Assert("usage_exit", diagnostic.ExitCode(fe) == diagnostic.ExitUsage, true, diagnostic.ExitCode(fe))

	log.PhaseEnd("exit_edges", testutil.OutcomeOK)
}
