//go:build linux

package e1

import (
	"os/exec"
	"strings"
)

// detectFSType returns the filesystem type for path (findmnt FSTYPE preferred).
func detectFSType(path string) string {
	out, err := exec.Command("findmnt", "-n", "-o", "FSTYPE", "-T", path).Output()
	if err == nil {
		label := strings.TrimSpace(string(out))
		if label != "" {
			return label
		}
	}
	return "unknown"
}
