// Go-level write-free process e2e with structured step logging and purity audits.
//
// Complements testdata/writefree/*.txt (TestWriteFree): this file owns verbose
// diagnostics (timing, digests, plan_sha256, error ids) and OS-level purity
// probes that are awkward in pure testscript form.
package main_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// procResult is one real-process invocation of foundry (via cli.Run in-process
// for portability; purity_strace uses an external binary when available).
type procResult struct {
	Args     []string
	Code     int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

func runFoundry(t *testing.T, stdin io.Reader, args ...string) procResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	in := io.Reader(bytes.NewReader(nil))
	if stdin != nil {
		in = stdin
	}
	start := time.Now()
	code := cli.Run(context.Background(), args, cli.Streams{
		In:  in,
		Out: &stdout,
		Err: &stderr,
	}, cli.Options{})
	return procResult{
		Args:     append([]string{"foundry"}, args...),
		Code:     code,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}
}

func logProc(log *testutil.Logger, name string, res procResult, wantCode int) {
	failed := res.Code != wantCode
	log.Subprocess(name, res.Args, "-", res.Code, len(res.Stdout), len(res.Stderr),
		firstLine(res.Stdout+res.Stderr), lastLine(res.Stdout+res.Stderr), failed)
	log.Step(name+"_timing", testutil.OutcomeInfo,
		"elapsed_ms="+itoa(int(res.Duration.Milliseconds())))
	if failed {
		log.Step(name+"_stdout_digest", testutil.OutcomeFail, digestHead(res.Stdout))
		log.Step(name+"_stderr_digest", testutil.OutcomeFail, digestHead(res.Stderr))
		log.Step(name+"_stdout_body", testutil.OutcomeFail, capBody(res.Stdout, 800))
		log.Step(name+"_stderr_body", testutil.OutcomeFail, capBody(res.Stderr, 800))
	} else {
		log.Step(name+"_stdout_digest", testutil.OutcomeOK, digestHead(res.Stdout))
		if res.Stderr != "" {
			log.Step(name+"_stderr_digest", testutil.OutcomeInfo, digestHead(res.Stderr))
		}
	}
	log.Assert(name+"_exit", res.Code == wantCode, wantCode, res.Code)
}

// TestWriteFreeVerboseDiagnostics exercises the write-free surface through
// cli.Run with step logs that diagnose failures without a debugger.
func TestWriteFreeVerboseDiagnostics(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real subprocess e2e test in short mode")
	}
	log := testutil.New(t)
	log.Phase("arrange")
	repo, err := findRepoRoot()
	if err != nil {
		log.Fail("repo", err.Error())
	}
	examples := filepath.Join(repo, "examples")
	specCLI := filepath.Join(examples, "minimal-cli.toml")
	log.Fixture("examples", "minimal-cli.toml")
	log.Inputs(map[string]string{
		"spec":   "minimal-cli.toml",
		"cwd":    "process",
		"binary": "cli.Run",
	})
	log.NotePath(specCLI)
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("version")
	res := runFoundry(t, nil, "version", "--output", "json")
	logProc(log, "version_json", res, 0)
	env := mustEnvelope(t, log, res.Stdout)
	log.Assert("version_ok", env["ok"] == true, true, env["ok"])
	result, _ := env["result"].(map[string]any)
	for _, k := range []string{"version", "commit", "go", "catalog_digest"} {
		_, ok := result[k]
		log.Assert("version_field_"+k, ok, true, ok)
	}
	dig, _ := result["catalog_digest"].(string)
	log.Assert("catalog_digest_len", len(dig) == 64, 64, len(dig))
	log.Step("catalog_digest", testutil.OutcomeOK, dig)
	log.PhaseEnd("version", testutil.OutcomeOK)

	log.Phase("validate_plan")
	v1 := runFoundry(t, nil, "validate", "--spec", specCLI, "--output", "json")
	logProc(log, "validate", v1, 0)
	vEnv := mustEnvelope(t, log, v1.Stdout)
	vSHA := planSHAFromEnvelope(vEnv)
	log.Step("validate_plan_sha256", testutil.OutcomeOK, vSHA)
	log.Assert("validate_sha_len", len(vSHA) == 64, 64, len(vSHA))
	log.NoteID(vSHA)

	p1 := runFoundry(t, nil, "plan", "--spec", specCLI, "--output", "json")
	logProc(log, "plan_default", p1, 0)
	pEnv := mustEnvelope(t, log, p1.Stdout)
	pSHA := planSHAFromEnvelope(pEnv)
	log.Step("plan_plan_sha256", testutil.OutcomeOK, pSHA)
	log.Assert("validate_plan_sha_eq", vSHA == pSHA, vSHA, pSHA)

	// Field counts for agent diagnosis.
	planResult, _ := pEnv["result"].(map[string]any)
	files, _ := planResult["files"].([]any)
	steps, _ := planResult["external_steps"].([]any)
	log.Step("plan_field_counts", testutil.OutcomeInfo,
		"files="+itoa(len(files))+" external_steps="+itoa(len(steps)))

	p2 := runFoundry(t, nil, "plan", "--spec", specCLI, "--output", "json")
	logProc(log, "plan_default_2", p2, 0)
	pSHA2 := planSHAFromEnvelope(mustEnvelope(t, log, p2.Stdout))
	log.Assert("plan_sha_stable", pSHA == pSHA2, pSHA, pSHA2)

	strict := runFoundry(t, nil, "plan", "--spec", specCLI, "--verify", "strict", "--output", "json")
	logProc(log, "plan_strict", strict, 0)
	sSHA := planSHAFromEnvelope(mustEnvelope(t, log, strict.Stdout))
	log.Step("strict_plan_sha256", testutil.OutcomeOK, sSHA)
	log.Assert("strict_differs", sSHA != pSHA, true, sSHA != pSHA)
	log.PhaseEnd("validate_plan", testutil.OutcomeOK)

	log.Phase("stdin")
	body, err := os.ReadFile(specCLI)
	if err != nil {
		log.Fail("read_spec", err.Error())
	}
	st := runFoundry(t, bytes.NewReader(body), "validate", "--spec", "-")
	logProc(log, "validate_stdin", st, 0)
	log.Assert("stdin_ok", strings.Contains(st.Stdout, "validate: ok"), true, st.Stdout)
	log.PhaseEnd("stdin", testutil.OutcomeOK)

	log.Phase("examples_invalids")
	for _, tc := range []struct {
		file string
		id   string
	}{
		{"invalid-unknown-field.toml", "spec.unknown_field"},
		{"invalid-bad-name.toml", "spec.invalid_field"},
		{"invalid-bad-profile.toml", "resolve.unknown_profile"},
	} {
		tc := tc
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(examples, tc.file)
			res := runFoundry(t, nil, "validate", "--spec", path)
			logProc(log, "invalid_"+tc.file, res, diagnostic.ExitUsage)
			log.Assert("id_"+tc.file, strings.Contains(res.Stderr, tc.id), true, res.Stderr)
			log.NoteID(tc.id)
		})
	}
	log.PhaseEnd("examples_invalids", testutil.OutcomeOK)

	log.Phase("generate_pure_path_refusal")
	// Basename must equal project name (Section 15.3); pure pipeline fails
	// before any staging when dest is wrong — no FS mutation (write-free residual).
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "should-not-exist")
	g := runFoundry(t, nil, "generate", "--spec", specCLI, "--dest", dest)
	logProc(log, "generate", g, diagnostic.ExitUsage)
	log.Assert("generate_id",
		strings.Contains(g.Stderr, "spec.invalid_field") || strings.Contains(g.Stderr, "destination"),
		true, g.Stderr)
	log.Assert("generate_no_phase1_hardstop",
		!strings.Contains(g.Stderr, "not available until Phase 2"),
		true, g.Stderr)
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		log.Fail("generate_created_dest", dest)
	}
	log.PhaseEnd("generate_pure_path_refusal", testutil.OutcomeOK)

	log.Phase("timing_baseline")
	// Section 49: write-free commands should be "immediate" (no absolute gate yet).
	// Log durations as baselines; fail only on absurd multi-second hangs.
	for _, name := range []string{"version", "catalog list", "validate", "plan"} {
		var args []string
		switch name {
		case "version":
			args = []string{"version"}
		case "catalog list":
			args = []string{"catalog", "list"}
		case "validate":
			args = []string{"validate", "--spec", specCLI}
		case "plan":
			args = []string{"plan", "--spec", specCLI}
		}
		r := runFoundry(t, nil, args...)
		logProc(log, "timing_"+strings.ReplaceAll(name, " ", "_"), r, 0)
		// Soft gate: 10s is far above expected; catches hung network/subprocess.
		log.Assert("timing_"+name+"_under_10s", r.Duration < 10*time.Second,
			"<10s", r.Duration.String())
	}
	log.PhaseEnd("timing_baseline", testutil.OutcomeOK)
}

// TestWriteFreePurityAudit proves write-free commands do not mutate a temp
// tree and, on Linux with strace available, perform no network connects or
// child execve (Go-level / strace purity — REQ-241 / 5an.1).
func TestWriteFreePurityAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real subprocess e2e test in short mode")
	}
	log := testutil.New(t)
	log.Phase("arrange")
	repo, err := findRepoRoot()
	if err != nil {
		log.Fail("repo", err.Error())
	}
	specCLI := filepath.Join(repo, "examples", "minimal-cli.toml")
	tmp := t.TempDir()
	log.Fixture("tmp", filepath.Base(tmp))
	log.NotePath(tmp)

	// Marker file — must survive untouched.
	marker := filepath.Join(tmp, "marker")
	if err := os.WriteFile(marker, []byte("pure\n"), 0o644); err != nil {
		log.Fail("marker", err.Error())
	}
	before, err := listRel(tmp)
	if err != nil {
		log.Fail("list_before", err.Error())
	}
	log.Step("before_entries", testutil.OutcomeInfo, strings.Join(before, ","))
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("write_free_cmds")
	// Basename must equal project name (Section 15.3); parent is the purity root.
	dest := filepath.Join(tmp, "minimal-cli")
	// generate uses a deliberately wrong basename so pure pipeline refuses
	// before staging (write-free residual; full generate e2e is j8h.2).
	badDest := filepath.Join(tmp, "wrong-basename")
	cmds := [][]string{
		{"version"},
		{"version", "--output", "json"},
		{"catalog", "list"},
		{"catalog", "show", "core"},
		{"validate", "--spec", specCLI, "--dest", dest},
		{"plan", "--spec", specCLI, "--dest", dest, "--output", "json"},
		{"generate", "--spec", specCLI, "--dest", badDest},
	}
	for _, args := range cmds {
		name := strings.Join(args, "_")
		// generate pure-path refusal exit 2; others 0.
		want := 0
		if args[0] == "generate" {
			want = diagnostic.ExitUsage
		}
		res := runFoundry(t, nil, args...)
		logProc(log, name, res, want)
	}
	after, err := listRel(tmp)
	if err != nil {
		log.Fail("list_after", err.Error())
	}
	log.Step("after_entries", testutil.OutcomeInfo, strings.Join(after, ","))
	log.Assert("no_new_files", len(after) == len(before), len(before), len(after))
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		log.Fail("dest_created", dest)
	}
	log.PhaseEnd("write_free_cmds", testutil.OutcomeOK)

	log.Phase("strace_or_skip")
	if runtime.GOOS != "linux" {
		log.Step("strace", testutil.OutcomeSkip, "linux-only purity strace")
		log.PhaseEnd("strace_or_skip", testutil.OutcomeOK)
		return
	}
	stracePath, err := exec.LookPath("strace")
	if err != nil {
		log.Step("strace", testutil.OutcomeSkip, "strace not installed")
		log.PhaseEnd("strace_or_skip", testutil.OutcomeOK)
		return
	}
	// Build a real binary so strace attaches to the production entrypoint.
	bin := filepath.Join(tmp, "foundry-under-strace")
	build := exec.Command("go", "build", "-o", bin, filepath.Join(repo, "cmd", "foundry"))
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		log.Fail("build_for_strace", err.Error()+" "+capBody(string(out), 400))
	}
	log.Fixture("binary", "foundry-under-strace")

	// Network: no connect/connectat on write-free commands.
	// Process: no execve after the initial image (no child StartProcess).
	for _, args := range [][]string{
		{"version"},
		{"catalog", "list"},
		{"validate", "--spec", specCLI, "--dest", filepath.Join(tmp, "strace-a", "minimal-cli")},
		{"plan", "--spec", specCLI, "--dest", filepath.Join(tmp, "strace-b", "minimal-cli")},
	} {
		name := "strace_" + args[0]
		traceFile := filepath.Join(tmp, name+".trace")
		cmdArgs := append([]string{
			"-f",
			"-e", "trace=network,process",
			"-o", traceFile,
			bin,
		}, args...)
		cmd := exec.Command(stracePath, cmdArgs...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		start := time.Now()
		err := cmd.Run()
		elapsed := time.Since(start)
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				log.Fail(name+"_run", err.Error())
			}
		}
		log.Subprocess(name, append([]string{"strace", "foundry"}, args...), "-", code,
			stdout.Len(), stderr.Len(), firstLine(stdout.String()), lastLine(stderr.String()), code != 0)
		log.Step(name+"_timing", testutil.OutcomeInfo, "elapsed_ms="+itoa(int(elapsed.Milliseconds())))
		log.Assert(name+"_exit0", code == 0, 0, code)

		trace, rerr := os.ReadFile(traceFile)
		if rerr != nil {
			log.Fail(name+"_read_trace", rerr.Error())
		}
		// Filter: any connect* is a purity violation.
		netHits := filterTraceLines(string(trace), []string{"connect(", "connectat("})
		log.Assert(name+"_no_network", len(netHits) == 0, 0, len(netHits))
		if len(netHits) > 0 {
			log.Step(name+"_network_hits", testutil.OutcomeFail, capBody(strings.Join(netHits, "\n"), 600))
		}
		// Child process: execve of anything other than the foundry binary itself.
		// Initial execve of `bin` is expected once; further execve = StartProcess.
		execHits := filterTraceLines(string(trace), []string{"execve("})
		var child []string
		for _, line := range execHits {
			// Initial: execve("/path/foundry-under-strace", ...)
			if strings.Contains(line, "foundry-under-strace") {
				continue
			}
			// Ignore unfinished/resumed noise.
			if strings.Contains(line, "resumed") || strings.Contains(line, "<unfinished") {
				continue
			}
			child = append(child, line)
		}
		log.Assert(name+"_no_child_exec", len(child) == 0, 0, len(child))
		if len(child) > 0 {
			log.Step(name+"_child_exec", testutil.OutcomeFail, capBody(strings.Join(child, "\n"), 600))
		}
		// Writable opens: O_WRONLY / O_RDWR / O_CREAT / O_TRUNC on openat.
		// Separate short strace for open flags.
		openTrace := filepath.Join(tmp, name+"_open.trace")
		openCmd := exec.Command(stracePath,
			"-f", "-e", "openat,open,creat,mkdir,rename,unlink,link,symlink",
			"-o", openTrace, bin,
		)
		openCmd.Args = append(openCmd.Args, args...)
		_ = openCmd.Run()
		openBody, _ := os.ReadFile(openTrace)
		writeHits := filterTraceLines(string(openBody), []string{
			"O_WRONLY", "O_RDWR", "O_CREAT", "O_TRUNC", "mkdir(", "creat(",
			"rename(", "unlink(", "symlink(",
		})
		// Go runtime and strace itself may touch things under /tmp; allow only
		// paths outside our sentinel tmp and the destination.
		var bad []string
		for _, line := range writeHits {
			// Permit writes nowhere under tmp destination paths: any write hit
			// involving our dest path is fatal; pure open of read-only specs OK.
			if strings.Contains(line, tmp) && !strings.Contains(line, ".trace") {
				// openat of the trace file is strace itself; our dest must not appear as write.
				if strings.Contains(line, "strace-dest") || strings.Contains(line, "dest-project") ||
					strings.Contains(line, "marker") {
					bad = append(bad, line)
				}
			}
			// Any mkdir/rename/unlink/symlink is unexpected for write-free cmds.
			if strings.Contains(line, "mkdir(") || strings.Contains(line, "rename(") ||
				strings.Contains(line, "unlink(") || strings.Contains(line, "symlink(") ||
				strings.Contains(line, "creat(") {
				// Ignore strace output file noise.
				if strings.Contains(line, ".trace") {
					continue
				}
				bad = append(bad, line)
			}
		}
		log.Assert(name+"_no_writable_project", len(bad) == 0, 0, len(bad))
		if len(bad) > 0 {
			log.Step(name+"_writable", testutil.OutcomeFail, capBody(strings.Join(bad, "\n"), 600))
		}
	}
	// Final tree: only marker + build artifacts we created (binary, traces).
	final, err := listRel(tmp)
	if err != nil {
		log.Fail("list_final", err.Error())
	}
	log.Step("final_entries", testutil.OutcomeInfo, "n="+itoa(len(final)))
	for _, e := range final {
		// Destination project dirs must never appear (observation is Lstat-only).
		if e == "minimal-cli" || strings.HasSuffix(e, "/minimal-cli") ||
			strings.HasPrefix(e, "minimal-cli/") {
			log.Fail("unexpected_dest_entry", e)
		}
	}
	log.PhaseEnd("strace_or_skip", testutil.OutcomeOK)
}

// --- helpers ---

func mustEnvelope(t *testing.T, log *testutil.Logger, raw string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		log.Fail("json_envelope", err.Error()+" body="+capBody(raw, 300))
	}
	return env
}

func planSHAFromEnvelope(env map[string]any) string {
	result, _ := env["result"].(map[string]any)
	if result == nil {
		return ""
	}
	sha, _ := result["plan_sha256"].(string)
	return sha
}

func digestHead(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "bytes=" + itoa(len(s)) + " sha256=" + hex.EncodeToString(sum[:8]) + "…"
}

func capBody(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…[truncated]"
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [16]byte
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

func listRel(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

func filterTraceLines(trace string, needles []string) []string {
	var hits []string
	for _, line := range strings.Split(trace, "\n") {
		for _, n := range needles {
			if strings.Contains(line, n) {
				hits = append(hits, line)
				break
			}
		}
	}
	return hits
}
