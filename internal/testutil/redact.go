package testutil

import (
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// RedactedSentinel is the failure-dump replacement marker. It matches the
// product diagnostic sentinel so agent/log scrapers see one consistent form.
const RedactedSentinel = diagnostic.RedactedSentinel

// RedactSecrets replaces secret-like values in s for test failure dumps.
// Delegates to diagnostic.Redact so Bearer/Basic headers, env assigns, and
// proxy userinfo use the same rules as product diagnostics (never leaves
// tokens in place after a marker insertion).
// Safe for empty/short strings; never panics.
func RedactSecrets(s string) string {
	return diagnostic.Redact(s)
}

// LooksSecret reports whether key (env or field name) should be treated as
// sensitive and never emitted in dumps.
func LooksSecret(key string) bool {
	return diagnostic.LooksSecret(key)
}
