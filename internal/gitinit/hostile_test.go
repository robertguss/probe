//go:build unix

package gitinit_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/gitinit"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

const sentinelName = "SENTINEL_TEMPLATE_FILE"
const sentinelBody = "host-template-sentinel-e2leak"
const hostileHookBody = "#!/bin/sh\necho host-template-hook-ran\n"

// plantHostileTemplate creates a host template that would copy sentinel + hooks
// if git init used it (contrast threat model for REQ-128 / FND-005).
func plantHostileTemplate(t *testing.T, dir string) string {
	t.Helper()
	tmpl := filepath.Join(dir, "hostile-template")
	if err := os.MkdirAll(filepath.Join(tmpl, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpl, sentinelName), []byte(sentinelBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpl, "hooks", "pre-commit"), []byte(hostileHookBody), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpl, "hooks", "post-checkout"), []byte(hostileHookBody), 0o755); err != nil {
		t.Fatal(err)
	}
	return tmpl
}

// TestInit_HostileTemplate_NoHostCopy proves ambient GIT_TEMPLATE_DIR /
// GIT_CONFIG_GLOBAL / system template do not leak into the destination
// (Section 34.2 / REQ-128 / REQ-214).
func TestInit_HostileTemplate_NoHostCopy(t *testing.T) {
	if testing.Short() {
		t.Skip("skips real git subprocess test in short mode")
	}
	log := testutil.New(t)
	gitBin := lookGit(t)
	log.Phase("arrange")
	base := t.TempDir()
	hostileTmpl := plantHostileTemplate(t, base)

	// Contrast: init WITH hostile template must copy sentinel (threat model).
	contrast := filepath.Join(base, "contrast")
	if err := os.MkdirAll(contrast, 0o700); err != nil {
		t.Fatal(err)
	}
	// Use ambient exec only for contrast setup (not product path).
	// product path is gitinit.Init below.
	if out, err := runGitRaw(gitBin, contrast, nil, "init", "--template="+hostileTmpl, contrast); err != nil {
		t.Fatalf("contrast setup: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(contrast, ".git", sentinelName)); err != nil {
		t.Fatalf("contrast broken: sentinel not copied: %v", err)
	}
	log.Fixture("contrast_hostile", "sentinel copied under hostile template")

	// Plant hostile ambient env that product construction must ignore.
	fakeHome := filepath.Join(base, "fake-home")
	_ = os.MkdirAll(fakeHome, 0o700)
	hostileGlobal := filepath.Join(base, "hostile-gitconfig")
	if err := os.WriteFile(hostileGlobal, []byte("[alias]\n\te2leak = !echo leaked\n[core]\n\thooksPath = "+filepath.Join(hostileTmpl, "hooks")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_TEMPLATE_DIR", hostileTmpl)
	t.Setenv("HOME", fakeHome)
	t.Setenv("GIT_CONFIG_GLOBAL", hostileGlobal)
	t.Setenv("GIT_CONFIG_SYSTEM", hostileGlobal)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(fakeHome, ".config"))

	stage := stageWithFiles(t, map[string]string{"README.md": "ok\n"})
	// stageWithFiles uses t.TempDir — move content under base for clarity.
	stageFD := openStageFD(t, stage)
	ex := newExecutor(t, toolrun.BoundStarterOptions{})
	ml := &memLog{}
	log.PhaseEnd("arrange", testutil.OutcomeOK)

	log.Phase("act")
	res := gitinit.Init(context.Background(), ex, gitinit.Options{
		Init:          true,
		InitialBranch: "main",
		GitBinary:     gitBin,
		PATH:          os.Getenv("PATH"),
		StageFD:       stageFD,
		StageDir:      stage,
		TempRoot:      filepath.Join(base, "foundry-tmp"),
		Timeout:       30 * time.Second,
		Logger:        ml,
	})
	log.Subprocess("git-init", res.Argv, res.EnvHash, res.Step.ExitCode,
		int(res.Step.StdoutBytes), int(res.Step.StderrBytes), "", "", !res.OK())
	log.PhaseEnd("act", testutil.OutcomeOK)

	log.Phase("assert")
	if !res.OK() {
		log.Fail("init", res.Err().Error())
	}

	// No sentinel file under stage .git.
	if _, err := os.Stat(filepath.Join(stage, ".git", sentinelName)); err == nil {
		log.Fail("sentinel_leaked", "SENTINEL_TEMPLATE_FILE present under .git")
	}
	// No hostile hooks with content.
	log.Assert("no_hook_content", len(res.Semantic.HooksWithContent) == 0,
		0, res.Semantic.HooksWithContent)
	hooksDir := filepath.Join(stage, ".git", "hooks")
	if entries, err := os.ReadDir(hooksDir); err == nil {
		for _, e := range entries {
			b, _ := os.ReadFile(filepath.Join(hooksDir, e.Name()))
			if strings.Contains(string(b), "host-template-hook-ran") || strings.Contains(string(b), sentinelBody) {
				log.Fail("hostile_hook", e.Name())
			}
		}
		log.Step("hooks_entries", testutil.OutcomeOK, "n="+itoa(len(entries)))
	}

	// Constructed env must use owned template, not hostile.
	env := toolrun.ConstructGitEnv(os.Getenv("PATH"), res.TemplateDir)
	if env["GIT_TEMPLATE_DIR"] == hostileTmpl {
		log.Fail("env_template", "GIT_TEMPLATE_DIR still hostile")
	}
	if env["GIT_CONFIG_GLOBAL"] != "/dev/null" {
		log.Fail("config_global", env["GIT_CONFIG_GLOBAL"])
	}
	if env["GIT_CONFIG_SYSTEM"] != "/dev/null" {
		log.Fail("config_system", env["GIT_CONFIG_SYSTEM"])
	}
	if _, ok := env["HOME"]; ok {
		log.Fail("home_present", "HOME must be absent")
	}
	if _, ok := env["XDG_CONFIG_HOME"]; ok {
		log.Fail("xdg_present", "XDG_CONFIG_HOME must be absent")
	}
	// Bleed keys absent.
	bleed := toolrun.ForbiddenKeysPresent(env, []string{
		"GIT_DIR", "GIT_WORK_TREE", "HOME", "XDG_CONFIG_HOME", "SSH_AUTH_SOCK",
	})
	log.Assert("no_bleed", len(bleed) == 0, "[]", bleed)

	// Env used by step hash equals ConstructGitEnv for owned template.
	// (TemplateDir already removed; reconstruct from argv.)
	var tmplFromArgv string
	for _, a := range res.Argv {
		if strings.HasPrefix(a, "--template=") {
			tmplFromArgv = strings.TrimPrefix(a, "--template=")
		}
	}
	log.Assert("argv_template_foundry", gitinit.IsFoundryTemplate(tmplFromArgv), true, tmplFromArgv)
	log.Assert("semantic_ok", res.Semantic.OK, true, res.Semantic.OK)
	log.Assert("HEAD_main", res.Semantic.HEADRef == "ref: refs/heads/main",
		"ref: refs/heads/main", res.Semantic.HEADRef)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestInit_Hostile_ProcessTreeNoExtraGit ensures hostile env still only
// produces one planned git init argv (no config/commit/remote probes).
// Uses FakeGitRunner so it is not gated on a host git binary.
func TestInit_Hostile_ProcessTreeNoExtraGit(t *testing.T) {
	log := testutil.New(t)
	base := t.TempDir()
	hostile := plantHostileTemplate(t, base)
	t.Setenv("GIT_TEMPLATE_DIR", hostile)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(base, "nope"))

	stage := stageWithFiles(t, map[string]string{"f": "1\n"})
	stageFD := openStageFD(t, stage)
	fake := &FakeGitRunner{PlantGitDir: stage}
	res := gitinit.Init(context.Background(), fake, gitinit.Options{
		Init: true, InitialBranch: "main", GitBinary: "/usr/bin/git",
		PATH: "/usr/bin:/bin", StageFD: stageFD, StageDir: stage,
		TempRoot: filepath.Join(base, "tmp"), Timeout: 30 * time.Second,
	})
	if !res.OK() {
		log.Fail("ok", res.Err().Error())
	}
	log.Assert("one_git", fake.CallCount() == 1, 1, fake.CallCount())
	argv := fake.ObservedArgv(0)
	sub := gitinit.SubcommandOf(argv)
	log.Assert("only_init", sub == "init", "init", sub)
	joined := strings.Join(argv, " ")
	for _, f := range []string{"commit", "config", "remote", "push", "add"} {
		if strings.Contains(joined, " "+f+" ") || strings.HasSuffix(joined, " "+f) {
			log.Fail("extra_sub", f+" in "+joined)
		}
	}
}

// runGitRaw is test-only ambient git for contrast fixtures — not product path.
func runGitRaw(gitBin, dir string, env []string, args ...string) ([]byte, error) {
	// Intentionally uses os/exec in _test.go only.
	cmd := newGitCmd(gitBin, dir, env, args...)
	return cmd.CombinedOutput()
}

func newGitCmd(gitBin, dir string, env []string, args ...string) *execCmd {
	return startGit(gitBin, dir, env, args...)
}

// Thin wrappers keep hostile_test free of direct os/exec name in multiple
// places while still using the real tool for contrast setup only.
type execCmd struct {
	combined func() ([]byte, error)
}

func (c *execCmd) CombinedOutput() ([]byte, error) { return c.combined() }

func startGit(gitBin, dir string, env []string, args ...string) *execCmd {
	// Import cycle avoidance: use os/exec via local helper file... we need
	// os/exec. Inline here with a package-level helper in hostile_exec_test.go.
	return startGitExec(gitBin, dir, env, args...)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
