//go:build unix

package fsx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"golang.org/x/sys/unix"
)

// openFlags is the openat flag set used for every parent component (31.2).
// Logged for agents; never follows symbolic links; requires a directory.
const openFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

// openFlagsLabel is the human/agent form of openFlags (stable log text).
const openFlagsLabel = "O_RDONLY|O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC"

// ParentHandle is the retained destination-parent directory descriptor
// (Section 31.2). All existence checks, stage creation, and commit operate
// relative to this handle. The handle identifies the same filesystem object
// regardless of subsequent pathname renames (SV-03).
//
// Close must be called when the transaction ends. The handle exposes no
// path-based mutation API.
type ParentHandle struct {
	fd       int
	authored string // authored parent path (diagnostics / reobserve only)
	base     string // destination basename
	id       FileID
	log      StepLogger
}

// AuthoredPath returns the parent path as authored (diagnostics only).
// MUST NOT be used for mutation — use the retained descriptor instead.
func (p *ParentHandle) AuthoredPath() string {
	if p == nil {
		return ""
	}
	return p.authored
}

// Basename returns the destination basename (equal to project name per 15.3).
func (p *ParentHandle) Basename() string {
	if p == nil {
		return ""
	}
	return p.base
}

// Identity returns the retained parent device/inode identity.
func (p *ParentHandle) Identity() FileID {
	if p == nil {
		return FileID{}
	}
	return p.id
}

// DirFD returns the retained parent directory descriptor for same-package
// descriptor-relative operations (stage create, commit). Callers outside
// fsx must not use this for pathname-based mutation.
func (p *ParentHandle) DirFD() int {
	if p == nil {
		return -1
	}
	return p.fd
}

// Close closes the parent directory descriptor.
func (p *ParentHandle) Close() error {
	if p == nil || p.fd < 0 {
		return nil
	}
	err := unix.Close(p.fd)
	p.fd = -1
	return err
}

// PreflightOptions configures parent acquisition and destination preflight.
type PreflightOptions struct {
	// Log receives structured walk/custody/destination steps. Nil is a no-op.
	Log StepLogger
}

// Preflight acquires the destination parent via the Section 31.2 no-follow
// component walk with Section 31.3 custody, then refuses if the destination
// basename exists in any form (Section 31.4 / REQ-003). On success the
// retained parent handle is returned; the caller must Close it.
//
// No path-based mutation is performed — only openat/fstat/fstatat relative
// to retained descriptors.
func Preflight(destination string, opts PreflightOptions) (*ParentHandle, error) {
	parent, err := AcquireParent(destination, opts)
	if err != nil {
		return nil, err
	}
	if err := parent.EnsureDestinationAbsent(); err != nil {
		_ = parent.Close()
		return nil, err
	}
	return parent, nil
}

// AcquireParent walks each destination-parent path component with
// O_NOFOLLOW|O_DIRECTORY and applies the namespace-custody check
// (Sections 31.2–31.3). The destination basename is recorded but not yet
// existence-checked — call EnsureDestinationAbsent or Preflight.
func AcquireParent(destination string, opts PreflightOptions) (*ParentHandle, error) {
	log := opts.Log
	dest, parentPath, base, err := normalizeDestination(destination)
	if err != nil {
		logStep(log, "parent_walk", "normalize", "fail", err.Error())
		return nil, err
	}
	_ = dest

	components := splitAbs(parentPath)
	if len(components) == 0 {
		return nil, errUnsafePath(parentPath, "empty parent path")
	}

	// Open the filesystem root as the walk start. Subsequent components are
	// opened relative to the previous descriptor (never a re-resolved path).
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		logStep(log, "parent_walk", "open_root", "fail",
			fmt.Sprintf("syscall=open flags=O_RDONLY|O_DIRECTORY errno=%s", errnoString(err)))
		return nil, wrapCause(errUnsafePath("/", "cannot open filesystem root"), err)
	}

	built := ""
	depth := 0
	for _, comp := range components {
		if comp == "" {
			built = "/"
			// Custody on root itself (system prefix, typically root-owned).
			if err := checkCustody(fd, "/", log); err != nil {
				_ = unix.Close(fd)
				return nil, err
			}
			continue
		}
		built = filepath.Join(built, comp)
		depth++
		next, err := unix.Openat(fd, comp, openFlags, 0)
		if err != nil {
			id := classifyParentOpenatFail(fd, comp, err)
			_ = unix.Close(fd)
			detail := fmt.Sprintf(
				"component=%q depth=%d flags=%s errno=%s id=%s built_base=%s",
				comp, depth, openFlagsLabel, errnoString(err), id, filepath.Base(built),
			)
			logStep(log, "parent_walk", "openat_component", "fail", detail)
			msg := fmt.Sprintf("parent component %q is not a safe directory", comp)
			var fe *diagnostic.FoundryError
			switch id {
			case diagnostic.IDFSParentMissing:
				fe = errParentMissing(built, msg)
			default:
				fe = errUnsafePath(built, msg)
			}
			return nil, wrapCause(fe, err)
		}
		_ = unix.Close(fd)
		fd = next

		if err := checkCustody(fd, built, log); err != nil {
			_ = unix.Close(fd)
			return nil, err
		}
		logStep(log, "parent_walk", "component", "pass",
			fmt.Sprintf("basename=%q depth=%d flags=%s custody=ok", comp, depth, openFlagsLabel))
	}

	id, err := fileIDFromFD(fd)
	if err != nil {
		_ = unix.Close(fd)
		return nil, wrapCause(errUnsafePath(parentPath, "cannot fstat acquired parent"), err)
	}
	logStep(log, "parent_walk", "acquired", "pass",
		fmt.Sprintf("depth=%d parent_base=%s identity=%s", depth, filepath.Base(parentPath), id))

	return &ParentHandle{
		fd:       fd,
		authored: parentPath,
		base:     base,
		id:       id,
		log:      log,
	}, nil
}

// normalizeDestination applies Section 15.3 lexical rules, then resolves to
// an absolute path. Returns (absDest, absParent, basename, err).
func normalizeDestination(destination string) (dest, parent, base string, err error) {
	if destination == "" {
		return "", "", "", errUnsafePath("<empty>", "destination must be a non-empty path")
	}
	if strings.Contains(destination, "\x00") {
		return "", "", "", errUnsafePath(destination, "destination must not contain NUL bytes")
	}
	if strings.Contains(destination, "$") {
		return "", "", "", errUnsafePath(destination, "destination must not contain environment-variable markers")
	}
	if strings.Contains(destination, "~") {
		return "", "", "", errUnsafePath(destination, "destination must not contain '~'")
	}
	// Reject ".." components before Clean collapses them (Section 15.3).
	for _, c := range strings.Split(destination, string(filepath.Separator)) {
		if c == ".." {
			return "", "", "", errUnsafePath(destination, `destination must not contain ".." components`)
		}
	}
	if destination == "." {
		return "", "", "", errUnsafePath(destination, `destination must not be "." (generation into the current directory is prohibited)`)
	}

	cleaned := filepath.Clean(destination)
	if !filepath.IsAbs(cleaned) {
		abs, absErr := filepath.Abs(cleaned)
		if absErr != nil {
			return "", "", "", wrapCause(errUnsafePath(destination, "cannot resolve destination path"), absErr)
		}
		cleaned = abs
	}
	base = filepath.Base(cleaned)
	if base == "." || base == string(filepath.Separator) || base == "" || base == ".." {
		return "", "", "", errUnsafePath(destination, "destination basename is empty or invalid")
	}
	parent = filepath.Dir(cleaned)
	if parent == cleaned {
		// Destination was "/" or similar — no parent directory to acquire.
		return "", "", "", errUnsafePath(destination, "destination has no parent directory")
	}
	// Expand only well-known Darwin directory aliases (/tmp → /private/tmp,
	// /var → /private/var) so O_NOFOLLOW walk does not refuse those system
	// directory symlinks. User-created symlink components still fail closed.
	parent = rewriteDarwinDirAliases(parent)
	cleaned = filepath.Join(parent, base)
	return cleaned, parent, base, nil
}

// rewriteDarwinDirAliases rewrites path prefixes that are Darwin system
// directory symlinks. On non-Darwin hosts the path is returned unchanged.
// Only the fixed aliases /tmp and /var are expanded — never arbitrary
// EvalSymlinks of the full parent (that would silently follow hostile links).
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
	if !strings.HasPrefix(path, string(filepath.Separator)) {
		return parts
	}
	if len(parts) > 0 && parts[0] != "" {
		parts = append([]string{""}, parts...)
	}
	return parts
}

func checkCustody(fd int, componentPath string, log StepLogger) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		logStep(log, "custody", "fstat", "fail",
			fmt.Sprintf("path_base=%s errno=%s", filepath.Base(componentPath), errnoString(err)))
		return wrapCause(errUnsafePath(componentPath, "cannot fstat parent component"), err)
	}
	uid := st.Uid
	euid := uint32(os.Geteuid())
	modeBits := uint32(st.Mode) & 07777

	// Sticky bit permits group/other write (e.g. /tmp 1777) — SV-05.
	sticky := (uint32(st.Mode) & uint32(unix.S_ISVTX)) != 0
	groupOrOtherWrite := (uint32(st.Mode)&uint32(unix.S_IWGRP)) != 0 ||
		(uint32(st.Mode)&uint32(unix.S_IWOTH)) != 0

	// Root-owned standard system prefixes are allowed even when euid != 0.
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
		msg := fmt.Sprintf(
			"component %s owner uid=%d euid=%d mode=%04o (custody: owner not euid or allowed system prefix)",
			componentPath, uid, euid, modeBits,
		)
		logStep(log, "custody", "owner", "fail",
			fmt.Sprintf("path_base=%s uid=%d euid=%d mode=%04o verdict=reject id=%s",
				filepath.Base(componentPath), uid, euid, modeBits, diagnostic.IDFSNamespaceNotPrivate))
		return errNamespaceNotPrivate(componentPath, msg)
	}
	if groupOrOtherWrite && !sticky {
		msg := fmt.Sprintf(
			"component %s shared-writable non-sticky owner=%d mode=%04o",
			componentPath, uid, modeBits,
		)
		logStep(log, "custody", "mode", "fail",
			fmt.Sprintf("path_base=%s uid=%d mode=%04o sticky=false verdict=reject id=%s",
				filepath.Base(componentPath), uid, modeBits, diagnostic.IDFSNamespaceNotPrivate))
		return errNamespaceNotPrivate(componentPath, msg)
	}
	logStep(log, "custody", "ok", "pass",
		fmt.Sprintf("path_base=%s uid=%d mode=%04o sticky=%v verdict=allow",
			filepath.Base(componentPath), uid, modeBits, sticky))
	return nil
}

func isSystemPrefix(path string) bool {
	switch path {
	case "/", "/home", "/Users", "/tmp", "/var", "/var/tmp",
		"/private", "/private/tmp", "/private/var", "/private/var/tmp":
		return true
	}
	// Darwin per-user temporary hierarchy: root-owned intermediates under
	// /private/var/folders/... (and the sticky /private/tmp tree).
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
	return FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, nil
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

// classifyParentOpenatFail maps openat(O_NOFOLLOW|O_DIRECTORY) failures:
//   - ENOENT → fs.parent_missing
//   - ENOTDIR on a regular/non-dir file → fs.parent_missing
//   - ENOTDIR/ELOOP on a symlink component → fs.unsafe_path
//   - other open failures → fs.unsafe_path (fail closed)
func classifyParentOpenatFail(dirfd int, comp string, err error) diagnostic.Identifier {
	if errorsIsNotExist(err) {
		return diagnostic.IDFSParentMissing
	}
	if errorsIsLoop(err) || isSymlinkComponent(dirfd, comp) {
		return diagnostic.IDFSUnsafePath
	}
	if errorsIsNotDir(err) {
		return diagnostic.IDFSParentMissing
	}
	return diagnostic.IDFSUnsafePath
}

func isSymlinkComponent(dirfd int, name string) bool {
	var st unix.Stat_t
	if err := unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false
	}
	return (uint32(st.Mode) & unix.S_IFMT) == unix.S_IFLNK
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
