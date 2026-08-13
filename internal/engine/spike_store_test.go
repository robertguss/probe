package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpikeStoreCommitAtomicSession(t *testing.T) {
	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	var out bytes.Buffer
	e := New(Options{
		Stdout:   &out,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: spike,
	})
	if res := e.Run(context.Background(), []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init exit=%d", res.ExitCode)
	}
	sp := spikePathsFor(spike)
	before, err := e.loadSession(sp)
	if err != nil {
		t.Fatal(err)
	}
	if before.NextID != 1 || before.LastID != "" {
		t.Fatalf("session before=%+v", before)
	}
	id, err := e.spikeStore(sp).Commit("atom", savedRequestFile{
		Method:  "GET",
		URL:     "https://ex.test/a",
		Headers: map[string]string{"Accept": "application/json"},
	}, savedResponseFile{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{}`,
	}, 5, NewRedactionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if id != "001-atom" {
		t.Fatalf("id=%q", id)
	}
	after, err := e.loadSession(sp)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastID != id {
		t.Fatalf("LastID=%q want %q", after.LastID, id)
	}
	if after.NextID != before.NextID+1 {
		t.Fatalf("NextID=%d want %d", after.NextID, before.NextID+1)
	}
}

func TestSpikeStoreLoadRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	var out bytes.Buffer
	e := New(Options{
		Stdout:   &out,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: spike,
	})
	if res := e.Run(context.Background(), []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init exit=%d", res.ExitCode)
	}
	store := e.spikeStore(spikePathsFor(spike))
	for _, id := range []string{"../escape", "/etc/passwd", "..", ".", "foo/../bar"} {
		_, _, err := store.Load(ByID(id))
		if err == nil {
			t.Fatalf("Load(%q) expected error", id)
		}
	}
}

func TestPromoteEndpointTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	cat := t.TempDir()
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    r,
		}, nil
	})
	var out bytes.Buffer
	e := New(Options{
		Stdout:     &out,
		HTTP:       rt,
		Getwd:      func() (string, error) { return dir, nil },
		SpikeDir:   spike,
		CatalogDir: cat,
	})
	ctx := context.Background()
	if res := e.Run(ctx, []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init: %d", res.ExitCode)
	}
	if res := e.Run(ctx, []string{"hit", "GET", "https://example.test/z", "--save", "z", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("hit: %d", res.ExitCode)
	}
	res := e.Run(ctx, []string{"promote", "demoapi", "--endpoint", "../escape", "--json"})
	if res.ExitCode == ExitSuccess {
		t.Fatal("promote --endpoint ../escape succeeded")
	}
}

func TestReplayPromoteRejectTraversalIDs(t *testing.T) {
	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	cat := t.TempDir()
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    r,
		}, nil
	})
	var out, errBuf bytes.Buffer
	e := New(Options{
		Stdout:     &out,
		Stderr:     &errBuf,
		HTTP:       rt,
		Getwd:      func() (string, error) { return dir, nil },
		SpikeDir:   spike,
		CatalogDir: cat,
	})
	ctx := context.Background()
	if res := e.Run(ctx, []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init: %d", res.ExitCode)
	}
	out.Reset()
	errBuf.Reset()
	res := e.Run(ctx, []string{"replay", "../etc/passwd", "--json"})
	if res.ExitCode == ExitSuccess {
		t.Fatalf("replay traversal succeeded: %s", out.String())
	}
	out.Reset()
	errBuf.Reset()
	res = e.Run(ctx, []string{"promote", "demoapi", "--request", "../escape", "--endpoint", "x", "--json"})
	if res.ExitCode == ExitSuccess {
		t.Fatalf("promote traversal succeeded: %s", out.String())
	}
}

func TestCatalogShowPathRejectTraversal(t *testing.T) {
	cat := t.TempDir()
	var out bytes.Buffer
	e := New(Options{Stdout: &out, CatalogDir: cat, JSONDefault: true})
	ctx := context.Background()
	for _, name := range []string{"../escape", "/tmp/x", "."} {
		res := e.Run(ctx, []string{"catalog", "show", name, "--json"})
		if res.ExitCode == ExitSuccess {
			t.Fatalf("catalog show %q succeeded", name)
		}
		res = e.Run(ctx, []string{"catalog", "path", name, "--json"})
		if res.ExitCode == ExitSuccess {
			t.Fatalf("catalog path %q succeeded", name)
		}
	}
}

func TestContainPath(t *testing.T) {
	root := t.TempDir()
	got, err := containPath(root, "001-ok.json")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "001-ok.json") {
		t.Fatalf("got %q", got)
	}
	if _, err := containPath(root, "foo..bar.json"); err != nil {
		t.Fatalf("foo..bar.json should be allowed: %v", err)
	}
	for _, name := range []string{"../x", "/abs", "a/b", "..", ".", ""} {
		if _, err := containPath(root, name); err == nil {
			t.Fatalf("containPath(%q) expected error", name)
		}
	}
}

func TestHitSaveNameWithDoubleDot(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    r,
		}, nil
	})
	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	var out, errBuf bytes.Buffer
	e := New(Options{
		Stdout:   &out,
		Stderr:   &errBuf,
		HTTP:     rt,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: spike,
	})
	ctx := context.Background()
	if res := e.Run(ctx, []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init: %d", res.ExitCode)
	}
	out.Reset()
	res := e.Run(ctx, []string{"hit", "GET", "https://example.test/z", "--save", "foo..bar", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("hit --save foo..bar exit=%d %s", res.ExitCode, errBuf.String())
	}
	id := res.Envelope.Meta.RequestID
	if id == "" || !strings.Contains(id, "foo..bar") {
		t.Fatalf("id=%q want foo..bar", id)
	}
	if _, err := os.Stat(filepath.Join(spike, "requests", id+".json")); err != nil {
		t.Fatal(err)
	}
}

func TestLastScrubsWhenAuthProfileMissing(t *testing.T) {
	const secret = "legacy-custom-secret-value"
	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	cat := t.TempDir()
	var out bytes.Buffer
	e := New(Options{
		Stdout:     &out,
		Getwd:      func() (string, error) { return dir, nil },
		SpikeDir:   spike,
		CatalogDir: cat,
	})
	ctx := context.Background()
	if res := e.Run(ctx, []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init: %d", res.ExitCode)
	}
	sp := spikePathsFor(spike)
	id, err := e.spikeStore(sp).Commit("legacy", savedRequestFile{
		Method:  "GET",
		URL:     "https://ex.test/legacy",
		Headers: map[string]string{"Accept": "application/json"},
		Auth:    "gone",
	}, savedResponseFile{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{}`,
	}, 1, NewRedactionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	// Legacy unredacted artifacts: custom header on both request and response.
	reqRaw := fmt.Sprintf("{\n  \"id\": %q,\n  \"method\": \"GET\",\n  \"url\": \"https://ex.test/legacy\",\n  \"headers\": {\n    \"Accept\": \"application/json\",\n    \"X-Custom-Token\": %q\n  },\n  \"auth\": \"gone\"\n}\n", id, secret)
	respRaw := fmt.Sprintf("{\n  \"id\": %q,\n  \"status\": 200,\n  \"headers\": {\n    \"Content-Type\": \"application/json\",\n    \"X-Custom-Token\": %q\n  },\n  \"body\": \"{}\"\n}\n", id, secret)
	if err := os.WriteFile(filepath.Join(sp.Requests, id+".json"), []byte(reqRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sp.Responses, id+".json"), []byte(respRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	res := e.Run(ctx, []string{"last", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("last exit=%d", res.ExitCode)
	}
	if strings.Contains(out.String(), secret) {
		t.Fatalf("secret leaked via last after profile delete: %s", out.String())
	}
	if !strings.Contains(out.String(), "[REDACTED]") {
		t.Fatalf("expected redaction in last: %s", out.String())
	}

	out.Reset()
	res = e.Run(ctx, []string{"promote", "demoapi", "--endpoint", "get-legacy", "--request", id, "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("promote exit=%d data=%+v", res.ExitCode, res.Envelope)
	}
	fixture, err := os.ReadFile(filepath.Join(cat, "demoapi", "fixtures", "get-legacy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fixture), secret) {
		t.Fatalf("secret leaked into promote fixture: %s", fixture)
	}
	if !strings.Contains(string(fixture), "[REDACTED]") {
		t.Fatalf("expected redaction in promote fixture: %s", fixture)
	}
}

func TestHitCommitViaSpikeStore(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Request:    r,
		}, nil
	})
	dir := t.TempDir()
	spike := filepath.Join(dir, ".probe")
	var out, errBuf bytes.Buffer
	e := New(Options{
		Stdout:   &out,
		Stderr:   &errBuf,
		HTTP:     rt,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: spike,
	})
	ctx := context.Background()
	if res := e.Run(ctx, []string{"init", "--json"}); res.ExitCode != ExitSuccess {
		t.Fatalf("init: %d", res.ExitCode)
	}
	out.Reset()
	res := e.Run(ctx, []string{"hit", "GET", "https://example.test/z", "--save", "via-store", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("hit exit=%d %s", res.ExitCode, errBuf.String())
	}
	id := res.Envelope.Meta.RequestID
	if id == "" {
		t.Fatal("missing request id")
	}
	if _, err := os.Stat(filepath.Join(spike, "requests", id+".json")); err != nil {
		t.Fatal(err)
	}
	sess, err := e.loadSession(spikePathsFor(spike))
	if err != nil {
		t.Fatal(err)
	}
	if sess.LastID != id || sess.NextID < 2 {
		t.Fatalf("session=%+v id=%s", sess, id)
	}
}
