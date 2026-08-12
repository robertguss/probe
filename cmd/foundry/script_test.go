// Process-boundary testscript harness for the real foundry binary (REQ-218).
//
// Write-free e2e lives under testdata/writefree/ and is owned by bead
// go-foundry-cli-5an.1 (P1.8.a). Generate e2e matrix: testdata/generate/ (j8h.2).
package main_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain registers the in-process foundry command so testscripts can
// `exec foundry …` without installing a separate binary on PATH.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"foundry": foundryMain,
	})
}

// foundryMain is the process entry for the testscript-registered foundry.
// Mirrors cmd/foundry/main.go without SIGPIPE/NotifyContext (test harness
// owns lifecycle); exits with the same codes as production.
func foundryMain() {
	code := cli.Run(context.Background(), os.Args[1:], cli.Streams{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}, cli.Options{})
	os.Exit(code)
}

// TestWriteFree runs process-boundary write-free e2e scripts (P1.8.a / 5an.1).
//
// Coverage (see testdata/writefree/*.txt):
//
//	version, catalog list/show, validate, plan, flags, stdin, profiles,
//	generate pure-path refusal, purity snapshot, help/dry-run contract.
//
// Run: go test ./cmd/foundry -run TestWriteFree
func TestWriteFree(t *testing.T) {
	repo, err := findRepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	t.Logf("STEPLOG package=cmd/foundry test=%s level=PHASE name=writefree_setup outcome=start detail=repo=%s",
		t.Name(), filepath.Base(repo))

	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "writefree"),
		Setup: func(e *testscript.Env) error {
			return writeFreeSetup(e, repo)
		},
		Cmds: writeFreeCmds(),
		Condition: func(cond string) (bool, error) {
			switch cond {
			case "root":
				// chmod-based permission-denied scenarios are meaningless
				// when running as root (all permission bits bypassed).
				return os.Geteuid() == 0, nil
			default:
				return false, fmt.Errorf("unknown condition %q", cond)
			}
		},
		// Explicit exec keeps process-boundary intent obvious in scripts.
		RequireExplicitExec: true,
	})
}

// TestPlanGenerateEquality runs process-boundary plan/generate plan_sha256
// equality scripts (Section 13.3 / j8h.4 / REQ-033).
//
// Coverage (testdata/plan_generate/*.txt):
//
//	default + strict verify, --dest override, smoke-cli fixture.
//
// Requires a working Go toolchain on PATH (real generate commits).
// Run: go test ./cmd/foundry -run TestPlanGenerateEquality
func TestPlanGenerateEquality(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real go subprocess testscript suite in short mode")
	}
	repo, err := findRepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	t.Logf("STEPLOG package=cmd/foundry test=%s level=PHASE name=plan_generate_equality_setup outcome=start detail=repo=%s",
		t.Name(), filepath.Base(repo))

	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "plan_generate"),
		Setup: func(e *testscript.Env) error {
			return planGenerateSetup(e, repo)
		},
		Cmds:                writeFreeCmds(),
		RequireExplicitExec: true,
	})
}

// TestGenerateE2E runs process-boundary generate matrix scripts (j8h.2 / P2.5.d).
//
// Coverage (testdata/generate/*.txt):
//
//	clean success default/strict, double-generation plan equality,
//	pre-existing destination exit 2, quiet/progress names,
//	TUI smoke success + TUI double-generation (hrc / afh).
//
// Full matrix (cancel/stream/injection/REQ-133/go test) lives under
// integration/generate/. Requires a working Go toolchain on PATH.
// Run: go test ./cmd/foundry -run TestGenerateE2E
func TestGenerateE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real go subprocess testscript suite in short mode")
	}
	repo, err := findRepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	t.Logf("STEPLOG package=cmd/foundry test=%s level=PHASE name=generate_e2e_setup outcome=start detail=repo=%s",
		t.Name(), filepath.Base(repo))

	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "generate"),
		Setup: func(e *testscript.Env) error {
			return generateE2ESetup(e, repo)
		},
		Cmds:                writeFreeCmds(),
		RequireExplicitExec: true,
	})
}

// planGenerateSetup seeds examples/ + smoke fixture for mutating e2e.
// Destinations must sit under a custody-safe parent (fsx 31.3): sticky /tmp
// plus a 0700 child — $WORK itself is typically 0775 and is refused.
//
// testscript defaults HOME=/no-home which empties module caches in the plan
// tool env; restore a real HOME + GO* cache roots so go mod tidy can run.
func planGenerateSetup(e *testscript.Env, repo string) error {
	if err := generateHostSetup(e, repo, "foundry-eq-"); err != nil {
		return err
	}
	// Integration smoke-cli fixture (plan golden source for Section 13.3).
	src := filepath.Join(repo, "integration", "fixtures", "foundry-smoke-cli")
	dst := filepath.Join(e.WorkDir, "fixtures", "foundry-smoke-cli")
	if err := copyDir(src, dst); err != nil {
		return fmt.Errorf("copy smoke-cli fixture: %w", err)
	}
	e.Setenv("EQ_ROOT", e.Getenv("GEN_ROOT"))
	return nil
}

// generateE2ESetup is planGenerateSetup + nogit + TUI smoke fixtures (j8h.2 / hrc).
func generateE2ESetup(e *testscript.Env, repo string) error {
	if err := generateHostSetup(e, repo, "foundry-gen-"); err != nil {
		return err
	}
	// nogit-cli fixture for double-generation / git.init=false cells.
	nogit := filepath.Join(e.WorkDir, "fixtures", "nogit-cli.toml")
	if err := os.MkdirAll(filepath.Dir(nogit), 0o755); err != nil {
		return err
	}
	body := []byte(`schema = 1
name = "nogit-cli"
module = "github.com/example/nogit-cli"
description = "generate e2e git.init=false"
archetype = "cli"
destination = "./nogit-cli"
profiles = []
[git]
init = false
`)
	if err := os.WriteFile(nogit, body, 0o644); err != nil {
		return err
	}
	// TUI smoke fixture (afh / hrc) — git init true for success path.
	smokeTUI := filepath.Join(e.WorkDir, "fixtures", "smoke-tui.toml")
	tuiBody := []byte(`schema = 1
name = "foundry-smoke-tui"
module = "github.com/example/foundry-smoke-tui"
description = "generate e2e TUI smoke"
archetype = "tui"
destination = "./foundry-smoke-tui"
profiles = []
[git]
init = true
initial_branch = "main"
`)
	if err := os.WriteFile(smokeTUI, tuiBody, 0o644); err != nil {
		return err
	}
	// TUI double-generation with git.init=false.
	tuiNogit := filepath.Join(e.WorkDir, "fixtures", "smoke-tui-nogit.toml")
	tuiNogitBody := []byte(`schema = 1
name = "foundry-smoke-tui"
module = "github.com/example/foundry-smoke-tui"
description = "generate e2e TUI double-generation git.init=false"
archetype = "tui"
destination = "./foundry-smoke-tui"
profiles = []
[git]
init = false
`)
	if err := os.WriteFile(tuiNogit, tuiNogitBody, 0o644); err != nil {
		return err
	}
	return nil
}

// generateHostSetup shares custody-safe GEN_ROOT + real HOME/GO* for generate e2e.
func generateHostSetup(e *testscript.Env, repo, tmpPrefix string) error {
	if err := writeFreeSetup(e, repo); err != nil {
		return err
	}
	genRoot, err := os.MkdirTemp(stickyTempBase(), tmpPrefix)
	if err != nil {
		return fmt.Errorf("mktemp generate parent: %w", err)
	}
	if err := os.Chmod(genRoot, 0o700); err != nil {
		_ = os.RemoveAll(genRoot)
		return fmt.Errorf("chmod generate parent: %w", err)
	}
	e.Setenv("GEN_ROOT", genRoot)
	e.Defer(func() { _ = os.RemoveAll(genRoot) })

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/tmp"
	}
	e.Setenv("HOME", home)
	if v := os.Getenv("GOMODCACHE"); v != "" {
		e.Setenv("GOMODCACHE", v)
	} else {
		e.Setenv("GOMODCACHE", filepath.Join(home, "go", "pkg", "mod"))
	}
	if v := os.Getenv("GOCACHE"); v != "" {
		e.Setenv("GOCACHE", v)
	} else {
		e.Setenv("GOCACHE", filepath.Join(home, ".cache", "go-build"))
	}
	if v := os.Getenv("GOPATH"); v != "" {
		e.Setenv("GOPATH", v)
	} else {
		e.Setenv("GOPATH", filepath.Join(home, "go"))
	}
	if v := os.Getenv("GOPROXY"); v != "" {
		e.Setenv("GOPROXY", v)
	} else {
		e.Setenv("GOPROXY", "https://proxy.golang.org,direct")
	}
	if v := os.Getenv("GOSUMDB"); v != "" {
		e.Setenv("GOSUMDB", v)
	} else {
		e.Setenv("GOSUMDB", "sum.golang.org")
	}
	tmp := filepath.Join(genRoot, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	e.Setenv("TMPDIR", tmp)
	return nil
}

// writeFreeSetup seeds $WORK with examples/ + specs/ fixtures and env vars
// so scripts never depend on absolute host homes in golden streams.
func writeFreeSetup(e *testscript.Env, repo string) error {
	e.Setenv("REPO", repo)
	e.Setenv("EXAMPLES", filepath.Join(repo, "examples"))
	e.Setenv("SPECS", filepath.Join(repo, "cmd", "foundry", "testdata", "specs"))
	e.Setenv("GOOS", runtime.GOOS)
	e.Setenv("GOARCH", runtime.GOARCH)

	// Copy product fixtures into the workdir so --spec uses relative paths
	// (host-independent logs; matches agent copy-paste workflow).
	if err := copyDir(
		filepath.Join(repo, "examples"),
		filepath.Join(e.WorkDir, "examples"),
	); err != nil {
		return fmt.Errorf("copy examples: %w", err)
	}
	if err := copyDir(
		filepath.Join(repo, "cmd", "foundry", "testdata", "specs"),
		filepath.Join(e.WorkDir, "specs"),
	); err != nil {
		return fmt.Errorf("copy specs: %w", err)
	}

	// Sentinel tree for purity / hard-stop mutation checks.
	sentinel := filepath.Join(e.WorkDir, "sentinel")
	if err := os.MkdirAll(sentinel, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sentinel, "marker"), []byte("pure\n"), 0o644); err != nil {
		return err
	}
	e.Setenv("SENTINEL", sentinel)
	return nil
}

// writeFreeCmds adds diagnosis helpers so failure output names exit codes,
// digests, plan_sha256, and error ids without a debugger.
func writeFreeCmds() map[string]func(ts *testscript.TestScript, neg bool, args []string) {
	return map[string]func(ts *testscript.TestScript, neg bool, args []string){
		// dump_digest path — print sha256 + byte length (basenames only in logs).
		"dump_digest": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("dump_digest does not support !")
			}
			if len(args) != 1 {
				ts.Fatalf("usage: dump_digest <file|stdout|stderr>")
			}
			data := readScriptFile(ts, args[0])
			sum := sha256.Sum256(data)
			ts.Logf("STEPLOG level=STEP name=dump_digest outcome=ok detail=path=%s bytes=%d sha256=%s",
				filepath.Base(args[0]), len(data), hex.EncodeToString(sum[:]))
		},
		// assert_plan_sha256_eq a.json b.json — equal plan_sha256 in two plan envelopes.
		"assert_plan_sha256_eq": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("assert_plan_sha256_eq does not support !")
			}
			if len(args) != 2 {
				ts.Fatalf("usage: assert_plan_sha256_eq <file1> <file2>")
			}
			a := extractPlanSHA256(ts, readScriptFile(ts, args[0]))
			b := extractPlanSHA256(ts, readScriptFile(ts, args[1]))
			if a == "" || b == "" {
				ts.Fatalf("missing plan_sha256 a=%q b=%q", a, b)
			}
			if a != b {
				ts.Fatalf("plan_sha256 mismatch: %s=%s %s=%s", args[0], a, args[1], b)
			}
			ts.Logf("STEPLOG level=ASSERT name=plan_sha256_eq outcome=ok detail=sha=%s", a)
		},
		// assert_plan_sha256_ne a.json b.json — strict vs default must differ.
		"assert_plan_sha256_ne": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("assert_plan_sha256_ne does not support !")
			}
			if len(args) != 2 {
				ts.Fatalf("usage: assert_plan_sha256_ne <file1> <file2>")
			}
			a := extractPlanSHA256(ts, readScriptFile(ts, args[0]))
			b := extractPlanSHA256(ts, readScriptFile(ts, args[1]))
			if a == "" || b == "" {
				ts.Fatalf("missing plan_sha256 a=%q b=%q", a, b)
			}
			if a == b {
				ts.Fatalf("plan_sha256 unexpectedly equal (%s): verify modes should differ", a)
			}
			ts.Logf("STEPLOG level=ASSERT name=plan_sha256_ne outcome=ok detail=a=%s b=%s", a, b)
		},
		// assert_error_id file id — JSON envelope or text stderr contains error id.
		"assert_error_id": func(ts *testscript.TestScript, neg bool, args []string) {
			if len(args) != 2 {
				ts.Fatalf("usage: [!] assert_error_id <file|stdout|stderr> <error_id>")
			}
			body := string(readScriptFile(ts, args[0]))
			id := args[1]
			found := strings.Contains(body, id)
			if neg {
				if found {
					ts.Fatalf("unexpected error id %q in %s", id, args[0])
				}
				return
			}
			if !found {
				// Cap body for diagnose-without-debugger requirement.
				ts.Fatalf("missing error id %q in %s; body_head=%q", id, args[0], truncateForLog(body, 400))
			}
			ts.Logf("STEPLOG level=ASSERT name=error_id outcome=ok detail=id=%s source=%s", id, args[0])
		},
		// list_tree dir — log relative entries (failure diagnosis / purity).
		"list_tree": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("list_tree does not support !")
			}
			if len(args) != 1 {
				ts.Fatalf("usage: list_tree <dir>")
			}
			dir := ts.MkAbs(args[0])
			var entries []string
			_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(dir, path)
				if rel == "." {
					return nil
				}
				entries = append(entries, rel)
				return nil
			})
			ts.Logf("STEPLOG level=STEP name=list_tree outcome=ok detail=dir=%s n=%d entries=%v",
				filepath.Base(args[0]), len(entries), entries)
		},
		// assert_tree_unchanged dir snapshot_file — purity: no new/removed paths.
		"assert_tree_unchanged": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("assert_tree_unchanged does not support !")
			}
			if len(args) != 2 {
				ts.Fatalf("usage: assert_tree_unchanged <dir> <snapshot_file>")
			}
			dir := ts.MkAbs(args[0])
			want := strings.TrimSpace(string(readScriptFile(ts, args[1])))
			got := strings.Join(listRelSorted(ts, dir), "\n")
			if got != want {
				ts.Fatalf("tree changed under %s\n--- want ---\n%s\n--- got ---\n%s",
					args[0], want, got)
			}
			ts.Logf("STEPLOG level=ASSERT name=tree_unchanged outcome=ok detail=dir=%s", args[0])
		},
		// assert_exit_code want_code foundry_args... — real process-boundary
		// exec via the registered "foundry" test command, asserting the
		// exact numeric exit code (not just zero/nonzero). stdout/stderr are
		// populated for subsequent stdout/stderr/assert_error_id directives,
		// same as a plain `exec` (bead go-foundry-cli-wet.2.6).
		"assert_exit_code": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("assert_exit_code does not support !")
			}
			if len(args) < 2 {
				ts.Fatalf("usage: assert_exit_code <want_code> foundry <args...>")
			}
			want, err := strconv.Atoi(args[0])
			if err != nil {
				ts.Fatalf("bad want_code %q: %v", args[0], err)
			}
			runErr := ts.Exec(args[1], args[2:]...)
			got := 0
			if runErr != nil {
				var exitErr *exec.ExitError
				if errors.As(runErr, &exitErr) {
					got = exitErr.ExitCode()
				} else {
					ts.Fatalf("assert_exit_code: %v", runErr)
				}
			}
			if got != want {
				ts.Fatalf("exit code mismatch: want %d got %d (argv=%v)\nstdout=%s\nstderr=%s",
					want, got, args[1:], ts.ReadFile("stdout"), ts.ReadFile("stderr"))
			}
			ts.Logf("STEPLOG level=ASSERT name=exit_code outcome=ok detail=code=%d argv=%v", got, args[1:])
		},
		// write_size path bytes — write a file of exactly N bytes (content is
		// arbitrary; used to synthesize spec.too_large without checking in a
		// >1MiB fixture — bead go-foundry-cli-wet.2.3).
		"write_size": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("write_size does not support !")
			}
			if len(args) != 2 {
				ts.Fatalf("usage: write_size <path> <bytes>")
			}
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 0 {
				ts.Fatalf("bad byte count %q: %v", args[1], err)
			}
			data := bytes.Repeat([]byte("x"), n)
			ts.Check(os.WriteFile(ts.MkAbs(args[0]), data, 0o644))
			ts.Logf("STEPLOG level=FIXTURE name=write_size outcome=ok detail=path=%s bytes=%d", args[0], n)
		},
		// snapshot_tree dir out_file — write sorted relative listing.
		"snapshot_tree": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("snapshot_tree does not support !")
			}
			if len(args) != 2 {
				ts.Fatalf("usage: snapshot_tree <dir> <out_file>")
			}
			dir := ts.MkAbs(args[0])
			body := strings.Join(listRelSorted(ts, dir), "\n")
			if body != "" {
				body += "\n"
			}
			ts.Check(os.WriteFile(ts.MkAbs(args[1]), []byte(body), 0o644))
			ts.Logf("STEPLOG level=FIXTURE name=snapshot_tree outcome=ok detail=dir=%s out=%s",
				args[0], args[1])
		},
	}
}

func readScriptFile(ts *testscript.TestScript, name string) []byte {
	switch name {
	case "stdout":
		return []byte(ts.ReadFile("stdout"))
	case "stderr":
		return []byte(ts.ReadFile("stderr"))
	default:
		return []byte(ts.ReadFile(name))
	}
}

func extractPlanSHA256(ts *testscript.TestScript, raw []byte) string {
	// Prefer JSON envelope result.plan_sha256; fall back to text "plan_sha256=".
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err == nil {
		if result, ok := env["result"].(map[string]any); ok {
			if sha, ok := result["plan_sha256"].(string); ok {
				return sha
			}
		}
		// Some success envelopes nest differently; scan top-level too.
		if sha, ok := env["plan_sha256"].(string); ok {
			return sha
		}
	}
	s := string(raw)
	const key = "plan_sha256="
	if i := strings.Index(s, key); i >= 0 {
		rest := s[i+len(key):]
		rest = strings.TrimSpace(rest)
		// Take hex run.
		j := 0
		for j < len(rest) && isHex(rest[j]) {
			j++
		}
		if j == 64 {
			return rest[:j]
		}
	}
	// JSON substring fallback.
	const jkey = `"plan_sha256":"`
	if i := strings.Index(s, jkey); i >= 0 {
		rest := s[i+len(jkey):]
		if k := strings.IndexByte(rest, '"'); k == 64 {
			return rest[:k]
		}
	}
	ts.Logf("STEPLOG level=FAIL name=extract_plan_sha256 outcome=fail detail=body_head=%q", truncateForLog(s, 200))
	return ""
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func listRelSorted(ts *testscript.TestScript, dir string) []string {
	var entries []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Normalize to slash for cross-platform snapshot equality.
		entries = append(entries, filepath.ToSlash(rel))
		return nil
	})
	ts.Check(err)
	sort.Strings(entries)
	return entries
}

func truncateForLog(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func findRepoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	// cmd/foundry/script_test.go → repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("go.mod not found at %s: %w", root, err)
	}
	return root, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
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
