package diagnostic_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

func TestRedactSentinels(t *testing.T) {
	cases := []struct {
		name string
		in   string
		hide []string
	}{
		{"password", "password=s3cr3t", []string{"s3cr3t"}},
		{"PASSWORD_env", "DB_PASSWORD=hunter2", []string{"hunter2"}},
		{"token", "API_TOKEN=tok_abc", []string{"tok_abc"}},
		{"secret", "CLIENT_SECRET=shh", []string{"shh"}},
		{"bearer", "Bearer aabbccdd", []string{"aabbccdd"}},
		{"basic", "Basic dXNlcjpwYXNz", []string{"dXNlcjpwYXNz"}},
		{"proxy_url", "http://user:p4ssw0rd@host:8080/path", []string{"p4ssw0rd", "user:p4ssw0rd"}},
		{"https_proxy", "https://alice:bob@proxy.internal:3128", []string{"bob", "alice:bob"}},
		{"mixed", "ok=1 GITHUB_TOKEN=ghp_xxx PATH=/bin", []string{"ghp_xxx"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := diagnostic.Redact(tc.in)
			for _, h := range tc.hide {
				if strings.Contains(out, h) {
					// Log key/case name only — never the secret value.
					t.Errorf("redaction failed for case %s: secret fragment still present", tc.name)
				}
			}
			if !strings.Contains(out, diagnostic.RedactedSentinel) && len(tc.hide) > 0 {
				t.Errorf("case %s: missing sentinel %q in %q", tc.name, diagnostic.RedactedSentinel, out)
			}
		})
	}
}

func TestRedactURL(t *testing.T) {
	in := "https://user:secret@proxy.example:8080/x"
	out := diagnostic.RedactURL(in)
	if strings.Contains(out, "secret") || strings.Contains(out, "user") {
		// username may be redacted too when password present
		if strings.Contains(out, "secret") {
			t.Fatalf("password leaked: %q", out)
		}
	}
	if !strings.Contains(out, diagnostic.RedactedSentinel) {
		t.Fatalf("expected sentinel: %q", out)
	}
	// Host path preserved.
	if !strings.Contains(out, "proxy.example") {
		t.Fatalf("host lost: %q", out)
	}
}

func TestRedactEnv(t *testing.T) {
	env := map[string]string{
		"HOME":         "/home/rob",
		"GITHUB_TOKEN": "should-not-appear",
		"MY_PASSWORD":  "pw",
		"HTTPS_PROXY":  "http://u:p@proxy:1",
		"GOPROXY":      "https://user:tok@proxy.golang.org,direct",
		"NORMAL":       "ok",
	}
	out := diagnostic.RedactEnv(env)
	if out["HOME"] != "/home/rob" || out["NORMAL"] != "ok" {
		t.Fatalf("non-secret mutated: %#v", out)
	}
	if out["GITHUB_TOKEN"] != diagnostic.RedactedSentinel {
		t.Fatalf("token: %q", out["GITHUB_TOKEN"])
	}
	if out["MY_PASSWORD"] != diagnostic.RedactedSentinel {
		t.Fatalf("password: %q", out["MY_PASSWORD"])
	}
	if strings.Contains(out["HTTPS_PROXY"], ":p@") || strings.Contains(out["HTTPS_PROXY"], "u:p") {
		t.Fatalf("proxy userinfo leaked: %q", out["HTTPS_PROXY"])
	}
	if strings.Contains(out["GOPROXY"], "tok") {
		t.Fatalf("goproxy userinfo leaked: %q", out["GOPROXY"])
	}
	// JSON form of env dump must not leak.
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"should-not-appear", "\"pw\"", "user:tok"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("JSON env dump leaked %q: %s", leak, raw)
		}
	}
}

func TestLooksSecret(t *testing.T) {
	yes := []string{"PASSWORD", "DB_PASSWORD", "GITHUB_TOKEN", "API_SECRET", "MY_API_KEY", "PRIVATE_KEY"}
	no := []string{"PATH", "HOME", "USER", "FOUNDRY_OUT", "GOFLAGS"}
	for _, k := range yes {
		if !diagnostic.LooksSecret(k) {
			t.Errorf("LooksSecret(%q) = false", k)
		}
	}
	for _, k := range no {
		if diagnostic.LooksSecret(k) {
			t.Errorf("LooksSecret(%q) = true", k)
		}
	}
}

func TestRedactEmptyAndIdempotent(t *testing.T) {
	if diagnostic.Redact("") != "" {
		t.Fatal("empty")
	}
	if diagnostic.RedactEnv(nil) != nil {
		t.Fatal("nil env")
	}
	s := "TOKEN=abc"
	once := diagnostic.Redact(s)
	twice := diagnostic.Redact(once)
	if once != twice {
		t.Fatalf("not idempotent: %q vs %q", once, twice)
	}
}
