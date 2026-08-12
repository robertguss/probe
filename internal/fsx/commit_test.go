//go:build unix

package fsx_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"golang.org/x/sys/unix"
)

func TestRenameSyscallName_Platform(t *testing.T) {
	name := fsx.RenameSyscallName()
	log := testutil.New(t)
	log.Phase("assert")
	switch runtime.GOOS {
	case "linux":
		log.Assert("linux_syscall", name == "Renameat2(RENAME_NOREPLACE)",
			"Renameat2(RENAME_NOREPLACE)", name)
	case "darwin":
		log.Assert("darwin_syscall", name == "RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY)",
			"RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY)", name)
	default:
		t.Logf("GOOS=%s rename=%s", runtime.GOOS, name)
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_CleanSuccess(t *testing.T) {
	log, tl, ml := newBridge(t)
	tl.Phase("arrange")
	parent, parentDir := arrangeParent(t, log, "commit-happy")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	if err := stage.Writer().WriteFile("hello.txt", "0644", []byte("hi\n")); err != nil {
		t.Fatal(err)
	}
	stageName := stage.Name()
	stageID := stage.Identity()
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	res := fsx.Commit(stage)
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	tl.Assert("class_committed", res.Class == fsx.ClassCommitted, fsx.ClassCommitted, res.Class)
	tl.Assert("exit_0", res.Exit == diagnostic.ExitSuccess, diagnostic.ExitSuccess, res.Exit)
	tl.Assert("committed_helper", res.Committed(), true, res.Committed())
	tl.Assert("err_nil", res.Err == nil, nil, res.Err)
	tl.Assert("error_id_empty", res.ErrorID() == "", "", res.ErrorID())
	tl.Assert("dest_name", res.DestName == "proj", "proj", res.DestName)
	tl.Assert("stage_name", res.StageName == stageName, stageName, res.StageName)
	tl.Assert("syscall_set", res.Syscall != "", "non-empty", res.Syscall)
	tl.Assert("obs_dest_is_stage", res.Observation.DestPresent && res.Observation.DestID.Equal(stageID),
		true, fmt.Sprintf("present=%v id=%s want=%s", res.Observation.DestPresent, res.Observation.DestID, stageID))
	tl.Assert("obs_stage_gone", !res.Observation.StagePresent, false, res.Observation.StagePresent)

	// Destination path holds the content.
	data, err := os.ReadFile(filepath.Join(parentDir, "proj", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("content", string(data) == "hi\n", "hi\\n", string(data))

	// Stage basename must not remain under parent.
	exists, _, err := parent.ChildLookup(stageName)
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("stage_consumed", !exists, false, exists)

	// Step logs: syscall name, class, outcome pass.
	tl.Assert("log_rename", ml.Has("commit", "rename", "info") || ml.Has("commit", "classified", "pass"), true, true)
	tl.Assert("log_classified_pass", ml.Has("commit", "classified", "pass"), true, true)
	tl.Assert("log_syscall", ml.DetailContains("syscall="), true, true)
	tl.Assert("log_class", ml.DetailContains("class=committed") || ml.DetailContains("outcome=committed"), true, true)
	home, _ := os.UserHomeDir()
	if home != "" {
		tl.Assert("no_home", !ml.DetailContains(home), true, true)
	}
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_TwoProcessEEXIST(t *testing.T) {
	// Concurrent exclusive commits to one destination: exactly one winner;
	// loser stage preserved with fs.destination_exists (Section 31.9 / REQ-131).
	log, tl, _ := newBridge(t)
	tl.Phase("arrange")
	parent, _ := arrangeParent(t, log, "commit-race")
	stageA, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stageA.Close() })
	stageB, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stageB.Close() })
	if err := stageA.Writer().WriteFile("a.txt", "0644", []byte("A")); err != nil {
		t.Fatal(err)
	}
	if err := stageB.Writer().WriteFile("b.txt", "0644", []byte("B")); err != nil {
		t.Fatal(err)
	}
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	var (
		mu      sync.Mutex
		results []fsx.CommitResult
		wg      sync.WaitGroup
	)
	wg.Add(2)
	run := func(st *fsx.Stage) {
		defer wg.Done()
		res := fsx.Commit(st)
		mu.Lock()
		results = append(results, res)
		mu.Unlock()
	}
	go run(stageA)
	go run(stageB)
	wg.Wait()
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	var winners, losers int
	for _, r := range results {
		switch r.Class {
		case fsx.ClassCommitted:
			winners++
			tl.Assert("winner_exit_0", r.Exit == 0, 0, r.Exit)
			tl.Assert("winner_err_nil", r.Err == nil, nil, r.Err)
		case fsx.ClassUncommitted, fsx.ClassConflict:
			losers++
			tl.Assert("loser_exit_2", r.Exit == diagnostic.ExitUsage, diagnostic.ExitUsage, r.Exit)
			tl.Assert("loser_id", r.ErrorID() == diagnostic.IDFSDestinationExists,
				diagnostic.IDFSDestinationExists, r.ErrorID())
			// Loser stage preserved under parent.
			exists, _, err := parent.ChildLookup(r.StageName)
			if err != nil {
				t.Fatal(err)
			}
			if r.Class != fsx.ClassCommitted {
				tl.Assert("loser_stage_preserved_"+r.StageName, exists, true, exists)
			}
		default:
			t.Fatalf("unexpected class %s: %+v", r.Class, r)
		}
	}
	tl.Assert("one_winner", winners == 1, 1, winners)
	tl.Assert("one_loser", losers == 1, 1, losers)

	// Destination exists exactly once.
	exists, destID, err := parent.ChildLookup("proj")
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("dest_present", exists, true, exists)
	tl.Assert("dest_id_nonzero", !destID.IsZero(), true, destID)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_InjectedAmbiguousAcknowledgment(t *testing.T) {
	// Syscall reports success without actually renaming → destination identity
	// does not match recorded stage → ClassAmbiguous, stage preserved (31.8).
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "commit-ambig")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	stageName := stage.Name()

	restore := fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
		// False success: do not rename.
		return nil
	})
	t.Cleanup(restore)

	tl.Phase("act")
	res := fsx.Commit(stage)
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	tl.Assert("class_ambiguous", res.Class == fsx.ClassAmbiguous, fsx.ClassAmbiguous, res.Class)
	tl.Assert("exit_1", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
	tl.Assert("id_ambiguous", res.ErrorID() == diagnostic.IDFSCommitAmbiguous,
		diagnostic.IDFSCommitAmbiguous, res.ErrorID())
	exists, _, err := parent.ChildLookup(stageName)
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("stage_preserved", exists, true, exists)
	destExists, _, err := parent.ChildLookup("proj")
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("dest_absent", !destExists, false, destExists)
	tl.Assert("log_fail", ml.Has("commit", "classified", "fail"), true, true)
	tl.Assert("log_class", ml.DetailContains("class=ambiguous"), true, true)
	tl.Assert("log_stage", ml.DetailContains(stageName), true, true)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_InjectedRenameUnsupported(t *testing.T) {
	// ENOTSUP without mutation → uncommitted + fs.rename_unsupported + stage preserved.
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "commit-unsup")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	stageName := stage.Name()

	restore := fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
		return unix.ENOTSUP
	})
	t.Cleanup(restore)

	tl.Phase("act")
	res := fsx.Commit(stage)
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	tl.Assert("class_uncommitted", res.Class == fsx.ClassUncommitted, fsx.ClassUncommitted, res.Class)
	tl.Assert("exit_1", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
	tl.Assert("id_rename_unsupported", res.ErrorID() == diagnostic.IDFSRenameUnsupported,
		diagnostic.IDFSRenameUnsupported, res.ErrorID())
	exists, _, err := parent.ChildLookup(stageName)
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("stage_preserved", exists, true, exists)
	if exists, _, _ := parent.ChildLookup("proj"); exists {
		t.Fatal("destination must not exist when rename unsupported")
	}
	tl.Assert("log_errno", ml.DetailContains("errno="), true, true)
	tl.Assert("log_id", ml.DetailContains(string(diagnostic.IDFSRenameUnsupported)), true, true)
	tl.Assert("log_stage_preserved", ml.DetailContains("stage_preserved="), true, true)
	// Remediation present.
	fe, ok := diagnostic.AsFoundryError(res.Err)
	if !ok {
		t.Fatalf("want FoundryError, got %T", res.Err)
	}
	tl.Assert("remediation", fe.Remediation() != "", "non-empty", fe.Remediation())
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_InjectedEXDEV(t *testing.T) {
	// Unexpected EXDEV → fs.commit_failed (Section 31.9), stage preserved.
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "commit-exdev")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	stageName := stage.Name()

	restore := fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
		return unix.EXDEV
	})
	t.Cleanup(restore)

	tl.Phase("act")
	res := fsx.Commit(stage)
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	tl.Assert("class_uncommitted", res.Class == fsx.ClassUncommitted, fsx.ClassUncommitted, res.Class)
	tl.Assert("exit_1", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
	tl.Assert("id_commit_failed", res.ErrorID() == diagnostic.IDFSCommitFailed,
		diagnostic.IDFSCommitFailed, res.ErrorID())
	exists, _, err := parent.ChildLookup(stageName)
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("stage_preserved", exists, true, exists)
	tl.Assert("log_fail", ml.Has("commit", "classified", "fail"), true, true)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_EEXIST_PreexistingDest(t *testing.T) {
	// Dest already present (sequential): exclusive rename must not replace;
	// stage preserved with destination_exists.
	log, tl, ml := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "commit-eexist")
	// Pre-create destination as empty dir.
	if err := os.Mkdir(filepath.Join(parentDir, "proj"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Preflight would refuse, but Commit itself must still fail closed when
	// dest exists: create stage via CreateStage (dest pre-exists is OK for stage
	// create; only commit checks exclusive dest).
	// Re-acquire parent without EnsureDestinationAbsent for this race case.
	parent2, err := fsx.AcquireParent(filepath.Join(parentDir, "proj"), fsx.PreflightOptions{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent2.Close() })
	_ = parent // original closed via arrange cleanup? arrangeParent cleans parent.
	// arrangeParent already holds parent open; use parent2 for stage under same dir.
	stage, err := fsx.CreateStage(parent2, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	// Marker inside pre-existing dest — must survive.
	marker := filepath.Join(parentDir, "proj", "sentinel.txt")
	if err := os.WriteFile(marker, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stageName := stage.Name()

	tl.Phase("act")
	res := fsx.Commit(stage)
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	// Class may be Uncommitted (EEXIST, dest other) or Conflict depending on
	// whether dest is observed with different identity after failure.
	if res.Class != fsx.ClassUncommitted && res.Class != fsx.ClassConflict {
		t.Fatalf("class=%s want uncommitted|conflict detail=%s", res.Class, res.Message)
	}
	tl.Assert("id_dest_exists", res.ErrorID() == diagnostic.IDFSDestinationExists,
		diagnostic.IDFSDestinationExists, res.ErrorID())
	tl.Assert("exit_2", res.Exit == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Exit)
	exists, _, err := parent2.ChildLookup(stageName)
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("stage_preserved", exists, true, exists)
	// Sentinel survives (no replace).
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("sentinel", string(data) == "keep\n", "keep\\n", string(data))
	tl.Assert("log_stage_preserved", ml.DetailContains("stage_preserved=") || ml.DetailContains(stageName), true, true)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_CaseSensitivityNative(t *testing.T) {
	// FND-018: name equivalence is decided by the host FS via native exact-child
	// lookup + exclusive commit. No synthetic Unicode fold scan.
	// On case-sensitive FS: "Proj" ≠ "proj" → commit of "proj" succeeds beside "Proj".
	// On case-insensitive FS: exclusive rename to conflicting case → EEXIST/conflict.
	log, tl, _ := newBridge(t)
	tl.Phase("arrange")
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	parentDir := privateParent(t, root, "case-parent")
	// Create sibling with different case.
	alt := "Proj"
	if err := os.Mkdir(filepath.Join(parentDir, alt), 0o700); err != nil {
		t.Fatal(err)
	}
	// Probe whether the FS treats "proj" as existing after creating "Proj".
	parent, err := fsx.AcquireParent(filepath.Join(parentDir, "proj"), fsx.PreflightOptions{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	existsProj, _, err := parent.ChildLookup("proj")
	if err != nil {
		t.Fatal(err)
	}
	existsAlt, altID, err := parent.ChildLookup(alt)
	if err != nil {
		t.Fatal(err)
	}
	if !existsAlt {
		t.Fatal("expected Alt case entry present")
	}
	caseInsensitive := existsProj
	tl.Inputs(map[string]string{
		"case_insensitive": fmt.Sprintf("%v", caseInsensitive),
		"goos":             runtime.GOOS,
	})
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	if err := stage.Writer().WriteFile("x.txt", "0644", []byte("x\n")); err != nil {
		t.Fatal(err)
	}
	stageName := stage.Name()
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	res := fsx.Commit(stage)
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	if caseInsensitive {
		// Native FS folds cases: exclusive commit must not replace "Proj".
		if res.Class == fsx.ClassCommitted {
			t.Fatal("case-insensitive FS committed over folded name — exclusive semantics broken")
		}
		if res.ErrorID() != diagnostic.IDFSDestinationExists {
			t.Fatalf("want destination_exists on case-insensitive collision; got %s class=%s", res.ErrorID(), res.Class)
		}
		exists, _, err := parent.ChildLookup(stageName)
		if err != nil || !exists {
			t.Fatalf("stage must be preserved: exists=%v err=%v", exists, err)
		}
		// Original "Proj" identity unchanged.
		exists, id, err := parent.ChildLookup(alt)
		if err != nil || !exists || !id.Equal(altID) {
			t.Fatalf("alt identity lost: exists=%v id=%s want=%s err=%v", exists, id, altID, err)
		}
		tl.Assert("case_insensitive_no_replace", res.Class != fsx.ClassCommitted, true, res.Class)
		tl.Assert("case_insensitive_dest_exists_id", res.ErrorID() == diagnostic.IDFSDestinationExists, diagnostic.IDFSDestinationExists, res.ErrorID())
		tl.Assert("case_insensitive_alt_kept", exists && id.Equal(altID), true, id)
	} else {
		// Case-sensitive: "proj" and "Proj" are distinct — commit succeeds.
		tl.Assert("case_sensitive_committed", res.Class == fsx.ClassCommitted, fsx.ClassCommitted, res.Class)
		exists, _, err := parent.ChildLookup("proj")
		if err != nil || !exists {
			t.Fatalf("proj dest missing: exists=%v err=%v", exists, err)
		}
		exists, id, err := parent.ChildLookup(alt)
		if err != nil || !exists || !id.Equal(altID) {
			t.Fatalf("Proj sibling lost: exists=%v id=%s want=%s err=%v", exists, id, altID, err)
		}
		tl.Assert("case_sensitive_sibling_ok", exists && id.Equal(altID), true, id)
	}
	// No code path should have performed a case-fold scan; we only used ChildLookup
	// for the exact basename "proj" and "Proj".
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_StepLoggerDetail(t *testing.T) {
	// Acceptance: step logger records syscall name, errno, CommitResult class,
	// preserved stage basename.
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "commit-log")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	// Force failure path for richer log detail (errno + stage_preserved).
	restore := fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
		return unix.EEXIST
	})
	t.Cleanup(restore)

	res := fsx.Commit(stage)
	if res.Class != fsx.ClassUncommitted && res.Class != fsx.ClassConflict {
		// With EEXIST inject and no real dest, class is uncommitted.
		// Dest is absent → uncommitted.
		if res.Class != fsx.ClassUncommitted {
			t.Fatalf("class=%s", res.Class)
		}
	}

	tl.Phase("assert")
	tl.Assert("has_classified", ml.Has("commit", "classified", "fail"), true, true)
	tl.Assert("syscall_name", ml.DetailContains("syscall="+fsx.RenameSyscallName()) ||
		ml.DetailContains("syscall="), true, true)
	tl.Assert("errno", ml.DetailContains("errno="), true, true)
	tl.Assert("class", ml.DetailContains("class="), true, true)
	tl.Assert("stage_basename", ml.DetailContains(res.StageName) || ml.DetailContains("stage_preserved="), true, true)
	// Ensure no home path leakage.
	home, _ := os.UserHomeDir()
	if home != "" && home != "/" {
		for _, e := range ml.Entries() {
			if strings.Contains(e.Detail, home) {
				t.Fatalf("home path leaked in log: %q", e.Detail)
			}
		}
	}
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_ClosedStage(t *testing.T) {
	res := fsx.Commit(nil)
	if res.Class != fsx.ClassAmbiguous {
		t.Fatalf("class=%s", res.Class)
	}
	if res.Err == nil {
		t.Fatal("expected error")
	}
}
