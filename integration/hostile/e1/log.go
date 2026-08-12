package e1

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// ProbeLog records every spike step (OS/FS/kernel, syscall, args, errno, pass/fail).
// Format is stable for promotion into P2.1.e detailed probe logging.
type ProbeLog struct {
	mu      sync.Mutex
	Entries []ProbeEntry `json:"entries"`
}

// ProbeEntry is one logged probe step.
type ProbeEntry struct {
	Time    string `json:"time"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Kernel  string `json:"kernel,omitempty"`
	FS      string `json:"fs,omitempty"`
	Probe   string `json:"probe"`
	Step    string `json:"step"`
	Syscall string `json:"syscall,omitempty"`
	Args    string `json:"args,omitempty"`
	Errno   string `json:"errno,omitempty"`
	Outcome string `json:"outcome"` // pass | fail | info
	Detail  string `json:"detail,omitempty"`
}

// NewProbeLog constructs an empty log.
func NewProbeLog() *ProbeLog {
	return &ProbeLog{Entries: make([]ProbeEntry, 0, 64)}
}

// Record appends a probe step.
func (l *ProbeLog) Record(e ProbeEntry) {
	if e.Time == "" {
		e.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if e.OS == "" {
		e.OS = runtime.GOOS
	}
	if e.Arch == "" {
		e.Arch = runtime.GOARCH
	}
	l.mu.Lock()
	l.Entries = append(l.Entries, e)
	l.mu.Unlock()
	// Also emit line-oriented stderr for evidence capture.
	fmt.Fprintf(os.Stderr, "E1PROBE\tos=%s arch=%s fs=%s probe=%s step=%s syscall=%s args=%q errno=%s outcome=%s detail=%q\n",
		e.OS, e.Arch, e.FS, e.Probe, e.Step, e.Syscall, e.Args, e.Errno, e.Outcome, e.Detail)
}

// JSON returns the full log as JSON bytes.
func (l *ProbeLog) JSON() ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return json.MarshalIndent(l, "", "  ")
}

// SummaryPassFail returns counts of pass/fail outcomes.
func (l *ProbeLog) SummaryPassFail() (pass, fail int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.Entries {
		switch e.Outcome {
		case "pass":
			pass++
		case "fail":
			fail++
		}
	}
	return pass, fail
}
