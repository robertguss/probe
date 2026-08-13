package engine

import (
	"bytes"
	"context"
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
	for _, id := range []string{"../escape", "/etc/passwd", "..", "foo/../bar"} {
		_, _, err := store.Load(ByID(id), NewRedactionPolicy())
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
	for _, name := range []string{"../escape", "/tmp/x"} {
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
	for _, name := range []string{"../x", "/abs", "a/b", "..", ""} {
		if _, err := containPath(root, name); err == nil {
			t.Fatalf("containPath(%q) expected error", name)
		}
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
