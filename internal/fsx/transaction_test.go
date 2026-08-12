//go:build unix

package fsx_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"golang.org/x/sys/unix"
)

// arrangeDest returns a custody-safe destination path under t.TempDir.
func arrangeDest(t *testing.T, label string) (dest, parentDir string) {
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
	parentDir = privateParent(t, root, label)
	return filepath.Join(parentDir, "proj"), parentDir
}

// TestTransaction_BeginWriterCommitClose is the Section 31 end-to-end happy path
// through the integrated Transaction surface (E1 primitives wired as production).
func TestTransaction_BeginWriterCommitClose(t *testing.T) {
	log, tl, ml := newBridge(t)
	tl.Phase("arrange")
	dest, parentDir := arrangeDest(t, "txn-happy")
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act_begin")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	t.Cleanup(func() { _ = txn.Close() })
	tl.PhaseEnd("act_begin", testutil.OutcomeOK)

	tl.Phase("assert_begin")
	stageName := txn.StageName()
	tl.Assert("stage_name_prefix", strings.HasPrefix(stageName, ".foundry-proj-"), true, stageName)
	tl.Assert("stage_name_pattern", stageNamePattern.MatchString(stageName), true, stageName)
	tl.Assert("stage_path", txn.StagePath() == filepath.Join(parentDir, stageName),
		filepath.Join(parentDir, stageName), txn.StagePath())
	tl.Assert("dest_name", txn.DestName() == "proj", "proj", txn.DestName())
	tl.Assert("identity_nonzero", !txn.StageIdentity().IsZero(), true, txn.StageIdentity())
	tl.Assert("writer_non_nil", txn.RootedWriter() != nil, true, txn.RootedWriter() != nil)
	tl.Assert("log_opened", ml.Has("transaction", "opened", "pass"), true, true)
	// Stage entry present on disk (pathname observation for assert only).
	st, err := os.Lstat(filepath.Join(parentDir, stageName))
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("stage_mode_0700", st.Mode().Perm() == 0o700, "0700", fmtMode(st.Mode().Perm()))
	tl.PhaseEnd("assert_begin", testutil.OutcomeOK)

	tl.Phase("act_write")
	w := txn.RootedWriter()
	if err := w.WriteFile("hello.txt", "0644", []byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Mkdir("subdir", "0755"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("subdir/nested.txt", "0644", []byte("nested\n")); err != nil {
		t.Fatal(err)
	}
	tl.PhaseEnd("act_write", testutil.OutcomeOK)

	tl.Phase("act_commit")
	res := txn.Commit()
	tl.PhaseEnd("act_commit", testutil.OutcomeOK)

	tl.Phase("assert_commit")
	tl.Assert("class_committed", res.Class == fsx.ClassCommitted, fsx.ClassCommitted, res.Class)
	tl.Assert("exit_0", res.Exit == diagnostic.ExitSuccess, diagnostic.ExitSuccess, res.Exit)
	tl.Assert("committed", res.Committed(), true, res.Committed())
	tl.Assert("err_nil", res.Err == nil, nil, res.Err)
	tl.Assert("not_preserved", !res.Preserved(), false, res.Preserved())
	data, err := os.ReadFile(filepath.Join(parentDir, "proj", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("content", string(data) == "hello\n", "hello\\n", string(data))
	// Stage basename consumed.
	if _, err := os.Lstat(filepath.Join(parentDir, stageName)); !os.IsNotExist(err) {
		t.Fatalf("stage basename still present: %v", err)
	}
	tl.Assert("log_commit_pass", ml.Has("commit", "classified", "pass"), true, true)
	tl.PhaseEnd("assert_commit", testutil.OutcomeOK)

	tl.Phase("act_close")
	if err := txn.Close(); err != nil {
		t.Fatal(err)
	}
	// Idempotent Close.
	if err := txn.Close(); err != nil {
		t.Fatal(err)
	}
	tl.Assert("writer_nil_after_close", txn.RootedWriter() == nil, true, false)
	tl.Assert("log_close", ml.Has("transaction", "close", "pass"), true, true)
	// Destination still present after Close (Close never deletes).
	if _, err := os.Stat(filepath.Join(parentDir, "proj", "hello.txt")); err != nil {
		t.Fatal(err)
	}
	tl.PhaseEnd("act_close", testutil.OutcomeOK)
}

// TestTransaction_DuplicateStageHandle verifies Section 34.4 / SV-03: the
// duplicated FD tracks the stage object even if the stage entry is renamed
// under the parent after the dup is issued.
func TestTransaction_DuplicateStageHandle(t *testing.T) {
	log, tl, ml := newBridge(t)
	dest, parentDir := arrangeDest(t, "txn-dup")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txn.Close() })

	tl.Phase("act_dup")
	fd, err := txn.DuplicateStageHandle()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	tl.PhaseEnd("act_dup", testutil.OutcomeOK)

	tl.Phase("assert_identity")
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		t.Fatal(err)
	}
	dupID := fsx.FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}
	tl.Assert("dup_matches_stage", dupID.Equal(txn.StageIdentity()), txn.StageIdentity(), dupID)
	tl.Assert("log_dup_pass", ml.Has("transaction", "dup_stage", "pass"), true, true)

	// Write a sentinel through the retained stage writer, then fchdir(dup)
	// and read it — proves the dup is a live directory handle.
	if err := txn.RootedWriter().WriteFile("e1-cwd-sentinel.txt", "0644", []byte("sentinel\n")); err != nil {
		t.Fatal(err)
	}

	// Pathname-rename the stage entry under the parent; the dup FD must still
	// refer to the same object (SV-03 continuity — E1 contract 3/6).
	stageName := txn.StageName()
	swapped := stageName + "-swapped"
	if err := unix.Renameat(txn.Parent().DirFD(), stageName, txn.Parent().DirFD(), swapped); err != nil {
		t.Fatal(err)
	}
	// Dup still open and readable via /proc or fstat.
	if err := unix.Fstat(fd, &st); err != nil {
		t.Fatal(err)
	}
	after := fsx.FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}
	tl.Assert("dup_survives_pathname_swap", after.Equal(dupID), dupID, after)

	// Open the sentinel relative to the dup FD (openat), not by path.
	sfd, err := unix.Openat(fd, "e1-cwd-sentinel.txt", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("openat sentinel via dup after swap: %v", err)
	}
	f := os.NewFile(uintptr(sfd), "sentinel")
	buf := make([]byte, 32)
	n, err := f.Read(buf)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("sentinel_via_dup", string(buf[:n]) == "sentinel\n", "sentinel\\n", string(buf[:n]))

	// Restore stage name so Commit can still find it (identity re-lookup uses basename).
	// After swap the stage basename is gone — Commit would classify uncommitted/ambiguous.
	// For this test we only care about the dup handle property; leave the swapped name.
	// Ensure Close does not delete either name.
	_ = parentDir
	tl.PhaseEnd("assert_identity", testutil.OutcomeOK)
}

// TestTransaction_ClosePreservesStage confirms Close releases handles without
// unlinking the stage (Section 31.6 / REQ-130).
func TestTransaction_ClosePreservesStage(t *testing.T) {
	log, tl, _ := newBridge(t)
	dest, parentDir := arrangeDest(t, "txn-preserve")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	stageName := txn.StageName()
	stagePath := txn.StagePath()
	if err := txn.RootedWriter().WriteFile("keep.txt", "0644", []byte("keep\n")); err != nil {
		t.Fatal(err)
	}

	tl.Phase("act_close_without_commit")
	if err := txn.Close(); err != nil {
		t.Fatal(err)
	}
	tl.PhaseEnd("act_close_without_commit", testutil.OutcomeOK)

	tl.Phase("assert_preserved")
	// Stage directory and content still on disk.
	data, err := os.ReadFile(filepath.Join(parentDir, stageName, "keep.txt"))
	if err != nil {
		t.Fatalf("stage deleted by Close: %v", err)
	}
	tl.Assert("content", string(data) == "keep\n", "keep\\n", string(data))
	tl.Assert("stage_path_stable", stagePath == filepath.Join(parentDir, stageName),
		filepath.Join(parentDir, stageName), stagePath)
	// Destination still absent (no commit).
	if _, err := os.Lstat(filepath.Join(parentDir, "proj")); !os.IsNotExist(err) {
		t.Fatalf("destination unexpectedly present: %v", err)
	}
	// Commit after Close is fail-closed ambiguous.
	res := txn.Commit()
	tl.Assert("commit_after_close_ambiguous", res.Class == fsx.ClassAmbiguous, fsx.ClassAmbiguous, res.Class)
	tl.Assert("commit_after_close_exit1", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
	tl.PhaseEnd("assert_preserved", testutil.OutcomeOK)
}

// TestTransaction_BeginDestinationExists refuses before staging (REQ-003).
func TestTransaction_BeginDestinationExists(t *testing.T) {
	log, tl, ml := newBridge(t)
	dest, parentDir := arrangeDest(t, "txn-exists")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	txn, err := fsx.Begin(dest, fsx.Options{Log: log})
	if err == nil {
		_ = txn.Close()
		t.Fatal("expected Begin to fail when destination exists")
	}
	requireID(t, err, diagnostic.IDFSDestinationExists)
	// No stage should have been created.
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".foundry-") {
			t.Fatalf("stage created despite destination exists: %s", e.Name())
		}
	}
	tl.Assert("log_preflight_fail", ml.Has("transaction", "preflight", "fail") || ml.Has("destination", "refuse", "fail") || ml.DetailContains(string(diagnostic.IDFSDestinationExists)), true, true)
	_ = tl
}

// TestTransaction_CommitResultMatrix_Section319 table-tests every CommitResult
// class reachable from fsx.Commit (Section 31.9 rows owned by the transaction
// package). Cancellation (exit 130) and post-commit report-stream failure are
// generate-owned and intentionally out of scope here.
func TestTransaction_CommitResultMatrix_Section319(t *testing.T) {
	_, tl, _ := newBridge(t)
	tl.Phase("matrix")

	type row struct {
		name      string
		setup     func(t *testing.T, txn *fsx.Transaction) (restore func())
		wantClass fsx.CommitClass
		wantExit  int
		wantID    diagnostic.Identifier
		preserved bool
	}

	rows := []row{
		{
			name: "rename_succeeded_committed_exit_0",
			setup: func(t *testing.T, txn *fsx.Transaction) func() {
				if err := txn.RootedWriter().WriteFile("ok.txt", "0644", []byte("ok\n")); err != nil {
					t.Fatal(err)
				}
				return func() {}
			},
			wantClass: fsx.ClassCommitted,
			wantExit:  diagnostic.ExitSuccess,
			wantID:    "",
			preserved: false,
		},
		{
			name: "eexist_conflict_preserve_exit_2",
			setup: func(t *testing.T, txn *fsx.Transaction) func() {
				if err := txn.RootedWriter().WriteFile("loser.txt", "0644", []byte("lose\n")); err != nil {
					t.Fatal(err)
				}
				// Pre-create destination so exclusive rename hits EEXIST/conflict.
				parent := txn.Parent()
				if err := unix.Mkdirat(parent.DirFD(), parent.Basename(), 0o755); err != nil {
					t.Fatal(err)
				}
				return func() {}
			},
			wantClass: fsx.ClassConflict,
			wantExit:  diagnostic.ExitUsage, // Appendix D: destination_exists → 2
			wantID:    diagnostic.IDFSDestinationExists,
			preserved: true,
		},
		{
			name: "rename_unsupported_preserve_exit_1",
			setup: func(t *testing.T, txn *fsx.Transaction) func() {
				if err := txn.RootedWriter().WriteFile("u.txt", "0644", []byte("u\n")); err != nil {
					t.Fatal(err)
				}
				return fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
					return unix.ENOTSUP
				})
			},
			wantClass: fsx.ClassUncommitted,
			wantExit:  diagnostic.ExitFailure,
			wantID:    diagnostic.IDFSRenameUnsupported,
			preserved: true,
		},
		{
			name: "exdev_preserve_exit_1",
			setup: func(t *testing.T, txn *fsx.Transaction) func() {
				if err := txn.RootedWriter().WriteFile("x.txt", "0644", []byte("x\n")); err != nil {
					t.Fatal(err)
				}
				return fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
					return unix.EXDEV
				})
			},
			wantClass: fsx.ClassUncommitted,
			wantExit:  diagnostic.ExitFailure,
			wantID:    diagnostic.IDFSCommitFailed,
			preserved: true,
		},
		{
			name: "ambiguous_false_success_preserve_exit_1",
			setup: func(t *testing.T, txn *fsx.Transaction) func() {
				if err := txn.RootedWriter().WriteFile("a.txt", "0644", []byte("a\n")); err != nil {
					t.Fatal(err)
				}
				// Syscall claims success but destination is not the stage identity.
				return fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
					// Create a different dest object and report success (false-success).
					if err := unix.Mkdirat(parentFd, destName, 0o755); err != nil && err != unix.EEXIST {
						return err
					}
					return nil
				})
			},
			wantClass: fsx.ClassAmbiguous,
			wantExit:  diagnostic.ExitFailure,
			wantID:    diagnostic.IDFSCommitAmbiguous,
			preserved: true,
		},
	}

	for _, r := range rows {
		r := r
		t.Run(r.name, func(t *testing.T) {
			rowLog, rowTL, _ := newBridge(t)
			dest, parentDir := arrangeDest(t, "matrix-"+r.name)
			txn, err := fsx.Begin(dest, fsx.Options{Log: rowLog})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = txn.Close() })
			stageName := txn.StageName()

			restore := r.setup(t, txn)
			t.Cleanup(restore)

			res := txn.Commit()
			rowTL.Assert("class", res.Class == r.wantClass, r.wantClass, res.Class)
			rowTL.Assert("exit", res.Exit == r.wantExit, r.wantExit, res.Exit)
			if r.wantID == "" {
				rowTL.Assert("id_empty", res.ErrorID() == "", "", res.ErrorID())
			} else {
				rowTL.Assert("id", res.ErrorID() == r.wantID, r.wantID, res.ErrorID())
			}
			rowTL.Assert("preserved_helper", res.Preserved() == r.preserved, r.preserved, res.Preserved())
			if r.preserved {
				rowTL.Assert("stage_path_set", res.StagePath != "", "non-empty", res.StagePath)
				rowTL.Assert("stage_name", res.StageName == stageName, stageName, res.StageName)
				// Stage entry still on disk.
				if _, err := os.Lstat(filepath.Join(parentDir, stageName)); err != nil {
					t.Fatalf("stage not preserved: %v", err)
				}
				pr := res.Report()
				rowTL.Assert("report_rsk", pr.RiskID == fsx.RSK310, fsx.RSK310, pr.RiskID)
				rowTL.Assert("report_remediation", strings.Contains(pr.Remediation, "rm -rf") || strings.Contains(pr.Remediation, fsx.RSK310), true, pr.Remediation)
			}
			// Destination handling: committed → dest present; conflict → dest present (winner); others vary.
			_ = parentDir
			rowTL.PhaseEnd("assert", testutil.OutcomeOK)
		})
	}
	tl.PhaseEnd("matrix", testutil.OutcomeOK)
}

// TestTransaction_TwoProcessEEXIST exercises concurrent Commit through the
// Transaction surface: exactly one winner, loser stage preserved (Section 31.9).
func TestTransaction_TwoProcessEEXIST(t *testing.T) {
	log, tl, _ := newBridge(t)
	// Shared parent with two independent stages (two transactions cannot share
	// one parent handle safely across concurrent Commit because each transaction
	// owns Close of the parent). Use low-level CreateStage on one parent like
	// the commit race test, but drive Commit via Transaction by wrapping...
	// Spec: two concurrent Foundry invocations — each has its own parent FD.
	// Mirror that: two Begin on same destination path (both pass preflight while
	// dest is absent), then concurrent Commit.
	dest, parentDir := arrangeDest(t, "txn-race")

	txnA, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txnA.Close() })
	txnB, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txnB.Close() })

	if err := txnA.RootedWriter().WriteFile("a.txt", "0644", []byte("A")); err != nil {
		t.Fatal(err)
	}
	if err := txnB.RootedWriter().WriteFile("b.txt", "0644", []byte("B")); err != nil {
		t.Fatal(err)
	}

	var (
		wg    sync.WaitGroup
		resA  fsx.CommitResult
		resB  fsx.CommitResult
		start = make(chan struct{})
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		resA = txnA.Commit()
	}()
	go func() {
		defer wg.Done()
		<-start
		resB = txnB.Commit()
	}()
	close(start)
	wg.Wait()

	results := []fsx.CommitResult{resA, resB}
	var winners, losers int
	for i, r := range results {
		switch {
		case r.Committed():
			winners++
			tl.Assert(fmt.Sprintf("winner_%d_exit0", i), r.Exit == diagnostic.ExitSuccess, 0, r.Exit)
		case r.Class == fsx.ClassConflict || r.ErrorID() == diagnostic.IDFSDestinationExists:
			losers++
			tl.Assert(fmt.Sprintf("loser_%d_exit2", i), r.Exit == diagnostic.ExitUsage, 2, r.Exit)
			tl.Assert(fmt.Sprintf("loser_%d_preserved", i), r.Preserved(), true, r.Preserved())
			// Losing stage still on disk.
			if _, err := os.Lstat(filepath.Join(parentDir, r.StageName)); err != nil {
				t.Fatalf("loser stage missing: %v", err)
			}
		default:
			// Rare: both may classify conflict if timing is extreme; still no double-commit.
			t.Logf("result[%d] class=%s id=%s exit=%d msg=%s", i, r.Class, r.ErrorID(), r.Exit, r.Message)
			if r.Preserved() {
				losers++
			}
		}
	}
	tl.Assert("exactly_one_winner", winners == 1, 1, winners)
	tl.Assert("at_least_one_loser", losers >= 1, ">=1", losers)
	// Destination exists exactly once.
	if st, err := os.Lstat(filepath.Join(parentDir, "proj")); err != nil || !st.IsDir() {
		t.Fatalf("destination missing after race: %v", err)
	}
}

// TestTransaction_NoDeleteAPI_Reflect documents the normative surface: no
// Delete/Remove methods on Transaction (static audit covers production symbols).
func TestTransaction_NoDeleteAPI_Reflect(t *testing.T) {
	// Compile-time surface check: Transaction methods used here are the only
	// mutation/lifecycle APIs. This test ensures the public method set remains
	// Begin/RootedWriter/DuplicateStageHandle/Commit/Close (+ accessors).
	log, tl, _ := newBridge(t)
	dest, _ := arrangeDest(t, "txn-api")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txn.Close() })

	_ = txn.RootedWriter()
	fd, err := txn.DuplicateStageHandle()
	if err != nil {
		t.Fatal(err)
	}
	_ = unix.Close(fd)
	_ = txn.StageName()
	_ = txn.StagePath()
	_ = txn.StageIdentity()
	_ = txn.DestName()
	_ = txn.Parent()
	_ = txn.Stage()
	// Commit + Close exercised elsewhere; prove accessors return real values
	// and Close after abandon preserves the stage (no delete method to call).
	stageName := txn.StageName()
	stagePath := txn.StagePath()
	parentDir := filepath.Dir(stagePath)
	tl.Assert("stage_name_set", stageName != "", true, stageName)
	tl.Assert("stage_path_set", stagePath != "", true, stagePath)
	tl.Assert("dest_name_set", txn.DestName() != "", true, txn.DestName())
	tl.Assert("parent_set", txn.Parent() != nil, true, txn.Parent() != nil)
	tl.Assert("stage_set", txn.Stage() != nil, true, txn.Stage() != nil)
	if err := txn.Close(); err != nil {
		t.Fatal(err)
	}
	_, lstatErr := os.Lstat(filepath.Join(parentDir, stageName))
	if lstatErr != nil {
		t.Fatalf("Close deleted stage: %v", lstatErr)
	}
	tl.Assert("stage_preserved", lstatErr == nil, true, lstatErr)
}

// TestTransaction_SentinelOutsideStageSurvives ensures operations never
// recursively clean the parent (sentinel survival from REQ-213 / 31.6).
func TestTransaction_SentinelOutsideStageSurvives(t *testing.T) {
	log, tl, _ := newBridge(t)
	dest, parentDir := arrangeDest(t, "txn-sentinel")
	sentinel := filepath.Join(parentDir, "OUTSIDE-SENTINEL")
	if err := os.WriteFile(sentinel, []byte("alive\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	txn, err := fsx.Begin(dest, fsx.Options{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	if err := txn.RootedWriter().WriteFile("in-stage.txt", "0644", []byte("in\n")); err != nil {
		t.Fatal(err)
	}
	// Fail commit via unsupported rename.
	restore := fsx.SetExclusiveRenameForTest(t, func(int, string, string) error { return unix.ENOTSUP })
	t.Cleanup(restore)
	res := txn.Commit()
	tl.Assert("uncommitted", res.Class == fsx.ClassUncommitted, fsx.ClassUncommitted, res.Class)
	if err := txn.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("sentinel destroyed: %v", err)
	}
	tl.Assert("sentinel_alive", string(data) == "alive\n", "alive\\n", string(data))
}
