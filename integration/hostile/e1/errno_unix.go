//go:build unix

package e1

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

var errExist = unix.EEXIST

func isErrno(err error, target error) bool {
	return errors.Is(err, target)
}

func underlyingIsExist(err error) bool {
	return errors.Is(err, unix.EEXIST) || errors.Is(err, syscall.EEXIST)
}

func isRenameUnsupported(err error) bool {
	// ENOTSUP / EOPNOTSUPP / EINVAL — filesystems without no-replace (FAT/exFAT).
	return errors.Is(err, unix.ENOTSUP) ||
		errors.Is(err, unix.EOPNOTSUPP) ||
		errors.Is(err, unix.EINVAL) ||
		errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.EINVAL)
}

func isEXDEV(err error) bool {
	return errors.Is(err, unix.EXDEV) || errors.Is(err, syscall.EXDEV)
}

func errnoString(err error) string {
	if err == nil {
		return ""
	}
	var errno unix.Errno
	if errors.As(err, &errno) {
		return errno.Error()
	}
	var errno2 syscall.Errno
	if errors.As(err, &errno2) {
		return errno2.Error()
	}
	return err.Error()
}

func renameUnsupportedSamples() []error {
	return []error{unix.ENOTSUP, unix.EOPNOTSUPP, unix.EINVAL}
}
