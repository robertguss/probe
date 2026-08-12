//go:build unix && !linux && !darwin

package e1

// detectFSType is a stub for non-Linux non-Darwin unix targets.
func detectFSType(path string) string {
	_ = path
	return "unknown"
}
