package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// Abs-path normalization policy (plan.destination.path; REQ-032/033):
//
//  1. The authored destination is either the specification's destination field
//     or an explicit --dest override (identical lexical rules as Section 15.3).
//  2. Relative destinations are resolved against the process working directory
//     via filepath.Abs (or Options.WorkingDir when injected for tests).
//  3. The absolute path is then filepath.Cleaned (no evaluation of symlinks).
//  4. Parent = filepath.Dir(abs); Basename = filepath.Base(abs).
//  5. Observation is a non-binding Lstat of the destination and its parent
//     only — never a write, mkdir, or open-for-write (Section 13.2 / 30).
//
// Observation values (plan identity only; generate re-establishes state):
//
//	absent          — destination path does not exist
//	exists          — destination path exists in any form
//	parent-missing  — parent is absent or not a directory
//
// Destination observation never refuses validate/plan: a non-empty
// parent-missing or exists observation is recorded so agents can inspect it
// before generate. Only generate enforces exclusive placement.

// PipelineHost holds host-dependent values injected into plan.Construct so
// the pure pipeline stays free of ambient FS/env capture in lower packages.
// When zero-valued fields are left empty, captureHost fills them at run time.
type PipelineHost struct {
	Host           plan.HostEnv
	GoBinary       string
	GitBinary      string
	GitTemplateDir string
}

// pipelineOutcome is the shared result of the pure validate/plan path.
type pipelineOutcome struct {
	Plan       *plan.Plan
	SpecSource plan.SpecSourceKind
	// SpecLabel is the path as authored or "-" for stdin (for text/JSON status).
	SpecLabel string
	// SpecByteLen is the input size (logging; never dump body on failure).
	SpecByteLen int
	// DestAuthored is the destination string before abs normalization.
	DestAuthored string
}

// runPurePipeline is the sole validate/plan/generate pure path (REQ-032/033/120).
//
// Stages: read spec bytes → Decode → Validate → load catalog → observe dest
// → plan.Pipeline (resolve + construct). validate discards the plan; plan
// prints it; generate will execute it (Phase 2).
//
// Write-free: only read opens (spec path / stdin), Lstat for destination
// observation, and exec.LookPath for tool binary location. No subprocess
// Start, no network, no writable opens.
func (r *root) runPurePipeline(stdin io.Reader, specFlag, destOverride, verify string) (*pipelineOutcome, error) {
	if err := requireSpec(specFlag); err != nil {
		return nil, err
	}
	verifyMode, err := validateVerify(verify)
	if err != nil {
		return nil, err
	}

	data, source, label, err := r.readSpecBytes(stdin, specFlag)
	if err != nil {
		return nil, err
	}
	byteLen := len(data)

	// Decode filename: path as authored, or "stdin" for diagnostics.
	decodeName := label
	if source == plan.SpecSourceStdin {
		decodeName = "stdin"
	}

	raw, err := spec.Decode(decodeName, data)
	if err != nil {
		return nil, err
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		return nil, err
	}

	cat, err := r.loadCatalog()
	if err != nil {
		return nil, err
	}

	authored := strings.TrimSpace(destOverride)
	if authored == "" {
		authored = vs.Destination()
	} else {
		// --dest under identical Section 15.3 lexical rules + basename==name.
		if err := checkDestOverride(authored, vs.Name()); err != nil {
			return nil, err
		}
	}

	destInfo, err := r.observeDestination(authored)
	if err != nil {
		return nil, err
	}

	host := r.pipelineHost(vs.GitInit())

	info := r.versionInfo()
	opts := plan.PipelineOptions{
		FoundryVersion: info.Version,
		FoundryCommit:  info.Commit,
		GoVersion:      info.Go,
		SpecSource:     source,
		SpecPath:       "",
		Destination:    destInfo,
		Verify:         plan.VerifyMode(verifyMode),
		GoBinary:       host.GoBinary,
		GitBinary:      host.GitBinary,
		GitTemplateDir: host.GitTemplateDir,
		Host:           host.Host,
	}
	if source == plan.SpecSourcePath {
		opts.SpecPath = label
	}

	p, err := plan.Pipeline(vs, cat, opts)
	if err != nil {
		return nil, err
	}

	return &pipelineOutcome{
		Plan:         p,
		SpecSource:   source,
		SpecLabel:    label,
		SpecByteLen:  byteLen,
		DestAuthored: authored,
	}, nil
}

// readSpecBytes loads the Project Specification from a path or stdin.
// Enforces MaxSpecBytes (1 MiB) before full materialization when possible.
func (r *root) readSpecBytes(stdin io.Reader, specFlag string) (data []byte, source plan.SpecSourceKind, label string, err error) {
	if specFlag == SpecStdin {
		if stdin == nil {
			stdin = os.Stdin
		}
		// Cap+1 so Decode (and we) can report too_large exactly.
		limited := io.LimitReader(stdin, int64(spec.MaxSpecBytes)+1)
		data, err = io.ReadAll(limited)
		if err != nil {
			return nil, "", SpecStdin, diagnostic.Wrap(
				diagnostic.IDSpecInvalidEncoding,
				"failed to read specification from stdin",
				diagnostic.SpecLocation("stdin", 0, 0),
				err,
			)
		}
		if len(data) > spec.MaxSpecBytes {
			return nil, "", SpecStdin, diagnostic.Newf(
				diagnostic.IDSpecTooLarge,
				diagnostic.SpecLocation("stdin", 0, 0),
				"specification is %d bytes; maximum is %d (1 MiB)",
				len(data), spec.MaxSpecBytes,
			)
		}
		return data, plan.SpecSourceStdin, SpecStdin, nil
	}

	// Path source — open read-only.
	label = specFlag
	data, err = r.readFile(specFlag)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", label, diagnostic.Newf(
				diagnostic.IDUsageInvalid,
				diagnostic.PathLocation(specFlag),
				"specification file not found: %s",
				specFlag,
			).WithRemediation("Pass an existing path to --spec, or use --spec - to read from stdin.")
		}
		return nil, "", label, diagnostic.Wrap(
			diagnostic.IDUsageInvalid,
			fmt.Sprintf("cannot read specification %q", specFlag),
			diagnostic.PathLocation(specFlag),
			err,
		).WithRemediation("Check the path is readable and is a regular file.")
	}
	// Decode enforces MaxSpecBytes; pre-check for clearer path-level errors.
	if len(data) > spec.MaxSpecBytes {
		return nil, "", label, diagnostic.Newf(
			diagnostic.IDSpecTooLarge,
			diagnostic.SpecLocation(specFlag, 0, 0),
			"specification is %d bytes; maximum is %d (1 MiB)",
			len(data), spec.MaxSpecBytes,
		)
	}
	return data, plan.SpecSourcePath, label, nil
}

func (r *root) readFile(path string) ([]byte, error) {
	if r.opts.ReadFile != nil {
		return r.opts.ReadFile(path)
	}
	return os.ReadFile(path)
}

// observeDestination builds plan.DestinationInfo from an authored path.
// See package comment on abs-path normalization policy.
func (r *root) observeDestination(authored string) (plan.DestinationInfo, error) {
	if r.opts.ObserveDestination != nil {
		return r.opts.ObserveDestination(authored)
	}
	wd, err := r.workingDir()
	if err != nil {
		return plan.DestinationInfo{}, err
	}
	return observeDestinationAt(authored, wd)
}

func (r *root) workingDir() (string, error) {
	if r.opts.WorkingDir != "" {
		return r.opts.WorkingDir, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", diagnostic.Wrap(
			diagnostic.IDInternalBug,
			"cannot determine working directory",
			diagnostic.Location{},
			err,
		)
	}
	return wd, nil
}

// observeDestinationAt normalizes authored to an absolute cleaned path under
// wd (when relative) and records a non-binding Lstat observation.
func observeDestinationAt(authored, wd string) (plan.DestinationInfo, error) {
	authored = strings.TrimSpace(authored)
	if authored == "" {
		return plan.DestinationInfo{}, diagnostic.New(
			diagnostic.IDSpecInvalidField,
			"destination path is empty",
			diagnostic.PathLocation("destination"),
		)
	}

	abs := authored
	if !filepath.IsAbs(abs) {
		base := wd
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				return plan.DestinationInfo{}, diagnostic.Wrap(
					diagnostic.IDInternalBug,
					"cannot determine working directory for destination resolution",
					diagnostic.PathLocation(authored),
					err,
				)
			}
		}
		abs = filepath.Join(base, abs)
	}
	abs = filepath.Clean(abs)
	parent := filepath.Dir(abs)
	base := filepath.Base(abs)

	obs := plan.ObservationAbsent
	// Parent first: if parent missing or not a dir → parent-missing.
	pst, perr := os.Lstat(parent)
	if perr != nil {
		if os.IsNotExist(perr) {
			obs = plan.ObservationParentMissing
		} else {
			// Unreadable parent: treat as parent-missing (non-binding; generate re-checks).
			obs = plan.ObservationParentMissing
		}
	} else if !pst.IsDir() {
		obs = plan.ObservationParentMissing
	} else {
		// Parent OK — observe destination itself.
		_, derr := os.Lstat(abs)
		if derr == nil {
			obs = plan.ObservationExists
		} else if !os.IsNotExist(derr) {
			// Permission etc.: record exists-or-unknown conservatively as exists
			// so generate does not assume absence from a soft observation.
			obs = plan.ObservationExists
		} else {
			obs = plan.ObservationAbsent
		}
	}

	return plan.DestinationInfo{
		Path:        abs,
		Parent:      parent,
		Basename:    base,
		Observation: obs,
	}, nil
}

// checkDestOverride enforces Section 15.3 lexical rules + basename==name for --dest.
func checkDestOverride(dest, projectName string) error {
	base, msg := destLexicalBase(dest)
	if msg != "" {
		return diagnostic.New(
			diagnostic.IDSpecInvalidField,
			msg,
			diagnostic.PathLocation("destination"),
		).WithRemediation("Fix --dest to a path whose basename equals the project name, with no .., ~, or $ expansions.")
	}
	if base != projectName {
		return diagnostic.Newf(
			diagnostic.IDSpecInvalidField,
			diagnostic.PathLocation("destination"),
			"destination basename %q must equal name %q",
			base, projectName,
		).WithRemediation("Set --dest so the final path component equals the project name field.")
	}
	return nil
}

// destLexicalBase mirrors internal/spec destinationLexicalOK for --dest.
// Kept local so the CLI boundary does not export private validators.
func destLexicalBase(dest string) (base string, errMsg string) {
	if dest == "" {
		return "", "destination must be a non-empty path"
	}
	if strings.ContainsRune(dest, 0) {
		return "", "destination must not contain NUL bytes"
	}
	if strings.Contains(dest, "${") || strings.Contains(dest, "$") {
		return "", "destination must not contain environment-variable markers ($ or ${)"
	}
	if strings.Contains(dest, "~") {
		return "", "destination must not contain '~' (no home-directory expansion)"
	}
	normalized := strings.ReplaceAll(dest, "\\", "/")
	if normalized == "." || normalized == "./" {
		return "", `destination must not be "." (generation into the current directory is prohibited)`
	}
	parts := strings.Split(normalized, "/")
	var comps []string
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			return "", `destination must not contain ".." components`
		}
		comps = append(comps, p)
	}
	if len(comps) == 0 {
		return "", "destination path has no basename"
	}
	base = comps[len(comps)-1]
	if base == "." || base == ".." || base == "" {
		return "", "destination basename is invalid"
	}
	return base, ""
}

// pipelineHost returns host capture for the write-free pure path
// (validate/plan and the pure half of generate). Injected Options override
// ambient capture so unit tests stay host-independent.
//
// Go binary resolution is intentionally probe-free (REQ-031 / Section 13.2):
//
//  1. Options.PipelineHost.GoBinary (tests / explicit inject)
//  2. FOUNDRY_GO_BIN (absolute override; generate preflight still version-checks)
//  3. exec.LookPath("go") — path only, no `go version` child process
//
// Pin discovery that runs `go version` (FindPinnedGoBinary) is generate-only;
// see generatePipelineHost. Leaving GoBinary empty yields tool.missing at
// plan.Construct — agent-actionable, still write-free.
func (r *root) pipelineHost(gitInit bool) PipelineHost {
	h := r.opts.PipelineHost

	if h.GoBinary == "" {
		if env := strings.TrimSpace(os.Getenv(toolrun.EnvFoundryGoBin)); env != "" {
			h.GoBinary = env
		} else if p, err := exec.LookPath("go"); err == nil {
			h.GoBinary = p
		} else {
			// Leave empty — plan.Construct fails with tool.missing (agent-actionable).
			h.GoBinary = ""
		}
	}
	if gitInit && h.GitBinary == "" {
		if p, err := exec.LookPath("git"); err == nil {
			h.GitBinary = p
		}
	}
	// GitTemplateDir empty → plan uses the stable pure placeholder token.
	// generate materializes a real scratch path before Construct in Phase 2.

	if h.Host == (plan.HostEnv{}) {
		h.Host = captureHostEnv()
	} else {
		// Partial injection: fill only empty fields from ambient env.
		amb := captureHostEnv()
		if h.Host.PATH == "" {
			h.Host.PATH = amb.PATH
		}
		if h.Host.HOME == "" {
			h.Host.HOME = amb.HOME
		}
		if h.Host.TMPDIR == "" {
			h.Host.TMPDIR = amb.TMPDIR
		}
		if h.Host.GOMODCACHE == "" {
			h.Host.GOMODCACHE = amb.GOMODCACHE
		}
		if h.Host.GOCACHE == "" {
			h.Host.GOCACHE = amb.GOCACHE
		}
		if h.Host.GOPATH == "" {
			h.Host.GOPATH = amb.GOPATH
		}
		if h.Host.GOPROXY == "" {
			h.Host.GOPROXY = amb.GOPROXY
		}
		if h.Host.GOSUMDB == "" {
			h.Host.GOSUMDB = amb.GOSUMDB
		}
	}
	return h
}

// generatePipelineHost extends pipelineHost for the mutating generate path.
// When Options/FOUNDRY_GO_BIN did not set GoBinary, prefer an exact catalog
// pin via toolrun.FindPinnedGoBinary (probes `go version` under
// GOTOOLCHAIN=local) over a bare LookPath hit so agents get the pinned
// toolchain (dogfood friction go-foundry-cli-0wc). Safe only after the pure
// plan is already constructed — never call from runPurePipeline.
//
// Note: the pure plan records whatever path pipelineHost resolved (LookPath
// or env). Generate preflight re-resolves tools via this host override; plan
// external_steps binary paths are informational for disclosure/equality of
// argv shape, while execution uses the preflighted path.
func (r *root) generatePipelineHost(gitInit bool) PipelineHost {
	// Detect whether the caller injected an explicit GoBinary before ambient fill.
	injected := strings.TrimSpace(r.opts.PipelineHost.GoBinary) != ""
	envSet := strings.TrimSpace(os.Getenv(toolrun.EnvFoundryGoBin)) != ""

	h := r.pipelineHost(gitInit)
	if injected || envSet {
		return h
	}
	if p := toolrun.FindPinnedGoBinary(toolrun.DefaultPinnedGoTag); p != "" {
		h.GoBinary = p
	}
	return h
}

// captureHostEnv records Section 34.2 allowlist values for plan env maps.
// Read-only getenv; no subprocess.
func captureHostEnv() plan.HostEnv {
	return plan.HostEnv{
		PATH:       os.Getenv("PATH"),
		HOME:       os.Getenv("HOME"),
		TMPDIR:     os.Getenv("TMPDIR"),
		GOMODCACHE: firstNonEmpty(os.Getenv("GOMODCACHE"), goEnvFallback("GOMODCACHE")),
		GOCACHE:    firstNonEmpty(os.Getenv("GOCACHE"), goEnvFallback("GOCACHE")),
		GOPATH:     firstNonEmpty(os.Getenv("GOPATH"), goEnvFallback("GOPATH")),
		GOPROXY:    firstNonEmpty(os.Getenv("GOPROXY"), "https://proxy.golang.org,direct"),
		GOSUMDB:    firstNonEmpty(os.Getenv("GOSUMDB"), "sum.golang.org"),
	}
}

// goEnvFallback returns "" without spawning `go env` (write-free / no subprocess).
// Plan records the allowlist; generate may refresh with real go env later.
// Prefer ambient env vars set by the host or test harness.
func goEnvFallback(key string) string {
	_ = key
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// validateResult is the stable JSON result for validate success.
type validateResult struct {
	Status     string `json:"status"`
	Spec       string `json:"spec"`
	PlanSHA256 string `json:"plan_sha256"`
}

// formatValidateSuccess builds text + JSON payload for validate success.
func formatValidateSuccess(out *pipelineOutcome) (validateResult, string) {
	label := out.SpecLabel
	// Human text: name stdin source explicitly (flag value remains "-" in JSON).
	textLabel := label
	if out.SpecSource == plan.SpecSourceStdin || label == SpecStdin {
		textLabel = "stdin"
	}
	res := validateResult{
		Status:     "ok",
		Spec:       label,
		PlanSHA256: out.Plan.PlanSHA256(),
	}
	text := fmt.Sprintf("validate: ok (spec=%s plan_sha256=%s)", textLabel, out.Plan.PlanSHA256())
	return res, text
}

// formatPlanSuccess builds a human-readable plan summary for text mode.
// JSON mode emits the complete Generation Plan document separately.
// When verbose is true, file digests, tools, and external-step argv expand.
func formatPlanSuccess(out *pipelineOutcome, verbose bool) string {
	p := out.Plan
	if p == nil {
		return "plan: <nil>"
	}
	proj := p.Project()
	dest := p.Destination()
	ver := p.Verification()
	net := p.Network()
	git := p.Git()

	var b strings.Builder
	fmt.Fprintf(&b, "plan: project=%s archetype=%s destination=%s\n", proj.Name, proj.Archetype, dest.Path)
	fmt.Fprintf(&b, "plan_sha256=%s\n", p.PlanSHA256())
	fmt.Fprintf(&b, "verify: %s", ver.Mode)
	if len(ver.Checks) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(ver.Checks, ", "))
	}
	b.WriteByte('\n')
	if profiles := p.Profiles(); len(profiles) > 0 {
		fmt.Fprintf(&b, "profiles: %s\n", strings.Join(profiles, ", "))
	} else {
		b.WriteString("profiles: []\n")
	}
	fmt.Fprintf(&b, "destination_observation: %s\n", dest.Observation)

	files := p.Files()
	fmt.Fprintf(&b, "files: %d\n", len(files))
	for _, f := range files {
		if verbose {
			fmt.Fprintf(&b, "  %s owner=%s render=%s content_sha256=%s\n",
				f.Path, f.Owner, f.Render, f.ContentSHA256)
		} else {
			fmt.Fprintf(&b, "  %s\n", f.Path)
		}
	}

	deps := p.Dependencies()
	fmt.Fprintf(&b, "dependencies: %d\n", len(deps))
	for _, d := range deps {
		fmt.Fprintf(&b, "  %s %s scope=%s\n", d.Module, d.Version, d.Scope)
	}

	if verbose {
		tools := p.Tools()
		fmt.Fprintf(&b, "tools: %d\n", len(tools))
		for _, t := range tools {
			name := t.Name
			if name == "" {
				name = t.Module
			}
			fmt.Fprintf(&b, "  %s %s\n", name, t.Version)
		}
	}

	steps := p.ExternalSteps()
	fmt.Fprintf(&b, "external_steps: %d\n", len(steps))
	for _, s := range steps {
		if verbose {
			fmt.Fprintf(&b, "  %s network=%s argv=%q\n", s.ID, s.Network, strings.Join(s.Argv, " "))
		} else {
			fmt.Fprintf(&b, "  %s network=%s\n", s.ID, s.Network)
		}
	}

	fmt.Fprintf(&b, "network: may_be_required=%v\n", net.MayBeRequired)
	for _, reason := range net.Reasons {
		fmt.Fprintf(&b, "  - %s\n", reason)
	}

	fmt.Fprintf(&b, "git: init=%v branch=%s\n", git.Init, git.InitialBranch)
	if git.Init {
		b.WriteString("git: isolated init only (no commits, no identity); after generate run git add && git commit\n")
	}

	fmt.Fprintf(&b, "go_pin: %s (generate requires exact pin; set FOUNDRY_GO_BIN if PATH differs)\n",
		toolrun.DefaultPinnedGoTag)

	return strings.TrimRight(b.String(), "\n")
}
