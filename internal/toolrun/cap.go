package toolrun

import (
	"bytes"
	"io"
	"sync"

	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// DefaultOutputCapBytes is the per-stream capture cap (Section 34.3 / REQ-154).
// Equal to plan.OutputCapBytes (4 MiB).
const DefaultOutputCapBytes = plan.OutputCapBytes

// CapBytes truncates b to at most max bytes. truncated is true when the input
// exceeded max. max <= 0 uses DefaultOutputCapBytes.
//
// Truncation is explicit (never silent): callers must record the truncated flag
// on StepResult and surface it on failure replay (Section 34.3).
func CapBytes(b []byte, max int) (out []byte, truncated bool) {
	if max <= 0 {
		max = DefaultOutputCapBytes
	}
	if len(b) <= max {
		// Return a copy so callers cannot mutate shared buffers accidentally.
		out = make([]byte, len(b))
		copy(out, b)
		return out, false
	}
	out = make([]byte, max)
	copy(out, b[:max])
	return out, true
}

// cappedWriter is an io.Writer that retains at most max bytes and records
// whether more data was offered (truncation). Concurrent Write is safe.
type cappedWriter struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
	// total is the number of bytes offered (including discarded).
	total int64
}

func newCappedWriter(max int) *cappedWriter {
	if max <= 0 {
		max = DefaultOutputCapBytes
	}
	return &cappedWriter{max: max}
}

// Write implements io.Writer.
func (w *cappedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.total += int64(len(p))
	remain := w.max - w.buf.Len()
	if remain <= 0 {
		w.truncated = true
		return len(p), nil // discard; pretend full write so pipe does not block forever
	}
	if len(p) > remain {
		_, _ = w.buf.Write(p[:remain])
		w.truncated = true
		return len(p), nil
	}
	_, err := w.buf.Write(p)
	return len(p), err
}

// Bytes returns a copy of retained bytes and whether truncation occurred.
func (w *cappedWriter) Bytes() (b []byte, truncated bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	b = make([]byte, w.buf.Len())
	copy(b, w.buf.Bytes())
	return b, w.truncated
}

// TotalOffered returns how many bytes were written (including discarded).
func (w *cappedWriter) TotalOffered() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.total
}

// MultiWriter that also implements the cap: used when tests need Tee.
var _ io.Writer = (*cappedWriter)(nil)
