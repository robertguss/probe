//go:build hostile && unix

package fsx_test

// Permanent REQ-213 injection probes that require export_test hooks
// (SetExclusiveRenameForTest). The integration/hostile/fsx suite exercises the
// same Section 31.9 outcomes via public APIs; this file covers the errno
// injection paths (ENOTSUP / EXDEV / false-success ambiguous) against the
// real Commit() pipeline with sentinel survival.
//
// Run with: go test -tags=hostile -count=1 ./internal/fsx/ -run Hostile

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestHostile_InjectedENOTSUP_StagePreserved(t *testing.T) {
	log, tl, ml := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "hostile-enotsup")
	sentinel := filepath.Join(parentDir, "SENTINEL.txt")
	if err := os.WriteFile(sentinel, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	stageName := stage.Name()
	_ = stage.Writer().WriteFile("u.txt", "0644", []byte("u\n"))

	restore := fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
		return unix.ENOTSUP
	})
	t.Cleanup(restore)

	res := fsx.Commit(stage)
	tl.Assert("class", res.Class == fsx.ClassUncommitted, fsx.ClassUncommitted, res.Class)
	tl.Assert("id", res.ErrorID() == diagnostic.IDFSRenameUnsupported,
		diagnostic.IDFSRenameUnsupported, res.ErrorID())
	tl.Assert("exit_1", res.Exit == diagnostic.ExitFailure, diagnostic.ExitFailure, res.Exit)
	exists, _, err := parent.ChildLookup(stageName)
	if err != nil || !exists {
		t.Fatalf("stage preserved: exists=%v err=%v", exists, err)
	}
	if _, err := os.ReadFile(sentinel); err != nil {
		t.Fatal("sentinel scavenged")
	}
	tl.Assert("log_errno", ml.DetailContains("errno="), true, true)
	tl.Assert("stage_path", res.StagePath != "", "non-empty", res.StagePath)
}

func TestHostile_InjectedEXDEV_StagePreserved(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "hostile-exdev")
	sentinel := filepath.Join(parentDir, "SENTINEL.txt")
	if err := os.WriteFile(sentinel, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
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

	res := fsx.Commit(stage)
	tl.Assert("class", res.Class == fsx.ClassUncommitted, fsx.ClassUncommitted, res.Class)
	tl.Assert("id", res.ErrorID() == diagnostic.IDFSCommitFailed,
		diagnostic.IDFSCommitFailed, res.ErrorID())
	exists, _, err := parent.ChildLookup(stageName)
	if err != nil || !exists {
		t.Fatalf("stage preserved: exists=%v err=%v", exists, err)
	}
	if _, err := os.ReadFile(sentinel); err != nil {
		t.Fatal("sentinel scavenged")
	}
}

func TestHostile_InjectedAmbiguous_StagePreserved(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "hostile-ambig")
	sentinel := filepath.Join(parentDir, "SENTINEL.txt")
	if err := os.WriteFile(sentinel, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	stageName := stage.Name()

	restore := fsx.SetExclusiveRenameForTest(t, func(parentFd int, stageName, destName string) error {
		// False success without rename.
		return nil
	})
	t.Cleanup(restore)

	res := fsx.Commit(stage)
	tl.Assert("class", res.Class == fsx.ClassAmbiguous, fsx.ClassAmbiguous, res.Class)
	tl.Assert("id", res.ErrorID() == diagnostic.IDFSCommitAmbiguous,
		diagnostic.IDFSCommitAmbiguous, res.ErrorID())
	exists, _, err := parent.ChildLookup(stageName)
	if err != nil || !exists {
		t.Fatalf("stage preserved: exists=%v err=%v", exists, err)
	}
	if exists, _, _ := parent.ChildLookup("proj"); exists {
		t.Fatal("dest must not exist on false-success ambiguous")
	}
	if _, err := os.ReadFile(sentinel); err != nil {
		t.Fatal("sentinel scavenged")
	}
	tl.Assert("stage_path", res.StagePath != "", "non-empty", res.StagePath)
	_ = testutil.OutcomeOK // step logger used via arrangeParent/newBridge
}
