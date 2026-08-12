package e2

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// GoAllowlistKeys are the exact keys permitted in every go-step environment
// (Section 34.2). Any other key is closed by empty-base construction.
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

// GitAllowlistKeys are the exact keys permitted for git init (Section 34.2).
// HOME, GIT_DIR, GIT_WORK_TREE, and XDG_CONFIG_HOME are intentionally absent.
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

// HostCapture holds host-effective values that Section 34.2 reuses deliberately
// (tool location, cache roots, proxy reachability). Captured once at startup.
type HostCapture struct {
	PATH       string
	HOME       string
	TMPDIR     string // may be empty
	GOMODCACHE string
	GOCACHE    string
	GOPATH     string
	GOPROXY    string
	GOSUMDB    string
}

// CaptureHost reads PATH/HOME/TMPDIR from the process environment and
// GOMODCACHE/GOCACHE/GOPATH/GOPROXY/GOSUMDB via `go env` (host-effective).
// Capture itself may use ambient env; only the returned allowlist is handed
// to children.
func CaptureHost() (HostCapture, error) {
	h := HostCapture{
		PATH:   os.Getenv("PATH"),
		HOME:   os.Getenv("HOME"),
		TMPDIR: os.Getenv("TMPDIR"),
	}
	if h.PATH == "" {
		return h, fmt.Errorf("host PATH is empty; cannot construct tool environment")
	}
	// go env for cache/proxy values — use ambient so we get true host-effective.
	keys := []string{"GOMODCACHE", "GOCACHE", "GOPATH", "GOPROXY", "GOSUMDB"}
	out, err := exec.Command("go", append([]string{"env"}, keys...)...).Output()
	if err != nil {
		return h, fmt.Errorf("go env capture: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
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
// empty base plus the allowlist. hostEnv is ignored except that CaptureHost
// already pulled the permitted host-effective values into h.
//
// Returns a sorted KEY=value slice suitable for exec.Cmd.Env.
func ConstructGoEnv(h HostCapture) []string {
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

	return mapToEnv(m)
}

// ConstructGitEnv builds the exact Section 34.2 git-init environment.
// templateDir is the absolute path of the Foundry-owned empty scratch template.
// HOME, GIT_DIR, GIT_WORK_TREE, XDG_CONFIG_HOME are absent by design.
func ConstructGitEnv(path, templateDir string) []string {
	m := map[string]string{
		"PATH":                path,
		"LC_ALL":              "C",
		"LANG":                "C",
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_SYSTEM":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TEMPLATE_DIR":    templateDir,
	}
	return mapToEnv(m)
}

// EnvMap parses KEY=value slice into a map (first = wins for robustness).
func EnvMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return m
}

// EnvKeys returns sorted keys of an env map.
func EnvKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ForbiddenKeysIn reports which of the given forbidden keys appear in env
// with a non-normative (sentinel) value. Empty-string fixed values are ok only
// when the value is exactly the normative empty string.
func ForbiddenKeysIn(env map[string]string, forbidden []string) []string {
	var hits []string
	for _, k := range forbidden {
		if _, ok := env[k]; ok {
			// Presence alone of a non-allowlist key is a hit; for allowlist
			// keys the caller checks normative values separately.
			hits = append(hits, k)
		}
	}
	sort.Strings(hits)
	return hits
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

// NormativeGoValue returns the expected value for a fixed go-env key, or
// ("", false) if the key is host-captured (not fixed).
func NormativeGoValue(key string) (string, bool) {
	v, ok := FixedGoEnv[key]
	return v, ok
}

func mapToEnv(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}
