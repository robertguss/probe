//go:build unix

package fsx_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"golang.org/x/sys/unix"
)

var stageNamePattern = regexp.MustCompile(`^\.foundry-[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}-[0-9a-f]{16}$`)

func arrangeParent(t *testing.T, log fsx.StepLogger, label string) (*fsx.ParentHandle, string) {
	t.Helper()
	root := t.TempDir()
	// Resolve symlinks so tests compare the real path on Darwin where
	// /var → /private/var (matching rewriteDarwinDirAliases in production).
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	parentDir := privateParent(t, root, label)
	dest := filepath.Join(parentDir, "proj")
	parent, err := fsx.Preflight(dest, fsx.PreflightOptions{Log: log})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	return parent, parentDir
}

func TestCreateStage_NamePatternAndMode0700(t *testing.T) {
	log, tl, ml := newBridge(t)
	tl.Phase("arrange")
	parent, parentDir := arrangeParent(t, log, "stage-name")
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		tl.Fail("create", err.Error())
	}
	t.Cleanup(func() { _ = stage.Close() })
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	name := stage.Name()
	tl.Assert("name_pattern", stageNamePattern.MatchString(name), true, name)
	tl.Assert("name_prefix", strings.HasPrefix(name, ".foundry-proj-"), true, name)
	tl.Assert("identity_nonzero", !stage.Identity().IsZero(), true, stage.Identity())
	tl.Assert("dirfd_open", stage.DirFD() >= 0, ">=0", stage.DirFD())

	// Mode 0700 on the directory entry (pathname observation for assert only).
	st, err := os.Lstat(filepath.Join(parentDir, name))
	if err != nil {
		t.Fatal(err)
	}
	got := st.Mode().Perm()
	tl.Assert("mode_0700", got == 0o700, "0700", fmtMode(got))
	tl.Assert("is_dir", st.IsDir(), true, st.IsDir())

	// Logs: parent identity, stage basename, created step.
	tl.Assert("log_created", ml.Has("stage_create", "created", "pass"), true, true)
	tl.Assert("log_identity", ml.DetailContains("identity="), true, true)
	tl.Assert("log_mode", ml.DetailContains("mode=0700"), true, true)
	tl.Assert("log_basename", ml.DetailContains("basename="+fmt.Sprintf("%q", name)) || ml.DetailContains(name), true, true)
	// No host home paths in log details.
	home, _ := os.UserHomeDir()
	if home != "" {
		tl.Assert("no_home_in_logs", !ml.DetailContains(home), true, true)
	}
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCreateStage_EEXISTRetryThenSuccess(t *testing.T) {
	// Force first suffix to collide, second to succeed (REQ-125 retry).
	log, tl, ml := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "stage-eexist")

	collide := "aaaaaaaaaaaaaaaa"
	success := "bbbbbbbbbbbbbbbb"
	// Pre-create the colliding stage name relative to parent.
	if err := unix.Mkdirat(parent.DirFD(), ".foundry-proj-"+collide, 0o700); err != nil {
		t.Fatal(err)
	}

	// Test hook lives in package fsx; drive via CreateStage with fixed suffixes
	// by writing through the package-level var from an internal test... we are
	// in fsx_test. Use the exported test helper.
	restore := fsx.SetStageSuffixForTest(t, []string{collide, success})
	t.Cleanup(restore)

	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	tl.Assert("name_success", stage.Name() == ".foundry-proj-"+success, ".foundry-proj-"+success, stage.Name())
	tl.Assert("retry_logged", ml.Has("stage_create", "mkdirat_retry", "info"), true, true)
	tl.Assert("created_logged", ml.Has("stage_create", "created", "pass"), true, true)
	tl.Assert("collide_still_there", childExists(t, parentDir, ".foundry-proj-"+collide), true, true)
	tl.Assert("success_exists", childExists(t, parentDir, stage.Name()), true, true)
	_ = log
}

func TestCreateStage_ExhaustEEXISTRetries(t *testing.T) {
	log, tl, ml := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "stage-exhaust")

	// Every attempt returns the same suffix that already exists → exhaust.
	suffix := "cccccccccccccccc"
	if err := unix.Mkdirat(parent.DirFD(), ".foundry-proj-"+suffix, 0o700); err != nil {
		t.Fatal(err)
	}
	// Return the same colliding suffix MaxStageCreateAttempts times.
	seq := make([]string, fsx.MaxStageCreateAttempts)
	for i := range seq {
		seq[i] = suffix
	}
	restore := fsx.SetStageSuffixForTest(t, seq)
	t.Cleanup(restore)

	_, err := fsx.CreateStage(parent, "proj")
	requireID(t, err, diagnostic.IDFSCommitFailed)
	tl.Assert("exhausted_logged", ml.Has("stage_create", "exhausted", "fail"), true, true)
	tl.Assert("first_errno", ml.DetailContains("first_errno=") || ml.DetailContains("errno="), true, true)
	// Stage entry from pre-create still present (we never delete).
	tl.Assert("precreate_preserved", childExists(t, parentDir, ".foundry-proj-"+suffix), true, true)
	_ = log
}

func TestCreateStage_ManyUniqueStages(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "stage-many")

	const n = 8
	names := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		s, err := fsx.CreateStage(parent, "proj")
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if _, dup := names[s.Name()]; dup {
			t.Fatalf("duplicate stage name %s", s.Name())
		}
		names[s.Name()] = struct{}{}
		_ = s.Close()
	}
	tl.Assert("unique_count", len(names) == n, n, len(names))
	tl.Assert("max_attempts_const", fsx.MaxStageCreateAttempts == 16, 16, fsx.MaxStageCreateAttempts)
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("stages_on_disk", len(entries) >= n, ">=n", len(entries))
}

func TestRootedWriter_RejectsEscapes(t *testing.T) {
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "writer-escape")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	w := stage.Writer()

	cases := []struct {
		name string
		path string
	}{
		{"dotdot", "../escape"},
		{"dotdot_nested", "a/../../escape"},
		{"absolute", "/etc/passwd"},
		{"abs_trailing", "/tmp/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := w.WriteFile(tc.path, fsx.DefaultFileMode, []byte("x\n"))
			if err == nil {
				t.Fatalf("WriteFile(%q) succeeded; want reject", tc.path)
			}
			id := mustFoundryID(t, err)
			// Escape → fs.unsafe_path; absolute similarly.
			if id != diagnostic.IDFSUnsafePath && id != diagnostic.IDRenderFailed {
				t.Fatalf("id=%s want fs.unsafe_path or render.failed", id)
			}
			tl.Assert("rejected_"+tc.name, true, id, id)
		})
	}
	// Valid write still works.
	if err := w.WriteFile("ok.txt", fsx.DefaultFileMode, []byte("ok\n")); err != nil {
		t.Fatal(err)
	}
	tl.Assert("valid_ok", ml.Has("rooted_writer", "write_file", "pass"), true, true)
}

func TestRootedWriter_ModeMatrixExact(t *testing.T) {
	// Plan inventory modes (Section 15.6): files 0644, dirs 0755, stage 0700.
	// Also admit 0755 files (executable) and 0600 where planned.
	log, tl, ml := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "writer-modes")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	w := stage.Writer()

	// Umask variation: set a restrictive umask and prove fchmod still applies exact modes.
	old := unix.Umask(0o077)
	t.Cleanup(func() { unix.Umask(old) })

	matrix := []struct {
		path string
		mode string
		dir  bool
	}{
		{"readme.txt", "0644", false},
		{"bin/tool", "0755", false},
		{"secrets/token", "0600", false},
		{"cmd", "0755", true},
		{"cmd/app", "0755", true},
	}

	for _, tc := range matrix {
		if tc.dir {
			if err := w.MkdirAll(tc.path, tc.mode); err != nil {
				t.Fatalf("MkdirAll %s: %v", tc.path, err)
			}
		} else {
			if err := w.WriteFile(tc.path, tc.mode, []byte("content\n")); err != nil {
				t.Fatalf("WriteFile %s: %v", tc.path, err)
			}
		}
	}

	stagePath := filepath.Join(parentDir, stage.Name())
	for _, tc := range matrix {
		st, err := os.Lstat(filepath.Join(stagePath, filepath.FromSlash(tc.path)))
		if err != nil {
			t.Fatalf("lstat %s: %v", tc.path, err)
		}
		want, err := parseTestMode(tc.mode)
		if err != nil {
			t.Fatal(err)
		}
		got := st.Mode().Perm()
		if got != want {
			t.Errorf("path %s mode=%04o want %04o (umask was 077)", tc.path, got, want)
		}
		tl.Assert("mode_"+tc.path, got == want, fmt.Sprintf("%04o", want), fmt.Sprintf("%04o", got))
	}
	tl.Assert("modes_logged", ml.DetailContains("mode=0644") || ml.Has("rooted_writer", "write_file", "pass"), true, true)
	_ = log
}

func TestStage_IdentityChangeMidWrite(t *testing.T) {
	log, tl, ml := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "stage-swap")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	// Rename the stage entry under the parent so re-lookup by name fails /
	// differs — identity verify must detect before write.
	oldName := stage.Name()
	newName := oldName + ".swapped"
	if err := unix.Renameat(parent.DirFD(), oldName, parent.DirFD(), newName); err != nil {
		t.Fatalf("renameat: %v", err)
	}
	// Keep the object for cleanup via new name (stage Close still closes FD).
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(parentDir, newName))
	})

	w := stage.Writer()
	err = w.WriteFile("after-swap.txt", fsx.DefaultFileMode, []byte("nope\n"))
	requireID(t, err, diagnostic.IDFSCommitFailed)
	tl.Assert("identity_fail_logged",
		ml.Has("stage_identity", "lookup", "fail") || ml.Has("stage_identity", "mismatch", "fail"),
		true, true)

	// File must not appear under the swapped object either via path open of
	// old name (absent) — retained FD still points at object, but VerifyIdentity
	// refused before create. Confirm no after-swap.txt via the new pathname.
	if _, err := os.Lstat(filepath.Join(parentDir, newName, "after-swap.txt")); !os.IsNotExist(err) {
		// If the writer somehow wrote through retained root before verify, fail.
		// Our order is verify-then-write, so file should be absent.
		if err == nil {
			t.Fatal("write occurred despite identity change")
		}
	}
	_ = log
}

func TestStage_NoDeleteAPI(t *testing.T) {
	// Static intent: Stage exports Close but no Delete/Remove/Cleanup symbols.
	// Runtime: Close leaves the stage on disk.
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "stage-preserve")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	name := stage.Name()
	if err := stage.Close(); err != nil {
		t.Fatal(err)
	}
	// Stage directory still present.
	st, err := os.Lstat(filepath.Join(parentDir, name))
	if err != nil {
		t.Fatalf("stage removed after Close: %v", err)
	}
	tl.Assert("still_dir", st.IsDir(), true, true)
	tl.Assert("still_0700", st.Mode().Perm() == 0o700, "0700", fmtMode(st.Mode().Perm()))
	_ = log
}

func TestStage_StepLoggerDetails(t *testing.T) {
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "stage-logs")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	w := stage.Writer()
	if err := w.WriteFile("a/b.txt", "0644", []byte("x\n")); err != nil {
		t.Fatal(err)
	}

	tl.Assert("attempts_logged", ml.DetailContains("attempts=") || ml.DetailContains("max_attempts="), true, true)
	tl.Assert("dev_ino", ml.DetailContains("dev=") && ml.DetailContains("ino="), true, true)
	tl.Assert("modes_applied", ml.DetailContains("mode=0644") || ml.DetailContains("mode=0755"), true, true)
	tl.Assert("parent_identity", ml.DetailContains("parent_identity="), true, true)
	// No home paths.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, e := range ml.Entries() {
			if strings.Contains(e.Detail, home) {
				t.Errorf("log detail contains home path: %q", e.Detail)
			}
		}
	}
}

func TestCreateStage_InvalidProjectName(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, _ := arrangeParent(t, log, "stage-badname")
	_, err := fsx.CreateStage(parent, "../evil")
	requireID(t, err, diagnostic.IDFSUnsafePath)
	tl.Assert("bad_name", true, diagnostic.IDFSUnsafePath, mustFoundryID(t, err))
	_ = log
}

func childExists(t *testing.T, parentDir, name string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(parentDir, name))
	return err == nil
}

func parseTestMode(mode string) (os.FileMode, error) {
	s := strings.TrimSpace(mode)
	if strings.HasPrefix(s, "0o") {
		s = s[2:]
	}
	var v uint64
	_, err := fmt.Sscanf(s, "%o", &v)
	if err != nil {
		return 0, err
	}
	return os.FileMode(v), nil
}

// Ensure syscall import used on platforms that need it for type assertions in helpers.
var _ = syscall.EEXIST
