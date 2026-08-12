package tui_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// privateParent creates a 0700 custody-safe directory under sticky /tmp.
func privateParent(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(stickyTempBase(), "foundry-tui-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir
}

// generateMinimalTUI runs foundry generate for a private TUI project and
// returns the destination path. Uses production pipeline (real verify).
func generateMinimalTUI(t *testing.T, log *testutil.Logger) string {
	t.Helper()
	return generateTUI(t, log, tuiGenOpts{
		name:     "smoke-tui",
		profiles: nil,
		public:   false,
	})
}

// generateDistributionTUI generates a public TUI with the distribution profile
// (product E2E C4 / go-foundry-cli-79a.6).
func generateDistributionTUI(t *testing.T, log *testutil.Logger) string {
	t.Helper()
	return generateTUI(t, log, tuiGenOpts{
		name:     "dist-tui",
		profiles: []string{"distribution"},
		public:   true,
	})
}

type tuiGenOpts struct {
	name     string
	profiles []string
	public   bool
}

func generateTUI(t *testing.T, log *testutil.Logger, o tuiGenOpts) string {
	t.Helper()
	parent := privateParent(t)
	name := o.name
	dest := filepath.Join(parent, name)
	prof := "[]"
	if len(o.profiles) > 0 {
		parts := make([]string, len(o.profiles))
		for i, p := range o.profiles {
			parts[i] = fmt.Sprintf("%q", p)
		}
		prof = "[" + strings.Join(parts, ", ") + "]"
	}
	vis := ""
	if o.public {
		vis = "visibility = \"public\"\n"
	}
	specBody := fmt.Sprintf(`schema = 1
name = %q
module = "github.com/example/%s"
description = "TUI product e2e fixture"
archetype = "tui"
destination = %q
%sprofiles = %s
[git]
init = true
initial_branch = "main"
`, name, name, dest, vis, prof)
	specPath := filepath.Join(parent, "foundry.toml")
	if err := os.WriteFile(specPath, []byte(specBody), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	log.Fixture("spec", specPath)
	log.Fixture("destination", dest)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := cli.Run(ctx, []string{"generate", "--spec", specPath, "--output", "json"}, cli.Streams{
		In:  bytes.NewReader(nil),
		Out: &stdout,
		Err: &stderr,
	}, cli.Options{})
	log.Step("generate", testutil.OutcomeOK, fmt.Sprintf("exit=%d stdout_bytes=%d stderr_bytes=%d", code, stdout.Len(), stderr.Len()))
	if code != 0 {
		log.Fail("generate", fmt.Sprintf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String()))
	}
	if _, err := os.Stat(filepath.Join(dest, "cmd", name, "main.go")); err != nil {
		log.Fail("dest_main", err.Error())
	}
	return dest
}

// buildTUIBinary builds cmd/<name> in dest and returns the binary path.
func buildTUIBinary(t *testing.T, log *testutil.Logger, dest, name string) string {
	t.Helper()
	bin := filepath.Join(dest, "bin-"+name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/"+name)
	cmd.Dir = dest
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.26.5", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	log.Step("build", testutil.OutcomeOK, fmt.Sprintf("err=%v out_bytes=%d", err, len(out)))
	if err != nil {
		log.Fail("build", string(out)+" err="+err.Error())
	}
	return bin
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
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
