package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/repo-map/internal/cli"
)

// TestInventoryConstructor exercises the dogfood domain subcommand through the
// process-boundary Run entry (constructor-level command test / REQ-066).
func TestInventoryConstructor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), []string{"inventory", dir, "--output", "text"}, cli.Streams{
		Out: &stdout,
		Err: &stderr,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("exit: got %d want %d; stderr=%q", code, cli.ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), "note.txt") {
		t.Fatalf("stdout missing note.txt: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	stdout.Reset()
	code = cli.Run(context.Background(), []string{"inventory", dir, "--output", "json"}, cli.Streams{
		Out: &stdout,
		Err: &stderr,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("json exit: got %d want %d; stderr=%q", code, cli.ExitSuccess, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"path": "note.txt"`) {
		t.Fatalf("json missing note.txt: %s", stdout.String())
	}
}
