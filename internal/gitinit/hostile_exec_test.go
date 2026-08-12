//go:build unix

package gitinit_test

import (
	"os"
	"os/exec"
)

// startGitExec runs ambient git for contrast fixtures only (test code).
// Product gitinit never imports os/exec.
func startGitExec(gitBin, dir string, env []string, args ...string) *execCmd {
	cmd := exec.Command(gitBin, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	} else {
		cmd.Env = os.Environ()
	}
	return &execCmd{combined: cmd.CombinedOutput}
}
