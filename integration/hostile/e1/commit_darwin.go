//go:build darwin

package e1

import "golang.org/x/sys/unix"

// exclusiveRename implements Section 31.7 Darwin primitive:
// RenameatxNp(parentFd, stage, parentFd, dest, RENAME_EXCL|RENAME_NOFOLLOW_ANY).
func exclusiveRename(parentFd int, stageName, destName string) error {
	return unix.RenameatxNp(
		parentFd, stageName,
		parentFd, destName,
		unix.RENAME_EXCL|unix.RENAME_NOFOLLOW_ANY,
	)
}

func renameSyscallName() string {
	return "RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY)"
}
