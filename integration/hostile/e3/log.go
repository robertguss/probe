package e3

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

// ProbeLog records every spike step for evidence capture (promotable format).
type ProbeLog struct {
	mu      sync.Mutex
	Entries []ProbeEntry `json:"entries"`
}

// ProbeEntry is one logged probe step.
type ProbeEntry struct {
	Time    string `json:"time"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Probe   string `json:"probe"`
	Step    string `json:"step"`
	Args    string `json:"args,omitempty"`
	Errno   string `json:"errno,omitempty"`
	Outcome string `json:"outcome"` // pass | fail | info | skip
	Detail  string `json:"detail,omitempty"`

	// Race-preflight fields required by E3 notes.
	CGO      string `json:"cgo,omitempty"`
	Compiler string `json:"compiler,omitempty"`
	Skip     string `json:"skip_notice,omitempty"`
}

// NewProbeLog constructs an empty log.
func NewProbeLog() *ProbeLog {
	return &ProbeLog{Entries: make([]ProbeEntry, 0, 64)}
}

// Record appends a probe step and emits a line-oriented stderr record.
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
	fmt.Fprintf(os.Stderr,
		"E3PROBE\tos=%s arch=%s probe=%s step=%s args=%q errno=%s outcome=%s cgo=%s compiler=%q skip=%q detail=%q\n",
		e.OS, e.Arch, e.Probe, e.Step, e.Args, e.Errno, e.Outcome,
		e.CGO, e.Compiler, e.Skip, e.Detail,
	)
}

// JSON returns the full log as indented JSON.
func (l *ProbeLog) JSON() ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return json.MarshalIndent(l, "", "  ")
}

// SummaryPassFail returns counts of pass/fail outcomes (skip/info ignored).
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
