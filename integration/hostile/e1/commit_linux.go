//go:build linux

package e1

import "golang.org/x/sys/unix"

// exclusiveRename implements Section 31.7 Linux primitive:
// Renameat2(parentFd, stage, parentFd, dest, RENAME_NOREPLACE).
func exclusiveRename(parentFd int, stageName, destName string) error {
	return unix.Renameat2(parentFd, stageName, parentFd, destName, unix.RENAME_NOREPLACE)
}

func renameSyscallName() string {
	return "Renameat2(RENAME_NOREPLACE)"
}
