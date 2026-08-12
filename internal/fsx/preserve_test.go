//go:build unix

package fsx_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"golang.org/x/sys/unix"
)

func TestStageLocation(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")
	cases := []struct {
		parent, name, want string
	}{
		{"/tmp/p", ".foundry-x-ab", filepath.Join("/tmp/p", ".foundry-x-ab")},
		{"", ".foundry-x-ab", ".foundry-x-ab"},
		{"/tmp/p", "", "/tmp/p"},
		{"", "", ""},
	}
	for _, tc := range cases {
		got := fsx.StageLocation(tc.parent, tc.name)
		log.Assert("loc_"+tc.parent+"_"+tc.name, got == tc.want, tc.want, got)
	}
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestManualRemovalRemediation_RSK310(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")
	empty := fsx.ManualRemovalRemediation("")
	log.Assert("empty_cites_rsk", strings.Contains(empty, fsx.RSK310), true, empty)
	log.Assert("empty_never_auto", strings.Contains(empty, "never auto-deletes"), true, empty)
	log.Assert("empty_rm", strings.Contains(empty, "rm -rf"), true, empty)

	path := filepath.Join("/tmp/parent", ".foundry-demo-deadbeef")
	with := fsx.ManualRemovalRemediation(path)
	log.Assert("path_embedded", strings.Contains(with, path), true, with)
	log.Assert("path_rsk", strings.Contains(with, fsx.RSK310), true, with)
	log.Assert("path_rm_cmd", strings.Contains(with, "rm -rf "+path), true, with)
	log.Assert("path_mv", strings.Contains(with, "mv"), true, with)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestStage_Path(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "stage-path")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	want := filepath.Join(parentDir, stage.Name())
	tl.Phase("assert")
	tl.Assert("path", stage.Path() == want, want, stage.Path())
	tl.Assert("path_basename", strings.HasSuffix(stage.Path(), stage.Name()), true, stage.Path())
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

// failureClassCase drives one non-committed CommitResult class and checks
// StagePath + RSK-310 remediation + step log fields (REQ-130 acceptance).
type failureClassCase struct {
	name   string
	inject func(parentFd int, stageName, destName string) error
	// preDest creates a real destination child when non-nil (conflict path).
	preDest bool
	// wantClass is the expected classification.
	wantClass fsx.CommitClass
	// wantID is the expected Appendix D identifier.
	wantID diagnostic.Identifier
}

func TestCommitResult_FailureClassesReportStagePath(t *testing.T) {
	// Unit tests §1: each CommitResult / failure class includes stage path
	// when stage exists; remediation carries RSK-310.
	cases := []failureClassCase{
		{
			name: "uncommitted_eexist",
			inject: func(parentFd int, stageName, destName string) error {
				return unix.EEXIST
			},
			wantClass: fsx.ClassUncommitted,
			wantID:    diagnostic.IDFSDestinationExists,
		},
		{
			name: "uncommitted_enotsup",
			inject: func(parentFd int, stageName, destName string) error {
				return unix.ENOTSUP
			},
			wantClass: fsx.ClassUncommitted,
			wantID:    diagnostic.IDFSRenameUnsupported,
		},
		{
			name: "uncommitted_exdev",
			inject: func(parentFd int, stageName, destName string) error {
				return unix.EXDEV
			},
			wantClass: fsx.ClassUncommitted,
			wantID:    diagnostic.IDFSCommitFailed,
		},
		{
			name: "ambiguous_false_success",
			inject: func(parentFd int, stageName, destName string) error {
				return nil // false success without rename
			},
			wantClass: fsx.ClassAmbiguous,
			wantID:    diagnostic.IDFSCommitAmbiguous,
		},
		{
			name: "conflict_preexisting_dest",
			inject: func(parentFd int, stageName, destName string) error {
				return unix.EEXIST
			},
			preDest:   true,
			wantClass: fsx.ClassConflict, // dest present different id → conflict
			wantID:    diagnostic.IDFSDestinationExists,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log, tl, ml := newBridge(t)
			tl.Phase("arrange")
			parent, parentDir := arrangeParent(t, log, "preserve-"+tc.name)
			if tc.preDest {
				if err := os.Mkdir(filepath.Join(parentDir, "proj"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			stage, err := fsx.CreateStage(parent, "proj")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stage.Close() })
			stageName := stage.Name()
			wantPath := filepath.Join(parentDir, stageName)
			restore := fsx.SetExclusiveRenameForTest(t, tc.inject)
			t.Cleanup(restore)
			tl.PhaseEnd("arrange", testutil.OutcomeOK)

			tl.Phase("act")
			res := fsx.Commit(stage)
			tl.PhaseEnd("act", testutil.OutcomeOK)

			tl.Phase("assert")
			tl.Assert("class", res.Class == tc.wantClass, tc.wantClass, res.Class)
			tl.Assert("error_id", res.ErrorID() == tc.wantID, tc.wantID, res.ErrorID())
			tl.Assert("preserved", res.Preserved(), true, res.Preserved())
			tl.Assert("stage_name", res.StageName == stageName, stageName, res.StageName)
			tl.Assert("stage_path", res.StagePath == wantPath, wantPath, res.StagePath)
			tl.Assert("not_committed", !res.Committed(), false, res.Committed())

			// Stage still on disk.
			exists, _, err := parent.ChildLookup(stageName)
			if err != nil {
				t.Fatal(err)
			}
			tl.Assert("on_disk", exists, true, exists)

			// Error remediation: RSK-310 + stage path + rm -rf.
			fe, ok := diagnostic.AsFoundryError(res.Err)
			if !ok {
				t.Fatalf("err not FoundryError: %T %v", res.Err, res.Err)
			}
			rem := fe.Remediation()
			tl.Assert("rem_rsk310", strings.Contains(rem, fsx.RSK310), true, rem)
			tl.Assert("rem_stage_path", strings.Contains(rem, wantPath) || strings.Contains(rem, stageName), true, rem)
			tl.Assert("rem_manual_rm", strings.Contains(rem, "rm -rf") || strings.Contains(rem, "never auto-deletes"), true, rem)

			// PreserveReport hook.
			pr := res.Report()
			tl.Assert("report_path", pr.StagePath == wantPath, wantPath, pr.StagePath)
			tl.Assert("report_name", pr.StageName == stageName, stageName, pr.StageName)
			tl.Assert("report_class", pr.Class == string(tc.wantClass), string(tc.wantClass), pr.Class)
			tl.Assert("report_id", pr.ErrorID == string(tc.wantID), string(tc.wantID), pr.ErrorID)
			tl.Assert("report_rsk", pr.RiskID == fsx.RSK310, fsx.RSK310, pr.RiskID)
			tl.Assert("report_rem_rsk", strings.Contains(pr.Remediation, fsx.RSK310), true, pr.Remediation)
			tl.Assert("report_exit", pr.Exit == res.Exit, res.Exit, pr.Exit)

			// Step logger: outcome class, stage basename, remediation/rsk id.
			tl.Assert("log_class", ml.DetailContains("class="+string(tc.wantClass)), true, true)
			tl.Assert("log_basename", ml.DetailContains(stageName), true, true)
			tl.Assert("log_id", ml.DetailContains("id="+string(tc.wantID)) || ml.DetailContains(string(tc.wantID)), true, true)
			tl.Assert("log_rsk", ml.DetailContains("rsk="+fsx.RSK310) || ml.DetailContains(fsx.RSK310), true, true)
			tl.Assert("log_stage_path", ml.DetailContains("stage_path=") || ml.DetailContains(wantPath), true, true)
			tl.PhaseEnd("assert", testutil.OutcomeOK)
		})
	}
}

func TestCommit_Success_NoPreserveRemediation(t *testing.T) {
	// Committed stages are consumed; Preserved() is false; StagePath still
	// names the pre-rename entry for logs but Report() omits RSK-310.
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "preserve-ok")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	stageName := stage.Name()
	wantPath := filepath.Join(parentDir, stageName)

	res := fsx.Commit(stage)
	tl.Phase("assert")
	tl.Assert("committed", res.Committed(), true, res.Committed())
	tl.Assert("not_preserved", !res.Preserved(), false, res.Preserved())
	tl.Assert("stage_path_set", res.StagePath == wantPath, wantPath, res.StagePath)
	pr := res.Report()
	tl.Assert("report_no_rsk", pr.RiskID == "", "", pr.RiskID)
	tl.Assert("report_no_rem", pr.Remediation == "", "", pr.Remediation)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestSentinelOutsideStage_SurvivesAllOps(t *testing.T) {
	// Unit tests §3: sentinel file outside the stage survives create, write,
	// failed commit, and Close — no recursive cleanup footguns (REQ-130).
	log, tl, _ := newBridge(t)
	tl.Phase("arrange")
	parent, parentDir := arrangeParent(t, log, "sentinel-surv")
	sentinelPath := filepath.Join(parentDir, "SENTINEL-outside-stage.txt")
	sentinelBody := []byte("do-not-touch\n")
	if err := os.WriteFile(sentinelPath, sentinelBody, 0o600); err != nil {
		t.Fatal(err)
	}
	// Also a sibling directory that must not be scavenged.
	sibling := filepath.Join(parentDir, "unrelated-sibling")
	if err := os.Mkdir(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "keep.txt"), []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	assertSentinel := func(label string) {
		t.Helper()
		data, err := os.ReadFile(sentinelPath)
		if err != nil {
			t.Fatalf("%s: sentinel missing: %v", label, err)
		}
		if string(data) != string(sentinelBody) {
			t.Fatalf("%s: sentinel mutated: %q", label, data)
		}
		if _, err := os.Lstat(filepath.Join(sibling, "keep.txt")); err != nil {
			t.Fatalf("%s: sibling scavenged: %v", label, err)
		}
	}

	tl.Phase("create_stage")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	assertSentinel("after_create")
	tl.PhaseEnd("create_stage", testutil.OutcomeOK)

	tl.Phase("write")
	if err := stage.Writer().WriteFile("inside.txt", "0644", []byte("in\n")); err != nil {
		t.Fatal(err)
	}
	if err := stage.Writer().MkdirAll("deep/dir", "0755"); err != nil {
		t.Fatal(err)
	}
	assertSentinel("after_write")
	tl.PhaseEnd("write", testutil.OutcomeOK)

	tl.Phase("failed_commit")
	restore := fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
		return unix.EEXIST
	})
	t.Cleanup(restore)
	res := fsx.Commit(stage)
	if res.Committed() {
		t.Fatal("expected non-committed result")
	}
	assertSentinel("after_failed_commit")
	// Stage preserved.
	exists, _, err := parent.ChildLookup(stage.Name())
	if err != nil || !exists {
		t.Fatalf("stage not preserved: exists=%v err=%v", exists, err)
	}
	tl.Assert("stage_path_reported", res.StagePath != "", "non-empty", res.StagePath)
	tl.PhaseEnd("failed_commit", testutil.OutcomeOK)

	tl.Phase("close")
	if err := stage.Close(); err != nil {
		t.Fatal(err)
	}
	assertSentinel("after_close")
	// Close must not delete stage entry either.
	if _, err := os.Lstat(filepath.Join(parentDir, res.StageName)); err != nil {
		t.Fatalf("stage removed after Close: %v", err)
	}
	assertSentinel("final")
	tl.PhaseEnd("close", testutil.OutcomeOK)
}

func TestPreserveReport_CancelStyleHook(t *testing.T) {
	// Cancellation is classified by generate, but fsx still exposes Stage.Path
	// and ManualRemovalRemediation so cancel/render/tool/verify failure
	// reporting can attach the same RSK-310 text (Section 31.6 / 31.9).
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "cancel-hook")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	stagePath := stage.Path()
	want := filepath.Join(parentDir, stage.Name())
	tl.Phase("assert")
	tl.Assert("path", stagePath == want, want, stagePath)
	rem := fsx.ManualRemovalRemediation(stagePath)
	tl.Assert("rsk", strings.Contains(rem, fsx.RSK310), true, rem)
	tl.Assert("path_in_rem", strings.Contains(rem, stagePath), true, rem)

	// Simulate a cancel-shaped CommitResult (generate would set this without
	// calling Commit when cancelled before rename).
	sim := fsx.CommitResult{
		Class:     fsx.ClassUncommitted,
		Exit:      diagnostic.ExitCancelled,
		StageName: stage.Name(),
		StagePath: stagePath,
		Message:   "cancelled before commit; stage preserved",
	}
	// Attach remediation as generate would via report.
	sim.Err = diagnostic.New(diagnostic.IDFSCommitFailed, sim.Message, diagnostic.PathLocation(stagePath)).
		WithRemediation(fsx.ManualRemovalRemediation(stagePath))
	// Note: cancel exit is 130, not commit_failed — generate owns that mapping.
	// Here we only prove the reporting hooks carry stage path + RSK-310.
	pr := sim.Report()
	tl.Assert("sim_preserved", sim.Preserved(), true, true)
	tl.Assert("sim_path", pr.StagePath == stagePath, stagePath, pr.StagePath)
	tl.Assert("sim_rsk", pr.RiskID == fsx.RSK310, fsx.RSK310, pr.RiskID)
	tl.Assert("sim_rem", strings.Contains(pr.Remediation, "rm -rf"), true, pr.Remediation)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestCommit_StepLogger_ClassBasenameRemediationID(t *testing.T) {
	// Unit tests §4: step logger records outcome class, stage basename,
	// remediation id (Appendix D) and RSK-310.
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "log-rsk")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	stageName := stage.Name()

	restore := fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
		return unix.EEXIST
	})
	t.Cleanup(restore)

	res := fsx.Commit(stage)
	tl.Phase("assert")
	tl.Assert("class_uncommitted", res.Class == fsx.ClassUncommitted || res.Class == fsx.ClassConflict,
		"uncommitted|conflict", res.Class)
	tl.Assert("log_classified", ml.Has("commit", "classified", "fail"), true, true)
	tl.Assert("log_class", ml.DetailContains("class="), true, true)
	tl.Assert("log_stage_basename", ml.DetailContains(stageName), true, true)
	tl.Assert("log_id", ml.DetailContains("id="), true, true)
	tl.Assert("log_rsk310", ml.DetailContains("rsk="+fsx.RSK310), true, true)
	tl.Assert("log_stage_path", ml.DetailContains("stage_path="), true, true)
	// No home path leakage.
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
		for _, e := range ml.Entries() {
			if strings.Contains(e.Detail, home) {
				t.Errorf("home path in log: %q", e.Detail)
			}
		}
	}
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}
