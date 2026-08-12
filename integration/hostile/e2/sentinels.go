package e2

import (
	"fmt"
	"os"
	"path/filepath"
)

// SentinelValue is a distinctive marker string used as hostile pollution.
// Never a real secret; chosen so accidental appearance is obvious in logs.
const SentinelValue = "FOUNDRY_E2_SENTINEL_LEAK"

// GoSentinelKeys are host environment keys that MUST NOT influence go tool
// behavior under Section 34.2 construction. Each is planted with a hostile
// value in the host, then proven absent or closed in the child env.
//
// Matrix coverage (issue notes + FND-005):
//
//	GOENV, GOFLAGS, GOCACHEPROG, GOAUTH, GOVCS,
//	GOPRIVATE, GONOPROXY, GONOSUMDB, GOINSECURE,
//	plus undeclared helpers GODEBUG, GOEXPERIMENT, GCCGO, CC, CXX.
var GoSentinelKeys = []string{
	"GOENV",
	"GOFLAGS",
	"GOCACHEPROG",
	"GOAUTH",
	"GOVCS",
	"GOPRIVATE",
	"GONOPROXY",
	"GONOSUMDB",
	"GOINSECURE",
	"GODEBUG",
	"GOEXPERIMENT",
	"GCCGO",
	"CC",
	"CXX",
	"CGO_CFLAGS",
	"CGO_LDFLAGS",
}

// GitSentinelKeys are host keys that must not appear in the git-init env
// (except the closed GIT_CONFIG_* / GIT_TEMPLATE_DIR controlled by construction).
var GitSentinelKeys = []string{
	"HOME",
	"GIT_DIR",
	"GIT_WORK_TREE",
	"XDG_CONFIG_HOME",
	"GIT_CONFIG",
	"GIT_CONFIG_COUNT",
	"GIT_EXEC_PATH",
	"GIT_TRACE",
	"EMAIL",
	"GIT_AUTHOR_NAME",
	"GIT_AUTHOR_EMAIL",
	"GIT_COMMITTER_NAME",
	"GIT_COMMITTER_EMAIL",
}

// SentinelPlant is one planted hostile fixture for the matrix.
type SentinelPlant struct {
	Key         string // env key name (logged; never values in failure dumps for secrets)
	Kind        string // go-fixed | go-absent | git-absent | goenv-file | helper-prog | git-template
	Description string
}

// FullSentinelMatrix is the complete E2 / REQ-214 plant list.
func FullSentinelMatrix() []SentinelPlant {
	plants := make([]SentinelPlant, 0, len(GoSentinelKeys)+len(GitSentinelKeys)+4)
	for _, k := range []string{"GOENV", "GOFLAGS", "GOCACHEPROG", "GOAUTH", "GOVCS",
		"GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GOINSECURE"} {
		plants = append(plants, SentinelPlant{
			Key: k, Kind: "go-fixed",
			Description: "Section 34.2 fixed/closed go surface; child must have normative value",
		})
	}
	for _, k := range []string{"GODEBUG", "GOEXPERIMENT", "GCCGO", "CC", "CXX", "CGO_CFLAGS", "CGO_LDFLAGS"} {
		plants = append(plants, SentinelPlant{
			Key: k, Kind: "go-absent",
			Description: "undeclared host var; must be absent from empty-base construction",
		})
	}
	for _, k := range GitSentinelKeys {
		plants = append(plants, SentinelPlant{
			Key: k, Kind: "git-absent",
			Description: "must be absent from git-init allowlist environment",
		})
	}
	plants = append(plants,
		SentinelPlant{Key: "GOENV_FILE", Kind: "goenv-file", Description: "persisted go env file must not be read (GOENV=off)"},
		SentinelPlant{Key: "GOFLAGS_TOOLEXEC", Kind: "helper-prog", Description: "GOFLAGS -toolexec sentinel must not execute"},
		SentinelPlant{Key: "GOCACHEPROG_HELPER", Kind: "helper-prog", Description: "GOCACHEPROG helper must not execute"},
		SentinelPlant{Key: "GIT_TEMPLATE", Kind: "git-template", Description: "host template files/hooks must not copy into init"},
	)
	return plants
}

// HostileHostEnv builds a process environment that is intentionally polluted
// with sentinel values for every GoSentinelKey and GitSentinelKey, layered on
// a base (typically os.Environ()). Used to prove construction ignores pollution.
func HostileHostEnv(base []string, helpersDir string) []string {
	// Start from base, then force-set sentinels (later entries win in EnvMap
	// first-wins, so we strip then append).
	strip := make(map[string]struct{})
	for _, k := range GoSentinelKeys {
		strip[k] = struct{}{}
	}
	for _, k := range GitSentinelKeys {
		strip[k] = struct{}{}
	}
	strip["GIT_TEMPLATE_DIR"] = struct{}{}
	strip["GIT_CONFIG_GLOBAL"] = struct{}{}
	strip["GIT_CONFIG_SYSTEM"] = struct{}{}

	out := make([]string, 0, len(base)+len(GoSentinelKeys)+len(GitSentinelKeys)+8)
	for _, e := range base {
		k, _, ok := splitEnv(e)
		if !ok {
			continue
		}
		if _, bad := strip[k]; bad {
			continue
		}
		out = append(out, e)
	}

	// Plant string sentinels.
	for _, k := range GoSentinelKeys {
		switch k {
		case "GOFLAGS":
			// Hostile mode flag + toolexec if helper present.
			toolexec := filepath.Join(helpersDir, "toolexec-sentinel.sh")
			out = append(out, fmt.Sprintf("GOFLAGS=-mod=mod -toolexec=%s", toolexec))
		case "GOCACHEPROG":
			out = append(out, "GOCACHEPROG="+filepath.Join(helpersDir, "cacheprog-sentinel.sh"))
		case "GOAUTH":
			out = append(out, "GOAUTH="+filepath.Join(helpersDir, "goauth-sentinel.sh"))
		case "GOENV":
			out = append(out, "GOENV="+filepath.Join(helpersDir, "hostile-goenv"))
		case "GOVCS":
			out = append(out, "GOVCS=public:git|hg|svn|fossil,private:all")
		case "GOPRIVATE", "GONOPROXY", "GONOSUMDB":
			out = append(out, k+"=*.e2-sentinel.invalid,"+SentinelValue)
		case "GOINSECURE":
			out = append(out, "GOINSECURE=*.e2-sentinel.invalid")
		default:
			out = append(out, k+"="+SentinelValue)
		}
	}
	for _, k := range GitSentinelKeys {
		out = append(out, k+"="+SentinelValue)
	}
	// Hostile template dir pointer (git would honor if inherited).
	out = append(out, "GIT_TEMPLATE_DIR="+filepath.Join(helpersDir, "hostile-template"))
	out = append(out, "GIT_CONFIG_GLOBAL="+filepath.Join(helpersDir, "hostile-gitconfig"))
	out = append(out, "GIT_CONFIG_SYSTEM="+filepath.Join(helpersDir, "hostile-gitconfig-system"))
	return out
}

// WriteSentinelHelpers creates helper programs and hostile config fixtures
// under dir. If any helper runs, it appends a line to markerPath.
func WriteSentinelHelpers(dir, markerPath string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// toolexec sentinel: invoked as `toolexec tool [args…]`; must exec tool for
	// go to continue, but records that it ran.
	toolexec := `#!/bin/sh
echo "toolexec-sentinel-ran" >> "` + markerPath + `"
exec "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "toolexec-sentinel.sh"), []byte(toolexec), 0o755); err != nil {
		return err
	}
	// GOCACHEPROG: any invocation records; exit 1 so go does not hang.
	cacheprog := `#!/bin/sh
echo "cacheprog-sentinel-ran" >> "` + markerPath + `"
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "cacheprog-sentinel.sh"), []byte(cacheprog), 0o755); err != nil {
		return err
	}
	// GOAUTH helper.
	goauth := `#!/bin/sh
echo "goauth-sentinel-ran" >> "` + markerPath + `"
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "goauth-sentinel.sh"), []byte(goauth), 0o755); err != nil {
		return err
	}
	// Hostile GOENV file: if read, would inject GOFLAGS.
	goenvBody := "GOFLAGS=-toolexec=" + filepath.Join(dir, "toolexec-sentinel.sh") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "hostile-goenv"), []byte(goenvBody), 0o644); err != nil {
		return err
	}
	// Hostile git config with alias that would be dangerous if loaded.
	gitcfg := "[alias]\n\te2leak = !echo git-config-sentinel-ran >> " + markerPath + "\n"
	gitcfg += "[core]\n\thooksPath = " + filepath.Join(dir, "hostile-hooks") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "hostile-gitconfig"), []byte(gitcfg), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "hostile-gitconfig-system"), []byte(gitcfg), 0o644); err != nil {
		return err
	}
	// Hostile template with sentinel file + hook.
	tmpl := filepath.Join(dir, "hostile-template")
	hooks := filepath.Join(tmpl, "hooks")
	if err := os.MkdirAll(hooks, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmpl, "SENTINEL_TEMPLATE_FILE"), []byte(SentinelValue+"\n"), 0o644); err != nil {
		return err
	}
	hook := "#!/bin/sh\necho host-template-hook-ran >> \"" + markerPath + "\"\n"
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte(hook), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(hook), 0o755); err != nil {
		return err
	}
	return nil
}

// MarkerContains reports whether marker file contains the needle.
func MarkerContains(markerPath, needle string) bool {
	b, err := os.ReadFile(markerPath)
	if err != nil {
		return false
	}
	return containsStr(string(b), needle)
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func splitEnv(e string) (key, val string, ok bool) {
	for i := 0; i < len(e); i++ {
		if e[i] == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return "", "", false
}
