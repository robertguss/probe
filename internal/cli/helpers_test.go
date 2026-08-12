package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/version"
)

type runResult struct {
	Code   int
	Stdout string
	Stderr string
}

// fixedPipelineHost pins plan external_steps binaries + host env so CLI plan
// JSON is host-independent (matches internal/plan golden conventions).
func fixedPipelineHost() cli.PipelineHost {
	return cli.PipelineHost{
		GoBinary:       "/usr/local/go/bin/go",
		GitBinary:      "/usr/bin/git",
		GitTemplateDir: "/tmp/foundry-git-template-scratch",
		Host: plan.HostEnv{
			PATH:       "/usr/bin",
			HOME:       "/home/foundry",
			TMPDIR:     "/tmp",
			GOMODCACHE: "/home/foundry/go/pkg/mod",
			GOCACHE:    "/home/foundry/.cache/go-build",
			GOPATH:     "/home/foundry/go",
			GOPROXY:    "https://proxy.golang.org,direct",
			GOSUMDB:    "sum.golang.org",
		},
	}
}

// fixedObserve pins destination observation without real Lstat (pure tests).
func fixedObserve(authored string) (plan.DestinationInfo, error) {
	p := authored
	if strings.HasPrefix(p, "./") {
		p = "/home/foundry/projects/" + strings.TrimPrefix(p, "./")
	} else if !filepath.IsAbs(p) {
		p = "/home/foundry/projects/" + p
	}
	parent := p
	base := p
	if i := strings.LastIndex(p, "/"); i >= 0 {
		parent = p[:i]
		if parent == "" {
			parent = "/"
		}
		base = p[i+1:]
	}
	return plan.DestinationInfo{
		Path:        p,
		Parent:      parent,
		Basename:    base,
		Observation: plan.ObservationAbsent,
	}, nil
}

func testOptions() cli.Options {
	return cli.Options{
		Version: version.Info{
			Version: "0.1.0-test",
			Commit:  "abc1234",
			Go:      "go1.26.5",
		},
		PipelineHost:       fixedPipelineHost(),
		ObserveDestination: fixedObserve,
	}
}

func runCLI(t *testing.T, args ...string) runResult {
	t.Helper()
	return runCLIOpts(t, context.Background(), nil, testOptions(), args...)
}

func runCLICtx(t *testing.T, ctx context.Context, stdin *bytes.Reader, args ...string) runResult {
	t.Helper()
	return runCLIOpts(t, ctx, stdin, testOptions(), args...)
}

func runCLIOpts(t *testing.T, ctx context.Context, stdin *bytes.Reader, opts cli.Options, args ...string) runResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	var in bytes.Reader
	if stdin != nil {
		in = *stdin
	}
	if ctx == nil {
		ctx = context.Background()
	}
	code := cli.Run(ctx, args, cli.Streams{
		In:  &in,
		Out: &stdout,
		Err: &stderr,
	}, opts)
	return runResult{
		Code:   code,
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
}

// repoRoot walks to the module root (contains go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/cli/helpers_test.go → repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// examplesPath returns absolute path under examples/.
func examplesPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "examples", name)
}

// readExample reads an examples/ fixture.
func readExample(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(examplesPath(t, name))
	if err != nil {
		t.Fatalf("read example %s: %v", name, err)
	}
	return b
}

// normalizeHelp strips trailing spaces per line and ensures LF for goldens.
func normalizeHelp(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	out := strings.Join(lines, "\n")
	return strings.TrimRight(out, "\n") + "\n"
}

// bannedSurfaceTokens must not appear in registered flags or help text.
// Assembled without contiguous forbidden spellings in this file's product...
// (this is a test file; testdata is skipped by redline scanner, but tests under
// internal/cli/*.go are scanned). Build tokens via concatenation.
func bannedSurfaceTokens() []string {
	return []string{
		"--" + "offline",
		"--" + "force",
		"--" + "overwrite",
		"--" + "dry" + "-" + "run",
	}
}
