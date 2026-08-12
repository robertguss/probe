package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/example/foundry-smoke-cli/internal/cli"
)

// TestPingConstructor exercises the extension-path subcommand through the
// process-boundary Run entry (constructor-level command test / REQ-066).
func TestPingConstructor(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), []string{"ping", "--target", "fixture"}, cli.Streams{
		Out: &stdout,
		Err: &stderr,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("exit: got %d want %d; stderr=%q", code, cli.ExitSuccess, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "pong fixture" {
		t.Fatalf("stdout: got %q want %q", got, "pong fixture")
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}
