package e4

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
	Outcome string `json:"outcome"` // pass | fail | info
	Detail  string `json:"detail,omitempty"`

	// Lifecycle-specific fields required by E4 notes.
	ActiveEffects int    `json:"active_effects,omitempty"`
	SignalOwner   string `json:"signal_owner,omitempty"`
	PTYRestore    string `json:"pty_restore,omitempty"`
	ExitCode      int    `json:"exit_code,omitempty"`
	QuitClass     string `json:"quit_class,omitempty"`
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
		"E4PROBE\tos=%s arch=%s probe=%s step=%s args=%q errno=%s outcome=%s active_effects=%d signal_owner=%q pty_restore=%q exit_code=%d quit_class=%q detail=%q\n",
		e.OS, e.Arch, e.Probe, e.Step, e.Args, e.Errno, e.Outcome,
		e.ActiveEffects, e.SignalOwner, e.PTYRestore, e.ExitCode, e.QuitClass, e.Detail,
	)
}

// JSON returns the full log as indented JSON.
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
