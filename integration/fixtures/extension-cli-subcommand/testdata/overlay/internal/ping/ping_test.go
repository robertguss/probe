package ping_test

import (
	"testing"

	"github.com/example/foundry-smoke-cli/internal/ping"
)

func TestFormatDefault(t *testing.T) {
	got := ping.Format("")
	want := "pong localhost"
	if got != want {
		t.Fatalf("Format(\"\"): got %q want %q", got, want)
	}
}

func TestFormatTarget(t *testing.T) {
	got := ping.Format("api.example")
	want := "pong api.example"
	if got != want {
		t.Fatalf("Format: got %q want %q", got, want)
	}
}
