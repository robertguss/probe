package testutil_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestRedactSecrets(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		hide string // substring that must not appear after redaction
	}{
		{"password=s3cr3t", "s3cr3t"},
		{"API_KEY=abcd1234", "abcd1234"},
		{"GITHUB_TOKEN=ghp_xxx", "ghp_xxx"},
		{"Authorization: bearer-value", "bearer-value"},
		{"Authorization: Bearer super-token-xyz", "super-token-xyz"},
		{"Bearer aabbccdd", "aabbccdd"},
		{"Authorization: Basic dXNlcjpwYXNz", "dXNlcjpwYXNz"},
		{"secret: hunter2", "hunter2"},
		{"normal=ok path=/tmp/x", ""}, // no hide required
	}
	for _, tc := range cases {
		tc := tc
		name := tc.in
		if len(name) > 40 {
			name = name[:40]
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out := testutil.RedactSecrets(tc.in)
			if tc.hide != "" && strings.Contains(out, tc.hide) {
				t.Errorf("RedactSecrets(%q) still contains %q → %q", tc.in, tc.hide, out)
			}
			if tc.hide != "" && !strings.Contains(out, testutil.RedactedSentinel) {
				t.Errorf("RedactSecrets(%q) missing redaction marker %q → %q", tc.in, testutil.RedactedSentinel, out)
			}
			// Bearer scheme must replace the token, not prepend a marker in front of it.
			if strings.Contains(tc.in, "Bearer ") && strings.Contains(out, "Bearer ") {
				if !strings.Contains(out, "Bearer "+testutil.RedactedSentinel) {
					t.Errorf("RedactSecrets(%q) did not redact Bearer form → %q", tc.in, out)
				}
			}
		})
	}
}

func TestLooksSecret(t *testing.T) {
	secrets := []string{"PASSWORD", "API_KEY", "GH_TOKEN", "private_key", "credential"}
	for _, k := range secrets {
		k := k
		t.Run("secret_"+k, func(t *testing.T) {
			if !testutil.LooksSecret(k) {
				t.Errorf("LooksSecret(%q)=false", k)
			}
		})
	}
	safe := []string{"PATH", "HOME", "GOCACHE", "spec", "archetype"}
	for _, k := range safe {
		k := k
		t.Run("safe_"+k, func(t *testing.T) {
			if testutil.LooksSecret(k) {
				t.Errorf("LooksSecret(%q)=true", k)
			}
		})
	}
}

func TestFailureDumpRedactsSecrets(t *testing.T) {
	log := testutil.New(t)
	log.Step("env", testutil.OutcomeFail, "GITHUB_TOKEN=should-not-leak PASSWORD=nope")
	// Inspect recorded detail is stored raw; DumpLast redacts on emit.
	// We verify RedactSecrets on the stored detail as DumpLast does.
	for _, s := range log.Steps() {
		redacted := testutil.RedactSecrets(s.Detail)
		if strings.Contains(redacted, "should-not-leak") || strings.Contains(redacted, "nope") {
			t.Fatalf("dump would leak secrets: %q", redacted)
		}
	}
	log.DumpLast() // must not panic; redaction applied in log lines
}
