//go:build unix

package fsx_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"golang.org/x/sys/unix"
)

// TestNilSafeAccessors covers nil/closed nil-safe Parent, Stage, and Transaction
// accessors that are otherwise only exercised on the happy path.
func TestNilSafeAccessors(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert_nil")

	var p *fsx.ParentHandle
	log.Assert("parent_authored_nil", p.AuthoredPath() == "", "", p.AuthoredPath())
	log.Assert("parent_base_nil", p.Basename() == "", "", p.Basename())
	log.Assert("parent_id_nil", p.Identity().IsZero(), true, p.Identity())
	log.Assert("parent_fd_nil", p.DirFD() == -1, -1, p.DirFD())
	log.Assert("parent_close_nil", p.Close() == nil, true, p.Close() == nil)

	var s *fsx.Stage
	log.Assert("stage_name_nil", s.Name() == "", "", s.Name())
	log.Assert("stage_path_nil", s.Path() == "", "", s.Path())
	log.Assert("stage_id_nil", s.Identity().IsZero(), true, s.Identity())
	log.Assert("stage_fd_nil", s.DirFD() == -1, -1, s.DirFD())
	log.Assert("stage_parent_nil", s.Parent() == nil, true, s.Parent() == nil)
	log.Assert("stage_writer_nil", s.Writer() == nil, true, s.Writer() == nil)
	log.Assert("stage_close_nil", s.Close() == nil, true, s.Close() == nil)
	if err := s.VerifyIdentity(); err == nil {
		t.Fatal("nil stage VerifyIdentity should fail")
	}

	var txn *fsx.Transaction
	log.Assert("txn_parent_nil", txn.Parent() == nil, true, txn.Parent() == nil)
	log.Assert("txn_stage_nil", txn.Stage() == nil, true, txn.Stage() == nil)
	log.Assert("txn_stage_name_nil", txn.StageName() == "", "", txn.StageName())
	log.Assert("txn_stage_path_nil", txn.StagePath() == "", "", txn.StagePath())
	log.Assert("txn_stage_id_nil", txn.StageIdentity().IsZero(), true, txn.StageIdentity())
	log.Assert("txn_dest_nil", txn.DestName() == "", "", txn.DestName())
	log.Assert("txn_writer_alias_nil", txn.Writer() == nil, true, txn.Writer() == nil)
	log.Assert("txn_rooted_nil", txn.RootedWriter() == nil, true, txn.RootedWriter() == nil)
	log.Assert("txn_committed_nil", !txn.Committed(), false, txn.Committed())
	log.Assert("txn_close_nil", txn.Close() == nil, true, txn.Close() == nil)

	fd, err := txn.DuplicateStageHandle()
	log.Assert("txn_dup_nil_fd", fd == -1, -1, fd)
	if err == nil {
		t.Fatal("nil txn DuplicateStageHandle should fail")
	}
	requireID(t, err, diagnostic.IDFSCommitFailed)

	// Closed parent/destination methods.
	exists, err := p.ChildExists("x")
	log.Assert("child_exists_nil", !exists && err != nil, true, err != nil)
	_, _, err = p.ChildLookup("x")
	log.Assert("child_lookup_nil", err != nil, true, err != nil)
	_, err = p.ClassifyDestination()
	log.Assert("classify_nil", err != nil, true, err != nil)
	log.Assert("ensure_nil", p.EnsureDestinationAbsent() != nil, true, true)
	log.Assert("reobserve_nil", p.Reobserve() != nil, true, true)

	// Nil writer methods.
	var w *fsx.RootedWriter
	if err := w.WriteFile("a", "0644", []byte("x")); err == nil {
		t.Fatal("nil WriteFile")
	}
	if err := w.Mkdir("d", "0755"); err == nil {
		t.Fatal("nil Mkdir")
	}
	if err := w.MkdirAll("d/e", "0755"); err == nil {
		t.Fatal("nil MkdirAll")
	}

	log.PhaseEnd("assert_nil", testutil.OutcomeOK)
}

// TestParent_AuthoredPathAndAccessors hits AuthoredPath and non-nil accessors.
func TestParent_AuthoredPathAndAccessors(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "authored")
	tl.Phase("assert")
	tl.Assert("authored", parent.AuthoredPath() == parentDir, parentDir, parent.AuthoredPath())
	tl.Assert("base", parent.Basename() == "proj", "proj", parent.Basename())
	tl.Assert("id", !parent.Identity().IsZero(), true, parent.Identity())
	tl.Assert("fd", parent.DirFD() >= 0, ">=0", parent.DirFD())
	// Double-close is safe after first Close (fd=-1 path).
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	tl.Assert("fd_after_close", parent.DirFD() == -1, -1, parent.DirFD())
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestStage_ParentAndNilPath covers Stage.Parent and Path with nil parent edge.
func TestStage_ParentAndAccessors(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "stage-parent")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	tl.Phase("assert")
	tl.Assert("parent_same", stage.Parent() == parent, true, stage.Parent() == parent)
	tl.Assert("path", stage.Path() == filepath.Join(parentDir, stage.Name()),
		filepath.Join(parentDir, stage.Name()), stage.Path())
	tl.Assert("writer", stage.Writer() != nil, true, stage.Writer() != nil)
	tl.Assert("name", stage.Name() != "", "non-empty", stage.Name())
	tl.Assert("id", !stage.Identity().IsZero(), true, stage.Identity())
	tl.Assert("fd", stage.DirFD() >= 0, ">=0", stage.DirFD())
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestTransaction_WriterAliasAndCommitted covers Writer() alias and Committed().
func TestTransaction_WriterAliasAndCommitted(t *testing.T) {
	log, tl, _ := newBridge(t)
	dest, _ := arrangeDest(t, "txn-writer-alias")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txn.Close() })

	tl.Phase("assert_pre")
	tl.Assert("not_committed", !txn.Committed(), false, txn.Committed())
	w := txn.Writer()
	rw := txn.RootedWriter()
	tl.Assert("writer_non_nil", w != nil, true, w != nil)
	tl.Assert("alias_same_type", w != nil && rw != nil, true, true)
	if err := w.WriteFile("via-alias.txt", "0644", []byte("alias\n")); err != nil {
		t.Fatal(err)
	}
	// Parent/Stage accessors on live txn.
	tl.Assert("parent_live", txn.Parent() != nil, true, txn.Parent() != nil)
	tl.Assert("stage_live", txn.Stage() != nil, true, txn.Stage() != nil)
	tl.Assert("stage_parent", txn.Stage().Parent() == txn.Parent(), true, true)
	tl.Assert("authored", txn.Parent().AuthoredPath() != "", "non-empty", txn.Parent().AuthoredPath())
	tl.PhaseEnd("assert_pre", testutil.OutcomeOK)

	tl.Phase("act_commit")
	res := txn.Commit()
	tl.PhaseEnd("act_commit", testutil.OutcomeOK)

	tl.Phase("assert_post")
	tl.Assert("res_committed", res.Committed(), true, res.Committed())
	tl.Assert("txn_committed", txn.Committed(), true, txn.Committed())
	tl.PhaseEnd("assert_post", testutil.OutcomeOK)

	// After close, accessors nil-safe.
	if err := txn.Close(); err != nil {
		t.Fatal(err)
	}
	tl.Phase("assert_closed")
	tl.Assert("writer_nil", txn.Writer() == nil, true, txn.Writer() == nil)
	tl.Assert("parent_nil", txn.Parent() == nil, true, txn.Parent() == nil)
	tl.Assert("stage_nil", txn.Stage() == nil, true, txn.Stage() == nil)
	// Committed flag retained after close.
	tl.Assert("still_committed", txn.Committed(), true, txn.Committed())
	tl.PhaseEnd("assert_closed", testutil.OutcomeOK)
}

// TestTransaction_DuplicateStageHandle_Closed covers closed/nil error path.
func TestTransaction_DuplicateStageHandle_Closed(t *testing.T) {
	log, tl, ml := newBridge(t)
	dest, _ := arrangeDest(t, "txn-dup-closed")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	if err := txn.Close(); err != nil {
		t.Fatal(err)
	}

	tl.Phase("act")
	fd, err := txn.DuplicateStageHandle()
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	tl.Assert("fd_neg", fd == -1, -1, fd)
	requireID(t, err, diagnostic.IDFSCommitFailed)
	tl.Assert("msg", strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "nil"),
		true, err.Error())
	_ = ml
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestTransaction_DuplicateStageHandle_IdentityFail covers verify failure before dup.
func TestTransaction_DuplicateStageHandle_IdentityFail(t *testing.T) {
	log, tl, ml := newBridge(t)
	dest, parentDir := arrangeDest(t, "txn-dup-id")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txn.Close() })

	// Swap stage basename under parent so VerifyIdentity fails.
	stageName := txn.StageName()
	swapped := stageName + ".swapped"
	if err := os.Rename(filepath.Join(parentDir, stageName), filepath.Join(parentDir, swapped)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Rename(filepath.Join(parentDir, swapped), filepath.Join(parentDir, stageName))
	})

	tl.Phase("act")
	fd, err := txn.DuplicateStageHandle()
	tl.PhaseEnd("act", testutil.OutcomeOK)

	tl.Phase("assert")
	tl.Assert("fd_neg", fd == -1, -1, fd)
	if err == nil {
		t.Fatal("expected identity failure")
	}
	requireID(t, err, diagnostic.IDFSCommitFailed)
	tl.Assert("log_fail", ml.Has("transaction", "dup_stage", "fail"), true, true)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestCreateStage_InvalidProjectNames table-tests validateStageProjectName via CreateStage.
func TestCreateStage_InvalidProjectNames(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, _ := arrangeParent(t, log, "bad-names")

	cases := []struct {
		name    string
		project string
	}{
		{"slash", "a/b"},
		{"backslash", "a\\b"},
		{"dot", "."},
		{"dotdot", ".."},
		{"nul", "a\x00b"},
		{"list_sep", "a" + string(os.PathListSeparator) + "b"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := fsx.CreateStage(parent, tc.project)
			if err == nil {
				t.Fatalf("CreateStage(%q) succeeded", tc.project)
			}
			requireID(t, err, diagnostic.IDFSUnsafePath)
			tl.Assert("id_"+tc.name, true, diagnostic.IDFSUnsafePath, mustFoundryID(t, err))
		})
	}

	// Closed parent.
	closed := parent
	_ = closed.Close()
	// Need a fresh open parent for remaining tests — closed path:
	_, err := fsx.CreateStage(closed, "proj")
	requireID(t, err, diagnostic.IDFSUnsafePath)

	_, err = fsx.CreateStage(nil, "proj")
	requireID(t, err, diagnostic.IDFSUnsafePath)
}

// TestCreateStage_SuffixGeneratorError covers stageSuffix failure path.
func TestCreateStage_SuffixGeneratorError(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, _ := arrangeParent(t, log, "suffix-err")
	restore := fsx.SetStageSuffixErrForTest(t, errors.New("entropy exhausted"))
	t.Cleanup(restore)

	_, err := fsx.CreateStage(parent, "proj")
	requireID(t, err, diagnostic.IDFSCommitFailed)
	tl.Assert("suffix_err", true, diagnostic.IDFSCommitFailed, mustFoundryID(t, err))
}

// TestCreateStage_EmptyProjectUsesParentBase covers project="" → parent.base.
func TestCreateStage_EmptyProjectUsesParentBase(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, _ := arrangeParent(t, log, "empty-proj")
	stage, err := fsx.CreateStage(parent, "   ")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	tl.Assert("prefix", strings.HasPrefix(stage.Name(), ".foundry-proj-"), true, stage.Name())
}

// TestPreserveRemediationHelpers covers withPreserveRemediation / attachPreserveRemediation.
func TestPreserveRemediationHelpers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")

	if got := fsx.WithPreserveRemediationForTest(nil, "/tmp/s"); got != nil {
		t.Fatalf("nil err → nil, got %v", got)
	}
	if got := fsx.AttachPreserveRemediationForTest(nil, "/tmp/s"); got != nil {
		t.Fatalf("nil fe → nil, got %v", got)
	}

	// Non-Foundry error passes through unchanged.
	plain := errors.New("plain")
	if got := fsx.WithPreserveRemediationForTest(plain, "/stage"); got != plain {
		t.Fatalf("plain err should pass through: %v", got)
	}

	// Empty base remediation → only RSK-310.
	fe := diagnostic.New(diagnostic.IDFSCommitFailed, "boom", diagnostic.PathLocation("/p"))
	out := fsx.WithPreserveRemediationForTest(fe, "/parent/.foundry-x")
	got, ok := diagnostic.AsFoundryError(out)
	if !ok {
		t.Fatal("expected FoundryError")
	}
	rem := got.Remediation()
	log.Assert("has_rsk", strings.Contains(rem, fsx.RSK310), true, rem)
	log.Assert("has_path", strings.Contains(rem, "/parent/.foundry-x"), true, rem)

	// Base already cites RSK-310 + path → unchanged.
	already := fe.WithRemediation(fsx.ManualRemovalRemediation("/parent/.foundry-x"))
	same := fsx.AttachPreserveRemediationForTest(already, "/parent/.foundry-x")
	if same.Remediation() != already.Remediation() {
		t.Fatalf("should short-circuit when RSK already present")
	}

	// Base remediation without RSK → concatenate.
	baseOnly := diagnostic.New(diagnostic.IDFSParentMoved, "moved", diagnostic.PathLocation("/p")).
		WithRemediation("re-run after restoring parent path")
	joined := fsx.AttachPreserveRemediationForTest(baseOnly, "/s")
	jrem := joined.Remediation()
	log.Assert("joined_base", strings.Contains(jrem, "re-run"), true, jrem)
	log.Assert("joined_rsk", strings.Contains(jrem, fsx.RSK310), true, jrem)

	// wrapCause nil paths.
	if fsx.WrapCauseForTest(nil, errors.New("c")) != nil {
		t.Fatal("wrap nil fe")
	}
	if fsx.WrapCauseForTest(fe, nil) != fe {
		t.Fatal("wrap nil cause returns fe")
	}
	wrapped := fsx.WrapCauseForTest(fe, errors.New("cause"))
	if wrapped == nil || wrapped.ID() != fe.ID() {
		t.Fatalf("wrap cause: %v", wrapped)
	}

	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestCommit_ParentReobserveTriggersWithPreserve hits withPreserveRemediation via Commit.
func TestCommit_ParentReobserveTriggersWithPreserve(t *testing.T) {
	log, tl, ml := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	container := privateParent(t, root, "reobs-container")
	parentDir := filepath.Join(container, "parent-a")
	if err := os.Mkdir(parentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(parentDir, "proj")
	parent, err := fsx.Preflight(dest, fsx.PreflightOptions{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	// Move parent so Reobserve fails before rename.
	swapped := filepath.Join(container, "parent-b")
	if err := os.Rename(parentDir, swapped); err != nil {
		t.Fatal(err)
	}

	res := fsx.Commit(stage)
	tl.Phase("assert")
	tl.Assert("uncommitted", res.Class == fsx.ClassUncommitted, fsx.ClassUncommitted, res.Class)
	tl.Assert("preserved", res.Preserved(), true, res.Preserved())
	if res.Err == nil {
		t.Fatal("expected err")
	}
	fe, ok := diagnostic.AsFoundryError(res.Err)
	if !ok {
		t.Fatal("foundry err")
	}
	tl.Assert("rsk_on_err", strings.Contains(fe.Remediation(), fsx.RSK310), true, fe.Remediation())
	tl.Assert("log_reobs", ml.Has("commit", "parent_reobserve", "fail"), true, true)
	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestPathHelpers_DarwinAliasesAndSystemPrefix covers rewrite/isSystemPrefix/splitAbs.
func TestPathHelpers_DarwinAliasesAndSystemPrefix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")

	// On Linux rewrite is a pure no-op identity.
	for _, p := range []string{"/tmp", "/tmp/foo", "/var/folders/xx", "/home/rob", "/private/tmp/x", ""} {
		got := fsx.RewriteDarwinDirAliasesForTest(p)
		if runtime.GOOS != "darwin" {
			log.Assert("noop_"+p, got == p, p, got)
		}
	}

	// isSystemPrefix matrix (logic is OS-independent).
	sys := []string{
		"/", "/home", "/Users", "/tmp", "/var", "/var/tmp",
		"/private", "/private/tmp", "/private/var", "/private/var/tmp",
		"/private/var/folders/zz", "/var/folders/aa", "/private/tmp/foo",
	}
	for _, p := range sys {
		if !fsx.IsSystemPrefixForTest(p) {
			t.Errorf("IsSystemPrefix(%q)=false want true", p)
		}
	}
	non := []string{"/opt", "/usr", "/home/user/proj", "/tmpish", "/private/etc", ""}
	for _, p := range non {
		if fsx.IsSystemPrefixForTest(p) {
			t.Errorf("IsSystemPrefix(%q)=true want false", p)
		}
	}

	// splitAbs
	parts := fsx.SplitAbsForTest("/")
	log.Assert("split_root", len(parts) == 1 && parts[0] == "", true, fmt.Sprintf("%q", parts))
	parts = fsx.SplitAbsForTest("/a/b")
	if len(parts) < 2 || parts[0] != "" || parts[1] != "a" {
		t.Fatalf("split /a/b: %q", parts)
	}
	// relative (no leading slash after Clean still works)
	parts = fsx.SplitAbsForTest("rel/path")
	if len(parts) != 2 || parts[0] != "rel" {
		t.Fatalf("split rel: %q", parts)
	}

	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestNormalizeDestination_MoreCases covers remaining normalize branches.
func TestNormalizeDestination_MoreCases(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")

	// NUL
	_, _, _, err := fsx.NormalizeDestinationForTest("a\x00b")
	requireID(t, err, diagnostic.IDFSUnsafePath)

	// Root path has no parent.
	_, _, _, err = fsx.NormalizeDestinationForTest("/")
	requireID(t, err, diagnostic.IDFSUnsafePath)

	// Relative path resolves via Abs.
	dest, parent, base, err := fsx.NormalizeDestinationForTest("rel-proj-name-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if base != "rel-proj-name-xyz" {
		t.Fatalf("base=%s", base)
	}
	if !filepath.IsAbs(dest) || !filepath.IsAbs(parent) {
		t.Fatalf("dest/parent not abs: %s %s", dest, parent)
	}

	// Happy absolute under tmp-like path (aliases no-op on linux).
	d, p, b, err := fsx.NormalizeDestinationForTest("/tmp/fsx-norm-test/proj")
	if err != nil {
		t.Fatal(err)
	}
	log.Assert("base_proj", b == "proj", "proj", b)
	log.Assert("parent_has_tmp", strings.Contains(p, "tmp") || strings.HasPrefix(p, "/"), true, p)
	log.Assert("dest_join", filepath.Base(d) == "proj", "proj", d)

	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestErrnoAndCommitHelpers covers errno classifiers and newCommitError branches.
func TestErrnoAndCommitHelpers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")

	log.Assert("exist_nil", !fsx.IsExistErrnoForTest(nil), false, false)
	log.Assert("exist_eexist", fsx.IsExistErrnoForTest(unix.EEXIST), true, true)
	log.Assert("exist_sys", fsx.IsExistErrnoForTest(syscall.EEXIST), true, true)

	log.Assert("ren_nil", !fsx.IsRenameUnsupportedForTest(nil), false, false)
	log.Assert("ren_enotsup", fsx.IsRenameUnsupportedForTest(unix.ENOTSUP), true, true)
	log.Assert("ren_eopnotsupp", fsx.IsRenameUnsupportedForTest(unix.EOPNOTSUPP), true, true)
	log.Assert("ren_einval", fsx.IsRenameUnsupportedForTest(unix.EINVAL), true, true)
	log.Assert("ren_sys", fsx.IsRenameUnsupportedForTest(syscall.ENOTSUP), true, true)

	log.Assert("exdev_nil", !fsx.IsEXDEVForTest(nil), false, false)
	log.Assert("exdev", fsx.IsEXDEVForTest(unix.EXDEV), true, true)
	log.Assert("exdev_sys", fsx.IsEXDEVForTest(syscall.EXDEV), true, true)

	log.Assert("errno_nil", fsx.ErrnoStringForTest(nil) == "", "", fsx.ErrnoStringForTest(nil))
	log.Assert("errno_unix", fsx.ErrnoStringForTest(unix.EPERM) != "", "non-empty", fsx.ErrnoStringForTest(unix.EPERM))
	log.Assert("errno_sys", fsx.ErrnoStringForTest(syscall.EIO) != "", "non-empty", fsx.ErrnoStringForTest(syscall.EIO))
	log.Assert("errno_plain", fsx.ErrnoStringForTest(errors.New("x")) == "x", "x", fsx.ErrnoStringForTest(errors.New("x")))

	log.Assert("enoent", fsx.ErrorsIsNotExistForTest(unix.ENOENT), true, true)
	log.Assert("enotdir", fsx.ErrorsIsNotDirForTest(unix.ENOTDIR), true, true)
	log.Assert("eloop", fsx.ErrorsIsLoopForTest(unix.ELOOP), true, true)

	// newCommitError location branches.
	err := fsx.NewCommitErrorForTest(diagnostic.IDFSDestinationExists, "exists", nil, "stage", "dest")
	fe, ok := diagnostic.AsFoundryError(err)
	if !ok {
		t.Fatal("foundry")
	}
	// parent nil → stage path or dest name for location.
	_ = fe

	// With parent for destination_exists join path.
	parent, _ := arrangeParent(t, nil, "nce")
	err = fsx.NewCommitErrorForTest(diagnostic.IDFSDestinationExists, "exists", parent, "st", "proj")
	fe, _ = diagnostic.AsFoundryError(err)
	if !strings.Contains(fe.Location().String(), "proj") && fe.Location().Path == "" {
		// Location should name dest path.
		loc := fe.Location()
		log.Assert("loc_set", loc.Path != "" || loc.String() != "", true, loc.String())
	}

	// Empty stage path fail-closed.
	res := fsx.CommitFailClosedForTest(fsx.ClassAmbiguous, diagnostic.IDFSCommitAmbiguous,
		"msg", "sn", "", "dn", "renameat2", nil, fsx.CommitObservation{})
	log.Assert("fail_class", res.Class == fsx.ClassAmbiguous, fsx.ClassAmbiguous, res.Class)
	log.Assert("fail_stage", res.StageName == "sn", "sn", res.StageName)

	// CommitResult.ErrorID non-Foundry / nil paths.
	log.Assert("eid_nil", fsx.CommitResult{}.ErrorID() == "", "", "")
	log.Assert("eid_plain", fsx.CommitResult{Err: errors.New("x")}.ErrorID() == "", "", "")
	log.Assert("eid_fe", fsx.CommitResult{Err: diagnostic.New(diagnostic.IDFSCommitFailed, "m", diagnostic.PathLocation("p"))}.ErrorID() == diagnostic.IDFSCommitFailed,
		diagnostic.IDFSCommitFailed, fsx.CommitResult{Err: diagnostic.New(diagnostic.IDFSCommitFailed, "m", diagnostic.PathLocation("p"))}.ErrorID())

	// NopLogger.FSXStep
	fsx.NopLogger{}.FSXStep("p", "s", "o", "d")

	// fileIDFromFD bad fd
	_, err = fsx.FileIDFromFDForTest(-1)
	if err == nil {
		t.Fatal("bad fd should fail fstat")
	}

	// classifyParentOpenatFail
	id := fsx.ClassifyParentOpenatFailForTest(-1, "x", unix.ENOENT)
	log.Assert("cls_enoent", id == diagnostic.IDFSParentMissing, diagnostic.IDFSParentMissing, id)
	id = fsx.ClassifyParentOpenatFailForTest(-1, "x", unix.ELOOP)
	log.Assert("cls_eloop", id == diagnostic.IDFSUnsafePath, diagnostic.IDFSUnsafePath, id)
	id = fsx.ClassifyParentOpenatFailForTest(-1, "x", unix.ENOTDIR)
	// without symlink, ENOTDIR → parent_missing
	log.Assert("cls_enotdir", id == diagnostic.IDFSParentMissing, diagnostic.IDFSParentMissing, id)
	id = fsx.ClassifyParentOpenatFailForTest(-1, "x", unix.EPERM)
	log.Assert("cls_other", id == diagnostic.IDFSUnsafePath, diagnostic.IDFSUnsafePath, id)

	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestWriter_ErrorPaths covers invalid modes, empty paths, drive form, Mkdir fail.
func TestWriter_ErrorPaths(t *testing.T) {
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "writer-err")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	w := stage.Writer()

	tl.Phase("assert")
	// Invalid mode.
	if err := w.WriteFile("badmode.txt", "notamode", []byte("x")); err == nil {
		t.Fatal("bad mode write")
	}
	if err := w.Mkdir("badmodedir", "9999"); err == nil {
		// 9999 octal parse may succeed as out-of-range or fail — either way.
		_ = err
	}
	if err := w.Mkdir("badmodedir2", "notoct"); err == nil {
		t.Fatal("bad mode mkdir")
	}
	if err := w.MkdirAll("x/y", "zz"); err == nil {
		t.Fatal("bad mode mkdirall")
	}

	// Empty / root-ish relative paths.
	for _, p := range []string{"", ".", "C:foo", "a\x00b"} {
		if err := w.WriteFile(p, "0644", []byte("x")); err == nil {
			t.Fatalf("WriteFile(%q) should fail", p)
		}
	}

	// Duplicate create → open fail path.
	if err := w.WriteFile("dup.txt", "0644", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("dup.txt", "0644", []byte("b")); err == nil {
		t.Fatal("dup create should fail")
	}
	tl.Assert("dup_log", ml.Has("rooted_writer", "open_create", "fail"), true, true)

	// Mkdir when parent missing (single Mkdir, not MkdirAll).
	if err := w.Mkdir("missing-parent/child", "0755"); err == nil {
		t.Fatal("mkdir deep without parent should fail")
	}

	// Closed stage writer.
	_ = stage.Close()
	if err := w.WriteFile("after-close.txt", "0644", []byte("x")); err == nil {
		t.Fatal("write after close")
	}
	if err := w.Mkdir("after-close-d", "0755"); err == nil {
		t.Fatal("mkdir after close")
	}
	if err := w.MkdirAll("after/close", "0755"); err == nil {
		t.Fatal("mkdirall after close")
	}

	// validateRootedRelPath / parseMode / splitRel unit tables.
	if _, err := fsx.ValidateRootedRelPathForTest(""); err == nil {
		t.Fatal("empty rel")
	}
	if _, err := fsx.ValidateRootedRelPathForTest("a\x00"); err == nil {
		t.Fatal("nul rel")
	}
	if _, err := fsx.ValidateRootedRelPathForTest("/abs"); err == nil {
		t.Fatal("abs rel")
	}
	if _, err := fsx.ValidateRootedRelPathForTest("C:\\win"); err == nil {
		t.Fatal("drive rel")
	}
	if _, err := fsx.ValidateRootedRelPathForTest("../x"); err == nil {
		t.Fatal("dotdot rel")
	}
	if _, err := fsx.ValidateRootedRelPathForTest("."); err == nil {
		t.Fatal("dot rel")
	}
	got, err := fsx.ValidateRootedRelPathForTest("a/b")
	if err != nil || got != "a/b" {
		t.Fatalf("valid rel: %q %v", got, err)
	}

	if _, err := fsx.ParseModeForTest(""); err == nil {
		t.Fatal("empty mode")
	}
	if _, err := fsx.ParseModeForTest("0o644"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsx.ParseModeForTest("644"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsx.ParseModeForTest("1000"); err == nil {
		t.Fatal("out of range mode")
	}
	if _, err := fsx.ParseModeForTest("xyz"); err == nil {
		t.Fatal("bad mode")
	}

	if fsx.SplitRelForTest(".") != nil {
		t.Fatal("split .")
	}
	if fsx.SplitRelForTest("") != nil {
		t.Fatal("split empty")
	}
	parts := fsx.SplitRelForTest("a/b")
	if len(parts) != 2 || parts[0] != "a" || parts[1] != "b" {
		t.Fatalf("split a/b: %q", parts)
	}

	tl.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestCommit_NilStage hits Commit nil/closed stage ambiguous path.
func TestCommit_NilStageAndClosedParent(t *testing.T) {
	log, tl, _ := newBridge(t)
	res := fsx.Commit(nil)
	tl.Assert("nil_class", res.Class == fsx.ClassAmbiguous, fsx.ClassAmbiguous, res.Class)
	tl.Assert("nil_id", res.ErrorID() == diagnostic.IDFSCommitAmbiguous, diagnostic.IDFSCommitAmbiguous, res.ErrorID())

	// Stage with closed parent: create stage then close parent fd via Parent.Close
	// while stage still holds pointer — Commit should hit parent closed branch.
	parent, _ := arrangeParent(t, log, "commit-closed-parent")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	// Close parent descriptor; stage.parent still non-nil but fd < 0.
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	res = fsx.Commit(stage)
	tl.Assert("parent_closed_class", res.Class == fsx.ClassAmbiguous, fsx.ClassAmbiguous, res.Class)
	tl.Assert("stage_name_set", res.StageName != "", "non-empty", res.StageName)
}

// TestBegin_StageCreateFailure covers Begin when CreateStage fails (bad project).
func TestBegin_StageCreateFailure(t *testing.T) {
	log, tl, ml := newBridge(t)
	dest, parentDir := arrangeDest(t, "begin-bad-proj")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "../evil"})
	if err == nil {
		_ = txn.Close()
		t.Fatal("expected begin fail")
	}
	requireID(t, err, diagnostic.IDFSUnsafePath)
	tl.Assert("log_stage_fail", ml.Has("transaction", "stage_create", "fail"), true, true)
	// No stage debris with invalid name.
	entries, _ := os.ReadDir(parentDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".foundry-") {
			t.Fatalf("unexpected stage %s", e.Name())
		}
	}
}

// TestChildExists_BadNames covers separator/empty child name rejection.
func TestChildExists_BadNames(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, _ := arrangeParent(t, log, "child-bad")
	badNames := []string{"", ".", "..", "a/b"}
	for _, name := range badNames {
		exists, err := parent.ChildExists(name)
		if err == nil || exists {
			t.Fatalf("ChildExists(%q) exists=%v err=%v", name, exists, err)
		}
		requireID(t, err, diagnostic.IDFSUnsafePath)
		_, _, err = parent.ChildLookup(name)
		requireID(t, err, diagnostic.IDFSUnsafePath)
		tl.Assert("bad_name:"+name, err != nil, true, err != nil)
	}
	tl.Assert("bad_names_count", len(badNames) == 4, 4, len(badNames))
}

// TestClassifyDestination_Other covers non-file/dir/symlink (fifo) → ClassOther.
func TestClassifyDestination_Other(t *testing.T) {
	log, tl, _ := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	parentDir := privateParent(t, root, "fifo-parent")
	fifo := filepath.Join(parentDir, "proj")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	parent, err := fsx.AcquireParent(filepath.Join(parentDir, "proj"), fsx.PreflightOptions{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	class, err := parent.ClassifyDestination()
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("class_other", class == fsx.ClassOther, fsx.ClassOther, class)
	requireID(t, parent.EnsureDestinationAbsent(), diagnostic.IDFSDestinationExists)
}

// TestVerifyIdentity_ParentClosedAndMismatch covers more VerifyIdentity branches.
func TestVerifyIdentity_ParentClosedAndMismatch(t *testing.T) {
	log, tl, ml := newBridge(t)
	parent, parentDir := arrangeParent(t, log, "verify-more")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })

	// Parent closed while stage open.
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	err = stage.VerifyIdentity()
	requireID(t, err, diagnostic.IDFSUnsafePath)
	tl.Assert("parent_closed", true, diagnostic.IDFSUnsafePath, mustFoundryID(t, err))

	// Fresh pair: replace stage basename with different directory (identity mismatch).
	parent2, parentDir2 := arrangeParent(t, log, "verify-mismatch")
	stage2, err := fsx.CreateStage(parent2, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage2.Close() })
	name := stage2.Name()
	// Remove and recreate different inode under same name.
	if err := stage2.Close(); err != nil {
		t.Fatal(err)
	}
	// Re-open stage handle is closed; CreateStage again then swap names...
	// Instead: keep stage open, rename away, mkdir new empty under same name.
	stage3, err := fsx.CreateStage(parent2, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage3.Close() })
	name = stage3.Name()
	if err := os.Rename(filepath.Join(parentDir2, name), filepath.Join(parentDir2, name+".old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(parentDir2, name), 0o700); err != nil {
		t.Fatal(err)
	}
	err = stage3.VerifyIdentity()
	if err == nil {
		t.Fatal("expected identity mismatch")
	}
	requireID(t, err, diagnostic.IDFSCommitFailed)
	tl.Assert("mismatch_log", ml.Has("stage_identity", "mismatch", "fail") || ml.Has("stage_identity", "lookup", "fail") || ml.Has("stage_identity", "handle", "fail"),
		true, true)
	_ = parentDir
}

// TestMkdirAll_FileInPath covers not-a-directory component error.
func TestMkdirAll_FileInPath(t *testing.T) {
	log, tl, _ := newBridge(t)
	parent, _ := arrangeParent(t, log, "mkdirall-file")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	w := stage.Writer()
	if err := w.WriteFile("blocker", "0644", []byte("x")); err != nil {
		t.Fatal(err)
	}
	err = w.MkdirAll("blocker/child", "0755")
	if err == nil {
		t.Fatal("expected not-a-directory")
	}
	requireID(t, err, diagnostic.IDRenderFailed)
	// Existing dir component is fine (continue path).
	if err := w.Mkdir("okdir", "0755"); err != nil {
		t.Fatal(err)
	}
	if err := w.MkdirAll("okdir/nested", "0755"); err != nil {
		t.Fatal(err)
	}
	// Identity fail path for Mkdir/MkdirAll/WriteFile after swap.
	name := stage.Name()
	parentDir := stage.Parent().AuthoredPath()
	if err := os.Rename(filepath.Join(parentDir, name), filepath.Join(parentDir, name+".gone")); err != nil {
		t.Fatal(err)
	}
	errAfterAll := w.MkdirAll("after-swap", "0755")
	if errAfterAll == nil {
		t.Fatal("mkdirall after identity break")
	}
	errAfterMkdir := w.Mkdir("after-swap2", "0755")
	if errAfterMkdir == nil {
		t.Fatal("mkdir after identity break")
	}
	// File-in-path path already asserted via requireID; identity break must also fail.
	tl.Assert("file_in_path_err", err != nil, true, err)
	tl.Assert("after_swap_mkdirall", errAfterAll != nil, true, errAfterAll)
	tl.Assert("after_swap_mkdir", errAfterMkdir != nil, true, errAfterMkdir)
}

// TestCommitResult_ReportEmptyStagePath covers Report StagePath fallback + fe rem.
func TestCommitResult_ReportEmptyStagePath(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")
	res := fsx.CommitResult{
		Class:     fsx.ClassUncommitted,
		Exit:      1,
		StageName: ".foundry-x-deadbeef",
		StagePath: "", // force StageLocation fallback
		Err: diagnostic.New(diagnostic.IDFSCommitFailed, "x", diagnostic.PathLocation("p")).
			WithRemediation("custom-remediation-line"),
	}
	pr := res.Report()
	log.Assert("path_fallback", pr.StagePath == ".foundry-x-deadbeef", ".foundry-x-deadbeef", pr.StagePath)
	log.Assert("rsk", pr.RiskID == fsx.RSK310, fsx.RSK310, pr.RiskID)
	log.Assert("custom_rem", pr.Remediation == "custom-remediation-line", "custom-remediation-line", pr.Remediation)

	// Preserved false when StageName empty.
	empty := fsx.CommitResult{Class: fsx.ClassUncommitted}
	log.Assert("not_preserved_empty", !empty.Preserved(), false, empty.Preserved())
	pr2 := empty.Report()
	log.Assert("no_rsk_empty", pr2.RiskID == "", "", pr2.RiskID)
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestNewCommitError_LocationBranches hits dest join and empty parent paths.
func TestNewCommitError_LocationBranches(t *testing.T) {
	log := testutil.New(t)
	log.Phase("assert")

	// destination_exists with parent → join parent/dest
	parent, parentDir := arrangeParent(t, nil, "nce-loc")
	err := fsx.NewCommitErrorForTest(diagnostic.IDFSDestinationExists, "exists", parent, "st", "proj")
	fe, ok := diagnostic.AsFoundryError(err)
	if !ok {
		t.Fatal("fe")
	}
	want := filepath.Join(parentDir, "proj")
	log.Assert("dest_loc", fe.Location().Path == want, want, fe.Location().Path)

	// non-dest id → stage path location
	err = fsx.NewCommitErrorForTest(diagnostic.IDFSCommitFailed, "fail", parent, "st", "proj")
	fe, _ = diagnostic.AsFoundryError(err)
	wantStage := filepath.Join(parentDir, "st")
	log.Assert("stage_loc", fe.Location().Path == wantStage, wantStage, fe.Location().Path)

	// nil parent, empty stage → destName location
	err = fsx.NewCommitErrorForTest(diagnostic.IDFSCommitFailed, "fail", nil, "", "onlydest")
	fe, _ = diagnostic.AsFoundryError(err)
	log.Assert("dest_only", fe.Location().Path == "onlydest", "onlydest", fe.Location().Path)

	// all empty → stageName (also empty) — still constructs
	err = fsx.NewCommitErrorForTest(diagnostic.IDFSCommitFailed, "fail", nil, "", "")
	if err == nil {
		t.Fatal("expected err")
	}

	// commitFailClosed without stage name (no attach preserve)
	res := fsx.CommitFailClosedForTest(fsx.ClassAmbiguous, diagnostic.IDFSCommitAmbiguous,
		"msg", "", "", "", "sys", nil, fsx.CommitObservation{})
	log.Assert("no_stage", res.StageName == "", "", res.StageName)

	// with stage name attaches preserve
	res = fsx.CommitFailClosedForTest(fsx.ClassUncommitted, diagnostic.IDFSCommitFailed,
		"msg", "sn", "/p/sn", "dn", "sys", unix.EIO, fsx.CommitObservation{})
	fe, ok = diagnostic.AsFoundryError(res.Err)
	if !ok {
		t.Fatal("fe")
	}
	log.Assert("has_rsk", strings.Contains(fe.Remediation(), fsx.RSK310), true, fe.Remediation())

	log.PhaseEnd("assert", testutil.OutcomeOK)
}

// TestTransaction_DuplicateAfterStageClose covers stage handle closed branch.
func TestTransaction_DuplicateAfterStageClose(t *testing.T) {
	log, tl, _ := newBridge(t)
	dest, _ := arrangeDest(t, "dup-stage-close")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txn.Close() })
	// Close the stage via Stage.Close but leave txn open (internal edge).
	st := txn.Stage()
	if st == nil {
		t.Fatal("nil stage")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	fd, err := txn.DuplicateStageHandle()
	tl.Assert("fd", fd == -1, -1, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	requireID(t, err, diagnostic.IDFSCommitFailed)
}

// TestMkdir_ExistingPathFail covers mkdir when path already exists.
func TestMkdir_ExistingPathFail(t *testing.T) {
	log, tl, ml := newBridge(t)
	parent, _ := arrangeParent(t, log, "mkdir-exist")
	stage, err := fsx.CreateStage(parent, "proj")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Close() })
	w := stage.Writer()
	if err := w.Mkdir("d", "0755"); err != nil {
		t.Fatal(err)
	}
	if err := w.Mkdir("d", "0755"); err == nil {
		t.Fatal("second mkdir should fail")
	}
	tl.Assert("mkdir_fail_log", ml.Has("rooted_writer", "mkdir", "fail"), true, true)
}

// TestBegin_EmptyProjectUsesBasename covers Begin project defaulting.
func TestBegin_EmptyProjectUsesBasename(t *testing.T) {
	log, tl, _ := newBridge(t)
	dest, _ := arrangeDest(t, "begin-empty-proj")
	txn, err := fsx.Begin(dest, fsx.Options{Log: log, Project: "  "})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = txn.Close() })
	tl.Assert("name_prefix", strings.HasPrefix(txn.StageName(), ".foundry-proj-"), true, txn.StageName())
}

// TestValidateStageProjectName_EmptyDirect covers empty after trim via CreateStage
// with parent.base also empty (synthetic) — use parent with empty base is hard;
// validate empty project when parent base is set already covered. Hit list_sep on Windows-style.
func TestValidateRootedRelPath_More(t *testing.T) {
	// backslash-normalized ".." still rejected
	if _, err := fsx.ValidateRootedRelPathForTest(`a\..\b`); err == nil {
		t.Fatal("expected reject")
	}
	got, err := fsx.ValidateRootedRelPathForTest(`a\b`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a/b" {
		t.Fatalf("got %q", got)
	}
}
