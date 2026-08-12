//go:build hostile && unix && !linux && !darwin

package hostilefsx

// detectFSType is a stub for non-Linux/non-Darwin unix (suite still builds).
func detectFSType(path string) string {
	_ = path
	return "unknown"
}
