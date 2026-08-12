//go:build hostile && unix

package hostilefsx_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	hostilefsx "github.com/robertguss/go-foundry-cli/integration/hostile/fsx"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestHostileFSX_ReadOnlyDestination_FailClosed attempts stage creation
// under a read-only destination parent and asserts fail-closed
// classification with no partial writes (bead go-foundry-cli-wet.4.2 /
// REQ-213).
//
// FOUNDRY_FSX_READONLY_ROOT, when set, points at a real read-only mount
// (e.g. via scripts/fsx-mount-matrix.sh or an explicit `mount -o ro`/loop
// device) for full-fidelity EROFS coverage. When unset — the default on a
// developer macOS machine without sudo mount access — this falls back to a
// chmod 0555 (read+traverse, no write) parent directory. That still drives
// stage creation's mkdirat through the identical non-EEXIST failure branch
// a real read-only mount's EROFS would (Section 31.5), just via EACCES
// instead; the classification and fail-closed contract are the same either
// way. See docs/dev/testing.md for the documented policy on this gap.
func TestHostileFSX_ReadOnlyDestination_FailClosed(t *testing.T) {
	log, tl, pl := hostilefsx.NewHarness(t, "readonly")
	tl.Phase("arrange")

	var parentDir string
	var sentinel string
	var body []byte
	if root := os.Getenv("FOUNDRY_FSX_READONLY_ROOT"); root != "" {
		parentDir = hostilefsx.PrivateParent(t, root, "readonly-parent-"+t.Name())
		tl.Step("mode", testutil.OutcomeOK, "real read-only mount via FOUNDRY_FSX_READONLY_ROOT="+root)
		sentinel, body = hostilefsx.PlantSentinel(t, parentDir)
	} else {
		root := t.TempDir()
		parentDir = hostilefsx.PrivateParent(t, root, "readonly-parent")
		sentinel, body = hostilefsx.PlantSentinel(t, parentDir)
		if err := os.Chmod(parentDir, 0o555); err != nil {
			t.Fatal(err)
		}
		// TempDir cleanup needs write access back on the parent; restore
		// before the test harness tears down.
		t.Cleanup(func() { _ = os.Chmod(parentDir, 0o700) })
		tl.Step("mode", testutil.OutcomeInfo,
			"no real read-only mount (FOUNDRY_FSX_READONLY_ROOT unset); "+
				"using chmod 0555 lightweight fallback per docs/dev/testing.md")
	}
	dest := filepath.Join(parentDir, "proj")
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	if err == nil {
		_ = txn.Close()
		t.Fatal("stage creation on a read-only destination parent unexpectedly succeeded")
	}
	var fe *diagnostic.FoundryError
	id := diagnostic.Identifier("")
	if e, ok := diagnostic.AsFoundryError(err); ok {
		fe = e
		id = fe.ID()
	}
	// Either the pre-stage parent acquisition or CreateStage's mkdirat can be
	// the failure point depending on exactly which permission bit blocks
	// first; both are fail-closed fs.* identifiers (never a silent success).
	failClosed := id == diagnostic.IDFSCommitFailed || id == diagnostic.IDFSUnsafePath
	tl.Assert("fail_closed_id", failClosed, "fs.commit_failed|fs.unsafe_path", id)

	entries, readErr := os.ReadDir(parentDir)
	if readErr != nil {
		t.Fatalf("ReadDir(parentDir): %v", readErr)
	}
	var stageDebris []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".foundry-") {
			stageDebris = append(stageDebris, e.Name())
		}
	}
	tl.Assert("no_stage_debris", len(stageDebris) == 0, "[]", stageDebris)

	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatalf("destination %q unexpectedly created on read-only parent", dest)
	}
	hostilefsx.AssertSentinelAlive(t, sentinel, body)

	pl.Record(hostilefsx.ProbeEntry{
		FS: "readonly", Kernel: hostilefsx.KernelVersion(),
		Probe: "readonly_destination", Step: "begin", Outcome: "pass",
		Detail: "id=" + string(id),
	})
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}
