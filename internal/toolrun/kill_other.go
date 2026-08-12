//go:build !unix

package toolrun

import "time"

func killProcessGroup(pid int, grace time.Duration) {
	// Non-unix: product platforms are macOS/Linux only (REQ-004).
	_ = pid
	_ = grace
}

func killProcessGroupImmediate(pid int) {
	_ = pid
}
