//go:build hostile && linux

package hostilefsx_test

import "golang.org/x/sys/unix"

// realExclusiveRename uses the production Linux primitive.
func realExclusiveRename(parentFd int, stageName, destName string) error {
	return unix.Renameat2(parentFd, stageName, parentFd, destName, unix.RENAME_NOREPLACE)
}
