//go:build unix

package fsx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"golang.org/x/sys/unix"
)

// ChildExists reports whether name exists as an exact child of the retained
// parent, without following a final symbolic link (Section 31.4). Any object
// form — file, directory, symlink, empty directory — counts as present.
//
// This is a descriptor-relative lookup (openat/fstatat on the retained fd).
// It never mutates the filesystem.
func (p *ParentHandle) ChildExists(name string) (bool, error) {
	if p == nil || p.fd < 0 {
		return false, errUnsafePath("<closed>", "parent handle is closed")
	}
	if name == "" || name == "." || name == ".." || stringsContainsSep(name) {
		return false, errUnsafePath(name, "child name must be a single path component")
	}
	var st unix.Stat_t
	err := unix.Fstatat(p.fd, name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil {
		if errorsIsNotExist(err) {
			return false, nil
		}
		return false, wrapCause(errUnsafePath(name, "cannot lstat destination child"), err)
	}
	return true, nil
}

// ChildLookup performs a no-follow exact-child lookup relative to the parent
// handle and returns the child's identity when available. Symlinks report
// exists=true with a zero identity (cannot open for fstat without O_PATH).
func (p *ParentHandle) ChildLookup(name string) (exists bool, id FileID, err error) {
	if p == nil || p.fd < 0 {
		return false, FileID{}, errUnsafePath("<closed>", "parent handle is closed")
	}
	if name == "" || name == "." || name == ".." || stringsContainsSep(name) {
		return false, FileID{}, errUnsafePath(name, "child name must be a single path component")
	}
	var st unix.Stat_t
	if err := unix.Fstatat(p.fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errorsIsNotExist(err) {
			return false, FileID{}, nil
		}
		return false, FileID{}, wrapCause(errUnsafePath(name, "cannot lstat destination child"), err)
	}
	mode := uint32(st.Mode) & unix.S_IFMT
	if mode == unix.S_IFLNK {
		// Symlink exists; identity of the link object itself via fstatat is
		// still valid (dev/ino of the symlink inode).
		return true, FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, nil
	}
	return true, FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, nil
}

// ClassifyDestination classifies the destination basename relative to the
// retained parent (REQ-003 refusal matrix). Does not mutate. Absent returns
// ClassAbsent with a nil error.
func (p *ParentHandle) ClassifyDestination() (DestinationClass, error) {
	if p == nil || p.fd < 0 {
		return "", errUnsafePath("<closed>", "parent handle is closed")
	}
	name := p.base
	var st unix.Stat_t
	err := unix.Fstatat(p.fd, name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil {
		if errorsIsNotExist(err) {
			return ClassAbsent, nil
		}
		return "", wrapCause(errUnsafePath(name, "cannot lstat destination"), err)
	}

	mode := uint32(st.Mode) & unix.S_IFMT
	switch mode {
	case unix.S_IFLNK:
		return ClassSymlink, nil
	case unix.S_IFREG:
		return ClassRegularFile, nil
	case unix.S_IFDIR:
		return classifyDir(p.fd, name)
	default:
		return ClassOther, nil
	}
}

func classifyDir(parentFD int, name string) (DestinationClass, error) {
	// Open the directory no-follow for .git probe and emptiness.
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		// Race or type change — fail closed as "other exists".
		if errorsIsLoop(err) {
			return ClassSymlink, nil
		}
		return ClassOther, nil
	}
	defer unix.Close(fd)

	// Prefer git_repo when .git is present (any form).
	var gitSt unix.Stat_t
	if err := unix.Fstatat(fd, ".git", &gitSt, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		return ClassGitRepo, nil
	} else if !errorsIsNotExist(err) {
		// Unexpected error reading .git — still an existing dir; treat as non-empty.
		return ClassNonEmptyDir, nil
	}

	empty, err := dirIsEmpty(fd)
	if err != nil {
		return ClassNonEmptyDir, nil
	}
	if empty {
		return ClassEmptyDir, nil
	}
	return ClassNonEmptyDir, nil
}

func dirIsEmpty(fd int) (bool, error) {
	// Dup so ReadDir does not disturb the caller's descriptor offset.
	// os.File.ReadDir omits "." and ".." — any returned entry means non-empty.
	// For n>0, an empty directory yields (nil, io.EOF).
	dup, err := unix.Dup(fd)
	if err != nil {
		return false, err
	}
	f := os.NewFile(uintptr(dup), "fsx-dir")
	defer f.Close()

	entries, err := f.ReadDir(1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return true, nil
		}
		return false, err
	}
	return len(entries) == 0, nil
}

// EnsureDestinationAbsent refuses if the destination basename exists in any
// form (Section 31.4 / REQ-003). Logs the refusal class and remediation id.
// On success logs class=absent. Never mutates.
func (p *ParentHandle) EnsureDestinationAbsent() error {
	if p == nil || p.fd < 0 {
		return errUnsafePath("<closed>", "parent handle is closed")
	}
	class, err := p.ClassifyDestination()
	if err != nil {
		logStep(p.log, "destination", "classify", "fail", err.Error())
		return err
	}
	if class == ClassAbsent {
		logStep(p.log, "destination", "preflight", "pass",
			fmt.Sprintf("basename=%q class=%s", p.base, class))
		return nil
	}
	// Destination path for location: parent/base (authored, not re-resolved
	// for mutation — diagnostic only).
	destPath := filepath.Join(p.authored, p.base)
	msg := fmt.Sprintf(
		"destination %q exists as %s; Foundry never overwrites, merges, or writes into existing trees",
		p.base, class,
	)
	logStep(p.log, "destination", "refuse", "fail",
		fmt.Sprintf("basename=%q class=%s id=%s remediation_id=%s",
			p.base, class, diagnostic.IDFSDestinationExists, diagnostic.IDFSDestinationExists))
	return errDestinationExists(destPath, msg)
}

// Reobserve re-opens the authored parent path and compares device/inode
// identity with the retained handle (pre-commit diagnostic, REQ-124).
// A mismatch or open failure yields fs.parent_moved. This is a diagnostic
// only — mutations continue through the retained descriptor when still valid.
func (p *ParentHandle) Reobserve() error {
	if p == nil || p.fd < 0 {
		return errUnsafePath("<closed>", "parent handle is closed")
	}
	// Open authored path as a directory. Intermediate symlink introduction
	// that redirects to a different object fails identity comparison;
	// disappearance fails open → parent_moved.
	fd, err := unix.Open(p.authored, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		logStep(p.log, "parent_reobserve", "open_authored", "fail",
			fmt.Sprintf("parent_base=%s errno=%s id=%s",
				filepath.Base(p.authored), errnoString(err), diagnostic.IDFSParentMoved))
		return wrapCause(
			errParentMoved(p.authored, "cannot re-open authored parent path (parent moved or replaced)"),
			err,
		)
	}
	defer unix.Close(fd)

	id, err := fileIDFromFD(fd)
	if err != nil {
		return wrapCause(errParentMoved(p.authored, "cannot fstat reobserved parent"), err)
	}
	if !id.Equal(p.id) {
		logStep(p.log, "parent_reobserve", "identity", "fail",
			fmt.Sprintf("retained=%s observed=%s id=%s", p.id, id, diagnostic.IDFSParentMoved))
		return errParentMoved(p.authored,
			fmt.Sprintf("parent identity changed: retained %s vs observed %s", p.id, id))
	}
	logStep(p.log, "parent_reobserve", "identity", "pass",
		fmt.Sprintf("parent_base=%s identity=%s", filepath.Base(p.authored), id))
	return nil
}

func stringsContainsSep(name string) bool {
	return filepath.Base(name) != name || name != filepath.Clean(name)
}
