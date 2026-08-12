package e2

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
//
// Leak is the isolation observation for sentinel probes:
//
//	false = no leakage (pass)
//	true  = leakage observed (fail)
//
// Key names only — never secret-bearing values (REQ-214 redaction).
type ProbeEntry struct {
	Time    string `json:"time"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Probe   string `json:"probe"`
	Step    string `json:"step"`
	Key     string `json:"key,omitempty"`  // sentinel / env key name
	Leak    *bool  `json:"leak,omitempty"` // observed leakage boolean
	Args    string `json:"args,omitempty"` // argv summary (no secrets)
	Errno   string `json:"errno,omitempty"`
	Outcome string `json:"outcome"` // pass | fail | info | skip
	Detail  string `json:"detail,omitempty"`
}

// NewProbeLog constructs an empty log.
func NewProbeLog() *ProbeLog {
	return &ProbeLog{Entries: make([]ProbeEntry, 0, 128)}
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

	leakStr := ""
	if e.Leak != nil {
		if *e.Leak {
			leakStr = "true"
		} else {
			leakStr = "false"
		}
	}
	fmt.Fprintf(os.Stderr,
		"E2PROBE\tos=%s arch=%s probe=%s step=%s key=%s leak=%s args=%q errno=%s outcome=%s detail=%q\n",
		e.OS, e.Arch, e.Probe, e.Step, e.Key, leakStr, e.Args, e.Errno, e.Outcome, e.Detail,
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

// BoolPtr returns a *bool for Leak fields.
func BoolPtr(v bool) *bool { return &v }
