package e3

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Stable skip-notice text (golden). Must appear when race prerequisites are
// missing so CI never records a silent pass for an unrun race job.
//
// Promote this constant into generated strict.yml / Foundry CI race steps.
const SkipNoticeMissingCompiler = "race detector skipped: host C compiler unavailable " +
	"(requires CGO_ENABLED=1 and a working C compiler); this is not a silent pass"

// SkipNoticeCGODisabled is emitted when CGO cannot be enabled for the race job.
const SkipNoticeCGODisabled = "race detector skipped: CGO_ENABLED=0 " +
	"(race requires CGO_ENABLED=1 after compiler preflight); this is not a silent pass"

// PreflightResult is the outcome of race-detector prerequisite checks.
type PreflightResult struct {
	OK           bool   // true only when race jobs may run
	SkipNotice   string // non-empty when !OK; stable golden text
	CGOEnabled   string // resolved CGO_ENABLED for the race job ("1" when OK)
	Compiler     string // basename (gcc, clang, cc, …)
	CompilerPath string // absolute path when found
	CompilerOut  string // version line / probe detail
	Detail       string
}

// Preflight discovers whether this host can run `go test -race`.
//
// Contract (Section 44.3 / FND-004):
//   - Race needs CGO_ENABLED=1 and a host C compiler.
//   - Missing prerequisites → explicit SkipNotice, never silent OK.
//   - Product/release builds remain CGO_ENABLED=0 (not checked here).
func Preflight() PreflightResult {
	return PreflightWithEnv(os.Environ())
}

// PreflightWithEnv is Preflight with a controlled environment (tests inject
// PATH / CC / CGO_ENABLED without mutating the process permanently).
func PreflightWithEnv(env []string) PreflightResult {
	envMap := envToMap(env)

	// Race jobs always request CGO=1; a hard force of 0 is a skip, not OK.
	if v, ok := envMap["CGO_ENABLED"]; ok && v == "0" {
		return PreflightResult{
			OK:         false,
			SkipNotice: SkipNoticeCGODisabled,
			CGOEnabled: "0",
			Detail:     "CGO_ENABLED=0 in preflight environment",
		}
	}

	ccName, ccPath, how := resolveCompiler(envMap)
	if ccPath == "" {
		return PreflightResult{
			OK:         false,
			SkipNotice: SkipNoticeMissingCompiler,
			CGOEnabled: "1",
			Compiler:   ccName,
			Detail:     "no C compiler on PATH or via CC; " + how,
		}
	}

	// Prove the compiler can actually produce an object (not just exist).
	verOut, err := probeCompiler(ccPath, env)
	if err != nil {
		return PreflightResult{
			OK:           false,
			SkipNotice:   SkipNoticeMissingCompiler,
			CGOEnabled:   "1",
			Compiler:     filepath.Base(ccPath),
			CompilerPath: ccPath,
			CompilerOut:  verOut,
			Detail:       fmt.Sprintf("compiler probe failed: %v; %s", err, how),
		}
	}

	return PreflightResult{
		OK:           true,
		CGOEnabled:   "1",
		Compiler:     filepath.Base(ccPath),
		CompilerPath: ccPath,
		CompilerOut:  strings.TrimSpace(verOut),
		Detail:       "compiler preflight ok; " + how,
	}
}

// FormatSkipNotice returns the notice that CI/scripts must print when !OK.
// Empty string when OK.
func (r PreflightResult) FormatSkipNotice() string {
	if r.OK {
		return ""
	}
	if r.SkipNotice != "" {
		return r.SkipNotice
	}
	return SkipNoticeMissingCompiler
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		m[k] = v
	}
	return m
}

// resolveCompiler prefers CC from the environment, then gcc, clang, cc.
// Returns (basename, absolute path, how-resolved).
func resolveCompiler(envMap map[string]string) (name, path, how string) {
	pathEnv := envMap["PATH"]
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}

	if cc := strings.TrimSpace(envMap["CC"]); cc != "" {
		if filepath.IsAbs(cc) {
			if st, err := os.Stat(cc); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
				return filepath.Base(cc), cc, "CC=" + cc
			}
		}
		if p, err := lookPathIn(cc, pathEnv); err == nil {
			return filepath.Base(p), p, "CC=" + cc + " via PATH"
		}
		return filepath.Base(cc), "", "CC=" + cc + " not executable"
	}

	// Prefer real compilers over a possibly-broken `cc` symlink/alias.
	// On some developer hosts `cc` may be an interactive shell alias; we only
	// resolve via PATH lookup of real binaries.
	candidates := []string{"gcc", "clang", "cc"}
	if runtime.GOOS == "darwin" {
		// Xcode/CLT often exposes clang as the system compiler first.
		candidates = []string{"clang", "gcc", "cc"}
	}
	for _, c := range candidates {
		if p, err := lookPathIn(c, pathEnv); err == nil {
			return c, p, "PATH lookup " + c
		}
	}
	return "", "", "PATH has no gcc/clang/cc"
}

// lookPathIn is exec.LookPath with an explicit PATH (does not consult aliases).
func lookPathIn(file, pathEnv string) (string, error) {
	if strings.Contains(file, string(os.PathSeparator)) {
		return exec.LookPath(file)
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		p := filepath.Join(dir, file)
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			continue
		}
		// Executable bit (unix); on Windows Mode bits differ but we only support macOS/Linux.
		if st.Mode()&0o111 == 0 {
			continue
		}
		return p, nil
	}
	return "", exec.ErrNotFound
}

// probeCompiler compiles a trivial C translation unit to prove the toolchain works.
func probeCompiler(ccPath string, env []string) (string, error) {
	dir, err := os.MkdirTemp("", "e3-cc-probe-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "probe.c")
	out := filepath.Join(dir, "probe.o")
	if err := os.WriteFile(src, []byte("int e3_race_preflight(void){return 0;}\n"), 0o644); err != nil {
		return "", err
	}

	// Version line (best-effort; some compilers use different flags).
	ver := runCapture(ccPath, env, "--version")
	if ver == "" {
		ver = runCapture(ccPath, env, "-v")
	}
	verLine := firstLine(ver)

	cmd := exec.Command(ccPath, "-c", src, "-o", out)
	cmd.Env = ensurePath(env)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return verLine, fmt.Errorf("%s -c failed: %w: %s", ccPath, err, strings.TrimSpace(stderr.String()))
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return verLine, fmt.Errorf("compiler produced no object at %s", out)
	}
	return verLine, nil
}

func runCapture(bin string, env []string, args ...string) string {
	cmd := exec.Command(bin, args...)
	cmd.Env = ensurePath(env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out))
	}
	return strings.TrimSpace(string(out))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func ensurePath(env []string) []string {
	hasPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		return append(append([]string{}, env...), "PATH="+os.Getenv("PATH"))
	}
	return env
}

// SanitizeEnvForRace builds an environment for a race subprocess:
// CGO_ENABLED=1 and an explicit CC when preflight succeeded.
func SanitizeEnvForRace(base []string, pf PreflightResult) []string {
	out := make([]string, 0, len(base)+2)
	for _, e := range base {
		k, _, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		switch k {
		case "CGO_ENABLED", "CC":
			continue
		}
		out = append(out, e)
	}
	out = append(out, "CGO_ENABLED=1")
	if pf.CompilerPath != "" {
		out = append(out, "CC="+pf.CompilerPath)
	}
	return out
}
