package generatee2e_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestGenerateWrongGoVersionProductProbe is product inventory E8: generate
// fails closed with tool.wrong_version when the resolved Go binary reports a
// non-catalog pin. Uses an injected PipelineHost.GoBinary (same path as
// FOUNDRY_GO_BIN) so the probe never mutates the host toolchain
// (beads go-foundry-cli-b0z / go-foundry-cli-n0y.1).
func TestGenerateWrongGoVersionProductProbe(t *testing.T) {
	skipIfShort(t)
	if runtime.GOOS == "windows" {
		t.Skip("product platforms are Linux and macOS; Windows is compile hygiene only")
	}
	log := testutil.New(t)
	log.Phase("e8_wrong_go_version")

	fakeGo := writeFakeGoVersion(t, "go1.22.0")
	parent := privateParent(t)
	dest := filepath.Join(parent, "minimal-cli")
	spec := examplesSpec(t, "minimal-cli.toml")

	opts := cli.Options{
		PipelineHost: cli.PipelineHost{
			GoBinary: fakeGo,
		},
	}
	res := runCLI(t, context.Background(), opts, nil,
		"generate", "--spec", spec, "--dest", dest, "--output", "json")
	// tool.wrong_version is exit 1 (tool failure), not usage (2).
	logProc(log, "generate_wrong_go", res, 1)

	env := mustEnvelope(t, log, res.Stdout)
	id := errorIDFromEnvelope(env)
	if id != string(diagnostic.IDToolWrongVersion) {
		t.Fatalf("expected tool.wrong_version, got id=%q stdout=%q stderr=%q",
			id, capBody(res.Stdout, 400), capBody(res.Stderr, 400))
	}
	log.Assert("error_id", id == string(diagnostic.IDToolWrongVersion), "tool.wrong_version", id)

	// Destination must not have been created (fail closed before commit).
	if _, err := os.Stat(filepath.Join(dest, "go.mod")); err == nil {
		t.Fatal("destination go.mod must not exist after wrong-go preflight failure")
	}
	log.PhaseEnd("e8_wrong_go_version", testutil.OutcomeOK)
}

// writeFakeGoVersion installs an executable "go" that only implements
// `go version` with the given version token (e.g. go1.22.0).
func writeFakeGoVersion(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "go")
	// Minimal sh stub: preflight runs `go version` with GOTOOLCHAIN=local.
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"version\" ]; then\n" +
		"  echo 'go version " + version + " linux/amd64'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo 'fake go: unexpected args' >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}
	return path
}
