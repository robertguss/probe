package toolrun

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// GoAllowlistKeys is the exact set of environment keys permitted in every go-step
// environment (Section 34.2). Any other key is closed by empty-base construction.
// Order is stable for documentation; runtime encoding always sorts.
var GoAllowlistKeys = []string{
	"PATH",
	"HOME",
	"TMPDIR",
	"GOMODCACHE",
	"GOCACHE",
	"GOPATH",
	"GOPROXY",
	"GOSUMDB",
	"GOPRIVATE",
	"GONOPROXY",
	"GONOSUMDB",
	"GOINSECURE",
	"GOENV",
	"GOFLAGS",
	"GOCACHEPROG",
	"GOTOOLCHAIN",
	"GOWORK",
	"GOVCS",
	"GOAUTH",
	"CGO_ENABLED",
	"LC_ALL",
	"LANG",
	"TERM",
}

// GitAllowlistKeys is the exact set of environment keys permitted for git init
// (Section 34.2). HOME, GIT_DIR, GIT_WORK_TREE, and XDG_CONFIG_HOME are
// intentionally absent.
var GitAllowlistKeys = []string{
	"PATH",
	"LC_ALL",
	"LANG",
	"GIT_CONFIG_GLOBAL",
	"GIT_CONFIG_SYSTEM",
	"GIT_CONFIG_NOSYSTEM",
	"GIT_TEMPLATE_DIR",
}

// FixedGoEnv is the normative fixed values for go-step construction.
// Host-captured keys (PATH/HOME/TMPDIR/cache/proxy) are filled at capture time.
// Must stay equal to plan.FixedGoEnvValues (parity tested).
var FixedGoEnv = map[string]string{
	"GOPRIVATE":   "",
	"GONOPROXY":   "",
	"GONOSUMDB":   "",
	"GOINSECURE":  "",
	"GOENV":       "off",
	"GOFLAGS":     "",
	"GOCACHEPROG": "",
	"GOTOOLCHAIN": "local",
	"GOWORK":      "off",
	"GOVCS":       "*:off",
	"GOAUTH":      "off",
	"CGO_ENABLED": "0",
	"LC_ALL":      "C",
	"LANG":        "C",
	"TERM":        "dumb",
}

// ForbiddenBleedKeys are host/sentinel keys that must never appear in a
// constructed go or git environment (REQ-154 / REQ-214). Presence of any is
// host bleed. Used by unit equality tables and sentinel promotion.
var ForbiddenBleedKeys = []string{
	"GODEBUG",
	"GOEXPERIMENT",
	"GO111MODULE",
	"GOROOT",
	"GOROOT_FINAL",
	"GO_EXTLINK_ENABLED",
	"CC",
	"CXX",
	"CGO_CFLAGS",
	"CGO_LDFLAGS",
	"GIT_CONFIG",
	"GIT_CONFIG_COUNT",
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_TEMPLATE_DIR", // forbidden on go steps; git steps set it deliberately
	"XDG_CONFIG_HOME",
	"http_proxy",
	"https_proxy",
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"ALL_PROXY",
	"NO_PROXY",
	"SSH_AUTH_SOCK",
	"SSH_AGENT_PID",
	"GPG_AGENT_INFO",
	"DOCKER_HOST",
	"KUBECONFIG",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"GH_TOKEN",
	"GITHUB_TOKEN",
}

// HostCapture holds host-effective values that Section 34.2 reuses deliberately
// (tool location, cache roots, proxy reachability). Captured once at startup.
//
// Equivalent field set to plan.HostEnv; toolrun owns capture via go env while
// plan receives an injected snapshot for pure Construct.
type HostCapture struct {
	PATH       string
	HOME       string
	TMPDIR     string // may be empty — key omitted when empty
	GOMODCACHE string
	GOCACHE    string
	GOPATH     string
	GOPROXY    string
	GOSUMDB    string
}

// ToPlanHost converts a capture into plan.HostEnv for plan.Construct injection.
func (h HostCapture) ToPlanHost() plan.HostEnv {
	return plan.HostEnv{
		PATH:       h.PATH,
		HOME:       h.HOME,
		TMPDIR:     h.TMPDIR,
		GOMODCACHE: h.GOMODCACHE,
		GOCACHE:    h.GOCACHE,
		GOPATH:     h.GOPATH,
		GOPROXY:    h.GOPROXY,
		GOSUMDB:    h.GOSUMDB,
	}
}

// HostCaptureFromPlan builds a HostCapture from a plan.HostEnv snapshot.
func HostCaptureFromPlan(h plan.HostEnv) HostCapture {
	return HostCapture{
		PATH:       h.PATH,
		HOME:       h.HOME,
		TMPDIR:     h.TMPDIR,
		GOMODCACHE: h.GOMODCACHE,
		GOCACHE:    h.GOCACHE,
		GOPATH:     h.GOPATH,
		GOPROXY:    h.GOPROXY,
		GOSUMDB:    h.GOSUMDB,
	}
}

// CaptureHost reads PATH/HOME/TMPDIR from the process environment and
// GOMODCACHE/GOCACHE/GOPATH/GOPROXY/GOSUMDB via `go env` (host-effective).
// Capture itself may use ambient env; only the returned allowlist is handed
// to children.
//
// goBinary, when non-empty, is the absolute path of go used for the capture
// query; otherwise PATH lookup is used.
func CaptureHost(goBinary string) (HostCapture, error) {
	h := HostCapture{
		PATH:   os.Getenv("PATH"),
		HOME:   os.Getenv("HOME"),
		TMPDIR: os.Getenv("TMPDIR"),
	}
	if h.PATH == "" {
		return h, fmt.Errorf("host PATH is empty; cannot construct tool environment")
	}
	bin := goBinary
	if bin == "" {
		p, err := exec.LookPath("go")
		if err != nil {
			return h, fmt.Errorf("go env capture: look path: %w", err)
		}
		bin = p
	}
	keys := []string{"GOMODCACHE", "GOCACHE", "GOPATH", "GOPROXY", "GOSUMDB"}
	// Intentionally ambient for host-effective values (Section 34.2).
	out, err := exec.Command(bin, append([]string{"env"}, keys...)...).Output()
	if err != nil {
		return h, fmt.Errorf("go env capture: %w", err)
	}
	lines := splitLinesPreserveEmpty(string(out))
	if len(lines) != len(keys) {
		return h, fmt.Errorf("go env capture: expected %d lines, got %d (%q)", len(keys), len(lines), string(out))
	}
	h.GOMODCACHE = lines[0]
	h.GOCACHE = lines[1]
	h.GOPATH = lines[2]
	h.GOPROXY = lines[3]
	h.GOSUMDB = lines[4]
	return h, nil
}

// ConstructGoEnv builds the exact Section 34.2 go-step environment from an
// empty base plus the allowlist. Returns a map suitable for plan.ExternalStep.Env
// and EnvSlice for exec.Cmd.Env.
//
// TMPDIR is omitted when empty (optional host key). All other allowlist keys
// are always present (fixed or host-captured).
func ConstructGoEnv(h HostCapture) map[string]string {
	m := make(map[string]string, len(GoAllowlistKeys))
	for k, v := range FixedGoEnv {
		m[k] = v
	}
	m["PATH"] = h.PATH
	m["HOME"] = h.HOME
	if h.TMPDIR != "" {
		m["TMPDIR"] = h.TMPDIR
	}
	m["GOMODCACHE"] = h.GOMODCACHE
	m["GOCACHE"] = h.GOCACHE
	m["GOPATH"] = h.GOPATH
	m["GOPROXY"] = h.GOPROXY
	m["GOSUMDB"] = h.GOSUMDB
	return m
}

// ConstructGitEnv builds the exact Section 34.2 git-init environment.
// templateDir is the absolute path of the Foundry-owned empty scratch template.
// HOME, GIT_DIR, GIT_WORK_TREE, XDG_CONFIG_HOME are absent by design.
func ConstructGitEnv(path, templateDir string) map[string]string {
	return map[string]string{
		"PATH":                path,
		"LC_ALL":              "C",
		"LANG":                "C",
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_SYSTEM":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TEMPLATE_DIR":    templateDir,
	}
}

// EnvSlice returns a sorted KEY=value slice suitable for exec.Cmd.Env.
// Sorting makes encoding deterministic (-count=2 stable).
func EnvSlice(env map[string]string) []string {
	keys := EnvKeys(env)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// EnvKeys returns sorted keys of an env map.
func EnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// CanonicalEnvEncoding returns the deterministic byte form of env used for
// hashing and equality: sorted KEY=value lines joined by '\n' (no trailing
// newline when empty). Values are included here only for hashing — step logs
// must use AllowlistHash + key names, never this string.
func CanonicalEnvEncoding(env map[string]string) string {
	keys := EnvKeys(env)
	if len(keys) == 0 {
		return ""
	}
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(env[k])
	}
	return b.String()
}

// AllowlistHash returns the SHA-256 hex digest of CanonicalEnvEncoding(env).
// Step logs and report fields MUST use this (or key names) and never emit
// env values (REQ-214 / acceptance: allowlist hash not values).
func AllowlistHash(env map[string]string) string {
	sum := sha256.Sum256([]byte(CanonicalEnvEncoding(env)))
	return hex.EncodeToString(sum[:])
}

// KeysOutsideAllowlist returns keys present in env that are not in allowlist.
func KeysOutsideAllowlist(env map[string]string, allowlist []string) []string {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, k := range allowlist {
		allowed[k] = struct{}{}
	}
	var bad []string
	for k := range env {
		if _, ok := allowed[k]; !ok {
			bad = append(bad, k)
		}
	}
	sort.Strings(bad)
	return bad
}

// ForbiddenKeysPresent returns which of forbidden appear as keys in env.
func ForbiddenKeysPresent(env map[string]string, forbidden []string) []string {
	var hits []string
	for _, k := range forbidden {
		if _, ok := env[k]; ok {
			hits = append(hits, k)
		}
	}
	sort.Strings(hits)
	return hits
}

// NormativeGoValue returns the expected fixed value for a go-env key, or
// ("", false) if the key is host-captured (not fixed).
func NormativeGoValue(key string) (string, bool) {
	v, ok := FixedGoEnv[key]
	return v, ok
}

// splitLinesPreserveEmpty splits process output into one line per requested
// key. A single trailing newline is dropped; empty values are preserved.
func splitLinesPreserveEmpty(s string) []string {
	if s == "" {
		return []string{}
	}
	if strings.HasSuffix(s, "\r\n") {
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
