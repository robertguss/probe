//go:build !unix

package toolrun

import (
	"context"
	"fmt"
	"time"
)

// CaptureOriginalCWD is unavailable on non-unix platforms (REQ-004: macOS/Linux only).
func CaptureOriginalCWD() (*OriginalCWD, error) {
	return nil, fmt.Errorf("toolrun: CaptureOriginalCWD unsupported on this platform")
}

// Close is a no-op on non-unix.
func (o *OriginalCWD) Close() error {
	if o != nil {
		o.fd = -1
	}
	return nil
}

func platformFchdir(fd int) error {
	return fmt.Errorf("toolrun: fchdir unsupported on this platform (fd=%d)", fd)
}

func platformFileID(fd int) (FileID, error) {
	return FileID{}, fmt.Errorf("toolrun: fstat unsupported on this platform (fd=%d)", fd)
}

func platformStartChild(ctx context.Context, binary string, args, env []string, dir string, capBytes int, killGrace time.Duration) (childProc, error) {
	return nil, fmt.Errorf("toolrun: bound child start unsupported on this platform")
}

func errnoString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// OpenDirFD is unavailable on non-unix.
func OpenDirFD(path string) (int, error) {
	return -1, fmt.Errorf("toolrun: OpenDirFD unsupported on this platform")
}

// CloseFD is a no-op on non-unix.
func CloseFD(fd int) error { return nil }
