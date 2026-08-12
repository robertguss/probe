package gitinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SemanticReport is the outcome of post-init `.git` semantic validation
// (Section 35.3 / REQ-128): expected object layout, HEAD naming the
// configured branch, no hooks with content, empty index.
type SemanticReport struct {
	OK               bool
	IsDirectory      bool
	HEADRef          string // e.g. "refs/heads/main"
	ExpectedBranch   string
	HasObjectsDir    bool
	HasRefsDir       bool
	HooksWithContent []string // basenames of non-empty hook files
	IndexEmpty       bool     // true when index absent or zero-length
	// Failures are human-readable reasons when OK is false.
	Failures []string
}

// ValidateSemanticGit inspects stageDir/.git without invoking git.
// branch is the planned initial branch (e.g. "main").
//
// Checks:
//  1. `.git` exists and is a directory (not a file masquerading)
//  2. HEAD contains `ref: refs/heads/<branch>`
//  3. `objects/` and `refs/` directories exist
//  4. hooks directory is absent, empty, or contains only empty files
//     (no host-template hook content)
//  5. index is absent or empty (no staged content after bare init)
func ValidateSemanticGit(stageDir, branch string) SemanticReport {
	r := SemanticReport{ExpectedBranch: branch}
	if branch == "" {
		branch = "main"
		r.ExpectedBranch = branch
	}
	gitDir := filepath.Join(stageDir, ".git")
	st, err := os.Lstat(gitDir)
	if err != nil {
		r.Failures = append(r.Failures, fmt.Sprintf(".git missing: %v", err))
		return r
	}
	if !st.IsDir() {
		r.Failures = append(r.Failures, fmt.Sprintf(".git is not a directory (mode=%v)", st.Mode()))
		return r
	}
	r.IsDirectory = true

	// HEAD
	headPath := filepath.Join(gitDir, "HEAD")
	headBytes, err := os.ReadFile(headPath)
	if err != nil {
		r.Failures = append(r.Failures, fmt.Sprintf("HEAD unreadable: %v", err))
	} else {
		head := strings.TrimSpace(string(headBytes))
		want := "ref: refs/heads/" + branch
		r.HEADRef = head
		if head != want {
			r.Failures = append(r.Failures, fmt.Sprintf("HEAD=%q want %q", head, want))
		}
	}

	// objects/
	obj := filepath.Join(gitDir, "objects")
	if st, err := os.Stat(obj); err != nil || !st.IsDir() {
		r.Failures = append(r.Failures, "objects/ missing or not a directory")
	} else {
		r.HasObjectsDir = true
	}

	// refs/
	refs := filepath.Join(gitDir, "refs")
	if st, err := os.Stat(refs); err != nil || !st.IsDir() {
		r.Failures = append(r.Failures, "refs/ missing or not a directory")
	} else {
		r.HasRefsDir = true
	}

	// hooks: absent or no content
	hooksDir := filepath.Join(gitDir, "hooks")
	if st, err := os.Lstat(hooksDir); err == nil {
		if !st.IsDir() {
			r.Failures = append(r.Failures, "hooks exists but is not a directory")
		} else {
			entries, err := os.ReadDir(hooksDir)
			if err != nil {
				r.Failures = append(r.Failures, fmt.Sprintf("hooks readdir: %v", err))
			} else {
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					p := filepath.Join(hooksDir, e.Name())
					b, err := os.ReadFile(p)
					if err != nil {
						r.Failures = append(r.Failures, fmt.Sprintf("hook %s: %v", e.Name(), err))
						continue
					}
					if len(b) > 0 {
						r.HooksWithContent = append(r.HooksWithContent, e.Name())
					}
				}
				if len(r.HooksWithContent) > 0 {
					r.Failures = append(r.Failures, fmt.Sprintf(
						"hooks with content (host template leak?): %s",
						strings.Join(r.HooksWithContent, ","),
					))
				}
			}
		}
	}

	// index: absent or empty
	indexPath := filepath.Join(gitDir, "index")
	if st, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			r.IndexEmpty = true
		} else {
			r.Failures = append(r.Failures, fmt.Sprintf("index stat: %v", err))
		}
	} else {
		if st.Size() == 0 {
			r.IndexEmpty = true
		} else {
			// Empty-template init normally has no index; a non-empty index
			// means something staged content unexpectedly.
			r.Failures = append(r.Failures, fmt.Sprintf("index non-empty size=%d", st.Size()))
		}
	}

	r.OK = len(r.Failures) == 0
	return r
}

// GitDirExists reports whether stageDir/.git exists in any form.
func GitDirExists(stageDir string) bool {
	if stageDir == "" {
		return false
	}
	_, err := os.Lstat(filepath.Join(stageDir, ".git"))
	return err == nil
}
