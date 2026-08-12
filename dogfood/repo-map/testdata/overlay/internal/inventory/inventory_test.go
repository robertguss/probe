package inventory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/repo-map/internal/inventory"
)

func TestWalkAndFormats(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("bye"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .git must be skipped.
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := inventory.Walk(inventory.Options{Root: dir})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	text := inventory.FormatText(entries)
	if !strings.Contains(text, "a.txt\n") {
		t.Fatalf("text missing a.txt: %q", text)
	}
	if !strings.Contains(text, "sub/") {
		t.Fatalf("text missing sub/: %q", text)
	}
	if strings.Contains(text, ".git") {
		t.Fatalf("text must skip .git: %q", text)
	}
	js, err := inventory.FormatJSON(entries)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	if !strings.Contains(js, `"path": "a.txt"`) {
		t.Fatalf("json missing a.txt: %s", js)
	}
}
