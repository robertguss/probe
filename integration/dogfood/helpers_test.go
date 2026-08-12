package dogfood_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// proc is one cli.Run result.
type proc struct {
	Args     []string
	Code     int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod not found at %s: %v", root, err)
	}
	return root
}

func smokeSpec(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "integration", "fixtures", "foundry-smoke-cli", "foundry.toml")
}

func repoMapSpec(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "dogfood", "repo-map", "foundry.toml")
}

func repoMapOverlay(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "dogfood", "repo-map", "testdata", "overlay")
}

// privateParent creates a 0700 custody-safe directory under sticky /tmp
// (fsx Section 31.3 — t.TempDir is often 0775 and fails namespace_not_private).
func privateParent(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(stickyTempBase(), "foundry-dogfood-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir
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
	home := os.Getenv("HOME")
	if override := os.Getenv("FOUNDRY_GO_BIN"); override != "" {
		if st, err := os.Stat(override); err == nil && !st.IsDir() {
			return override
		}
		t.Logf("FOUNDRY_GO_BIN=%q is not a usable file; falling back to discovery", override)
	}

	var candidates []string
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

	verRE := regexp.MustCompile(`go1\.26(\.\d+)?\b`)
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err != nil || st.IsDir() {
			continue
		}
		cmd := exec.Command(c, "version")
		cmd.Env = []string{
			"PATH=" + filepath.Dir(c),
			"HOME=" + home,
			"GOTOOLCHAIN=local",
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			continue
		}
		if verRE.Match(out) {
			return c
		}
	}
	return ""
}

func dogfoodOpts(t *testing.T, goBin string) cli.Options {
	t.Helper()
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	// Prepend pinned go so production LookPath and child tools see go1.26.5.
	path := filepath.Dir(goBin) + string(os.PathListSeparator) + firstNonEmpty(os.Getenv("PATH"), "/usr/bin:/bin")
	t.Setenv("PATH", path)
	t.Setenv("GOROOT", resolveGoroot(goBin))
	t.Setenv("GOTOOLCHAIN", "local")

	return cli.Options{
		PipelineHost: cli.PipelineHost{
			GoBinary:  goBin,
			GitBinary: lookPath("git"),
			Host: plan.HostEnv{
				PATH:       path,
				HOME:       home,
				TMPDIR:     os.TempDir(),
				GOMODCACHE: firstNonEmpty(os.Getenv("GOMODCACHE"), filepath.Join(home, "go", "pkg", "mod")),
				GOCACHE:    firstNonEmpty(os.Getenv("GOCACHE"), filepath.Join(home, ".cache", "go-build")),
				GOPATH:     firstNonEmpty(os.Getenv("GOPATH"), filepath.Join(home, "go")),
				GOPROXY:    firstNonEmpty(os.Getenv("GOPROXY"), "https://proxy.golang.org,direct"),
				GOSUMDB:    firstNonEmpty(os.Getenv("GOSUMDB"), "sum.golang.org"),
			},
		},
	}
}

func runCLI(t *testing.T, ctx context.Context, opts cli.Options, args ...string) proc {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	var stdout, stderr bytes.Buffer
	start := time.Now()
	code := cli.Run(ctx, args, cli.Streams{
		In:  bytes.NewReader(nil),
		Out: &stdout,
		Err: &stderr,
	}, opts)
	return proc{
		Args:     append([]string{"foundry"}, args...),
		Code:     code,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}
}

func logProc(log *testutil.Logger, name string, res proc, wantCode int) {
	failed := res.Code != wantCode
	log.Subprocess(name, res.Args, "-", res.Code, len(res.Stdout), len(res.Stderr),
		firstLine(res.Stdout+res.Stderr), lastLine(res.Stdout+res.Stderr), failed)
	log.Step(name+"_timing", testutil.OutcomeInfo,
		fmt.Sprintf("elapsed_ms=%d", res.Duration.Milliseconds()))
	if failed {
		log.Step(name+"_stdout_head", testutil.OutcomeFail, capBody(res.Stdout, 800))
		log.Step(name+"_stderr_head", testutil.OutcomeFail, capBody(res.Stderr, 800))
	}
	log.Assert(name+"_exit", res.Code == wantCode, wantCode, res.Code)
}

func planSHAFromText(stdout string) string {
	// "generate: wrote destination=... plan_sha256=<hex>"
	const marker = "plan_sha256="
	for _, ln := range strings.Split(stdout, "\n") {
		if i := strings.Index(ln, marker); i >= 0 {
			rest := strings.TrimSpace(ln[i+len(marker):])
			if j := strings.IndexAny(rest, " \t\r"); j >= 0 {
				rest = rest[:j]
			}
			return rest
		}
	}
	return ""
}

func applyOverlay(t *testing.T, projectRoot, overlayRoot string) {
	t.Helper()
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
}

// wireInventoryCmd patches root.go AddCommand list (manual growth step).
func wireInventoryCmd(t *testing.T, projectRoot string) {
	t.Helper()
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
		newInventoryCmd(),
	)`
	updated := strings.Replace(string(body), old, new, 1)
	if updated == string(body) {
		old2 := "newVersionCmd(),\n\t\tnewCompletionCmd(),"
		new2 := "newVersionCmd(),\n\t\tnewCompletionCmd(),\n\t\tnewInventoryCmd(),"
		updated = strings.Replace(string(body), old2, new2, 1)
	}
	if updated == string(body) {
		t.Fatalf("could not wire newInventoryCmd into root.go; content:\n%s", body)
	}
	if err := os.WriteFile(rootPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write root.go: %v", err)
	}
}

func goTestClean(t *testing.T, dir, goBin string) {
	t.Helper()
	cmd := exec.Command(goBin, "test", "-count=1", "./...")
	cmd.Dir = dir
	home, _ := os.UserHomeDir()
	cmd.Env = cleanGoEnv(t, goBin, home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./... failed: %v\n%s", err, out)
	}
}

func goBuild(t *testing.T, dir, goBin, packagePath, outBin string) {
	t.Helper()
	cmd := exec.Command(goBin, "build", "-o", outBin, packagePath)
	cmd.Dir = dir
	home, _ := os.UserHomeDir()
	cmd.Env = cleanGoEnv(t, goBin, home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
}

func runBin(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var outB, errB bytes.Buffer
	cmd.Stdout = &outB
	cmd.Stderr = &errB
	err := cmd.Run()
	code = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %s: %v", bin, err)
		}
	}
	return outB.String(), errB.String(), code
}

// resolveGoroot derives GOROOT from goBin, resolving symlinks first
// (bead go-foundry-cli-wet.3.1): many system installs (e.g. Homebrew's
// /opt/homebrew/bin/go) are a symlink into a Cellar path, and naively taking
// two directories up from the symlink itself yields a bogus GOROOT (e.g.
// "/opt"), which breaks `go vet`/`go test` ("no such tool"). Falls back to
// the naive two-dirs-up computation if symlink resolution fails.
func resolveGoroot(goBin string) string {
	real, err := filepath.EvalSymlinks(goBin)
	if err != nil || real == "" {
		real = goBin
	}
	return filepath.Dir(filepath.Dir(real))
}

func cleanGoEnv(t *testing.T, goBin, home string) []string {
	t.Helper()
	return []string{
		"HOME=" + home,
		"PATH=" + filepath.Dir(goBin) + string(os.PathListSeparator) + "/usr/bin:/bin",
		"GOROOT=" + resolveGoroot(goBin),
		"GOPATH=" + firstNonEmpty(os.Getenv("GOPATH"), filepath.Join(home, "go")),
		"GOMODCACHE=" + firstNonEmpty(os.Getenv("GOMODCACHE"), filepath.Join(home, "go", "pkg", "mod")),
		"GOCACHE=" + t.TempDir(),
		"GOPROXY=" + firstNonEmpty(os.Getenv("GOPROXY"), "https://proxy.golang.org,direct"),
		"GOSUMDB=" + firstNonEmpty(os.Getenv("GOSUMDB"), "sum.golang.org"),
		"GOTOOLCHAIN=local",
		"CGO_ENABLED=0",
	}
}

func listNonTestGeneratedFiles(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		// Count non-test product files for FND-014 retention metric.
		base := d.Name()
		if strings.HasSuffix(base, "_test.go") {
			return nil
		}
		if strings.Contains(rel, string(filepath.Separator)+"testdata"+string(filepath.Separator)) ||
			strings.HasPrefix(rel, "testdata"+string(filepath.Separator)) {
			return nil
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return paths
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func capBody(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
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
