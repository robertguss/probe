//go:build hostile && darwin

package hostilefsx_test

import "golang.org/x/sys/unix"

// realExclusiveRename uses the production Darwin primitive.
func realExclusiveRename(parentFd int, stageName, destName string) error {
	return unix.RenameatxNp(parentFd, stageName, parentFd, destName,
		unix.RENAME_EXCL|unix.RENAME_NOFOLLOW_ANY)
}
