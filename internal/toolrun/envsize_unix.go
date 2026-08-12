//go:build unix && !linux

package toolrun

// sysconfARGMAX returns a conservative default environment+argument size
// limit on Unix platforms where _SC_ARG_MAX is not exposed by x/sys/unix. The
// value is chosen below common macOS/BSD kern.argmax defaults so the early
// guard fires before exec(2) rejects the oversized vector.
func sysconfARGMAX() int64 { return 256 << 10 }
