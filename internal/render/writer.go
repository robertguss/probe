package render

import (
	"fmt"
	"sort"
	"sync"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// Writer is the destination write seam for rendered content (REQ-184).
//
// Phase 1 uses MemoryWriter (pure buffers for plan digests). Phase 2 wires
// the same interface to fsx RootedWriter. Render never opens destination
// paths itself.
type Writer interface {
	// WriteFile records path (destination-relative), mode (e.g. "0644"), and
	// a defensive copy of data. Implementations must not retain the caller's
	// data slice.
	WriteFile(path, mode string, data []byte) error
}

// writtenFile is one MemoryWriter entry.
type writtenFile struct {
	Mode    string
	Content []byte
}

// MemoryWriter is a pure in-memory Writer for Phase 1 plan digests and tests.
// Safe for concurrent WriteFile after construction.
type MemoryWriter struct {
	mu    sync.Mutex
	files map[string]writtenFile
}

// NewMemoryWriter constructs an empty pure buffer writer.
func NewMemoryWriter() *MemoryWriter {
	return &MemoryWriter{files: make(map[string]writtenFile)}
}

// WriteFile stores a defensive copy of data under path with mode.
// Duplicate path writes fail closed (exact-one-owner invariant for buffers).
func (w *MemoryWriter) WriteFile(path, mode string, data []byte) error {
	if w == nil {
		return diagnostic.New(
			diagnostic.IDRenderFailed,
			"memory writer is nil",
			diagnostic.PathLocation(path),
		)
	}
	if path == "" {
		return diagnostic.New(
			diagnostic.IDRenderFailed,
			"write path must be non-empty",
			diagnostic.PathLocation("<empty>"),
		)
	}
	cp := make([]byte, len(data))
	copy(cp, data)

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.files == nil {
		w.files = make(map[string]writtenFile)
	}
	if _, exists := w.files[path]; exists {
		return diagnostic.Newf(
			diagnostic.IDRenderFailed,
			diagnostic.PathLocation(path),
			"duplicate write to path %q",
			path,
		)
	}
	w.files[path] = writtenFile{Mode: mode, Content: cp}
	return nil
}

// Get returns a copy of content and mode for path, or false if missing.
func (w *MemoryWriter) Get(path string) (content []byte, mode string, ok bool) {
	if w == nil {
		return nil, "", false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	f, ok := w.files[path]
	if !ok {
		return nil, "", false
	}
	cp := make([]byte, len(f.Content))
	copy(cp, f.Content)
	return cp, f.Mode, true
}

// Paths returns sorted written paths.
func (w *MemoryWriter) Paths() []string {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.files))
	for p := range w.files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Len returns the number of written files.
func (w *MemoryWriter) Len() int {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.files)
}

// String returns a short debug summary.
func (w *MemoryWriter) String() string {
	if w == nil {
		return "MemoryWriter(nil)"
	}
	return fmt.Sprintf("MemoryWriter{files=%d}", w.Len())
}
