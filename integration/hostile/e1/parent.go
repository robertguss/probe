//go:build unix

package e1

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// ParentHandle is the retained destination-parent directory descriptor (31.2).
// All existence checks, stage creation, and commit operate relative to this handle.
type ParentHandle struct {
	FD       int
	Authored string // authored parent path (for diagnostics only; never used for mutation)
	ID       FileID
	log      *ProbeLog
	fsLabel  string
}

// Close closes the parent directory descriptor.
func (p *ParentHandle) Close() error {
	if p == nil || p.FD < 0 {
		return nil
	}
	err := unix.Close(p.FD)
	p.FD = -1
	return err
}

// AcquireParent walks each path component with O_NOFOLLOW|O_DIRECTORY and
// applies the namespace-custody check (Sections 31.2–31.3).
// destination is the full destination path; the parent of its basename is acquired.
func AcquireParent(destination string, log *ProbeLog, fsLabel string) (*ParentHandle, string, error) {
	if log == nil {
		log = NewProbeLog()
	}
	destination = filepath.Clean(destination)
	if !filepath.IsAbs(destination) {
		abs, err := filepath.Abs(destination)
		if err != nil {
			return nil, "", err
		}
		destination = abs
	}
	base := filepath.Base(destination)
	parentPath := filepath.Dir(destination)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return nil, "", spikeErr(ErrUnsafePath, "destination basename empty", nil)
	}
	// Expand Darwin system directory aliases only (/tmp, /var) before the
	// O_NOFOLLOW walk. User-created symlink components still fail closed.
	parentPath = rewriteDarwinDirAliases(parentPath)
	destination = filepath.Join(parentPath, base)

	components := splitAbs(parentPath)
	if len(components) == 0 {
		return nil, "", spikeErr(ErrUnsafePath, "empty parent path", nil)
	}

	// Open the first component (always "/" for absolute paths after splitAbs).
	// We start at the root directory descriptor.
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_walk", Step: "open_root",
			Syscall: "open", Args: "/", Errno: errnoString(err), Outcome: "fail"})
		return nil, "", err
	}

	built := ""
	for i, comp := range components {
		// Skip empty; root already open for leading "".
		if comp == "" {
			built = "/"
			continue
		}
		built = filepath.Join(built, comp)
		next, err := unix.Openat(fd, comp, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			// Classify while dirfd is still open so ENOTDIR can be disambiguated
			// via fstatat(AT_SYMLINK_NOFOLLOW): Linux often reports ENOTDIR for
			// symlink components under O_NOFOLLOW|O_DIRECTORY rather than ELOOP.
			id := classifyParentOpenatFail(fd, comp, err)
			_ = unix.Close(fd)
			log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_walk", Step: "openat_component",
				Syscall: "openat", Args: fmt.Sprintf("dirfd + %q O_NOFOLLOW|O_DIRECTORY", comp),
				Errno: errnoString(err), Outcome: "fail",
				Detail: fmt.Sprintf("component=%s built=%s id=%s", comp, built, id)})
			return nil, "", spikeErr(id, fmt.Sprintf("component %q (path %s)", comp, built), err)
		}
		_ = unix.Close(fd)
		fd = next

		// Custody check through the just-opened descriptor (31.3).
		if err := checkCustody(fd, built, log, fsLabel); err != nil {
			_ = unix.Close(fd)
			return nil, "", err
		}
		log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_walk", Step: fmt.Sprintf("component_%d", i),
			Syscall: "openat+fstat", Args: fmt.Sprintf("%q", comp), Outcome: "pass",
			Detail: fmt.Sprintf("built=%s", built)})
	}

	id, err := fileIDFromFD(fd)
	if err != nil {
		_ = unix.Close(fd)
		return nil, "", err
	}
	log.Record(ProbeEntry{FS: fsLabel, Probe: "parent_walk", Step: "acquired",
		Syscall: "openat", Args: parentPath, Outcome: "pass", Detail: id.String()})

	return &ParentHandle{FD: fd, Authored: parentPath, ID: id, log: log, fsLabel: fsLabel}, base, nil
}

// rewriteDarwinDirAliases expands only /tmp and /var system aliases on Darwin.
func rewriteDarwinDirAliases(path string) string {
	if runtime.GOOS != "darwin" {
		return path
	}
	for _, alias := range []string{"/tmp", "/var"} {
		if path == alias || strings.HasPrefix(path, alias+string(filepath.Separator)) {
			resolved, err := filepath.EvalSymlinks(alias)
			if err != nil || resolved == "" {
				return path
			}
			return resolved + strings.TrimPrefix(path, alias)
		}
	}
	return path
}

func splitAbs(path string) []string {
	path = filepath.Clean(path)
	if path == "/" {
		return []string{""}
	}
	parts := strings.Split(path, string(filepath.Separator))
	// Ensure leading empty for absolute.
	if !strings.HasPrefix(path, string(filepath.Separator)) {
		return parts
	}
	if len(parts) > 0 && parts[0] != "" {
		parts = append([]string{""}, parts...)
	}
	return parts
}

func checkCustody(fd int, componentPath string, log *ProbeLog, fsLabel string) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		log.Record(ProbeEntry{FS: fsLabel, Probe: "custody", Step: "fstat",
			Syscall: "fstat", Errno: errnoString(err), Outcome: "fail"})
		return err
	}
	uid := st.Uid
	euid := uint32(os.Geteuid())
	// Mode field width differs by platform (uint32 Linux, uint16 Darwin).
	modeBits := uint32(st.Mode) & 07777

	// Sticky bit permits group/other write (e.g. /tmp 1777) — SV-05.
	sticky := (uint32(st.Mode) & uint32(unix.S_ISVTX)) != 0
	groupOrOtherWrite := (uint32(st.Mode)&uint32(unix.S_IWGRP)) != 0 || (uint32(st.Mode)&uint32(unix.S_IWOTH)) != 0

	// Root-owned standard prefixes are allowed even when euid != 0.
	var ownerOK bool
	switch {
	case uid == euid:
		ownerOK = true
	case uid == 0 && isSystemPrefix(componentPath):
		ownerOK = true
	default:
		ownerOK = false
	}

	if !ownerOK {
		msg := fmt.Sprintf("component %s owner uid=%d euid=%d mode=%04o", componentPath, uid, euid, modeBits)
		log.Record(ProbeEntry{FS: fsLabel, Probe: "custody", Step: "owner", Outcome: "fail", Detail: msg})
		return spikeErr(ErrNamespaceNotPrivate, msg, nil)
	}
	if groupOrOtherWrite && !sticky {
		msg := fmt.Sprintf("component %s shared-writable non-sticky owner=%d mode=%04o", componentPath, uid, modeBits)
		log.Record(ProbeEntry{FS: fsLabel, Probe: "custody", Step: "mode", Outcome: "fail", Detail: msg})
		return spikeErr(ErrNamespaceNotPrivate, msg, nil)
	}
	log.Record(ProbeEntry{FS: fsLabel, Probe: "custody", Step: "ok", Outcome: "pass",
		Detail: fmt.Sprintf("path=%s uid=%d mode=%04o sticky=%v", componentPath, uid, modeBits, sticky)})
	return nil
}

func isSystemPrefix(path string) bool {
	switch path {
	case "/", "/home", "/Users", "/tmp", "/var", "/var/tmp", "/private", "/private/tmp", "/private/var", "/private/var/tmp":
		return true
	}
	// Darwin per-user temporary hierarchy (root-owned intermediates).
	if strings.HasPrefix(path, "/private/var/folders") ||
		strings.HasPrefix(path, "/var/folders") ||
		strings.HasPrefix(path, "/private/tmp/") {
		return true
	}
	return false
}

func fileIDFromFD(fd int) (FileID, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return FileID{}, err
	}
	return FileID{Dev: uint64(st.Dev), Ino: st.Ino}, nil
}

func errorsIsNotExist(err error) bool {
	return errors.Is(err, unix.ENOENT) || errors.Is(err, syscall.ENOENT)
}

func errorsIsNotDir(err error) bool {
	return errors.Is(err, unix.ENOTDIR) || errors.Is(err, syscall.ENOTDIR)
}

func errorsIsLoop(err error) bool {
	return errors.Is(err, unix.ELOOP) || errors.Is(err, syscall.ELOOP)
}

// classifyParentOpenatFail maps openat(O_NOFOLLOW|O_DIRECTORY) failures to
// Appendix D identifiers:
//   - ENOENT → fs.parent_missing (parent absent)
//   - ENOTDIR on a regular/non-dir file → fs.parent_missing (not a directory)
//   - ENOTDIR/ELOOP on a symlink component → fs.unsafe_path
//   - other open failures → fs.unsafe_path (fail closed on unsafe shapes)
//
// dirfd must still be open; used for AT_SYMLINK_NOFOLLOW lstat disambiguation.
func classifyParentOpenatFail(dirfd int, comp string, err error) string {
	if errorsIsNotExist(err) {
		return ErrParentMissing
	}
	if errorsIsLoop(err) || isSymlinkComponent(dirfd, comp) {
		return ErrUnsafePath
	}
	if errorsIsNotDir(err) {
		return ErrParentMissing
	}
	return ErrUnsafePath
}

func isSymlinkComponent(dirfd int, name string) bool {
	var st unix.Stat_t
	if err := unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false
	}
	return (uint32(st.Mode) & unix.S_IFMT) == unix.S_IFLNK
}

// ChildLookup performs a no-follow exact-child lookup relative to the parent handle (31.4).
func (p *ParentHandle) ChildLookup(name string) (exists bool, id FileID, err error) {
	fd, err := unix.Openat(p.FD, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if err == unix.ENOENT || err == syscall.ENOENT {
			return false, FileID{}, nil
		}
		// Symlink without O_PATH may yield ELOOP — still "exists".
		if err == unix.ELOOP || err == syscall.ELOOP {
			return true, FileID{}, nil
		}
		return false, FileID{}, err
	}
	defer unix.Close(fd)
	id, err = fileIDFromFD(fd)
	if err != nil {
		return true, FileID{}, err
	}
	return true, id, nil
}

// OpenRootFromFD opens an os.Root on the directory referenced by fd via /proc
// (Linux) — object continuity is preserved (SV-01 + SV-03). Used for stage
// content operations after handle acquisition; never for untrusted path starts.
func OpenRootFromFD(fd int) (*os.Root, error) {
	// Linux and modern Darwin expose the open fd through the process fd table.
	// Using the already-open descriptor path avoids pathname re-resolution of
	// the destination parent.
	path := fmt.Sprintf("/dev/fd/%d", fd)
	if _, err := os.Stat(path); err != nil {
		path = fmt.Sprintf("/proc/self/fd/%d", fd)
	}
	return os.OpenRoot(path)
}
