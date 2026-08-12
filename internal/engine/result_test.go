package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvelopeJSONAndExitCodes(t *testing.T) {
	e := New(Options{})

	tests := []struct {
		name       string
		result     Result
		wantExit   ExitCode
		wantOK     bool
		wantSubstr []string
		wantAbsent []string
	}{
		{
			name:     "ok version",
			result:   e.ok("version", versionData{Version: Version}),
			wantExit: ExitSuccess,
			wantOK:   true,
			wantSubstr: []string{
				`"ok":true`,
				`"command":"version"`,
				`"error":null`,
				`"exitCode":0`,
				`"version":"0.1.0"`,
			},
		},
		{
			name:     "usage error",
			result:   e.usageError("hit", "missing URL", "probe hit GET https://example.com --json"),
			wantExit: ExitUsage,
			wantOK:   false,
			wantSubstr: []string{
				`"ok":false`,
				`"command":"hit"`,
				`"code":"usage"`,
				`"exitCode":2`,
				`"probe hit GET https://example.com --json"`,
				`"next":["probe hit GET https://example.com --json"]`,
			},
		},
		{
			name:     "rate limited",
			result:   e.fail("hit", ExitRateLimited, "rate_limited", "retries exhausted", "wait and retry", []string{"probe hit GET /x --json"}),
			wantExit: ExitRateLimited,
			wantOK:   false,
			wantSubstr: []string{
				`"code":"rate_limited"`,
				`"exitCode":5`,
			},
		},
		{
			name:     "http 4xx",
			result:   e.fail("hit", ExitHTTP4xx, "http_error", "not found", "", nil),
			wantExit: ExitHTTP4xx,
			wantOK:   false,
			wantSubstr: []string{
				`"exitCode":3`,
			},
			wantAbsent: []string{`"next"`},
		},
		{
			name:     "http 5xx",
			result:   e.fail("hit", ExitHTTP5xx, "http_error", "server error", "", nil),
			wantExit: ExitHTTP5xx,
			wantOK:   false,
			wantSubstr: []string{
				`"exitCode":4`,
			},
		},
		{
			name:     "transport",
			result:   e.fail("hit", ExitTransport, "transport", "connection refused", "", nil),
			wantExit: ExitTransport,
			wantOK:   false,
			wantSubstr: []string{
				`"exitCode":1`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.ExitCode != tt.wantExit {
				t.Fatalf("ExitCode=%d want %d", tt.result.ExitCode, tt.wantExit)
			}
			if tt.result.Envelope.Meta.ExitCode != tt.wantExit {
				t.Fatalf("Meta.ExitCode=%d want %d", tt.result.Envelope.Meta.ExitCode, tt.wantExit)
			}
			if tt.result.Envelope.OK != tt.wantOK {
				t.Fatalf("OK=%v want %v", tt.result.Envelope.OK, tt.wantOK)
			}
			if tt.result.Envelope.Meta.Version != Version {
				t.Fatalf("Meta.Version=%q want %q", tt.result.Envelope.Meta.Version, Version)
			}

			b, err := json.Marshal(tt.result.Envelope)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			got := string(b)
			for _, s := range tt.wantSubstr {
				if !strings.Contains(got, s) {
					t.Fatalf("JSON missing %q\ngot: %s", s, got)
				}
			}
			for _, s := range tt.wantAbsent {
				if strings.Contains(got, s) {
					t.Fatalf("JSON unexpectedly contains %q\ngot: %s", s, got)
				}
			}

			b2, err := json.Marshal(tt.result.Envelope)
			if err != nil {
				t.Fatalf("Marshal again: %v", err)
			}
			if string(b2) != got {
				t.Fatalf("JSON not deterministic:\n%s\nvs\n%s", got, b2)
			}
		})
	}
}

func TestFailRateLimitedInvariant(t *testing.T) {
	e := New(Options{})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for mismatched rate_limited")
		}
	}()
	_ = e.fail("hit", ExitHTTP5xx, "rate_limited", "bad", "", nil)
}
