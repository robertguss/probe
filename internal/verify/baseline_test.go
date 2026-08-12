//go:build unix

package verify_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/verify"
)

func TestBaseline_FreezeAndCompare(t *testing.T) {
	log := testutil.New(t)
	log.Phase("freeze_compare")

	stage := writeStage(t, map[string]string{
		"go.mod":    "module m\n\ngo 1.26.0\n",
		"main.go":   "package main\n",
		"README.md": "hi\n",
	})
	// Plant a .git tree that must be ignored.
	gitDir := filepath.Join(stage, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	base, err := verify.Freeze(stage)
	if err != nil {
		log.Fail("freeze", err.Error())
		return
	}
	log.Assert("digest_len", len(base.AggregateDigest) == 64, 64, len(base.AggregateDigest))
	log.Assert("has_main", base.Entries["main.go"].Type == "file", "file", base.Entries["main.go"].Type)
	if _, ok := base.Entries[".git"]; ok {
		log.Fail("git_included", ".git must be excluded")
	}
	if _, ok := base.Entries[".git/HEAD"]; ok {
		log.Fail("git_head_included", ".git/HEAD must be excluded")
	}
	log.Step("freeze", testutil.OutcomeOK, "digest="+base.AggregateDigest[:12]+" entries="+itoa(len(base.Keys)))

	// Identical re-snapshot → no diffs.
	after, err := verify.SnapshotNonGit(stage)
	if err != nil {
		log.Fail("snapshot", err.Error())
		return
	}
	diffs := verify.Compare(base, after)
	log.Assert("identical", len(diffs) == 0, 0, len(diffs))

	// Byte drift.
	if err := os.WriteFile(filepath.Join(stage, "main.go"), []byte("package main\n// mutated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffs, err = verify.Conform(base, stage)
	if err == nil {
		log.Fail("expected_unplanned", "want verify.unplanned_mutation")
	} else {
		fe, ok := diagnostic.AsFoundryError(err)
		if !ok || fe.ID() != diagnostic.IDVerifyUnplannedMutation {
			log.Fail("id", err.Error())
		} else {
			log.Step("byte_drift", testutil.OutcomeOK, err.Error())
		}
	}
	log.Assert("diff_main", len(diffs) >= 1 && diffs[0].Rel == "main.go", "main.go", diffRels(diffs))

	// Extra path.
	stage2 := writeStage(t, map[string]string{"a.txt": "a\n"})
	base2, _ := verify.Freeze(stage2)
	if err := os.WriteFile(filepath.Join(stage2, "extra.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffs, err = verify.Conform(base2, stage2)
	if err == nil {
		log.Fail("extra_path", "want error")
	} else {
		log.Step("extra", testutil.OutcomeOK, err.Error())
	}
	foundExtra := false
	for _, d := range diffs {
		if d.Rel == "extra.txt" && d.Reason == "extra" {
			foundExtra = true
		}
	}
	log.Assert("extra_reason", foundExtra, true, diffRels(diffs))

	// Missing path.
	stage3 := writeStage(t, map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	base3, _ := verify.Freeze(stage3)
	_ = os.Remove(filepath.Join(stage3, "b.txt"))
	diffs, err = verify.Conform(base3, stage3)
	if err == nil {
		log.Fail("missing", "want error")
	}
	foundMissing := false
	for _, d := range diffs {
		if d.Rel == "b.txt" && d.Reason == "missing" {
			foundMissing = true
		}
	}
	log.Assert("missing_reason", foundMissing, true, diffRels(diffs))

	// .git mutation must NOT affect non-.git conformance.
	stage4 := writeStage(t, map[string]string{"a.txt": "a\n"})
	base4, _ := verify.Freeze(stage4)
	if err := os.MkdirAll(filepath.Join(stage4, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage4, ".git", "HEAD"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diffs, err = verify.Conform(base4, stage4)
	log.Assert("git_ignored", err == nil && len(diffs) == 0, true, err != nil || len(diffs) > 0)

	log.PhaseEnd("freeze_compare", testutil.OutcomeOK)
}

func TestBaseline_AggregateDigestStable(t *testing.T) {
	log := testutil.New(t)
	stage := writeStage(t, map[string]string{
		"z.txt": "z\n",
		"a.txt": "a\n",
	})
	b1, err := verify.Freeze(stage)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := verify.Freeze(stage)
	if err != nil {
		t.Fatal(err)
	}
	log.Assert("stable", b1.AggregateDigest == b2.AggregateDigest, b1.AggregateDigest, b2.AggregateDigest)
	log.Assert("keys_sorted", b1.Keys[0] == "a.txt" && b1.Keys[1] == "z.txt",
		"a.txt,z.txt", b1.Keys[0]+","+b1.Keys[1])
}

func diffRels(diffs []verify.Diff) string {
	var b string
	for i, d := range diffs {
		if i > 0 {
			b += ","
		}
		b += d.Rel + ":" + d.Reason
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
