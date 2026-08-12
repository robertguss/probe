//go:build linux

package toolrun

// sysconfARGMAX returns the host ARG_MAX budget used for early env-size checks.
//
// golang.org/x/sys/unix does not export Sysconf / SC_ARG_MAX on Linux (only on
// Solaris), so we cannot call libc sysconf without cgo. Product builds are
// CGO_ENABLED=0. Use the common Linux default of 2 MiB (2097152), which matches
// getconf ARG_MAX on typical glibc hosts and the previous error-path fallback.
//
// Tests that need a tighter limit inject BoundStarterOptions.EnvSizeLimit.
func sysconfARGMAX() int64 {
	return 2 << 20
}
