package toolrun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Step identifiers for preflight (Appendix E).
const (
	StepGoPreflight  = "go-preflight"
	StepGitPreflight = "git-preflight"
)

// DefaultPinnedGoTag is the catalog toolchain pin used when callers omit an
// explicit RequireGoVersion. Matches catalog/versions.toml [toolchain].go_tag.
const DefaultPinnedGoTag = "go1.26.5"

// DefaultPreflightTimeout is the plan-declared preflight timeout (Section 34.3).
const DefaultPreflightTimeout = 60 * time.Second

// Fail class strings for structured logs (mirror diagnostic identifiers).
const (
	FailClassMissing      = "tool.missing"
	FailClassWrongVersion = "tool.wrong_version"
)

// Runner executes binaries under a constructed environment. Production uses
// OSRunner; unit tests inject fakes.
type Runner interface {
	// LookPath resolves file on PATH (or absolute) to an absolute path.
	LookPath(file string) (string, error)
	// Run starts binary with args and env slice (args exclude the binary path).
	Run(ctx context.Context, binary string, args []string, env []string) (stdout, stderr []byte, err error)
}

// OSRunner is the production Runner using os/exec.
type OSRunner struct {
	// PathEnv, when non-empty, replaces PATH for LookPath only.
	PathEnv string
}

// LookPath implements Runner. When PathEnv is set, searches those directories
// directly without modifying the global PATH (thread-safe).
func (r OSRunner) LookPath(file string) (string, error) {
	if file == "" {
		return "", exec.ErrNotFound
	}
	if filepath.IsAbs(file) {
		if err := ensureExecutable(file); err != nil {
			return "", err
		}
		return file, nil
	}
	pathEnv := r.PathEnv
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	// Scan directories directly to avoid mutating global PATH state.
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		p := filepath.Join(dir, file)
		if err := ensureExecutable(p); err == nil {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// Run implements Runner.
func (r OSRunner) Run(ctx context.Context, binary string, args []string, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// PreflightOptions configures binary location and version checks.
type PreflightOptions struct {
	// Host supplies Section 34.2 host-captured values for the closed go env
	// used when running `go version`. Required (PATH at minimum).
	Host HostCapture

	// GoBinary is an absolute path override; empty means LookPath("go").
	GoBinary string
	// GitBinary is an absolute path override; empty means LookPath("git") when RequireGit.
	GitBinary string

	// RequireGoVersion is the exact pinned toolchain tag (e.g. "go1.26.5").
	// Empty uses DefaultPinnedGoTag.
	RequireGoVersion string
	// RequireGit enables git --version preflight (when [git] init is planned).
	RequireGit bool

	// Runner executes version probes. Nil uses OSRunner with Host.PATH.
	Runner Runner

	// Timeout bounds each version probe. Zero uses DefaultPreflightTimeout.
	Timeout time.Duration
}

// PreflightResult is the outcome of tool preflight (logged, then handed to plan).
//
// On success Err() is nil and FailClass is empty. On failure Err() is a
// *diagnostic.FoundryError with ID tool.missing or tool.wrong_version.
type PreflightResult struct {
	GoPath       string
	GitPath      string
	GoVersion    string // observed, e.g. "go1.26.5"
	GitVersion   string // observed raw first line, e.g. "git version 2.43.0"
	RequiredGo   string
	GoEnvHash    string   // AllowlistHash of constructed go env (never values)
	GoEnvKeys    []string // key names only
	FailClass    string   // empty | tool.missing | tool.wrong_version
	FailStep     string   // go-preflight | git-preflight
	BinaryLogged string   // path named in failure message
	err          error
}

// Preflight locates go (always) and optionally git, runs version probes under
// the closed Section 34.2 environment, and fail-closes on missing/wrong tools
// before any staging work (Section 34.1 / REQ-153).
func Preflight(ctx context.Context, opts PreflightOptions) PreflightResult {
	res := PreflightResult{
		RequiredGo: strings.TrimSpace(opts.RequireGoVersion),
	}
	if res.RequiredGo == "" {
		res.RequiredGo = DefaultPinnedGoTag
	}

	runner := opts.Runner
	if runner == nil {
		runner = OSRunner{PathEnv: opts.Host.PATH}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultPreflightTimeout
	}

	goEnv := ConstructGoEnv(opts.Host)
	res.GoEnvHash = AllowlistHash(goEnv)
	res.GoEnvKeys = EnvKeys(goEnv)
	envSlice := EnvSlice(goEnv)

	// --- go ---
	goPath, err := resolveBinary(runner, opts.GoBinary, "go")
	if err != nil {
		res.FailClass = FailClassMissing
		res.FailStep = StepGoPreflight
		res.BinaryLogged = firstNonEmpty(opts.GoBinary, "go")
		res.err = missingToolError(StepGoPreflight, res.BinaryLogged, err)
		return res
	}
	res.GoPath = goPath
	res.BinaryLogged = goPath

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	stdout, stderr, runErr := runner.Run(probeCtx, goPath, []string{"version"}, envSlice)
	cancel()
	if runErr != nil {
		res.FailClass = FailClassMissing
		res.FailStep = StepGoPreflight
		detail := strings.TrimSpace(string(stderr))
		if detail == "" {
			detail = runErr.Error()
		}
		res.err = diagnostic.Wrapf(
			diagnostic.IDToolMissing,
			diagnostic.StepLocation(StepGoPreflight),
			runErr,
			"go binary %q failed preflight version probe: %s",
			goPath, detail,
		).WithRemediation(
			"Install a working Go toolchain on PATH and re-run. Foundry requires an executable `go` that responds to `go version`.",
		)
		return res
	}
	observed, perr := ParseGoVersionOutput(string(stdout))
	if perr != nil {
		res.FailClass = FailClassWrongVersion
		res.FailStep = StepGoPreflight
		res.GoVersion = strings.TrimSpace(string(stdout))
		nearby := FindPinnedGoBinaryWithRunner(res.RequiredGo, runner)
		res.err = diagnostic.Newf(
			diagnostic.IDToolWrongVersion,
			diagnostic.StepLocation(StepGoPreflight),
			"could not parse go version from %q (stdout %q): %v",
			goPath, truncateForMsg(string(stdout), 120), perr,
		).WithRemediation(WrongVersionRemediation(res.RequiredGo, goPath, nearby))
		return res
	}
	res.GoVersion = observed
	if observed != res.RequiredGo {
		res.FailClass = FailClassWrongVersion
		res.FailStep = StepGoPreflight
		// Name a nearby matching binary when present so agents need not hunt
		// the module-cache toolchain path (dogfood friction go-foundry-cli-0wc).
		nearby := FindPinnedGoBinaryWithRunner(res.RequiredGo, runner)
		res.err = diagnostic.Newf(
			diagnostic.IDToolWrongVersion,
			diagnostic.StepLocation(StepGoPreflight),
			"go version %s at %q does not match pinned toolchain %s",
			observed, goPath, res.RequiredGo,
		).WithRemediation(WrongVersionRemediation(res.RequiredGo, goPath, nearby))
		return res
	}

	// --- git (optional) ---
	if opts.RequireGit {
		gitPath, gerr := resolveBinary(runner, opts.GitBinary, "git")
		if gerr != nil {
			res.FailClass = FailClassMissing
			res.FailStep = StepGitPreflight
			res.BinaryLogged = firstNonEmpty(opts.GitBinary, "git")
			res.err = missingToolError(StepGitPreflight, res.BinaryLogged, gerr)
			return res
		}
		res.GitPath = gitPath

		// git --version does not need full git-init isolation; PATH + C locale.
		gitEnv := EnvSlice(map[string]string{
			"PATH":   opts.Host.PATH,
			"LC_ALL": "C",
			"LANG":   "C",
		})
		gctx, gcancel := context.WithTimeout(ctx, timeout)
		gout, gerrOut, grerr := runner.Run(gctx, gitPath, []string{"--version"}, gitEnv)
		gcancel()
		if grerr != nil {
			res.FailClass = FailClassMissing
			res.FailStep = StepGitPreflight
			res.BinaryLogged = gitPath
			detail := strings.TrimSpace(string(gerrOut))
			if detail == "" {
				detail = grerr.Error()
			}
			res.err = diagnostic.Wrapf(
				diagnostic.IDToolMissing,
				diagnostic.StepLocation(StepGitPreflight),
				grerr,
				"git binary %q failed preflight version probe: %s",
				gitPath, detail,
			).WithRemediation(
				"Install git on PATH when [git] init is enabled, then re-run.",
			)
			return res
		}
		res.GitVersion = strings.TrimSpace(firstLine(string(gout)))
	}

	return res
}

// Err returns the preflight failure, if any.
func (r PreflightResult) Err() error { return r.err }

// OK reports whether preflight succeeded.
func (r PreflightResult) OK() bool { return r.err == nil && r.FailClass == "" }

// LogFields returns structured, non-secret fields for step/report logging.
// Env values are never included — only hash and key names.
func (r PreflightResult) LogFields() map[string]string {
	m := map[string]string{
		"go_path":     r.GoPath,
		"go_version":  r.GoVersion,
		"required_go": r.RequiredGo,
		"env_hash":    r.GoEnvHash,
		"env_keys":    strings.Join(r.GoEnvKeys, ","),
		"fail_class":  r.FailClass,
		"fail_step":   r.FailStep,
	}
	if r.GitPath != "" {
		m["git_path"] = r.GitPath
	}
	if r.GitVersion != "" {
		m["git_version"] = r.GitVersion
	}
	return m
}

// ParseGoVersionOutput extracts the toolchain tag from `go version` stdout.
// Expected form: "go version go1.26.5 linux/amd64" → "go1.26.5".
func ParseGoVersionOutput(stdout string) (string, error) {
	line := strings.TrimSpace(firstLine(stdout))
	if line == "" {
		return "", fmt.Errorf("empty go version output")
	}
	const prefix = "go version "
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unexpected go version format %q", line)
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	fields := strings.Fields(rest)
	if len(fields) < 1 {
		return "", fmt.Errorf("missing version tag in %q", line)
	}
	tag := fields[0]
	if !strings.HasPrefix(tag, "go") || len(tag) < 3 {
		return "", fmt.Errorf("invalid version tag %q", tag)
	}
	return tag, nil
}

func resolveBinary(runner Runner, explicit, name string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		p := explicit
		if !filepath.IsAbs(p) {
			return runner.LookPath(p)
		}
		if err := ensureExecutable(p); err != nil {
			return "", err
		}
		return p, nil
	}
	return runner.LookPath(name)
}

func ensureExecutable(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: not found", path)
		}
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%s: is a directory", path)
	}
	// Product platforms are macOS/Linux only (REQ-004).
	if st.Mode()&0o111 == 0 {
		return fmt.Errorf("%s: not executable", path)
	}
	return nil
}

func missingToolError(step, name string, cause error) error {
	msg := fmt.Sprintf("required tool %q not found or not executable", name)
	if cause != nil {
		msg = fmt.Sprintf("%s: %v", msg, cause)
	}
	return diagnostic.Wrap(
		diagnostic.IDToolMissing,
		msg,
		diagnostic.StepLocation(step),
		cause,
	).WithRemediation(
		"Install the missing tool and ensure it is on PATH. " +
			"`go` is always required; `git` is required when [git] init is enabled. Re-run after installation.",
	)
}

// wrongVersionRemediation is retained for older call sites; prefer
// WrongVersionRemediation which can name a nearby pin path.
func wrongVersionRemediation(required string) string {
	return WrongVersionRemediation(required, "", "")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncateForMsg(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
