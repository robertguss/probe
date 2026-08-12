//go:build !unix

package fsx

import "fmt"

// ParentHandle is a stub on non-unix platforms (Foundry supports macOS+Linux only).
type ParentHandle struct {
	authored string
	base     string
	id       FileID
	log      StepLogger
}

// PreflightOptions configures parent acquisition and destination preflight.
type PreflightOptions struct {
	Log StepLogger
}

func unsupported(op string) error {
	return errUnsafePath(op, fmt.Sprintf("fsx.%s is only supported on macOS and Linux (unix)", op))
}

// AuthoredPath implements the unix API surface.
func (p *ParentHandle) AuthoredPath() string {
	if p == nil {
		return ""
	}
	return p.authored
}

// Basename implements the unix API surface.
func (p *ParentHandle) Basename() string {
	if p == nil {
		return ""
	}
	return p.base
}

// Identity implements the unix API surface.
func (p *ParentHandle) Identity() FileID {
	if p == nil {
		return FileID{}
	}
	return p.id
}

// DirFD implements the unix API surface.
func (p *ParentHandle) DirFD() int { return -1 }

// Close implements the unix API surface.
func (p *ParentHandle) Close() error { return nil }

// Preflight is unsupported off unix.
func Preflight(destination string, opts PreflightOptions) (*ParentHandle, error) {
	return nil, unsupported("Preflight")
}

// AcquireParent is unsupported off unix.
func AcquireParent(destination string, opts PreflightOptions) (*ParentHandle, error) {
	return nil, unsupported("AcquireParent")
}

// ChildExists is unsupported off unix.
func (p *ParentHandle) ChildExists(name string) (bool, error) {
	return false, unsupported("ChildExists")
}

// ChildLookup is unsupported off unix.
func (p *ParentHandle) ChildLookup(name string) (bool, FileID, error) {
	return false, FileID{}, unsupported("ChildLookup")
}

// ClassifyDestination is unsupported off unix.
func (p *ParentHandle) ClassifyDestination() (DestinationClass, error) {
	return "", unsupported("ClassifyDestination")
}

// EnsureDestinationAbsent is unsupported off unix.
func (p *ParentHandle) EnsureDestinationAbsent() error {
	return unsupported("EnsureDestinationAbsent")
}

// Reobserve is unsupported off unix.
func (p *ParentHandle) Reobserve() error {
	return unsupported("Reobserve")
}
