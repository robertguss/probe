package toolrun

import (
	"fmt"
	"os"
)

// EmptyTemplateDir creates a Foundry-owned empty scratch directory for
// `git init --template=` (Section 34.2 / 29.2 stage 16).
//
// Mode 0700. The directory is empty (no hooks, no info, no description samples).
// Caller removes after use (RemoveAll). Absolute path is the only legal value
// for GIT_TEMPLATE_DIR in ConstructGitEnv.
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

// GitTemplateDir reports the GIT_TEMPLATE_DIR value from a constructed git env,
// or ("", false) if absent. Used by template isolation assertions.
func GitTemplateDir(env map[string]string) (string, bool) {
	v, ok := env["GIT_TEMPLATE_DIR"]
	return v, ok
}
