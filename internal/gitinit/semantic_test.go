package gitinit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/gitinit"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestValidateSemanticGit_Happy(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	plantMinimalGitPortable(t, stage, "main")
	rep := gitinit.ValidateSemanticGit(stage, "main")
	log.Assert("ok", rep.OK, true, rep.OK)
	log.Assert("dir", rep.IsDirectory, true, rep.IsDirectory)
	log.Assert("HEAD", rep.HEADRef == "ref: refs/heads/main", "ref: refs/heads/main", rep.HEADRef)
	log.Assert("objects", rep.HasObjectsDir, true, rep.HasObjectsDir)
	log.Assert("refs", rep.HasRefsDir, true, rep.HasRefsDir)
	log.Assert("index", rep.IndexEmpty, true, rep.IndexEmpty)
}

func TestValidateSemanticGit_WrongBranch(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	plantMinimalGitPortable(t, stage, "main")
	rep := gitinit.ValidateSemanticGit(stage, "develop")
	log.Assert("not_ok", !rep.OK, false, rep.OK)
}

func TestValidateSemanticGit_HookContent(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	plantMinimalGitPortable(t, stage, "main")
	hooks := filepath.Join(stage, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho x\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rep := gitinit.ValidateSemanticGit(stage, "main")
	log.Assert("not_ok", !rep.OK, false, rep.OK)
	log.Assert("hook_named", len(rep.HooksWithContent) == 1, 1, rep.HooksWithContent)
}

func TestValidateSemanticGit_EmptyHookFilesOK(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	plantMinimalGitPortable(t, stage, "main")
	hooks := filepath.Join(stage, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := gitinit.ValidateSemanticGit(stage, "main")
	log.Assert("ok", rep.OK, true, rep.OK)
}

func TestSnapshotNonGit_IgnoresDotGit(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plantMinimalGitPortable(t, stage, "main")
	snap, err := gitinit.SnapshotNonGit(stage)
	if err != nil {
		t.Fatal(err)
	}
	log.Assert("has_a", snap.Entries["a.txt"].Type == "file", "file", snap.Entries["a.txt"].Type)
	if _, ok := snap.Entries[".git"]; ok {
		log.Fail("git_in_snap", ".git must be excluded")
	}
	if _, ok := snap.Entries[".git/HEAD"]; ok {
		log.Fail("git_head_in_snap", ".git/HEAD must be excluded")
	}
	// Round-trip equality.
	snap2, err := gitinit.SnapshotNonGit(stage)
	if err != nil {
		t.Fatal(err)
	}
	diffs := gitinit.CompareNonGit(snap, snap2)
	log.Assert("equal", len(diffs) == 0, 0, len(diffs))
}

func TestCompareNonGit_DetectsExtraAndBytes(t *testing.T) {
	log := testutil.New(t)
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := gitinit.SnapshotNonGit(stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "a.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "extra.txt"), []byte("e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := gitinit.SnapshotNonGit(stage)
	if err != nil {
		t.Fatal(err)
	}
	diffs := gitinit.CompareNonGit(before, after)
	log.Assert("n_diffs", len(diffs) >= 2, ">=2", len(diffs))
	reasons := map[string]string{}
	for _, d := range diffs {
		reasons[d.Rel] = d.Reason
	}
	log.Assert("bytes", reasons["a.txt"] == "bytes", "bytes", reasons["a.txt"])
	log.Assert("extra", reasons["extra.txt"] == "extra", "extra", reasons["extra.txt"])
}

func TestEmptyTemplate_ModeAndEmpty(t *testing.T) {
	log := testutil.New(t)
	parent := t.TempDir()
	dir, err := gitinit.EmptyTemplate(parent)
	if err != nil {
		t.Fatal(err)
	}
	log.Assert("prefix", gitinit.IsFoundryTemplate(dir), true, dir)
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.Assert("mode0700", st.Mode().Perm() == 0o700, 0o700, st.Mode().Perm())
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.Assert("empty", len(entries) == 0, 0, len(entries))
	if err := gitinit.RemoveTemplate(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		log.Fail("still_exists", dir)
	}
}

func TestRemoveTemplate_RefusesNonPrefix(t *testing.T) {
	log := testutil.New(t)
	dir := t.TempDir()
	err := gitinit.RemoveTemplate(dir)
	log.Assert("err", err != nil, true, err == nil)
}

// plantMinimalGitPortable works without unix build tag (semantic_test is portable).
func plantMinimalGitPortable(t *testing.T, stageDir, branch string) {
	t.Helper()
	if branch == "" {
		branch = "main"
	}
	git := filepath.Join(stageDir, ".git")
	for _, d := range []string{
		filepath.Join(git, "objects", "info"),
		filepath.Join(git, "objects", "pack"),
		filepath.Join(git, "refs", "heads"),
		filepath.Join(git, "refs", "tags"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(git, "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(git, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
