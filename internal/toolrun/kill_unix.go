//go:build unix

package toolrun

import (
	"time"

	"golang.org/x/sys/unix"
)

// killProcessGroup sends SIGTERM then, after grace, SIGKILL to the process
// group whose leader is pid (Section 34.3). Negative pid form targets the group.
//
// Children started with Setpgid inherit the group; killing the group reaps
// grandchildren that a plain Process.Kill would leave behind.
func killProcessGroup(pid int, grace time.Duration) {
	if pid <= 0 {
		return
	}
	pgid := -pid
	// Best-effort polite stop first.
	_ = unix.Kill(pgid, unix.SIGTERM)
	if grace <= 0 {
		grace = DefaultKillGrace
	}
	// Bounded grace then hard kill.
	timer := time.NewTimer(grace)
	defer timer.Stop()
	<-timer.C
	_ = unix.Kill(pgid, unix.SIGKILL)
}

// killProcessGroupImmediate sends SIGKILL to the process group without grace.
// Used when the plan timeout already consumed the step budget.
func killProcessGroupImmediate(pid int) {
	if pid <= 0 {
		return
	}
	_ = unix.Kill(-pid, unix.SIGKILL)
}
