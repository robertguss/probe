//go:build hostile && darwin

package hostilefsx

import (
	"strings"

	"golang.org/x/sys/unix"
)

// detectFSType returns the filesystem type for path using Statfs Fstypename
// (e.g. "apfs"). Used by the hostile matrix to label APFS evidence rows.
func detectFSType(path string) string {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return "unknown"
	}
	b := make([]byte, 0, len(st.Fstypename))
	for _, c := range st.Fstypename {
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	label := strings.ToLower(string(b))
	if label == "" {
		return "unknown"
	}
	return label
}
