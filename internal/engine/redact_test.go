package engine

import (
	"strings"
	"testing"
)

func TestRedactHeaders(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want map[string]string
	}{
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty",
			in:   map[string]string{},
			want: map[string]string{},
		},
		{
			name: "authorization cookie set-cookie",
			in: map[string]string{
				"Authorization": "Bearer secret-token",
				"Cookie":        "session=abc",
				"Set-Cookie":    "session=abc; HttpOnly",
				"Accept":        "application/json",
			},
			want: map[string]string{
				"Authorization": "[REDACTED]",
				"Cookie":        "[REDACTED]",
				"Set-Cookie":    "[REDACTED]",
				"Accept":        "application/json",
			},
		},
		{
			name: "case insensitive names",
			in: map[string]string{
				"authorization": "Bearer x",
				"COOKIE":        "a=b",
				"set-cookie":    "a=b",
				"X-Request-Id":  "1",
			},
			want: map[string]string{
				"authorization": "[REDACTED]",
				"COOKIE":        "[REDACTED]",
				"set-cookie":    "[REDACTED]",
				"X-Request-Id":  "1",
			},
		},
		{
			name: "does not mutate input",
			in: map[string]string{
				"Authorization": "Bearer keep",
			},
			want: map[string]string{
				"Authorization": "[REDACTED]",
			},
		},
		{
			name: "denylist x-api-key",
			in: map[string]string{
				"X-API-Key": "super-secret-apikey-value",
				"Accept":    "application/json",
			},
			want: map[string]string{
				"X-API-Key": "[REDACTED]",
				"Accept":    "application/json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var orig string
			if tt.in != nil {
				orig = tt.in["Authorization"]
			}
			got, _ := NewRedactionPolicy().redactForPersist(tt.in, "")
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %#v want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len=%d want %d got=%#v", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("key %q: got %q want %q", k, got[k], v)
				}
			}
			if orig != "" && tt.in["Authorization"] != orig {
				t.Fatalf("input mutated: %q", tt.in["Authorization"])
			}
			for _, v := range got {
				if strings.Contains(strings.ToLower(v), "bearer keep") || strings.Contains(v, "secret-token") {
					t.Fatalf("secret leaked in redacted map: %#v", got)
				}
			}
		})
	}
}

func TestRedactionPolicyExtraNames(t *testing.T) {
	in := map[string]string{
		"X-Custom-Token": "super-secret-apikey-value",
		"Accept":         "application/json",
	}
	got, _ := NewRedactionPolicy("X-Custom-Token").redactForPersist(in, "")
	if got["X-Custom-Token"] != redacted {
		t.Fatalf("X-Custom-Token=%q want %q", got["X-Custom-Token"], redacted)
	}
	if got["Accept"] != "application/json" {
		t.Fatalf("Accept mutated: %q", got["Accept"])
	}
	if in["X-Custom-Token"] != "super-secret-apikey-value" {
		t.Fatal("input mutated")
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no query", in: "https://api.example.com/v1", want: "https://api.example.com/v1"},
		{name: "safe query", in: "https://api.example.com/v1?page=1", want: "https://api.example.com/v1?page=1"},
		{
			name: "token",
			in:   "https://api.example.com/oauth?token=sekrit&page=1",
			want: "https://api.example.com/oauth?page=1&token=%5BREDACTED%5D",
		},
		{
			name: "access_token",
			in:   "https://ex.test/x?access_token=abc",
			want: "https://ex.test/x?access_token=%5BREDACTED%5D",
		},
		{
			name: "api_key and apikey",
			in:   "https://ex.test/x?api_key=a&apikey=b",
			want: "https://ex.test/x?api_key=%5BREDACTED%5D&apikey=%5BREDACTED%5D",
		},
		{
			name: "key secret password auth",
			in:   "https://ex.test/x?key=1&secret=2&password=3&auth=4",
			want: "https://ex.test/x?auth=%5BREDACTED%5D&key=%5BREDACTED%5D&password=%5BREDACTED%5D&secret=%5BREDACTED%5D",
		},
		{
			name: "case insensitive param names",
			in:   "https://ex.test/x?TOKEN=zzz",
			want: "https://ex.test/x?TOKEN=%5BREDACTED%5D",
		},
		{
			name: "invalid url passthrough",
			in:   "://bad",
			want: "://bad",
		},
		{
			name: "userinfo stripped",
			in:   "https://user:pass@host.example/path",
			want: "https://host.example/path",
		},
		{
			name: "userinfo and token query",
			in:   "https://user:pass@host.example/x?token=sekrit",
			want: "https://host.example/x?token=%5BREDACTED%5D",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := NewRedactionPolicy().redactForPersist(nil, tt.in)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if strings.Contains(got, "sekrit") || strings.Contains(got, "user:pass") || strings.Contains(got, "=zzz") {
				t.Fatalf("secret leaked: %q", got)
			}
		})
	}
}

func TestWireSecretsApplyOnly(t *testing.T) {
	w := WireSecrets{headers: map[string]string{"Authorization": "Bearer super-secret"}}
	overlay := w.overlayNames(map[string]string{"Accept": "application/json"})
	if overlay["Authorization"] != redacted {
		t.Fatalf("overlay Authorization=%q", overlay["Authorization"])
	}
	if strings.Contains(overlay["Authorization"], "super-secret") {
		t.Fatal("secret in overlay")
	}
}
