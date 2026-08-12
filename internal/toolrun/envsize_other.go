//go:build !unix

package toolrun

// sysconfARGMAX returns a conservative default environment+argument size
// limit on non-Unix platforms. Production builds on these platforms still
// benefit from an early deterministic rejection before invoking exec.
func sysconfARGMAX() int64 { return 2 << 20 }
