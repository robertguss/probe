package plan_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/resolve"
	"github.com/robertguss/go-foundry-cli/internal/spec"
)

// fixedHostEnv is a host-independent allowlist capture for goldens / determinism.
func fixedHostEnv() plan.HostEnv {
	return plan.HostEnv{
		PATH:       "/usr/bin",
		HOME:       "/home/foundry",
		TMPDIR:     "/tmp",
		GOMODCACHE: "/home/foundry/go/pkg/mod",
		GOCACHE:    "/home/foundry/.cache/go-build",
		GOPATH:     "/home/foundry/go",
		GOPROXY:    "https://proxy.golang.org,direct",
		GOSUMDB:    "sum.golang.org",
	}
}

// fixedToolBinaries are absolute path placeholders for pure plan goldens.
const (
	fixedGoBinary  = "/usr/local/go/bin/go"
	fixedGitBinary = "/usr/bin/git"
	fixedGitTmpl   = "/tmp/foundry-git-template-scratch"
)

func mustLoadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return c
}

func mustValidate(t *testing.T, name string, toml string) *spec.ValidatedSpecification {
	t.Helper()
	raw, err := spec.Decode(name, []byte(toml))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return vs
}

func mustResolve(t *testing.T, vs *spec.ValidatedSpecification, cat *catalog.Catalog) *resolve.ResolvedProject {
	t.Helper()
	rp, err := resolve.Resolve(vs, cat)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return rp
}

// fixedDestination builds a deterministic DestinationInfo from an authored path.
// Does not observe the real filesystem.
func fixedDestination(authoredPath string, observation plan.DestinationObservation) plan.DestinationInfo {
	// Treat relative destinations as under a fixed pure root for goldens.
	p := authoredPath
	if strings.HasPrefix(p, "./") {
		p = "/home/foundry/projects/" + strings.TrimPrefix(p, "./")
	}
	// Dir/Base with forward slashes (no path/filepath — pure string ops).
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
		Observation: observation,
	}
}

func defaultPipelineOpts(dest plan.DestinationInfo) plan.PipelineOptions {
	return plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  "testcommit000000000000000000000000000001",
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       "foundry.toml",
		Destination:    dest,
		Verify:         plan.VerifyDefault,
		GoBinary:       fixedGoBinary,
		GitBinary:      fixedGitBinary,
		GitTemplateDir: fixedGitTmpl,
		Host:           fixedHostEnv(),
	}
}

func defaultInputs(rp *resolve.ResolvedProject, cat *catalog.Catalog, dest plan.DestinationInfo) plan.Inputs {
	return plan.Inputs{
		Resolved:       rp,
		Catalog:        cat,
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  "testcommit000000000000000000000000000001",
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       "foundry.toml",
		Destination:    dest,
		Verify:         plan.VerifyDefault,
		GoBinary:       fixedGoBinary,
		GitBinary:      fixedGitBinary,
		GitTemplateDir: fixedGitTmpl,
		Host:           fixedHostEnv(),
	}
}

func minimalCLI(t *testing.T) *spec.ValidatedSpecification {
	t.Helper()
	return mustValidate(t, "minimal-cli.toml", `
schema = 1
name = "minimal-cli"
module = "github.com/example/minimal-cli"
description = "Minimal Foundry CLI example for validate, plan, and generate"
archetype = "cli"
destination = "./minimal-cli"
profiles = []
`)
}

func minimalTUI(t *testing.T) *spec.ValidatedSpecification {
	t.Helper()
	return mustValidate(t, "minimal-tui.toml", `
schema = 1
name = "minimal-tui"
module = "github.com/example/minimal-tui"
description = "Minimal Foundry TUI example for validate, plan, and generate"
archetype = "tui"
destination = "./minimal-tui"
profiles = []
`)
}

func appendixBPrivateCLI(t *testing.T) *spec.ValidatedSpecification {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "internal/spec/testdata/appendix_b/private-cli.toml"))
	if err != nil {
		t.Fatalf("read appendix b: %v", err)
	}
	return mustValidate(t, "private-cli.toml", string(b))
}

func appendixBPrivateTUI(t *testing.T) *spec.ValidatedSpecification {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "internal/spec/testdata/appendix_b/private-tui.toml"))
	if err != nil {
		t.Fatalf("read appendix b: %v", err)
	}
	return mustValidate(t, "private-tui.toml", string(b))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/plan/helpers_test.go → repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func mustConstruct(t *testing.T, in plan.Inputs) *plan.Plan {
	t.Helper()
	p, err := plan.Construct(in)
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	return p
}

func decodePlanMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, truncate(string(raw), 400))
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var neg bool
	if n < 0 {
		neg = true
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
