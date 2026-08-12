package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type doctorData struct {
	Version         string          `json:"version"`
	GoVersion       string          `json:"go_version"`
	ProbeDirExists  bool            `json:"probe_dir_exists"`
	CatalogRoot     string          `json:"catalog_root"`
	CatalogWritable bool            `json:"catalog_writable"`
	AuthEnv         map[string]bool `json:"auth_env"`
	FnoxOnPATH      bool            `json:"fnox_on_path"`
	LocalEnvGitRisk bool            `json:"local_env_would_be_tracked"`
	Hints           []string        `json:"hints,omitempty"`
}

func (e *Engine) doctor(_ context.Context) Result {
	data := doctorData{
		Version:   Version,
		GoVersion: runtime.Version(),
		AuthEnv:   map[string]bool{},
	}

	if sp, err := e.openSpike(); err == nil {
		if _, err := os.Stat(sp.Root); err == nil {
			data.ProbeDirExists = true
			if cfg, err := e.loadConfig(sp); err == nil {
				for name, p := range cfg.Auth {
					switch p.Type {
					case AuthBearer:
						data.AuthEnv[name+"."+p.TokenEnv] = e.envPresent(p.TokenEnv)
					case AuthBasic:
						data.AuthEnv[name+"."+p.UserEnv] = e.envPresent(p.UserEnv)
						data.AuthEnv[name+"."+p.PassEnv] = e.envPresent(p.PassEnv)
					case AuthHeader:
						data.AuthEnv[name+"."+p.ValueEnv] = e.envPresent(p.ValueEnv)
					}
				}
			}
		}
	}

	if cat, err := e.resolveCatalog(); err == nil {
		data.CatalogRoot = cat.Root
		data.CatalogWritable = catalogWritable(cat.Root)
	}

	_, err := exec.LookPath("fnox")
	data.FnoxOnPATH = err == nil
	if !data.FnoxOnPATH {
		data.Hints = append(data.Hints, "fnox not on PATH; secrets typically enter via fnox exec -- probe …")
	}

	cwd, err := e.cwd()
	if err == nil {
		envPath := filepath.Join(cwd, ".env")
		if _, err := os.Stat(envPath); err == nil {
			if tracked, warn := gitWouldTrack(cwd, ".env"); warn {
				data.LocalEnvGitRisk = tracked
				if tracked {
					data.Hints = append(data.Hints, ".env exists and appears trackable by git; probe does not load .env")
				}
			}
		}
	}

	return e.ok("doctor", data)
}

func (e *Engine) envPresent(name string) bool {
	v, ok := e.environ[name]
	return ok && v != ""
}

func catalogWritable(root string) bool {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(root, ".probe-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func gitWouldTrack(cwd, rel string) (tracked bool, ok bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return false, false
	}
	cmd := exec.Command("git", "-C", cwd, "check-ignore", "-q", rel)
	err := cmd.Run()
	if err == nil {
		return false, true // ignored
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return true, true // not ignored
	}
	return false, false
}

type quickstartData struct {
	Steps         []string `json:"steps"`
	RateLimitNote string   `json:"rate_limit_note"`
	SecretsNote   string   `json:"secrets_note"`
}

func (e *Engine) quickstart(_ context.Context) Result {
	return e.ok("quickstart", quickstartData{
		Steps: []string{
			`probe init --json`,
			`probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json`,
			`fnox exec -- probe hit GET /api/v1/courses --auth canvas --base "$BASE" --save courses --json`,
			`probe note "observed Link header pagination" --json`,
			`probe promote canvas --endpoint get-courses --json`,
			`probe catalog show canvas --json`,
		},
		RateLimitNote: "HTTP 429 after retries → exit 5 and error.code=rate_limited",
		SecretsNote:   "probe never loads .env; wrap with fnox exec -- probe …",
	})
}

type schemaData struct {
	Version   string         `json:"version"`
	Envelope  map[string]any `json:"envelope"`
	ExitCodes map[string]int `json:"exit_codes"`
	Commands  []string       `json:"commands"`
	BodyFlag  string         `json:"body_flag"`
	JSONFlag  string         `json:"json_flag"`
}

func (e *Engine) schema(_ context.Context) Result {
	return e.ok("schema", schemaData{
		Version: Version,
		Envelope: map[string]any{
			"ok":      "bool",
			"command": "string",
			"data":    "object|null",
			"error":   "object|null{code,message,hint,next[]}",
			"meta":    "object{exitCode,version,requestId}",
		},
		ExitCodes: map[string]int{
			"success":      0,
			"transport":    1,
			"usage":        2,
			"http_4xx":     3,
			"http_5xx":     4,
			"rate_limited": 5,
		},
		Commands: []string{
			"version", "quickstart", "schema", "doctor", "init",
			"auth set|list|show", "hit", "replay", "last", "find", "note", "summary",
			"promote", "catalog list|show|path",
		},
		BodyFlag: "--body (request body; not the envelope flag)",
		JSONFlag: "--json (JSON envelope on stdout)",
	})
}
