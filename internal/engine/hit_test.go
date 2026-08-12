package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClassifyHTTP(t *testing.T) {
	tests := []struct {
		status  int
		limited bool
		want    ExitCode
	}{
		{200, false, ExitSuccess},
		{201, false, ExitSuccess},
		{301, false, ExitSuccess},
		{400, false, ExitHTTP4xx},
		{404, false, ExitHTTP4xx},
		{429, false, ExitHTTP4xx},
		{429, true, ExitRateLimited},
		{500, false, ExitHTTP5xx},
		{503, false, ExitHTTP5xx},
		{503, true, ExitRateLimited},
	}
	for _, tt := range tests {
		got := classifyHTTP(tt.status, tt.limited)
		if got != tt.want {
			t.Fatalf("status=%d limited=%v got=%d want=%d", tt.status, tt.limited, got, tt.want)
		}
	}
}

func TestHitExitCodesFromStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   ExitCode
		code   string
	}{
		{name: "200", status: 200, body: `{"ok":true}`, want: ExitSuccess},
		{name: "404", status: 404, body: `missing`, want: ExitHTTP4xx, code: "http_error"},
		{name: "500", status: 500, body: `err`, want: ExitHTTP5xx, code: "http_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{
					StatusCode: tt.status,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Request:    r,
				}, nil
			})
			var out, errBuf bytes.Buffer
			e := New(Options{
				Stdout: &out,
				Stderr: &errBuf,
				HTTP:   rt,
				Getwd:  func() (string, error) { return t.TempDir(), nil },
			})
			res := e.Run(context.Background(), []string{"hit", "GET", "https://example.test/x", "--no-save", "--json"})
			if res.ExitCode != tt.want {
				t.Fatalf("exit=%d want %d stderr=%s out=%s", res.ExitCode, tt.want, errBuf.String(), out.String())
			}
			if tt.code != "" && (res.Envelope.Error == nil || res.Envelope.Error.Code != tt.code) {
				t.Fatalf("error=%+v", res.Envelope.Error)
			}
			if calls.Load() != 1 {
				t.Fatalf("calls=%d", calls.Load())
			}
		})
	}
}

func TestHit429ExhaustedExit5(t *testing.T) {
	var calls atomic.Int32
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		h := make(http.Header)
		h.Set("Retry-After", "0")
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     h,
			Body:       io.NopCloser(strings.NewReader(`{"error":"slow down"}`)),
			Request:    r,
		}, nil
	})
	var out, errBuf bytes.Buffer
	e := New(Options{Stdout: &out, Stderr: &errBuf, HTTP: rt})
	res := e.Run(context.Background(), []string{
		"hit", "GET", "https://example.test/limited",
		"--retries", "2", "--max-wait", "5s", "--no-save", "--json",
	})
	if res.ExitCode != ExitRateLimited {
		t.Fatalf("exit=%d want 5 out=%s err=%s", res.ExitCode, out.String(), errBuf.String())
	}
	if res.Envelope.Error == nil || res.Envelope.Error.Code != "rate_limited" {
		t.Fatalf("error=%+v", res.Envelope.Error)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d want 3", calls.Load())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("envelope=%s", out.String())
	}
}

func TestHitRetryAfterHonored(t *testing.T) {
	var calls atomic.Int32
	var gaps []time.Duration
	var last time.Time
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := calls.Add(1)
		now := time.Now()
		if !last.IsZero() {
			gaps = append(gaps, now.Sub(last))
		}
		last = now
		h := make(http.Header)
		if n < 2 {
			h.Set("Retry-After", "0")
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     h,
				Body:       io.NopCloser(strings.NewReader("busy")),
				Request:    r,
			}, nil
		}
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    r,
		}, nil
	})
	var out bytes.Buffer
	e := New(Options{Stdout: &out, HTTP: rt})
	res := e.Run(context.Background(), []string{
		"hit", "GET", "https://example.test/retry",
		"--retries", "3", "--no-save", "--json",
	})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("exit=%d out=%s", res.ExitCode, out.String())
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
	if len(gaps) != 1 {
		t.Fatalf("gaps=%v", gaps)
	}
	if gaps[0] > 150*time.Millisecond {
		t.Fatalf("Retry-After not honored; gap=%v", gaps[0])
	}
}

func TestHitEnvelopeShape(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"hello":"world"}`)),
			Request:    r,
		}, nil
	})
	var out bytes.Buffer
	e := New(Options{Stdout: &out, HTTP: rt})
	res := e.Run(context.Background(), []string{"hit", "GET", "https://example.test/ok", "--no-save", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("exit=%d", res.ExitCode)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Command != "hit" || env.Error != nil {
		t.Fatalf("env=%+v", env)
	}
	if env.Meta.ExitCode != ExitSuccess || env.Meta.Version != Version {
		t.Fatalf("meta=%+v", env.Meta)
	}
	raw := out.String()
	for _, s := range []string{`"ok":true`, `"command":"hit"`, `"error":null`, `"status":200`, `"method":"GET"`} {
		if !strings.Contains(raw, s) {
			t.Fatalf("missing %s in %s", s, raw)
		}
	}

	out.Reset()
	rtFail := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("nope")),
			Request:    r,
		}, nil
	})
	e = New(Options{Stdout: &out, HTTP: rtFail})
	res = e.Run(context.Background(), []string{"hit", "GET", "https://example.test/missing", "--no-save", "--json"})
	if res.ExitCode != ExitHTTP4xx {
		t.Fatalf("exit=%d", res.ExitCode)
	}
	raw = out.String()
	if !strings.Contains(raw, `"ok":false`) || !strings.Contains(raw, `"code":"http_error"`) {
		t.Fatalf("fail envelope=%s", raw)
	}
}

func TestHitAuthEnvMissingBeforeNetwork(t *testing.T) {
	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	var calls atomic.Int32
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
	})
	var out, errBuf bytes.Buffer
	e := New(Options{
		Stdout:   &out,
		Stderr:   &errBuf,
		HTTP:     rt,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: spike,
		Environ:  nil,
	})
	if res := e.Run(context.Background(), []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init: %d", res.ExitCode)
	}
	out.Reset()
	errBuf.Reset()
	if res := e.Run(context.Background(), []string{"auth", "set", "canvas", "--type", "bearer", "--token-env", "CANVAS_TOKEN", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("auth set: %d %s", res.ExitCode, errBuf.String())
	}
	out.Reset()
	errBuf.Reset()
	res := e.Run(context.Background(), []string{
		"hit", "GET", "https://example.test/courses",
		"--auth", "canvas", "--json",
	})
	if res.ExitCode != ExitUsage {
		t.Fatalf("exit=%d want 2 out=%s err=%s", res.ExitCode, out.String(), errBuf.String())
	}
	if res.Envelope.Error == nil || res.Envelope.Error.Code != "auth_env_missing" {
		t.Fatalf("error=%+v", res.Envelope.Error)
	}
	if !strings.Contains(out.String(), "fnox exec") {
		t.Fatalf("missing fnox hint: %s", out.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("network called %d times", calls.Load())
	}
}

func TestHitDryRunNoNetwork(t *testing.T) {
	var calls atomic.Int32
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, nil
	})
	var out bytes.Buffer
	e := New(Options{Stdout: &out, HTTP: rt})
	res := e.Run(context.Background(), []string{
		"hit", "GET", "https://example.test/x?token=sekrit",
		"--header", "Authorization: Bearer secret",
		"--dry-run", "--json",
	})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("exit=%d out=%s", res.ExitCode, out.String())
	}
	if calls.Load() != 0 {
		t.Fatal("network called")
	}
	raw := out.String()
	if strings.Contains(raw, "sekrit") || strings.Contains(raw, "Bearer secret") {
		t.Fatalf("secret leaked: %s", raw)
	}
	if !strings.Contains(raw, "[REDACTED]") || !strings.Contains(raw, `"dry_run":true`) {
		t.Fatalf("dry-run envelope=%s", raw)
	}
}

func TestHitGoldenPathHTTPT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token-value" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte("unauthorized"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "sid=abc")
		_, _ = w.Write([]byte(`{"courses":[]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	var out, errBuf bytes.Buffer
	e := New(Options{
		Stdout:   &out,
		Stderr:   &errBuf,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: spike,
		Environ:  []string{"CANVAS_TOKEN=test-token-value"},
	})

	steps := [][]string{
		{"init", "--json"},
		{"auth", "set", "canvas", "--type", "bearer", "--token-env", "CANVAS_TOKEN", "--json"},
		{"hit", "GET", "/api/v1/courses", "--auth", "canvas", "--base", srv.URL, "--save", "courses", "--json"},
		{"last", "--json"},
	}
	var hitID string
	for i, args := range steps {
		out.Reset()
		errBuf.Reset()
		res := e.Run(context.Background(), args)
		if res.ExitCode != ExitSuccess {
			t.Fatalf("step %d %v exit=%d err=%s out=%s", i, args, res.ExitCode, errBuf.String(), out.String())
		}
		if i == 2 {
			var env Envelope
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			hitID = env.Meta.RequestID
			if hitID == "" {
				t.Fatalf("missing request id: %s", out.String())
			}
			reqPath := filepath.Join(spike, "requests", hitID+".json")
			b, err := os.ReadFile(reqPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "test-token-value") {
				t.Fatalf("token in saved request: %s", b)
			}
			if !strings.Contains(string(b), `"Authorization": "[REDACTED]"`) && !strings.Contains(string(b), `"Authorization":"[REDACTED]"`) {
				t.Fatalf("authorization not redacted: %s", b)
			}
		}
		if i == 3 {
			if !strings.Contains(out.String(), hitID) {
				t.Fatalf("last missing id %s: %s", hitID, out.String())
			}
		}
	}
}

func TestHitHelpHasExamples(t *testing.T) {
	var out bytes.Buffer
	e := New(Options{Stdout: &out, Stderr: &out})
	res := e.Run(context.Background(), []string{"hit", "--help"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("exit=%d", res.ExitCode)
	}
	if !strings.Contains(out.String(), "Examples:") {
		t.Fatalf("missing Examples: %s", out.String())
	}
}
