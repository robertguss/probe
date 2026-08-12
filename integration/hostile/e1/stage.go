//go:build unix

package e1

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Stage is a handle-relative staging directory (Section 31.5). No deletion API.
type Stage struct {
	Name   string // basename under parent: .foundry-<name>-<random>
	ID     FileID
	FD     int
	Root   *os.Root // SV-01 descriptor-relative writer
	Parent *ParentHandle
	log    *ProbeLog
	fs     string
}

// Close releases the stage FD and Root. Does NOT unlink the stage (31.6).
func (s *Stage) Close() error {
	var first error
	if s.Root != nil {
		if err := s.Root.Close(); err != nil && first == nil {
			first = err
		}
		s.Root = nil
	}
	if s.FD >= 0 {
		if err := unix.Close(s.FD); err != nil && first == nil {
			first = err
		}
		s.FD = -1
	}
	return first
}

// CreateStage creates `.foundry-<project>-<random>` relative to parent via mkdirat,
// mode 0700, retrying EEXIST up to 16 times (Section 31.5).
func CreateStage(parent *ParentHandle, project string, log *ProbeLog, fsLabel string) (*Stage, error) {
	if log == nil {
		log = NewProbeLog()
	}
	const maxAttempts = 16
	for attempt := 0; attempt < maxAttempts; attempt++ {
		suffix, err := randomHex(8)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf(".foundry-%s-%s", project, suffix)
		err = unix.Mkdirat(parent.FD, name, 0700)
		if err != nil {
			if err == unix.EEXIST {
				log.Record(ProbeEntry{FS: fsLabel, Probe: "stage_create", Step: "mkdirat_retry",
					Syscall: "mkdirat", Args: fmt.Sprintf("name=%s mode=0700", name),
					Errno: errnoString(err), Outcome: "info", Detail: fmt.Sprintf("attempt=%d", attempt)})
				continue
			}
			log.Record(ProbeEntry{FS: fsLabel, Probe: "stage_create", Step: "mkdirat",
				Syscall: "mkdirat", Args: fmt.Sprintf("name=%s mode=0700", name),
				Errno: errnoString(err), Outcome: "fail"})
			return nil, spikeErr(ErrStageCreateFailed, name, err)
		}

		// Open stage with O_NOFOLLOW|O_DIRECTORY relative to parent.
		fd, err := unix.Openat(parent.FD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			log.Record(ProbeEntry{FS: fsLabel, Probe: "stage_create", Step: "openat_stage",
				Syscall: "openat", Args: name, Errno: errnoString(err), Outcome: "fail"})
			return nil, spikeErr(ErrStageCreateFailed, "open stage", err)
		}
		id, err := fileIDFromFD(fd)
		if err != nil {
			_ = unix.Close(fd)
			return nil, err
		}

		// Enforce mode 0700 via fchmod on the open descriptor (umask-safe).
		if err := unix.Fchmod(fd, 0700); err != nil {
			// Non-fatal on some FS (e.g. FAT ignores); log and continue when not supported.
			log.Record(ProbeEntry{FS: fsLabel, Probe: "stage_create", Step: "fchmod",
				Syscall: "fchmod", Args: "0700", Errno: errnoString(err), Outcome: "info"})
		}

		root, err := OpenRootFromFD(fd)
		if err != nil {
			_ = unix.Close(fd)
			log.Record(ProbeEntry{FS: fsLabel, Probe: "stage_create", Step: "os.Root",
				Syscall: "OpenRoot", Errno: errnoString(err), Outcome: "fail"})
			return nil, err
		}

		// Re-lookup relative to parent and verify identity (31.5).
		exists, lookupID, err := parent.ChildLookup(name)
		if err != nil || !exists || !lookupID.Equal(id) {
			_ = root.Close()
			_ = unix.Close(fd)
			log.Record(ProbeEntry{FS: fsLabel, Probe: "stage_create", Step: "identity_verify",
				Outcome: "fail", Detail: fmt.Sprintf("exists=%v lookup=%s created=%s err=%v", exists, lookupID, id, err)})
			return nil, spikeErr(ErrStageCreateFailed, "identity mismatch after create", err)
		}

		log.Record(ProbeEntry{FS: fsLabel, Probe: "stage_create", Step: "created",
			Syscall: "mkdirat+openat", Args: name, Outcome: "pass", Detail: id.String()})

		return &Stage{Name: name, ID: id, FD: fd, Root: root, Parent: parent, log: log, fs: fsLabel}, nil
	}
	return nil, spikeErr(ErrStageCreateFailed, "exhausted EEXIST retries", nil)
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// WriteFile writes through the stage's os.Root (SV-01). Absolute paths and ".." rejected by Root.
func (s *Stage) WriteFile(name string, data []byte, perm os.FileMode) error {
	f, err := s.Root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
