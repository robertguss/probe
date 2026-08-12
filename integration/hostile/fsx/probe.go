//go:build hostile && unix

package hostilefsx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/fsx"
)

// ProbeLog records every hostile-suite step (OS/FS/kernel, syscall, args,
// errno, pass/fail, stage identity). Format is the promoted E1 ProbeLog shape
// (integration/hostile/e1/log.go) for stable evidence capture.
type ProbeLog struct {
	mu      sync.Mutex
	Entries []ProbeEntry `json:"entries"`
}

// ProbeEntry is one logged probe step.
type ProbeEntry struct {
	Time          string `json:"time"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	Kernel        string `json:"kernel,omitempty"`
	FS            string `json:"fs,omitempty"`
	Probe         string `json:"probe"`
	Step          string `json:"step"`
	Syscall       string `json:"syscall,omitempty"`
	Args          string `json:"args,omitempty"`
	Errno         string `json:"errno,omitempty"`
	Outcome       string `json:"outcome"` // pass | fail | info
	Detail        string `json:"detail,omitempty"`
	StageName     string `json:"stage_name,omitempty"`
	StageIdentity string `json:"stage_identity,omitempty"`
	StagePath     string `json:"stage_path,omitempty"`
}

// NewProbeLog constructs an empty log.
func NewProbeLog() *ProbeLog {
	return &ProbeLog{Entries: make([]ProbeEntry, 0, 64)}
}

// Record appends a probe step and emits a line-oriented record for CI capture.
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
		"HOSTILEFSX\tos=%s arch=%s fs=%s kernel=%s probe=%s step=%s syscall=%s args=%q errno=%s outcome=%s stage=%s id=%s detail=%q\n",
		e.OS, e.Arch, e.FS, e.Kernel, e.Probe, e.Step, e.Syscall, e.Args, e.Errno, e.Outcome,
		e.StageName, e.StageIdentity, e.Detail)
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

// DumpArtifact writes the probe log JSON to FOUNDRY_HOSTILE_FSX_ARTIFACT_DIR
// (or dir when non-empty). Returns the path written, or empty if disabled.
func (l *ProbeLog) DumpArtifact(name string) (string, error) {
	dir := os.Getenv("FOUNDRY_HOSTILE_FSX_ARTIFACT_DIR")
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Sanitize name to basename only — no host paths in artifact filenames.
	base := filepath.Base(name)
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, base)
	if base == "" || base == "." {
		base = "probe"
	}
	path := filepath.Join(dir, base+".json")
	data, err := l.JSON()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// probeBridge adapts ProbeLog (+ optional step hook) to fsx.StepLogger so
// production steps appear in the detailed probe dump. The optional step hook
// is filled from *_test.go (testutil) so this non-test file never imports
// internal/testutil (archtest testutil_import rule).
type probeBridge struct {
	pl     *ProbeLog
	step   func(name, outcome, detail string) // outcome: pass|fail|info|ok
	fs     string
	kernel string
}

func (b *probeBridge) FSXStep(probe, step, outcome, detail string) {
	if b.step != nil {
		b.step(probe+"."+step, outcome, detail)
	}
	if b.pl != nil {
		b.pl.Record(ProbeEntry{
			FS:        b.fs,
			Kernel:    b.kernel,
			Probe:     probe,
			Step:      step,
			Outcome:   outcome,
			Detail:    detail,
			Syscall:   extractField(detail, "syscall="),
			Errno:     extractField(detail, "errno="),
			Args:      extractField(detail, "stage=") + "->" + extractField(detail, "dest="),
			StageName: extractQuoted(detail, "stage="),
			StagePath: extractQuoted(detail, "stage_path="),
		})
	}
}

func extractField(detail, key string) string {
	i := strings.Index(detail, key)
	if i < 0 {
		return ""
	}
	rest := detail[i+len(key):]
	// Stop at space.
	if j := strings.IndexByte(rest, ' '); j >= 0 {
		rest = rest[:j]
	}
	return strings.Trim(rest, `"'`)
}

func extractQuoted(detail, key string) string {
	i := strings.Index(detail, key)
	if i < 0 {
		return ""
	}
	rest := detail[i+len(key):]
	if len(rest) == 0 {
		return ""
	}
	if rest[0] == '"' {
		rest = rest[1:]
		if j := strings.IndexByte(rest, '"'); j >= 0 {
			return rest[:j]
		}
	}
	if j := strings.IndexByte(rest, ' '); j >= 0 {
		rest = rest[:j]
	}
	return strings.Trim(rest, `"'`)
}

// ensure probeBridge implements fsx.StepLogger at compile time.
var _ fsx.StepLogger = (*probeBridge)(nil)
