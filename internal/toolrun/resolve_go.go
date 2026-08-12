package toolrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// EnvFoundryGoBin is the optional absolute-path override for the pinned go
// binary used by Foundry preflight and generate. Scripts (dogfood-cli,
// perf capture) and agents set this when PATH has a nearby-but-wrong version
// (for example mise go1.26.4 while the catalog pins go1.26.5).
const EnvFoundryGoBin = "FOUNDRY_GO_BIN"

// FindPinnedGoBinary returns an absolute path to a go binary that reports the
// exact required toolchain tag under GOTOOLCHAIN=local, or "" when none is
// found. Candidate order:
//
//  1. FOUNDRY_GO_BIN (when set and executable)
//  2. Host module-cache toolchain (GOMODCACHE or $HOME/go/pkg/mod)
//  3. Common install prefixes (mise, $HOME/sdk, /usr/local/go)
//  4. LookPath("go")
//
// Each candidate is probed with GOTOOLCHAIN=local so auto-toolchain
// upgrades (which would mask a nearby PATH version) do not count as a match.
//
// FindPinnedGoBinary is the production entry point: it probes candidates
// using OSRunner. See FindPinnedGoBinaryWithRunner for test injection.
func FindPinnedGoBinary(required string) string {
	return FindPinnedGoBinaryWithRunner(required, OSRunner{})
}

// FindPinnedGoBinaryWithRunner returns an absolute path to a go binary that
// reports the exact required toolchain tag under GOTOOLCHAIN=local, or "" when
// none is found. The Runner injection lets unit tests avoid real subprocess
// execution while still exercising candidate ordering and version matching.
//
// required defaults to DefaultPinnedGoTag when empty.
func FindPinnedGoBinaryWithRunner(required string, runner Runner) string {
	required = strings.TrimSpace(required)
	if required == "" {
		required = DefaultPinnedGoTag
	}
	if runner == nil {
		runner = OSRunner{}
	}
	for _, c := range pinnedGoCandidates(required) {
		if c == "" {
			continue
		}
		if tag, err := ProbeGoVersionLocalWithRunner(context.Background(), c, runner); err == nil && tag == required {
			return c
		}
	}
	return ""
}

// ProbeGoVersionLocal runs `go version` under GOTOOLCHAIN=local and returns
// the parsed tag (e.g. "go1.26.5"). The binary may be absolute or on PATH.
func ProbeGoVersionLocal(goBinary string) (string, error) {
	return ProbeGoVersionLocalWithRunner(context.Background(), goBinary, OSRunner{})
}

// ProbeGoVersionLocalWithRunner probes a go binary via the supplied Runner so
// tests can inject a fake instead of spawning a real go binary.
func ProbeGoVersionLocalWithRunner(ctx context.Context, goBinary string, runner Runner) (string, error) {
	goBinary = strings.TrimSpace(goBinary)
	if goBinary == "" {
		return "", fmt.Errorf("empty go binary")
	}
	if runner == nil {
		runner = OSRunner{}
	}
	if !filepath.IsAbs(goBinary) {
		p, err := runner.LookPath(goBinary)
		if err != nil {
			return "", err
		}
		goBinary = p
	}
	// OSRunner.LookPath already checks executability; FakeRunner does not model
	// file permissions, so only enforce this for real filesystem paths.
	if _, ok := runner.(OSRunner); ok {
		if err := ensureExecutable(goBinary); err != nil {
			return "", err
		}
	}
	// Minimal env: closed enough that ambient GOTOOLCHAIN=auto cannot promote
	// a nearby base toolchain to the catalog pin during the probe.
	env := []string{
		"PATH=" + filepath.Dir(goBinary) + string(os.PathListSeparator) + "/usr/bin:/bin",
		"HOME=" + os.Getenv("HOME"),
		"GOTOOLCHAIN=local",
		"GOENV=off",
		"CGO_ENABLED=0",
	}
	if ctx == nil {
		ctx = context.Background()
	}
	out, _, err := runner.Run(ctx, goBinary, []string{"version"}, env)
	if err != nil {
		return "", err
	}
	return ParseGoVersionOutput(string(out))
}

// pinnedGoCandidates lists absolute (or LookPath-able) go paths to try.
// Order matches scripts/dogfood-cli.sh resolve_go and integration helpers.
func pinnedGoCandidates(required string) []string {
	var out []string
	if v := strings.TrimSpace(os.Getenv(EnvFoundryGoBin)); v != "" {
		out = append(out, v)
	}
	// Module-cache toolchain downloaded by cmd/go (GOTOOLCHAIN=auto / go.mod).
	modcache := strings.TrimSpace(os.Getenv("GOMODCACHE"))
	if modcache == "" {
		if home := os.Getenv("HOME"); home != "" {
			modcache = filepath.Join(home, "go", "pkg", "mod")
		}
	}
	if modcache != "" {
		// golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64
		name := fmt.Sprintf("golang.org/toolchain@v0.0.1-%s.%s-%s",
			required, runtime.GOOS, runtime.GOARCH)
		out = append(out, filepath.Join(modcache, name, "bin", "go"))
	}
	// Common install prefixes and version managers.
	if home := os.Getenv("HOME"); home != "" {
		// mise: ~/.local/share/mise/installs/go/1.26.5/bin/go
		ver := strings.TrimPrefix(required, "go")
		out = append(out,
			filepath.Join(home, ".local/share/mise/installs/go", ver, "bin", "go"),
			filepath.Join(home, "sdk", required, "bin", "go"),
		)
	}
	out = append(out, "/usr/local/go/bin/go")
	if p, err := exec.LookPath("go"); err == nil {
		out = append(out, p)
	}
	return out
}

// WrongVersionRemediation builds the agent-actionable remediation for
// tool.wrong_version. When a matching binary exists nearby, its absolute path
// is named so agents need not hunt the module cache by hand.
func WrongVersionRemediation(required, observedPath, nearbyPin string) string {
	required = strings.TrimSpace(required)
	if required == "" {
		required = DefaultPinnedGoTag
	}
	base := fmt.Sprintf(
		"Install or select the exact Go toolchain %s (see catalog/versions.toml). Foundry does not accept nearby versions.",
		required,
	)
	nearbyPin = strings.TrimSpace(nearbyPin)
	if nearbyPin == "" || nearbyPin == strings.TrimSpace(observedPath) {
		// Still name FOUNDRY_GO_BIN so scripts/docs stay discoverable.
		return base + fmt.Sprintf(
			" Set %s to the absolute path of a %s `go` binary, or put that binary first on PATH with GOTOOLCHAIN=local.",
			EnvFoundryGoBin, required,
		)
	}
	return base + fmt.Sprintf(
		" A matching binary was found at %q. Export %s=%q (or put its directory first on PATH with GOTOOLCHAIN=local) and re-run.",
		nearbyPin, EnvFoundryGoBin, nearbyPin,
	)
}
