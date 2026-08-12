//go:build !unix

package fsx

// MaxStageCreateAttempts is the bounded EEXIST retry budget (Section 31.5).
const MaxStageCreateAttempts = 16

// Default file/directory modes from the plan inventory / Section 15.6.
const (
	DefaultFileMode = "0644"
	DefaultDirMode  = "0755"
)

// Stage is a stub on non-unix platforms (Foundry supports macOS+Linux only).
type Stage struct {
	name   string
	id     FileID
	parent *ParentHandle
	log    StepLogger
}

// Name implements the unix API surface.
func (s *Stage) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Path implements the unix API surface (diagnostic stage location).
func (s *Stage) Path() string {
	if s == nil {
		return ""
	}
	parentPath := ""
	if s.parent != nil {
		parentPath = s.parent.AuthoredPath()
	}
	return StageLocation(parentPath, s.name)
}

// Identity implements the unix API surface.
func (s *Stage) Identity() FileID {
	if s == nil {
		return FileID{}
	}
	return s.id
}

// DirFD implements the unix API surface.
func (s *Stage) DirFD() int { return -1 }

// Parent implements the unix API surface.
func (s *Stage) Parent() *ParentHandle {
	if s == nil {
		return nil
	}
	return s.parent
}

// Close implements the unix API surface (no deletion).
func (s *Stage) Close() error { return nil }

// Writer implements the unix API surface.
func (s *Stage) Writer() *RootedWriter {
	if s == nil {
		return nil
	}
	return &RootedWriter{stage: s, log: s.log}
}

// VerifyIdentity is unsupported off unix.
func (s *Stage) VerifyIdentity() error { return unsupported("VerifyIdentity") }

// CreateStage is unsupported off unix.
func CreateStage(parent *ParentHandle, project string) (*Stage, error) {
	return nil, unsupported("CreateStage")
}

// RootedWriter is a stub on non-unix platforms.
type RootedWriter struct {
	stage *Stage
	log   StepLogger
}

// WriteFile is unsupported off unix.
func (w *RootedWriter) WriteFile(relPath, mode string, data []byte) error {
	return unsupported("RootedWriter.WriteFile")
}

// Mkdir is unsupported off unix.
func (w *RootedWriter) Mkdir(relPath, mode string) error {
	return unsupported("RootedWriter.Mkdir")
}

// MkdirAll is unsupported off unix.
func (w *RootedWriter) MkdirAll(relPath, mode string) error {
	return unsupported("RootedWriter.MkdirAll")
}
