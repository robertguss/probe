package gitinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// templatePrefix is the MkdirTemp pattern for Foundry-owned empty Git templates
// (Section 34.2 / 29.2 stage 16). Lives under the Foundry temp root — never
// under the destination-parent namespace.
const templatePrefix = "foundry-git-template-*"

// templateMode is the exclusive scratch mode (0700).
const templateMode = 0o700

// EmptyTemplate creates a Foundry-owned empty scratch directory for
// `git init --template=` (Section 34.2). Mode 0700. Caller must RemoveTemplate
// after use (stage 16).
//
// parent is the Foundry temporary root; empty uses os.TempDir(). The directory
// is verified empty (no hooks, description samples, or host-copied files).
func EmptyTemplate(parent string) (string, error) {
	if parent == "" {
		parent = os.TempDir()
	}
	// Ensure parent exists when tests inject a custom root.
	if err := os.MkdirAll(parent, templateMode); err != nil {
		return "", fmt.Errorf("gitinit: template parent: %w", err)
	}
	dir, err := os.MkdirTemp(parent, templatePrefix)
	if err != nil {
		return "", fmt.Errorf("gitinit: create empty template: %w", err)
	}
	if err := os.Chmod(dir, templateMode); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("gitinit: chmod template 0700: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("gitinit: read template: %w", err)
	}
	if len(entries) != 0 {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("gitinit: template scratch not empty: %d entries", len(entries))
	}
	return dir, nil
}

// RemoveTemplate deletes a Foundry-owned template scratch via RemoveAll.
// Empty path is a no-op. Safe to call on already-removed paths (IsNotExist).
//
// This is the only recursive delete gitinit performs, and it targets Foundry
// temp space — never the destination namespace or stage (Section 31.6).
func RemoveTemplate(dir string) error {
	if dir == "" {
		return nil
	}
	// Refuse to remove pathologically short / root-like paths.
	clean := filepath.Clean(dir)
	if clean == "." || clean == "/" || clean == string(filepath.Separator) {
		return fmt.Errorf("gitinit: refuse to remove template path %q", dir)
	}
	// Require the foundry-git-template marker in the basename for safety.
	base := filepath.Base(clean)
	if !strings.HasPrefix(base, "foundry-git-template-") {
		return fmt.Errorf("gitinit: refuse to remove non-template path basename %q", base)
	}
	if err := os.RemoveAll(clean); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("gitinit: remove template: %w", err)
	}
	return nil
}

// IsFoundryTemplate reports whether path looks like a Foundry-owned empty
// template scratch (basename prefix). Used by tests and logging assertions.
func IsFoundryTemplate(path string) bool {
	if path == "" {
		return false
	}
	return strings.HasPrefix(filepath.Base(filepath.Clean(path)), "foundry-git-template-")
}
