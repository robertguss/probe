//go:build unix

package generate

import (
	"fmt"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"golang.org/x/sys/unix"
)

// dupStageDescriptor returns a CLOEXEC duplicate of the stage directory FD
// for toolrun bound starts. Caller must CloseFD the result.
func dupStageDescriptor(stage *fsx.Stage) (int, error) {
	if stage == nil {
		return -1, diagnostic.New(
			diagnostic.IDInternalBug,
			"dup stage: nil stage",
			diagnostic.Location{},
		)
	}
	fd := stage.DirFD()
	if fd < 0 {
		return -1, diagnostic.New(
			diagnostic.IDInternalBug,
			"dup stage: stage directory descriptor is closed",
			diagnostic.PathLocation(stage.Path()),
		)
	}
	if err := stage.VerifyIdentity(); err != nil {
		return -1, err
	}
	n, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		n, err = unix.Dup(fd)
		if err != nil {
			return -1, diagnostic.Wrap(
				diagnostic.IDInternalBug,
				fmt.Sprintf("duplicate stage handle failed: %v", err),
				diagnostic.PathLocation(stage.Path()),
				err,
			)
		}
		unix.CloseOnExec(n)
	}
	return n, nil
}
