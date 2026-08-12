package testutil_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// FuzzRedactSecrets ensures RedactSecrets never panics and assignment-style
// secret seeds lose their values (ipk.14).
func FuzzRedactSecrets(f *testing.F) {
	f.Add("password=s3cr3t")
	f.Add("API_KEY=abcd1234")
	f.Add("GITHUB_TOKEN=ghp_xxx")
	f.Add("Authorization: Bearer super-token")
	f.Add("normal=ok path=/tmp/x")
	f.Add("")
	f.Fuzz(func(t *testing.T, in string) {
		out := testutil.RedactSecrets(in)
		// Assignment / Bearer forms from seeds must not retain the secret value.
		seeds := []struct {
			prefix string
			secret string
		}{
			{"password=", "s3cr3t"},
			{"API_KEY=", "abcd1234"},
			{"GITHUB_TOKEN=", "ghp_xxx"},
			{"Bearer ", "super-token"},
		}
		for _, s := range seeds {
			if strings.Contains(in, s.prefix+s.secret) && strings.Contains(out, s.secret) {
				t.Fatalf("redaction leaked %q under %q: in=%q out=%q", s.secret, s.prefix, in, out)
			}
		}
		_ = len(out)
	})
}
