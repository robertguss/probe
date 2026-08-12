package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDoctorNoSecrets(t *testing.T) {
	spike := t.TempDir()
	cat := t.TempDir()
	e := New(Options{
		SpikeDir:    spike,
		CatalogDir:  cat,
		JSONDefault: true,
		Environ:     []string{"CANVAS_TOKEN=super-secret-token"},
	})
	ctx := context.Background()
	_ = e.Run(ctx, []string{"init", "--json"})
	_ = e.Run(ctx, []string{"auth", "set", "canvas", "--type", "bearer", "--token-env", "CANVAS_TOKEN", "--json"})
	res := e.Run(ctx, []string{"doctor", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("doctor: %+v", res.Envelope)
	}
	b, _ := json.Marshal(res.Envelope)
	if strings.Contains(string(b), "super-secret-token") {
		t.Fatalf("secret leaked: %s", b)
	}
	raw, _ := json.Marshal(res.Envelope.Data)
	if !strings.Contains(string(raw), `"canvas.CANVAS_TOKEN":true`) {
		t.Fatalf("expected auth env presence bool: %s", raw)
	}
}

func TestQuickstartAndSchema(t *testing.T) {
	e := New(Options{JSONDefault: true})
	ctx := context.Background()
	qs := e.Run(ctx, []string{"quickstart", "--json"})
	if qs.ExitCode != ExitSuccess {
		t.Fatalf("quickstart: %+v", qs.Envelope)
	}
	b, _ := json.Marshal(qs.Envelope.Data)
	if !strings.Contains(string(b), "fnox exec") {
		t.Fatalf("quickstart missing fnox: %s", b)
	}
	if !strings.Contains(string(b), "rate_limited") {
		t.Fatalf("quickstart missing rate limit note: %s", b)
	}
	sc := e.Run(ctx, []string{"schema", "--json"})
	if sc.ExitCode != ExitSuccess {
		t.Fatalf("schema: %+v", sc.Envelope)
	}
	sb, _ := json.Marshal(sc.Envelope.Data)
	if !strings.Contains(string(sb), `"rate_limited":5`) {
		t.Fatalf("schema missing exit 5: %s", sb)
	}
	if !strings.Contains(string(sb), "--body") {
		t.Fatalf("schema missing body flag note: %s", sb)
	}
}

func TestDoctorHelpExamples(t *testing.T) {
	var out strings.Builder
	e := New(Options{Stdout: &out})
	_ = e.Run(context.Background(), []string{"doctor", "--help"})
	if !strings.Contains(out.String(), "Examples:") {
		t.Fatalf("missing Examples:\n%s", out.String())
	}
}
