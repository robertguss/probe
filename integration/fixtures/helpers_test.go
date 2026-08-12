package fixtures_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// fixedHostEnv matches internal/plan golden conventions (host-independent).
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

const (
	fixedGoBinary  = "/usr/local/go/bin/go"
	fixedGitBinary = "/usr/bin/git"
	fixedGitTmpl   = "/tmp/foundry-git-template-scratch"
	fixedCommit    = "testcommit000000000000000000000000000001"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// integration/fixtures/helpers_test.go → repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func fixtureDir(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "integration", "fixtures", rel)
}

func mustLoadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return c
}

func smokeCLISpecBytes(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(fixtureDir(t, "foundry-smoke-cli/foundry.toml"))
	if err != nil {
		t.Fatalf("read smoke spec: %v", err)
	}
	return b
}

func mustValidateSmoke(t *testing.T) *spec.ValidatedSpecification {
	t.Helper()
	raw, err := spec.Decode("foundry.toml", smokeCLISpecBytes(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return vs
}

func smokePipelineOpts(t *testing.T) plan.PipelineOptions {
	t.Helper()
	vs := mustValidateSmoke(t)
	dest := vs.Destination()
	// Pure fixed destination (no real Lstat) for goldens.
	p := dest
	if strings.HasPrefix(p, "./") {
		p = "/home/foundry/projects/" + strings.TrimPrefix(p, "./")
	}
	parent, base := splitPath(p)
	return plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  fixedCommit,
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       "foundry.toml",
		Destination: plan.DestinationInfo{
			Path:        p,
			Parent:      parent,
			Basename:    base,
			Observation: plan.ObservationAbsent,
		},
		Verify:         plan.VerifyDefault,
		GoBinary:       fixedGoBinary,
		GitBinary:      fixedGitBinary,
		GitTemplateDir: fixedGitTmpl,
		Host:           fixedHostEnv(),
	}
}

func mustSmokePlan(t *testing.T) (*plan.Plan, *catalog.Catalog) {
	t.Helper()
	cat := mustLoadCatalog(t)
	vs := mustValidateSmoke(t)
	opts := smokePipelineOpts(t)
	p, err := plan.Pipeline(vs, cat, opts)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	return p, cat
}

func smokeTUISpecBytes(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(fixtureDir(t, "foundry-smoke-tui/foundry.toml"))
	if err != nil {
		t.Fatalf("read smoke-tui spec: %v", err)
	}
	return b
}

func mustValidateSmokeTUI(t *testing.T) *spec.ValidatedSpecification {
	t.Helper()
	raw, err := spec.Decode("foundry.toml", smokeTUISpecBytes(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return vs
}

func smokeTUIPipelineOpts(t *testing.T) plan.PipelineOptions {
	t.Helper()
	vs := mustValidateSmokeTUI(t)
	dest := vs.Destination()
	p := dest
	if strings.HasPrefix(p, "./") {
		p = "/home/foundry/projects/" + strings.TrimPrefix(p, "./")
	}
	parent, base := splitPath(p)
	return plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  fixedCommit,
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       "foundry.toml",
		Destination: plan.DestinationInfo{
			Path:        p,
			Parent:      parent,
			Basename:    base,
			Observation: plan.ObservationAbsent,
		},
		Verify:         plan.VerifyDefault,
		GoBinary:       fixedGoBinary,
		GitBinary:      fixedGitBinary,
		GitTemplateDir: fixedGitTmpl,
		Host:           fixedHostEnv(),
	}
}

func mustSmokeTUIPlan(t *testing.T) (*plan.Plan, *catalog.Catalog) {
	t.Helper()
	cat := mustLoadCatalog(t)
	vs := mustValidateSmokeTUI(t)
	opts := smokeTUIPipelineOpts(t)
	p, err := plan.Pipeline(vs, cat, opts)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	return p, cat
}

// runGenerateSmokeTUI runs production generate for foundry-smoke-tui into absDest.
func runGenerateSmokeTUI(t *testing.T, goBin, absDest string) (planSHA string) {
	t.Helper()
	cat := mustLoadCatalog(t)
	absDest, err := filepath.Abs(absDest)
	if err != nil {
		t.Fatalf("abs dest: %v", err)
	}
	parent, base := filepath.Dir(absDest), filepath.Base(absDest)
	opts := plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  fixedCommit,
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       fixtureDir(t, "foundry-smoke-tui/foundry.toml"),
		Destination: plan.DestinationInfo{
			Path:        absDest,
			Parent:      parent,
			Basename:    base,
			Observation: plan.ObservationAbsent,
		},
		Verify:         plan.VerifyDefault,
		GoBinary:       goBin,
		GitBinary:      firstNonEmpty(lookPath("git"), fixedGitBinary),
		GitTemplateDir: "",
		Host: plan.HostEnv{
			PATH:       os.Getenv("PATH"),
			HOME:       hostCapture().HOME,
			TMPDIR:     os.TempDir(),
			GOMODCACHE: hostCapture().GOMODCACHE,
			GOCACHE:    hostCapture().GOCACHE,
			GOPATH:     hostCapture().GOPATH,
			GOPROXY:    hostCapture().GOPROXY,
			GOSUMDB:    hostCapture().GOSUMDB,
		},
	}
	vs := mustValidateSmokeTUIGit(t, false)
	p, err := plan.Pipeline(vs, cat, opts)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	planSHA = p.PlanSHA256()

	host := hostCapture()
	host.PATH = filepath.Dir(goBin) + string(os.PathListSeparator) + host.PATH
	orch := &generate.Orchestrator{
		Plan:      p,
		Catalog:   cat,
		Host:      host,
		GoBinary:  goBin,
		GitBinary: opts.GitBinary,
		TempRoot:  t.TempDir(),
	}
	defer func() { _ = orch.Close() }()
	m := generate.New(generate.Config{Stages: orch.Stages()})
	res := m.Run(t.Context())
	if res.Outcome != generate.OutcomeCommitted {
		t.Fatalf("generate outcome=%s exit=%d stage=%s err=%v path=%s",
			res.Outcome, res.Exit, res.FailedStage, res.Err, res.StagePath)
	}
	return planSHA
}

func mustValidateSmokeTUIGit(t *testing.T, init bool) *spec.ValidatedSpecification {
	t.Helper()
	toml := string(smokeTUISpecBytes(t))
	if !init {
		if strings.Contains(toml, "init = true") {
			toml = strings.Replace(toml, "init = true", "init = false", 1)
		} else if !strings.Contains(toml, "[git]") {
			toml += "\n[git]\ninit = false\n"
		}
	}
	raw, err := spec.Decode("foundry.toml", []byte(toml))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return vs
}

func splitPath(p string) (parent, base string) {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ".", p
	}
	parent = p[:i]
	if parent == "" {
		parent = "/"
	}
	return parent, p[i+1:]
}

// treeGoldenLines formats sorted "path mode render content_sha256" rows.
func treeGoldenLines(p *plan.Plan) []byte {
	files := p.Files()
	lines := make([]string, 0, len(files))
	for _, f := range files {
		lines = append(lines, fmt.Sprintf("%s %s %s %s", f.Path, f.Mode, f.Render, f.ContentSHA256))
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "\n") + "\n")
}

// metaGolden records plan_sha256 + aggregate content digest for step logs / goldens.
func metaGolden(p *plan.Plan) []byte {
	agg := aggregateContentDigest(p)
	return []byte(fmt.Sprintf(
		"plan_sha256=%s\nfile_count=%d\ncontent_digest_aggregate=%s\n",
		p.PlanSHA256(), p.FileCount(), agg,
	))
}

func aggregateContentDigest(p *plan.Plan) string {
	files := p.Files()
	type row struct {
		path, dig string
	}
	rows := make([]row, 0, len(files))
	for _, f := range files {
		rows = append(rows, row{f.Path, f.ContentSHA256})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].path < rows[j].path })
	h := sha256.New()
	for _, r := range rows {
		_, _ = fmt.Fprintf(h, "%s=%s\n", r.path, r.dig)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// dumpPathList logs sorted paths (mismatch diagnostics).
func dumpPathList(t *testing.T, label string, paths []string) {
	t.Helper()
	sort.Strings(paths)
	t.Logf("%s path list (%d):\n%s", label, len(paths), strings.Join(paths, "\n"))
}

// writeMemoryToDisk materializes a MemoryWriter under root (0644/0755 dirs).
func writeMemoryToDisk(t *testing.T, w *render.MemoryWriter, root string) {
	t.Helper()
	for _, p := range w.Paths() {
		content, mode, ok := w.Get(p)
		if !ok {
			t.Fatalf("missing path in writer: %s", p)
		}
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		perm := os.FileMode(0o644)
		if mode == "0755" {
			perm = 0o755
		}
		if err := os.WriteFile(full, content, perm); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

// renderPlanToDir pure-renders plan files into dir (no generate lifecycle).
func renderPlanToDir(t *testing.T, p *plan.Plan, cat *catalog.Catalog, dir string) {
	t.Helper()
	jobs, err := generate.JobsFromPlan(p, cat)
	if err != nil {
		t.Fatalf("JobsFromPlan: %v", err)
	}
	mw := render.NewMemoryWriter()
	if _, err := render.RenderAll(cat, jobs, mw); err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	writeMemoryToDisk(t, mw, dir)
}

// nonGitTreeDigest walks dir excluding .git; returns sorted path=sha256 and aggregate.
func nonGitTreeDigest(t *testing.T, root string) (map[string]string, string, []string) {
	t.Helper()
	digests := map[string]string{}
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		// Exclude .git object store (time-based ids) from envelope.
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		digests[rel] = hex.EncodeToString(sum[:])
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		_, _ = fmt.Fprintf(h, "%s=%s\n", p, digests[p])
	}
	return digests, hex.EncodeToString(h.Sum(nil)), paths
}

// pinnedGoBinary returns an absolute go1.26.x toolchain binary usable for
// integration tests, or "" if none is discoverable (bead go-foundry-cli-wet.3.1).
//
// FOUNDRY_GO_BIN, when set, is trusted directly (docs/dev/testing.md /
// dogfood/README.md "Exact go1.26.5" override) — no version probing, so an
// operator-provided toolchain always wins. Otherwise this probes toolchain
// module-cache installs (any GOOS/GOARCH, any go1.26 patch — not just the
// exact go.mod-pinned go1.26.5) plus common system install locations, then
// falls back to PATH. Accepting the whole go1.26.x line here (rather than
// only go1.26.5) avoids spurious skips on typical dev machines running a
// slightly different 1.26 patch; catalog/versions.toml + go.mod remain the
// single source of truth for the exact generated-project pin.
func pinnedGoBinary(t *testing.T) string {
	t.Helper()
	return discoverPinnedGoBinary(t)
}

var goVersionLineRE = regexp.MustCompile(`go1\.26(\.\d+)?\b`)

// discoverPinnedGoBinary implements pinnedGoBinary's search; factored out so
// integration/fixtures and integration/dogfood share identical behavior.
func discoverPinnedGoBinary(t *testing.T) string {
	t.Helper()
	if override := os.Getenv("FOUNDRY_GO_BIN"); override != "" {
		if st, err := os.Stat(override); err == nil && !st.IsDir() {
			return override
		}
		t.Logf("FOUNDRY_GO_BIN=%q is not a usable file; falling back to discovery", override)
	}

	var candidates []string
	home := os.Getenv("HOME")
	if home != "" {
		modCache := os.Getenv("GOMODCACHE")
		if modCache == "" {
			modCache = filepath.Join(home, "go", "pkg", "mod")
		}
		pattern := filepath.Join(modCache, "golang.org",
			"toolchain@v0.0.1-go1.26.*."+runtime.GOOS+"-"+runtime.GOARCH, "bin", "go")
		if matches, err := filepath.Glob(pattern); err == nil {
			candidates = append(candidates, matches...)
		}
	}
	candidates = append(candidates,
		"/usr/local/go/bin/go",
		"/opt/homebrew/bin/go",
		"/usr/local/bin/go",
	)
	if p, err := exec.LookPath("go"); err == nil {
		candidates = append(candidates, p)
	}

	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err != nil || st.IsDir() {
			continue
		}
		out, err := exec.Command(c, "version").CombinedOutput()
		if err != nil {
			continue
		}
		if goVersionLineRE.Match(out) {
			return c
		}
	}
	return ""
}

// hostCapture builds toolrun.HostCapture from the real host for generate runs.
func hostCapture() toolrun.HostCapture {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return toolrun.HostCapture{
		PATH:       os.Getenv("PATH"),
		HOME:       home,
		TMPDIR:     os.TempDir(),
		GOMODCACHE: firstNonEmpty(os.Getenv("GOMODCACHE"), filepath.Join(home, "go", "pkg", "mod")),
		GOCACHE:    firstNonEmpty(os.Getenv("GOCACHE"), filepath.Join(home, ".cache", "go-build")),
		GOPATH:     firstNonEmpty(os.Getenv("GOPATH"), filepath.Join(home, "go")),
		GOPROXY:    firstNonEmpty(os.Getenv("GOPROXY"), "https://proxy.golang.org,direct"),
		GOSUMDB:    firstNonEmpty(os.Getenv("GOSUMDB"), "sum.golang.org"),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// privateParent creates a 0700 custody-safe directory (fsx Section 31.3).
// t.TempDir() is often 0775 and fails fs.namespace_not_private.
func privateParent(t *testing.T) string {
	t.Helper()
	// Prefer sticky /tmp (always admitted) then nest a 0700 child.
	dir, err := os.MkdirTemp(stickyTempBase(), "foundry-fix-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	return dir
}

// runGenerateOnce runs production generate into a private parent/foundry-smoke-cli.
// Returns destination path and plan_sha256.
func runGenerateOnce(t *testing.T, goBin string) (dest, planSHA string) {
	t.Helper()
	parentDir := privateParent(t)
	dest = filepath.Join(parentDir, "foundry-smoke-cli")
	planSHA = runGenerateTo(t, goBin, dest)
	return dest, planSHA
}

// runGenerateTo runs production generate into absDest (parent must be custody-safe).
// Returns plan_sha256. Prefer git init false so double-gen excludes .git noise.
func runGenerateTo(t *testing.T, goBin, absDest string) (planSHA string) {
	t.Helper()
	cat := mustLoadCatalog(t)

	absDest, err := filepath.Abs(absDest)
	if err != nil {
		t.Fatalf("abs dest: %v", err)
	}
	parent, base := filepath.Dir(absDest), filepath.Base(absDest)

	opts := plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  fixedCommit,
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       fixtureDir(t, "foundry-smoke-cli/foundry.toml"),
		Destination: plan.DestinationInfo{
			Path:        absDest,
			Parent:      parent,
			Basename:    base,
			Observation: plan.ObservationAbsent,
		},
		Verify:         plan.VerifyDefault,
		GoBinary:       goBin,
		GitBinary:      firstNonEmpty(lookPath("git"), fixedGitBinary),
		GitTemplateDir: "",
		Host: plan.HostEnv{
			PATH:       os.Getenv("PATH"),
			HOME:       hostCapture().HOME,
			TMPDIR:     os.TempDir(),
			GOMODCACHE: hostCapture().GOMODCACHE,
			GOCACHE:    hostCapture().GOCACHE,
			GOPATH:     hostCapture().GOPATH,
			GOPROXY:    hostCapture().GOPROXY,
			GOSUMDB:    hostCapture().GOSUMDB,
		},
	}
	// Prefer no git for double-generation (exclude .git object noise entirely).
	vs := mustValidateSmokeGit(t, false)

	p, err := plan.Pipeline(vs, cat, opts)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	planSHA = p.PlanSHA256()

	host := hostCapture()
	// Ensure go binary directory is first on PATH for LookPath fallbacks.
	host.PATH = filepath.Dir(goBin) + string(os.PathListSeparator) + host.PATH

	orch := &generate.Orchestrator{
		Plan:      p,
		Catalog:   cat,
		Host:      host,
		GoBinary:  goBin,
		GitBinary: opts.GitBinary,
		TempRoot:  t.TempDir(),
	}
	defer func() { _ = orch.Close() }()

	m := generate.New(generate.Config{
		Stages: orch.Stages(),
	})
	res := m.Run(t.Context())
	if res.Outcome != generate.OutcomeCommitted {
		t.Fatalf("generate outcome=%s exit=%d stage=%s err=%v path=%s",
			res.Outcome, res.Exit, res.FailedStage, res.Err, res.StagePath)
	}
	return planSHA
}

func mustValidateSmokeGit(t *testing.T, init bool) *spec.ValidatedSpecification {
	t.Helper()
	toml := string(smokeCLISpecBytes(t))
	if !init {
		if !strings.Contains(toml, "[git]") {
			toml += "\n[git]\ninit = false\n"
		}
	}
	raw, err := spec.Decode("foundry.toml", []byte(toml))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return vs
}

func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// applyExtensionOverlay copies overlay files and wires newPingCmd into root.go.
func applyExtensionOverlay(t *testing.T, projectRoot string) {
	t.Helper()
	// Overlay lives under testdata/ so Foundry archtest does not treat it as
	// product code (cobra is confined to cmd/foundry + internal/cli only).
	overlayRoot := fixtureDir(t, "extension-cli-subcommand/testdata/overlay")
	err := filepath.WalkDir(overlayRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(overlayRoot, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(projectRoot, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatalf("copy overlay: %v", err)
	}

	// Wire newPingCmd into AddCommand list (same growth step owners apply by hand).
	rootPath := filepath.Join(projectRoot, "internal", "cli", "root.go")
	body, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("read root.go: %v", err)
	}
	old := `cmd.AddCommand(
		newVersionCmd(),
		newCompletionCmd(),
	)`
	new := `cmd.AddCommand(
		newVersionCmd(),
		newCompletionCmd(),
		newPingCmd(),
	)`
	updated := strings.Replace(string(body), old, new, 1)
	if updated == string(body) {
		// Tolerate different indentation from templates.
		old2 := "newVersionCmd(),\n\t\tnewCompletionCmd(),"
		new2 := "newVersionCmd(),\n\t\tnewCompletionCmd(),\n\t\tnewPingCmd(),"
		updated = strings.Replace(string(body), old2, new2, 1)
	}
	if updated == string(body) {
		t.Fatalf("could not wire newPingCmd into root.go; content:\n%s", body)
	}
	if err := os.WriteFile(rootPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write root.go: %v", err)
	}
}

// goTestClean runs `go test -count=1 ./...` with a GOROOT-consistent env.
func goTestClean(t *testing.T, dir, goBin string) {
	t.Helper()
	cmd := exec.Command(goBin, "test", "-count=1", "./...")
	cmd.Dir = dir
	// Clean env so GOROOT comes from the go binary itself.
	home, _ := os.UserHomeDir()
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + filepath.Dir(goBin) + string(os.PathListSeparator) + "/usr/bin:/bin",
		"GOPATH=" + firstNonEmpty(os.Getenv("GOPATH"), filepath.Join(home, "go")),
		"GOMODCACHE=" + firstNonEmpty(os.Getenv("GOMODCACHE"), filepath.Join(home, "go", "pkg", "mod")),
		"GOCACHE=" + t.TempDir(),
		"GOPROXY=" + firstNonEmpty(os.Getenv("GOPROXY"), "https://proxy.golang.org,direct"),
		"GOSUMDB=" + firstNonEmpty(os.Getenv("GOSUMDB"), "sum.golang.org"),
		"GOTOOLCHAIN=local",
		"CGO_ENABLED=0",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./... failed: %v\n%s", err, out)
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// stickyTempBase returns a directory suitable for custody-private parents.
// On Unix prefer sticky /tmp when present (fsx §31.3 realism); elsewhere
// use os.TempDir so windows-unit hygiene can compile and run without
// hardcoded /tmp failures (go-foundry-cli-ipk.6).
func stickyTempBase() string {
	if runtime.GOOS != "windows" {
		if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
			return "/tmp"
		}
	}
	return os.TempDir()
}
