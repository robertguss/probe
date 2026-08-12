//go:build unix

package e4

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// TermiosSnapshot captures local flags used to prove raw-mode enter/restore.
type TermiosSnapshot struct {
	IFlag uint32
	OFlag uint32
	CFlag uint32
	LFlag uint32
}

// ICANON and ECHO presence indicate cooked (restored) mode on typical Unix.
func (s TermiosSnapshot) Cooked() bool {
	return s.LFlag&uint32(unix.ICANON) != 0 && s.LFlag&uint32(unix.ECHO) != 0
}

func (s TermiosSnapshot) String() string {
	return fmt.Sprintf("iflag=%#x oflag=%#x cflag=%#x lflag=%#x cooked=%v",
		s.IFlag, s.OFlag, s.CFlag, s.LFlag, s.Cooked())
}

// SnapshotTermios reads termios for fd (Linux TCGETS / BSD TIOCGETA).
func SnapshotTermios(fd int) (TermiosSnapshot, error) {
	t, err := unix.IoctlGetTermios(fd, termiosGet)
	if err != nil {
		return TermiosSnapshot{}, err
	}
	return TermiosSnapshot{
		IFlag: uint32(t.Iflag),
		OFlag: uint32(t.Oflag),
		CFlag: uint32(t.Cflag),
		LFlag: uint32(t.Lflag),
	}, nil
}

// Equal reports whether two snapshots match exactly.
func (s TermiosSnapshot) Equal(o TermiosSnapshot) bool {
	return s.IFlag == o.IFlag && s.OFlag == o.OFlag &&
		s.CFlag == o.CFlag && s.LFlag == o.LFlag
}
