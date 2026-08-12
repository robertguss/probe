package engine

import (
	"encoding/json"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var orig string
			if tt.in != nil {
				orig = tt.in["Authorization"]
			}
			got := RedactHeaders(tt.in)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactURL(tt.in)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if strings.Contains(got, "sekrit") || strings.Contains(got, "zzz") && !strings.Contains(got, "[REDACTED]") {
				if strings.Contains(got, "sekrit") || strings.Contains(got, "=zzz") {
					t.Fatalf("secret leaked: %q", got)
				}
			}
		})
	}
}

func TestAuthorizationHeaderNeverPrintsSecret(t *testing.T) {
	h := newAuthorizationHeader("Bearer super-secret")
	if s := h.String(); s != "[REDACTED]" {
		t.Fatalf("String=%q", s)
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"[REDACTED]"` {
		t.Fatalf("JSON=%s", b)
	}
	if strings.Contains(string(b), "super-secret") {
		t.Fatal("secret in JSON")
	}
	if h.materialize() != "Bearer super-secret" {
		t.Fatal("materialize broken")
	}
}
