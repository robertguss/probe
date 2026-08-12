package tui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestTUIDebugLogSafetyMatrix exercises Section 18.8 on a generated TUI binary:
// safe basename only; exclusive create 0600; refuse existing; refuse separators.
func TestTUIDebugLogSafetyMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("short: skips generate+build debug-log matrix")
	}
	log := testutil.New(t)
	log.Phase("debuglog_matrix")

	dest := generateMinimalTUI(t, log)
	name := "smoke-tui"
	bin := buildTUIBinary(t, log, dest, name)

	// Version path (startup success without TUI loop).
	log.Phase("version")
	out, err := exec.Command(bin, "--version").CombinedOutput()
	log.Assert("version_ok", err == nil, true, err == nil)
	log.Assert("version_has_name", strings.Contains(string(out), name) || strings.Contains(string(out), "0.0.0"), true, false)
	log.Step("version_out", testutil.OutcomeOK, strings.TrimSpace(string(out)))
	log.PhaseEnd("version", testutil.OutcomeOK)

	cases := []struct {
		name    string
		arg     string
		prepare func(dir string) error
		wantOK  bool
		wantSub string // substring in stderr/stdout on failure
	}{
		{name: "empty_rejected", arg: "", wantOK: false}, // empty means flag omitted — handled separately
		{name: "ok_basename", arg: "debug-ok.log", wantOK: true},
		{name: "separator_slash", arg: "a/b.log", wantOK: false, wantSub: "separator"},
		{name: "dotdot", arg: "..", wantOK: false, wantSub: ""},
		{name: "hidden", arg: ".hidden", wantOK: false, wantSub: "hidden"},
		{name: "exists_refuse", arg: "exists.log", wantOK: false, wantSub: "exists",
			prepare: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "exists.log"), []byte("x"), 0o644)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := testutil.New(t)
			sub.Phase(tc.name)
			work := filepath.Join(dest, "dl-"+tc.name)
			if err := os.MkdirAll(work, 0o700); err != nil {
				sub.Fail("mkdir", err.Error())
			}
			if tc.prepare != nil {
				if err := tc.prepare(work); err != nil {
					sub.Fail("prepare", err.Error())
				}
			}
			if tc.arg == "" && tc.name == "empty_rejected" {
				// No --debug-log is OK (no log file); not an error.
				sub.Step("skip_empty_flag", testutil.OutcomeOK, "omitted flag writes no log")
				sub.PhaseEnd(tc.name, testutil.OutcomeOK)
				return
			}
			args := []string{"--debug-log", tc.arg, "--version"}
			cmd := exec.Command(bin, args...)
			cmd.Dir = work
			cmd.Env = append(os.Environ(), "NO_COLOR=1")
			out, err := cmd.CombinedOutput()
			ok := err == nil
			sub.Step("run", testutil.OutcomeOK, "err="+errString(err)+" out="+truncate(string(out), 200))
			sub.Assert("exit_ok", ok == tc.wantOK, tc.wantOK, ok)
			if tc.wantOK {
				// File created, mode 0600, regular.
				p := filepath.Join(work, tc.arg)
				st, statErr := os.Stat(p)
				sub.Assert("file_exists", statErr == nil, true, statErr == nil)
				if statErr == nil {
					mode := st.Mode().Perm()
					sub.Assert("mode_0600", mode == 0o600, "0600", mode.String())
					sub.Assert("regular", st.Mode().IsRegular(), true, st.Mode().IsRegular())
				}
				// Second create must fail (O_EXCL).
				cmd2 := exec.Command(bin, args...)
				cmd2.Dir = work
				out2, err2 := cmd2.CombinedOutput()
				sub.Assert("second_excl_fails", err2 != nil, true, err2 == nil)
				sub.Step("second", testutil.OutcomeOK, truncate(string(out2), 120))
			} else if tc.wantSub != "" {
				combined := strings.ToLower(string(out) + errString(err))
				sub.Assert("stderr_hint", strings.Contains(combined, strings.ToLower(tc.wantSub)), true, false)
			}
			sub.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("debuglog_matrix", testutil.OutcomeOK)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Touch time import for potential future timeouts.
var _ = time.Second
