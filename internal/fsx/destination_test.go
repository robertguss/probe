//go:build unix

package fsx_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestDestinationRefusalMatrix covers REQ-003 / Section 31.4 — every existing
// form must fail closed with fs.destination_exists before staging, with an
// actionable refusal class in the step log.
func TestDestinationRefusalMatrix(t *testing.T) {
	type setupFn func(t *testing.T, parentDir, base string)

	cases := []struct {
		name      string
		class     fsx.DestinationClass
		setup     setupFn
		wantClass fsx.DestinationClass
	}{
		{
			name:      "regular_file",
			wantClass: fsx.ClassRegularFile,
			setup: func(t *testing.T, parentDir, base string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(parentDir, base), []byte("data\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:      "empty_dir",
			wantClass: fsx.ClassEmptyDir,
			setup: func(t *testing.T, parentDir, base string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(parentDir, base), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:      "non_empty_dir",
			wantClass: fsx.ClassNonEmptyDir,
			setup: func(t *testing.T, parentDir, base string) {
				t.Helper()
				dir := filepath.Join(parentDir, base)
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:      "symlink",
			wantClass: fsx.ClassSymlink,
			setup: func(t *testing.T, parentDir, base string) {
				t.Helper()
				target := filepath.Join(parentDir, "symlink-target")
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(parentDir, base)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:      "git_repo",
			wantClass: fsx.ClassGitRepo,
			setup: func(t *testing.T, parentDir, base string) {
				t.Helper()
				dir := filepath.Join(parentDir, base)
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				// Minimal .git entry (dir) — Foundry must refuse regardless of
				// whether git init was run; presence of .git is enough.
				if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:      "git_repo_file_git",
			wantClass: fsx.ClassGitRepo,
			setup: func(t *testing.T, parentDir, base string) {
				t.Helper()
				dir := filepath.Join(parentDir, base)
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				// Git worktree style: .git is a file.
				if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /tmp/elsewhere\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			log, tl, ml := newBridge(t)
			tl.Phase("arrange")
			root := t.TempDir()
			_ = os.Chmod(root, 0o700)
			parentDir := privateParent(t, root, "refuse-"+tc.name)
			base := "proj"
			tc.setup(t, parentDir, base)
			dest := filepath.Join(parentDir, base)
			tl.Inputs(map[string]string{"case": tc.name, "class": string(tc.wantClass)})
			tl.PhaseEnd("arrange", testutil.OutcomeOK)

			tl.Phase("act")
			// Acquire first so we can classify, then EnsureDestinationAbsent /
			// Preflight must refuse.
			parent, err := fsx.AcquireParent(dest, fsx.PreflightOptions{Log: log})
			if err != nil {
				tl.Fail("acquire", err.Error())
			}
			t.Cleanup(func() { _ = parent.Close() })

			class, err := parent.ClassifyDestination()
			if err != nil {
				tl.Fail("classify", err.Error())
			}
			tl.Assert("class", class == tc.wantClass, tc.wantClass, class)

			refuseErr := parent.EnsureDestinationAbsent()
			requireID(t, refuseErr, diagnostic.IDFSDestinationExists)

			// Full Preflight also refuses (and does not leave a handle).
			_, preErr := fsx.Preflight(dest, fsx.PreflightOptions{Log: log})
			requireID(t, preErr, diagnostic.IDFSDestinationExists)
			tl.PhaseEnd("act", testutil.OutcomeOK)

			tl.Phase("assert")
			tl.Assert("refuse_logged", ml.Has("destination", "refuse", "fail"), true, true)
			tl.Assert("class_in_log", ml.DetailContains("class="+string(tc.wantClass)), true, true)
			tl.Assert("id_in_log", ml.DetailContains(string(diagnostic.IDFSDestinationExists)), true, true)
			// Never mutated: original object still present.
			if _, err := os.Lstat(dest); err != nil {
				tl.Fail("destination_preserved", err.Error())
			}
			tl.PhaseEnd("assert", testutil.OutcomeOK)
		})
	}
}

func TestDestinationRefusal_MissingParentComponent(t *testing.T) {
	// Matrix item 6: non-directory / missing parent → fs.parent_missing
	// (not destination_exists). Covered in parent_test; assert Preflight path.
	log, tl, _ := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	base := privateParent(t, root, "preflight-missing")
	dest := filepath.Join(base, "ghost", "proj")

	_, err := fsx.Preflight(dest, fsx.PreflightOptions{Log: log})
	requireID(t, err, diagnostic.IDFSParentMissing)
	tl.Assert("preflight_parent_missing", true, diagnostic.IDFSParentMissing, mustFoundryID(t, err))
}

func TestDestinationRefusal_RealGitInit(t *testing.T) {
	// Optional: if git is available, refuse a fully initialized repository.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	log, tl, ml := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	parentDir := privateParent(t, root, "git-init-parent")
	dir := filepath.Join(parentDir, "proj")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--quiet", dir)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "HOME=" + root}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
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
	tl.Assert("class_git", class == fsx.ClassGitRepo, fsx.ClassGitRepo, class)
	requireID(t, parent.EnsureDestinationAbsent(), diagnostic.IDFSDestinationExists)
	tl.Assert("log_git", ml.DetailContains("class=git_repo"), true, true)
}

func TestNoPathThenMutate_APISurface(t *testing.T) {
	// Static intent: this package's public preflight APIs only open/fstat —
	// they do not create, rename, or remove. Exercise Preflight on an absent
	// destination and confirm nothing was created under the parent.
	log, tl, _ := newBridge(t)
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	parentDir := privateParent(t, root, "no-mutate")
	dest := filepath.Join(parentDir, "proj")

	parent, err := fsx.Preflight(dest, fsx.PreflightOptions{Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })

	entries, err := os.ReadDir(parentDir)
	if err != nil {
		t.Fatal(err)
	}
	tl.Assert("parent_empty", len(entries) == 0, 0, len(entries))
	// Sentinel: create after preflight via test harness only (not product API).
	sentinel := filepath.Join(parentDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Preflight again must refuse (file exists) and leave sentinel intact.
	_, err = fsx.Preflight(filepath.Join(parentDir, "sentinel.txt"), fsx.PreflightOptions{Log: log})
	// basename is sentinel.txt — destination exists as file
	// Wait — Preflight uses full path; basename sentinel.txt under parentDir.
	// Actually destination is parentDir/sentinel.txt which EXISTS as file.
	requireID(t, err, diagnostic.IDFSDestinationExists)
	if _, err := os.Stat(sentinel); err != nil {
		tl.Fail("sentinel_survived", err.Error())
	}
}

func TestChildLookup_SymlinkAndFile(t *testing.T) {
	root := t.TempDir()
	_ = os.Chmod(root, 0o700)
	parentDir := privateParent(t, root, "lookup-parent")
	if err := os.WriteFile(filepath.Join(parentDir, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f", filepath.Join(parentDir, "l")); err != nil {
		t.Fatal(err)
	}
	parent, err := fsx.AcquireParent(filepath.Join(parentDir, "unused"), fsx.PreflightOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })

	exists, id, err := parent.ChildLookup("f")
	if err != nil || !exists || id.IsZero() {
		t.Fatalf("file lookup exists=%v id=%s err=%v", exists, id, err)
	}
	exists, id, err = parent.ChildLookup("l")
	if err != nil || !exists {
		t.Fatalf("symlink lookup exists=%v err=%v", exists, err)
	}
	// Symlink has its own inode.
	if id.IsZero() {
		t.Fatal("symlink should report inode identity")
	}
	exists, _, err = parent.ChildLookup("missing")
	if err != nil || exists {
		t.Fatalf("missing exists=%v err=%v", exists, err)
	}
}
