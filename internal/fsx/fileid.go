package fsx

import "fmt"

// FileID is a device/inode identity used for object continuity (SV-03).
// Equality means "same filesystem object," independent of pathname.
type FileID struct {
	Dev uint64
	Ino uint64
}

// String returns a stable diagnostic form (no host paths).
func (id FileID) String() string {
	return fmt.Sprintf("dev=%d ino=%d", id.Dev, id.Ino)
}

// Equal reports whether two identities name the same object.
func (id FileID) Equal(other FileID) bool {
	return id.Dev == other.Dev && id.Ino == other.Ino
}

// IsZero reports whether the identity is unset.
func (id FileID) IsZero() bool {
	return id.Dev == 0 && id.Ino == 0
}
