package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromoteAndCatalogShow(t *testing.T) {
	spike := t.TempDir()
	cat := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	e := New(Options{
		SpikeDir:    spike,
		CatalogDir:  cat,
		JSONDefault: true,
		Environ:     []string{"TOK=secret-value"},
	})
	ctx := context.Background()
	if res := e.Run(ctx, []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init: %+v", res.Envelope)
	}
	if res := e.Run(ctx, []string{"auth", "set", "api", "--type", "bearer", "--token-env", "TOK", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("auth set: %+v", res.Envelope)
	}
	if res := e.Run(ctx, []string{"hit", "GET", srv.URL + "/v1/items", "--auth", "api", "--save", "items", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("hit: %+v", res.Envelope)
	}

	res := e.Run(ctx, []string{"promote", "demoapi", "--endpoint", "get-items", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("promote: %+v", res.Envelope)
	}
	show := e.Run(ctx, []string{"catalog", "show", "demoapi", "--json"})
	if show.ExitCode != ExitSuccess {
		t.Fatalf("catalog show: %+v", show.Envelope)
	}
	b, _ := json.Marshal(show.Envelope.Data)
	if strings.Contains(string(b), "secret-value") {
		t.Fatalf("secret leaked into catalog show: %s", b)
	}
	fixPath := filepath.Join(cat, "demoapi", "fixtures", "get-items.json")
	fb, err := os.ReadFile(fixPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fb), "secret-value") {
		t.Fatalf("secret leaked into fixture: %s", fb)
	}
	if !strings.Contains(string(fb), "[REDACTED]") {
		t.Fatalf("expected redacted auth in fixture: %s", fb)
	}
}

func TestPromoteBaseConflict(t *testing.T) {
	spike := t.TempDir()
	cat := t.TempDir()
	apiDir := filepath.Join(cat, "demoapi")
	if err := os.MkdirAll(filepath.Join(apiDir, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "api.yaml"), []byte("name: demoapi\nbase: https://other.example\nendpoints: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	e := New(Options{SpikeDir: spike, CatalogDir: cat, JSONDefault: true})
	ctx := context.Background()
	_ = e.Run(ctx, []string{"init", "--json"})
	if res := e.Run(ctx, []string{"hit", "GET", srv.URL + "/x", "--save", "x", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("hit: %+v", res.Envelope)
	}
	res := e.Run(ctx, []string{"promote", "demoapi", "--endpoint", "get-x", "--json"})
	if res.ExitCode != ExitUsage {
		t.Fatalf("exit=%d want usage", res.ExitCode)
	}
	if res.Envelope.Error == nil || res.Envelope.Error.Code != "base_conflict" {
		t.Fatalf("want base_conflict, got %+v", res.Envelope.Error)
	}
}

func TestPromoteFixtureExists(t *testing.T) {
	spike := t.TempDir()
	cat := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	e := New(Options{SpikeDir: spike, CatalogDir: cat, JSONDefault: true})
	ctx := context.Background()
	_ = e.Run(ctx, []string{"init", "--json"})
	_ = e.Run(ctx, []string{"hit", "GET", srv.URL + "/y", "--save", "y", "--json"})
	if res := e.Run(ctx, []string{"promote", "demoapi", "--endpoint", "get-y", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("first promote: %+v", res.Envelope)
	}
	_ = e.Run(ctx, []string{"hit", "GET", srv.URL + "/y", "--save", "y2", "--json"})
	res := e.Run(ctx, []string{"promote", "demoapi", "--endpoint", "get-y", "--json"})
	if res.ExitCode != ExitUsage {
		t.Fatalf("exit=%d want usage", res.ExitCode)
	}
	if res.Envelope.Error == nil || res.Envelope.Error.Code != "fixture_exists" {
		t.Fatalf("want fixture_exists, got %+v", res.Envelope.Error)
	}
}

func TestPromoteDryRunNoWrite(t *testing.T) {
	spike := t.TempDir()
	cat := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	e := New(Options{SpikeDir: spike, CatalogDir: cat, JSONDefault: true})
	ctx := context.Background()
	_ = e.Run(ctx, []string{"init", "--json"})
	_ = e.Run(ctx, []string{"hit", "GET", srv.URL + "/z", "--save", "z", "--json"})
	res := e.Run(ctx, []string{"promote", "demoapi", "--endpoint", "get-z", "--dry-run", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("dry-run promote: %+v", res.Envelope)
	}
	if _, err := os.Stat(filepath.Join(cat, "demoapi", "api.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote api.yaml")
	}
}

func TestCatalogHelpExamples(t *testing.T) {
	var out strings.Builder
	e := New(Options{Stdout: &out, JSONDefault: false})
	_ = e.Run(context.Background(), []string{"catalog", "show", "--help"})
	if !strings.Contains(out.String(), "Examples:") {
		t.Fatalf("missing Examples in help:\n%s", out.String())
	}
}
