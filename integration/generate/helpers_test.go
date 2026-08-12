package generatee2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	generatee2e "github.com/robertguss/go-foundry-cli/integration/generate"
	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
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

func examplesSpec(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "examples", name)
}

func smokeSpec(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "integration", "fixtures", "foundry-smoke-cli", "foundry.toml")
}

// privateParent creates a 0700 custody-safe directory under sticky /tmp.
func privateParent(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(stickyTempBase(), "foundry-e2e-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	return dir
}

// writeGitFalseSpec writes a minimal CLI spec with [git] init=false.
func writeGitFalseSpec(t *testing.T, dir, name string) string {
	t.Helper()
	body := fmt.Sprintf(`schema = 1
name = %q
module = "github.com/example/%s"
description = "generate e2e fixture git.init=false"
archetype = "cli"
destination = "./%s"
profiles = []
[git]
init = false
`, name, name, name)
	path := filepath.Join(dir, "foundry.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// runCLI runs foundry via cli.Run (production stages unless opts set).
func runCLI(t *testing.T, ctx context.Context, opts cli.Options, stdin io.Reader, args ...string) proc {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	var stdout, stderr bytes.Buffer
	in := io.Reader(bytes.NewReader(nil))
	if stdin != nil {
		in = stdin
	}
	start := time.Now()
	code := cli.Run(ctx, args, cli.Streams{
		In:  in,
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

// runCLIWriters uses custom writers (stream-failure dimensions).
func runCLIWriters(t *testing.T, ctx context.Context, opts cli.Options, out, errW io.Writer, args ...string) proc {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if errW == nil {
		errW = io.Discard
	}
	var errBuf bytes.Buffer
	teeErr := io.MultiWriter(errW, &errBuf)
	var outBuf bytes.Buffer
	teeOut := io.MultiWriter(out, &outBuf)
	start := time.Now()
	code := cli.Run(ctx, args, cli.Streams{
		In:  bytes.NewReader(nil),
		Out: teeOut,
		Err: teeErr,
	}, opts)
	return proc{
		Args:     append([]string{"foundry"}, args...),
		Code:     code,
		Stdout:   outBuf.String(),
		Stderr:   errBuf.String(),
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
		log.Step(name+"_stdout_head", testutil.OutcomeFail, capBody(res.Stdout, 600))
		log.Step(name+"_stderr_head", testutil.OutcomeFail, capBody(res.Stderr, 600))
	}
	log.Assert(name+"_exit", res.Code == wantCode, wantCode, res.Code)
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

func mustEnvelope(t *testing.T, log *testutil.Logger, stdout string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		log.Fail("json", err.Error()+" body="+capBody(stdout, 200))
	}
	return env
}

func planSHAFromEnvelope(env map[string]any) string {
	if env == nil {
		return ""
	}
	if r, ok := env["result"].(map[string]any); ok {
		if s, ok := r["plan_sha256"].(string); ok {
			return s
		}
	}
	return ""
}

func commitOutcomeFromEnvelope(env map[string]any) string {
	if env == nil {
		return ""
	}
	if r, ok := env["result"].(map[string]any); ok {
		if s, ok := r["commit_outcome"].(string); ok {
			return s
		}
	}
	return ""
}

func errorIDFromEnvelope(env map[string]any) string {
	if env == nil {
		return ""
	}
	if e, ok := env["error"].(map[string]any); ok {
		if s, ok := e["error_id"].(string); ok {
			return s
		}
	}
	return ""
}

func stagePathFromStreams(stdout, stderr string) string {
	for _, body := range []string{stderr, stdout} {
		const key = "stage_path:"
		if i := strings.Index(body, key); i >= 0 {
			rest := strings.TrimSpace(body[i+len(key):])
			if j := strings.IndexAny(rest, "\n\r"); j >= 0 {
				rest = rest[:j]
			}
			return strings.TrimSpace(rest)
		}
		const jkey = `"stage_path":"`
		if i := strings.Index(body, jkey); i >= 0 {
			rest := body[i+len(jkey):]
			if k := strings.IndexByte(rest, '"'); k >= 0 {
				return rest[:k]
			}
		}
	}
	return ""
}

func dumpFailure(t *testing.T, log *testutil.Logger, a generatee2e.FailureArtifact) {
	t.Helper()
	path, err := generatee2e.WriteFailureArtifact("", a)
	if err != nil {
		t.Logf("artifact write: %v", err)
		return
	}
	if path != "" {
		log.Step("failure_artifact", testutil.OutcomeInfo, filepath.Base(path))
		log.NotePath(path)
		t.Logf("generate_e2e_artifact=%s", path)
	}
	log.Step("failure_summary", testutil.OutcomeFail,
		fmt.Sprintf("stage=%s cause=%s exit=%d commit=%s plan=%s stage_path=%s",
			a.Stage, a.Cause, a.Exit, a.CommitResult, a.PlanSHA256, filepath.Base(a.StagePath)))
}

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

// scanContentREQ133 fails if host home path appears in non-git text files.
// Timestamps/usernames are checked conservatively (home path is the hard REQ-133 gate).
func scanContentREQ133(t *testing.T, log *testutil.Logger, root string) {
	t.Helper()
	home, _ := os.UserHomeDir()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !isMostlyText(b) {
			return nil
		}
		s := string(b)
		if home != "" && home != "/tmp" && home != "/" && strings.Contains(s, home) {
			log.Fail("req133_hostpath", rel+": contains host home path")
		}
		// RFC3339-ish wall clock with timezone must not appear.
		if strings.Contains(s, "T") && (strings.Contains(s, "Z") || strings.Contains(s, "+00:00")) {
			// Only flag if looks like full timestamp (YYYY-MM-DDTHH:MM).
			for i := 0; i+20 <= len(s); i++ {
				if s[i] >= '0' && s[i] <= '9' && i+19 < len(s) && s[i+4] == '-' && s[i+7] == '-' && s[i+10] == 'T' {
					log.Fail("req133_timestamp", rel+": possible wall-clock timestamp")
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Fail("walk_req133", err.Error())
	}
	log.Step("req133_scan", testutil.OutcomeOK, "root="+filepath.Base(root))
}

func isMostlyText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	n := len(b)
	if n > 4096 {
		n = 4096
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return false
		}
	}
	return true
}

func goTestGenerated(t *testing.T, log *testutil.Logger, dest string) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		log.Skip("go not on PATH")
		return
	}
	cmd := exec.Command(goBin, "test", "-count=1", "./...")
	cmd.Dir = dest
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
		log.Fail("go_test", fmt.Sprintf("%v\n%s", err, capBody(string(out), 800)))
	}
	log.Step("go_test", testutil.OutcomeOK, "ok")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func progressNamesFromStdout(stdout string) []string {
	var out []string
	for _, ln := range strings.Split(stdout, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "progress: ") {
			out = append(out, strings.TrimPrefix(ln, "progress: "))
		}
	}
	return out
}

func disclosureBeforeCreate(stdout string) (ok bool, detail string) {
	netIdx := strings.Index(stdout, "network disclosure:")
	if netIdx < 0 {
		netIdx = strings.Index(stdout, `"network_disclosure"`)
	}
	createIdx := strings.Index(stdout, "progress: create-stage")
	if createIdx < 0 {
		createIdx = strings.Index(stdout, "state: stage-created")
	}
	if netIdx < 0 {
		return false, "network disclosure missing"
	}
	if createIdx < 0 {
		return true, "network present; create progress suppressed or JSON-only"
	}
	if netIdx < createIdx {
		return true, "network before create-stage"
	}
	return false, "network disclosure after create-stage"
}

// failWriter fails after n successful Write calls (0 = fail immediately).
type failWriter struct {
	n     int
	calls int
	err   error
}

func (w *failWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.n {
		if w.err != nil {
			return 0, w.err
		}
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

// planExternalStepIDs runs plan JSON and returns ordered external_steps ids + plan_sha256.
func planExternalStepIDs(t *testing.T, log *testutil.Logger, specPath, absDest, verify string) (ids []string, sha string) {
	t.Helper()
	args := []string{"plan", "--spec", specPath, "--dest", absDest, "--output", "json"}
	if verify == "strict" {
		args = append(args, "--verify", "strict")
	}
	res := runCLI(t, context.Background(), cli.Options{}, nil, args...)
	logProc(log, "plan_"+verify, res, 0)
	if res.Code != 0 {
		return nil, ""
	}
	env := mustEnvelope(t, log, res.Stdout)
	sha = planSHAFromEnvelope(env)
	result, _ := env["result"].(map[string]any)
	steps, _ := result["external_steps"].([]any)
	for _, s := range steps {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := m["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids, sha
}

func wantToolIDs(verify string, gitInit bool) []string {
	mode := plan.VerifyDefault
	if verify == "strict" {
		mode = plan.VerifyStrict
	}
	return generate.PlannedToolStepIDs(mode, gitInit)
}

// buildRealPlan constructs a plan.Plan for stage injection (same pure path as generate).
func buildRealPlan(t *testing.T, specPath, absDest string, verify plan.VerifyMode, gitInit bool) *plan.Plan {
	t.Helper()
	b, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	// Override git.init when requested via rewrite for git-false fixtures already on disk.
	_ = gitInit
	raw, err := spec.Decode(filepath.Base(specPath), b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	absDest, err = filepath.Abs(absDest)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	parent, base := filepath.Dir(absDest), filepath.Base(absDest)
	host, err := toolrun.CaptureHost("")
	if err != nil {
		t.Fatalf("CaptureHost: %v", err)
	}
	goBin, _ := exec.LookPath("go")
	gitBin, _ := exec.LookPath("git")
	if goBin == "" {
		goBin = "/usr/local/go/bin/go"
	}
	if gitBin == "" {
		gitBin = "/usr/bin/git"
	}
	opts := plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  "testcommit000000000000000000000000000001",
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       specPath,
		Destination: plan.DestinationInfo{
			Path:        absDest,
			Parent:      parent,
			Basename:    base,
			Observation: plan.ObservationAbsent,
		},
		Verify:    verify,
		GoBinary:  goBin,
		GitBinary: gitBin,
		Host: plan.HostEnv{
			PATH:       host.PATH,
			HOME:       host.HOME,
			TMPDIR:     host.TMPDIR,
			GOMODCACHE: host.GOMODCACHE,
			GOCACHE:    host.GOCACHE,
			GOPATH:     host.GOPATH,
			GOPROXY:    host.GOPROXY,
			GOSUMDB:    host.GOSUMDB,
		},
	}
	p, err := plan.Pipeline(vs, cat, opts)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	return p
}

// productionStagesWithMutation builds real orchestrator stages then mutates the map.
func productionStagesWithMutation(t *testing.T, p *plan.Plan, mut func(map[generate.StageID]generate.StageFunc)) (
	map[generate.StageID]generate.StageFunc,
	func() error,
) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	host, err := toolrun.CaptureHost("")
	if err != nil {
		t.Fatalf("CaptureHost: %v", err)
	}
	goBin, _ := exec.LookPath("go")
	gitBin, _ := exec.LookPath("git")
	orch := &generate.Orchestrator{
		Plan:      p,
		Catalog:   cat,
		Host:      host,
		GoBinary:  goBin,
		GitBinary: gitBin,
		TempRoot:  t.TempDir(),
	}
	stages := orch.Stages()
	if mut != nil {
		mut(stages)
	}
	return stages, orch.Close
}

func exitClass(code int) string {
	switch code {
	case diagnostic.ExitSuccess:
		return "success"
	case diagnostic.ExitFailure:
		return "failure"
	case diagnostic.ExitUsage:
		return "usage"
	case diagnostic.ExitCancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("exit_%d", code)
	}
}

// skipIfShort skips multi-minute generate e2e under go test -short (ipk.5 / docs/dev/testing.md).
func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("short: multi-minute generate e2e; use go test without -short or make integration")
	}
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
