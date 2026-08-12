//go:build unix

package fsx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestAcquireParent_HappyPath(t *testing.T) {
	log, tl, ml := newBridge(t)
	tl.Phase("arrange")
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	parentDir := privateParent(t, root, "happy-parent")
	dest := filepath.Join(parentDir, "proj")
	tl.Fixture("parent", filepath.Base(parentDir))
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
	if err != nil {
		tl.Fail("acquire", err.Error())
	}
	t.Cleanup(func() { _ = parent.Close() })
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	tl.Assert("basename", parent.Basename() == "proj", "proj", parent.Basename())
	tl.Assert("identity_nonzero", !parent.Identity().IsZero(), true, !parent.Identity().IsZero())
	tl.Assert("dirfd_open", parent.DirFD() >= 0, ">=0", parent.DirFD())
	tl.Assert("walk_logged", ml.Has("parent_walk", "acquired", "pass"), true, ml.Has("parent_walk", "acquired", "pass"))
	tl.Assert("custody_logged", ml.Has("custody", "ok", "pass"), true, ml.Has("custody", "ok", "pass"))
	tl.Assert("flags_in_log", ml.DetailContains("O_NOFOLLOW"), true, ml.DetailContains("O_NOFOLLOW"))
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestPreflight_AbsentDestination(t *testing.T) {
	log, tl, ml := newBridge(t)
	tl.Phase("arrange")
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	parentDir := privateParent(t, root, "preflight-parent")
	dest := filepath.Join(parentDir, "new-proj")
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	parent, err := fsx.Preflight(dest, fsx.PreflightOptions{Log: log})
	if err != nil {
		tl.Fail("preflight", err.Error())
	}
	t.Cleanup(func() { _ = parent.Close() })
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	tl.Assert("class_absent_log", ml.DetailContains("class=absent"), true, true)
	exists, err := parent.ChildExists("new-proj")
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("still_absent", !exists, false, exists)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestAcquireParent_SymlinkComponent_UnsafePath(t *testing.T) {
	log, tl, _ := newBridge(t)
	tl.Phase("arrange")
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	base := privateParent(t, root, "sym-base")
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(link, "proj")
	tl.Fixture("symlink_component", "link")
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	_, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	requireID(t, err, diagnostic.IDFSUnsafePath)
	tl.Assert("id", mustFoundryID(t, err) == diagnostic.IDFSUnsafePath, diagnostic.IDFSUnsafePath, mustFoundryID(t, err))
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestAcquireParent_MissingParent_ParentMissing(t *testing.T) {
	log, tl, _ := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	base := privateParent(t, root, "missing-base")
	dest := filepath.Join(base, "no-such-dir", "proj")

	_, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
	requireID(t, err, diagnostic.IDFSParentMissing)
	tl.Assert("id", true, diagnostic.IDFSParentMissing, mustFoundryID(t, err))
}

func TestAcquireParent_NonDirectoryParent_ParentMissing(t *testing.T) {
	log, tl, _ := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	base := privateParent(t, root, "file-parent-base")
	filePath := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(filePath, "proj")

	_, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
	requireID(t, err, diagnostic.IDFSParentMissing)
	tl.Assert("id", true, diagnostic.IDFSParentMissing, mustFoundryID(t, err))
}

func TestCustody_SharedWritableNonSticky_Rejected(t *testing.T) {
	log, tl, ml := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	shared := filepath.Join(root, "shared-ww")
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shared, 0o700) })
	dest := filepath.Join(shared, "proj")

	_, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
	requireID(t, err, diagnostic.IDFSNamespaceNotPrivate)
	tl.Assert("custody_fail_logged", ml.Has("custody", "mode", "fail") || ml.Has("custody", "owner", "fail"),
		true, true)
	tl.Assert("id_in_detail", ml.DetailContains(string(diagnostic.IDFSNamespaceNotPrivate)), true, true)
}

func TestCustody_StickyWorldWritable_Permitted(t *testing.T) {
	// /tmp is typically 1777; walk through sticky world-writable must pass.
	log, tl, ml := newBridge(t)
	tmp, err := os.MkdirTemp("/tmp", "fsx-sticky-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })
	if err := os.Chmod(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(tmp, "proj")

	parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
	if err != nil {
		tl.Fail("sticky_tmp", err.Error())
	}
	t.Cleanup(func() { _ = parent.Close() })
	tl.Assert("acquired", parent != nil && err == nil, true, err)
	tl.Assert("authored_path", parent.AuthoredPath() != "", true, parent.AuthoredPath())
	tl.Assert("custody_pass", ml.Has("custody", "ok", "pass"), true, ml.Has("custody", "ok", "pass"))
}

func TestCustody_OwnerPrivate_Matrix(t *testing.T) {
	// Owner=euid with private modes (0700, 0750, 0755) must pass.
	// 0775 (group-writable non-sticky) must fail when group write is set.
	cases := []struct {
		name    string
		mode    os.FileMode
		wantErr diagnostic.Identifier // empty = success
	}{
		{"mode_0700", 0o700, ""},
		{"mode_0750", 0o750, ""},
		{"mode_0755", 0o755, ""},
		{"mode_0770", 0o770, diagnostic.IDFSNamespaceNotPrivate}, // group write, no sticky
		{"mode_0777", 0o777, diagnostic.IDFSNamespaceNotPrivate},
		// Go FileMode sticky is ModeSticky (not raw 0o1000); os.Chmod maps it to S_ISVTX.
		{"mode_1777_sticky", 0o777 | os.ModeSticky, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			log, tl, _ := newBridge(t)
			root := t.TempDir()
			_ = os.Chmod(root, 0o700)
			dir := filepath.Join(root, "mode-dir")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, tc.mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			dest := filepath.Join(dir, "proj")

			parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
			if tc.wantErr == "" {
				if err != nil {
					tl.Fail("unexpected", err.Error())
				}
				_ = parent.Close()
				tl.Assert("mode_ok", true, fmtMode(tc.mode), fmtMode(tc.mode))
				return
			}
			requireID(t, err, tc.wantErr)
			tl.Assert("mode_reject", true, tc.wantErr, mustFoundryID(t, err))
		})
	}
}

func TestNormalize_RejectsDotDotAndTilde(t *testing.T) {
	cases := []string{
		"/tmp/../etc/proj",
		"~/proj",
		"$HOME/proj",
		".",
		"",
	}
	for _, dest := range cases {
		dest := dest
		t.Run(dest, func(t *testing.T) {
			_, err := fsx.AcquireParent(dest, fsx.PreflightOptions{})
			if err == nil {
				t.Fatalf("expected error for %q", dest)
			}
			id := mustFoundryID(t, err)
			if id != diagnostic.IDFSUnsafePath && id != diagnostic.IDFSParentMissing {
				t.Fatalf("id=%s for %q", id, dest)
			}
		})
	}
}

func TestParentSwap_RetainedHandleAndReobserve(t *testing.T) {
	log, tl, ml := newBridge(t)
	tl.Phase("arrange")
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	container := privateParent(t, root, "swap-container")
	parentDir := filepath.Join(container, "parent-a")
	if err := os.Mkdir(parentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(parentDir, "proj")
	tl.PhaseEnd("arrange", testutil.OutcomeOK)

	tl.Phase("act")
	parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
	if err != nil {
		tl.Fail("acquire", err.Error())
	}
	t.Cleanup(func() { _ = parent.Close() })
	retained := parent.Identity()

	// Swap pathname of the parent directory after acquisition.
	swapped := filepath.Join(container, "parent-b-swapped")
	if err := os.Rename(parentDir, swapped); err != nil {
		t.Fatal(err)
	}
	tl.Step("rename_parent", testutil.OutcomeOK, "parent-a -> parent-b-swapped")

	// Reobserve must fail closed (fs.parent_moved).
	reErr := parent.Reobserve()
	requireID(t, reErr, diagnostic.IDFSParentMoved)

	// Retained handle still usable for descriptor-relative lookup.
	exists, err := parent.ChildExists("proj")
	if err != nil {
		tl.Fail("child_via_fd", err.Error())
	}
	tl.Assert("dest_still_absent_via_fd", !exists, false, exists)
	tl.Assert("identity_stable", parent.Identity().Equal(retained), retained.String(), parent.Identity().String())
	tl.Assert("reobserve_fail_logged", ml.Has("parent_reobserve", "identity", "fail") || ml.Has("parent_reobserve", "open_authored", "fail"),
		true, true)
	tl.PhaseEnd("act", testutil.OutcomeOK)
}

func TestReobserve_StableParent_Passes(t *testing.T) {
	log, tl, ml := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	parentDir := privateParent(t, root, "stable-parent")
	dest := filepath.Join(parentDir, "proj")

	parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })

	if err := parent.Reobserve(); err != nil {
		tl.Fail("reobserve", err.Error())
	}
	tl.Assert("reobserve_pass", ml.Has("parent_reobserve", "identity", "pass"), true, true)
}
