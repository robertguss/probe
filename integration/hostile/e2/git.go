package e2

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EmptyTemplateDir creates a Foundry-owned empty scratch directory for
// `git init --template=` (Section 34.2 / 29.2 stage 16).
// Mode 0700. Caller removes after use (RemoveAll).
func EmptyTemplateDir(parent string) (string, error) {
	if parent == "" {
		parent = os.TempDir()
	}
	dir, err := os.MkdirTemp(parent, "foundry-git-template-*")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	// Must be empty: no hooks, no info, no description samples.
	entries, err := os.ReadDir(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if len(entries) != 0 {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("template scratch not empty: %d entries", len(entries))
	}
	return dir, nil
}

// IsolatedGitInit runs git init under the Section 34.2 environment in workDir.
// branch is the initial branch name (e.g. "main").
// Returns the constructed env for process-tree recording.
func IsolatedGitInit(gitBinary, workDir, branch, templateDir, path string) (env []string, err error) {
	if gitBinary == "" {
		gitBinary, err = exec.LookPath("git")
		if err != nil {
			return nil, fmt.Errorf("git not found: %w", err)
		}
	}
	if branch == "" {
		branch = "main"
	}
	env = ConstructGitEnv(path, templateDir)
	cmd := exec.Command(gitBinary, "init", "--initial-branch="+branch, "--template="+templateDir, ".")
	cmd.Dir = workDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return env, fmt.Errorf("git init: %w\n%s", err, out)
	}
	return env, nil
}

// TemplateLeakReport inspects a .git directory for host-template leakage.
type TemplateLeakReport struct {
	SentinelFilePresent bool     // .git/SENTINEL_TEMPLATE_FILE
	HostileHooks        []string // hook basenames that match hostile names we planted
	HooksDirEntries     []string
	NonEmptyHooksDir    bool
}

// InspectGitTemplateLeak checks workDir/.git for evidence of host template copy.
func InspectGitTemplateLeak(workDir string) (TemplateLeakReport, error) {
	var r TemplateLeakReport
	gitDir := filepath.Join(workDir, ".git")
	if st, err := os.Stat(gitDir); err != nil || !st.IsDir() {
		return r, fmt.Errorf(".git missing under %s: %v", workDir, err)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "SENTINEL_TEMPLATE_FILE")); err == nil {
		r.SentinelFilePresent = true
	}
	hooksDir := filepath.Join(gitDir, "hooks")
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		// Empty owned template → git may omit hooks dir entirely (good).
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}
	for _, e := range entries {
		name := e.Name()
		r.HooksDirEntries = append(r.HooksDirEntries, name)
		// Sample hooks from default templates are often *.sample; our hostile
		// plant uses post-checkout and pre-commit without .sample.
		if name == "post-checkout" || name == "pre-commit" {
			// Verify content is not our sentinel if present.
			b, _ := os.ReadFile(filepath.Join(hooksDir, name))
			if strings.Contains(string(b), "host-template-hook-ran") || strings.Contains(string(b), SentinelValue) {
				r.HostileHooks = append(r.HostileHooks, name)
			}
		}
	}
	r.NonEmptyHooksDir = len(r.HooksDirEntries) > 0
	return r, nil
}

// GitConfigEffective runs `git config --list --show-origin` under isolated env
// and returns raw output (for proving system/global are null).
func GitConfigEffective(gitBinary, workDir string, env []string) (string, error) {
	if gitBinary == "" {
		var err error
		gitBinary, err = exec.LookPath("git")
		if err != nil {
			return "", err
		}
	}
	cmd := exec.Command(gitBinary, "config", "--list", "--show-origin")
	cmd.Dir = workDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	// git config --list exits 0 even when empty; non-zero only on hard errors.
	return string(out), err
}
