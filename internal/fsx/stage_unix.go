//go:build unix

package fsx

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"golang.org/x/sys/unix"
)

// MaxStageCreateAttempts is the bounded EEXIST retry budget (Section 31.5).
const MaxStageCreateAttempts = 16

// stageMode is the exclusive staging directory mode (0700; Section 15.6 / 31.5).
const stageMode = 0o700

// stageSuffix generates the random hex suffix for stage basenames.
// Tests may replace this to force EEXIST retries / exhaust (REQ-125).
var stageSuffix = func() (string, error) { return randomHex(8) }

// Stage is a handle-relative staging directory created under a retained parent
// (Section 31.5). It records device/inode identity and owns a rooted writer.
//
// Close releases descriptors only. There is no deletion method (Section 31.6 /
// REQ-130/184) — stages are always preserved for manual inspection.
type Stage struct {
	name   string // basename under parent: .foundry-<name>-<random>
	id     FileID
	fd     int
	root   *os.Root
	parent *ParentHandle
	log    StepLogger
}

// Name returns the stage basename under the parent (not a full path).
func (s *Stage) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Path returns the diagnostic stage location: authored parent path joined with
// the stage basename (Section 31.6). For reporting only — never use for
// pathname-based mutation of the destination namespace.
func (s *Stage) Path() string {
	if s == nil {
		return ""
	}
	parentPath := ""
	if s.parent != nil {
		parentPath = s.parent.authored
	}
	return StageLocation(parentPath, s.name)
}

// Identity returns the recorded stage device/inode (SV-03 continuity).
func (s *Stage) Identity() FileID {
	if s == nil {
		return FileID{}
	}
	return s.id
}

// DirFD returns the retained stage directory descriptor. Callers outside fsx
// must not use this for pathname-based mutation of the destination namespace.
func (s *Stage) DirFD() int {
	if s == nil {
		return -1
	}
	return s.fd
}

// Parent returns the retained parent handle used to create this stage.
func (s *Stage) Parent() *ParentHandle {
	if s == nil {
		return nil
	}
	return s.parent
}

// Close closes the stage Root and directory descriptor. It does NOT unlink or
// remove the stage entry (Section 31.6).
func (s *Stage) Close() error {
	if s == nil {
		return nil
	}
	var first error
	if s.root != nil {
		if err := s.root.Close(); err != nil && first == nil {
			first = err
		}
		s.root = nil
	}
	if s.fd >= 0 {
		if err := unix.Close(s.fd); err != nil && first == nil {
			first = err
		}
		s.fd = -1
	}
	return first
}

// Writer returns a RootedWriter bound to this stage. All writes/mkdirs go
// through the descriptor-relative os.Root (SV-01). The writer never exposes
// a raw host path for mutation.
func (s *Stage) Writer() *RootedWriter {
	if s == nil {
		return nil
	}
	return &RootedWriter{stage: s, log: s.log}
}

// CreateStage creates `.foundry-<project>-<random>` relative to the retained
// parent handle via mkdirat, mode 0700, retrying EEXIST up to
// MaxStageCreateAttempts times (Section 31.5 / REQ-125).
//
// On success the stage identity is recorded, the directory is opened with
// O_NOFOLLOW|O_DIRECTORY, fchmod 0700 is applied (umask-safe), and an os.Root
// is bound to the stage object. Identity is re-verified via parent lookup.
//
// The caller must Close the stage. The stage is never deleted by this package.
func CreateStage(parent *ParentHandle, project string) (*Stage, error) {
	if parent == nil || parent.fd < 0 {
		return nil, errUnsafePath("<closed>", "parent handle is closed")
	}
	project = strings.TrimSpace(project)
	if project == "" {
		project = parent.base
	}
	if err := validateStageProjectName(project); err != nil {
		return nil, err
	}

	log := parent.log
	logStep(log, "stage_create", "begin", "info",
		fmt.Sprintf("parent_identity=%s project=%q max_attempts=%d", parent.id, project, MaxStageCreateAttempts))

	var firstErrno string
	for attempt := 1; attempt <= MaxStageCreateAttempts; attempt++ {
		suffix, err := stageSuffix()
		if err != nil {
			return nil, wrapCause(
				errStageCreate(project, "cannot generate stage name suffix"),
				err,
			)
		}
		name := fmt.Sprintf(".foundry-%s-%s", project, suffix)

		err = unix.Mkdirat(parent.fd, name, stageMode)
		if err != nil {
			errno := errnoString(err)
			if firstErrno == "" {
				firstErrno = errno
			}
			if errorsIsExist(err) {
				logStep(log, "stage_create", "mkdirat_retry", "info",
					fmt.Sprintf("basename=%q attempt=%d/%d errno=%s", name, attempt, MaxStageCreateAttempts, errno))
				continue
			}
			logStep(log, "stage_create", "mkdirat", "fail",
				fmt.Sprintf("basename=%q attempt=%d errno=%s", name, attempt, errno))
			return nil, wrapCause(
				errStageCreate(name, fmt.Sprintf("mkdirat stage failed (errno=%s)", errno)),
				err,
			)
		}

		// Open stage with O_NOFOLLOW|O_DIRECTORY relative to parent.
		fd, err := unix.Openat(parent.fd, name,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			logStep(log, "stage_create", "openat_stage", "fail",
				fmt.Sprintf("basename=%q errno=%s", name, errnoString(err)))
			return nil, wrapCause(
				errStageCreate(name, "openat stage after mkdirat failed"),
				err,
			)
		}

		id, err := fileIDFromFD(fd)
		if err != nil {
			_ = unix.Close(fd)
			return nil, wrapCause(
				errStageCreate(name, "cannot fstat created stage"),
				err,
			)
		}

		// Enforce mode 0700 via fchmod on the open descriptor (umask-safe).
		if err := unix.Fchmod(fd, stageMode); err != nil {
			// Log but do not fail hard: some exotic FS ignore mode; identity
			// and exclusive create still hold. Final mode is re-checked in tests
			// on real Unix FS.
			logStep(log, "stage_create", "fchmod", "info",
				fmt.Sprintf("basename=%q mode=%04o errno=%s", name, stageMode, errnoString(err)))
		} else {
			logStep(log, "stage_create", "fchmod", "pass",
				fmt.Sprintf("basename=%q mode=%04o", name, stageMode))
		}

		root, err := openRootFromFD(fd)
		if err != nil {
			_ = unix.Close(fd)
			logStep(log, "stage_create", "os_root", "fail",
				fmt.Sprintf("basename=%q errno=%s", name, errnoString(err)))
			return nil, wrapCause(
				errStageCreate(name, "cannot open os.Root on stage descriptor"),
				err,
			)
		}

		// Re-lookup relative to parent and verify identity (31.5).
		exists, lookupID, err := parent.ChildLookup(name)
		if err != nil || !exists || !lookupID.Equal(id) {
			_ = root.Close()
			_ = unix.Close(fd)
			detail := fmt.Sprintf("exists=%v lookup=%s created=%s err=%v", exists, lookupID, id, err)
			logStep(log, "stage_create", "identity_verify", "fail", detail)
			return nil, errStageCreate(name,
				fmt.Sprintf("stage identity mismatch after create: %s", detail))
		}

		logStep(log, "stage_create", "created", "pass",
			fmt.Sprintf("basename=%q attempts=%d identity=%s mode=%04o parent_identity=%s",
				name, attempt, id, stageMode, parent.id))

		return &Stage{
			name:   name,
			id:     id,
			fd:     fd,
			root:   root,
			parent: parent,
			log:    log,
		}, nil
	}

	logStep(log, "stage_create", "exhausted", "fail",
		fmt.Sprintf("project=%q max_attempts=%d first_errno=%s id=%s",
			project, MaxStageCreateAttempts, firstErrno, diagnostic.IDFSCommitFailed))
	return nil, errStageCreate(project,
		fmt.Sprintf("exhausted %d EEXIST retries creating stage (first_errno=%s)",
			MaxStageCreateAttempts, firstErrno))
}

// VerifyIdentity re-looks up the stage basename relative to the parent and
// compares device/inode with the retained stage handle (Section 31.5). Call
// before writes and any time object continuity must be confirmed.
func (s *Stage) VerifyIdentity() error {
	if s == nil || s.fd < 0 {
		return errUnsafePath("<closed>", "stage handle is closed")
	}
	if s.parent == nil || s.parent.fd < 0 {
		return errUnsafePath(s.name, "parent handle is closed")
	}

	// Identity of the retained stage descriptor (object continuity of the handle).
	fdID, err := fileIDFromFD(s.fd)
	if err != nil {
		return wrapCause(errStageCreate(s.name, "cannot fstat stage handle"), err)
	}
	if !fdID.Equal(s.id) {
		logStep(s.log, "stage_identity", "handle", "fail",
			fmt.Sprintf("basename=%q recorded=%s handle=%s", s.name, s.id, fdID))
		return errStageIdentity(s.name,
			fmt.Sprintf("stage handle identity changed: recorded %s vs handle %s", s.id, fdID))
	}

	exists, lookupID, err := s.parent.ChildLookup(s.name)
	if err != nil {
		logStep(s.log, "stage_identity", "lookup", "fail",
			fmt.Sprintf("basename=%q errno=%s", s.name, errnoString(err)))
		return wrapCause(errStageIdentity(s.name, "cannot re-lookup stage under parent"), err)
	}
	if !exists {
		logStep(s.log, "stage_identity", "lookup", "fail",
			fmt.Sprintf("basename=%q missing under parent recorded=%s", s.name, s.id))
		return errStageIdentity(s.name, "stage basename no longer present under parent")
	}
	if !lookupID.Equal(s.id) {
		logStep(s.log, "stage_identity", "mismatch", "fail",
			fmt.Sprintf("basename=%q recorded=%s lookup=%s", s.name, s.id, lookupID))
		return errStageIdentity(s.name,
			fmt.Sprintf("stage identity changed under parent: recorded %s vs lookup %s", s.id, lookupID))
	}
	logStep(s.log, "stage_identity", "ok", "pass",
		fmt.Sprintf("basename=%q identity=%s", s.name, s.id))
	return nil
}

func validateStageProjectName(project string) error {
	if project == "" {
		return errUnsafePath("<empty>", "stage project name must be non-empty")
	}
	if strings.Contains(project, "\x00") {
		return errUnsafePath(project, "stage project name must not contain NUL")
	}
	if strings.Contains(project, string(filepath.Separator)) || strings.Contains(project, "/") || strings.Contains(project, "\\") {
		return errUnsafePath(project, "stage project name must be a single path component")
	}
	if project == "." || project == ".." {
		return errUnsafePath(project, "stage project name must not be . or ..")
	}
	if strings.Contains(project, string(filepath.ListSeparator)) {
		return errUnsafePath(project, "stage project name contains invalid separator")
	}
	return nil
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func errorsIsExist(err error) bool {
	return errors.Is(err, unix.EEXIST) || errors.Is(err, syscall.EEXIST) || os.IsExist(err)
}

// openRootFromFD opens an os.Root on the directory referenced by fd via the
// process fd table (/dev/fd or /proc/self/fd). Object continuity is preserved
// (SV-01 + SV-03); the path is only a handle to an already-open descriptor.
func openRootFromFD(fd int) (*os.Root, error) {
	path := fmt.Sprintf("/dev/fd/%d", fd)
	if _, err := os.Stat(path); err != nil {
		path = fmt.Sprintf("/proc/self/fd/%d", fd)
	}
	return os.OpenRoot(path)
}
