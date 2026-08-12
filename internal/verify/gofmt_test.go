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

func TestGoFmt_CleanAndDirty(t *testing.T) {
	log := testutil.New(t)
	log.Phase("gofmt")

	clean := writeStage(t, map[string]string{
		"main.go": "package main\n\nfunc main() {}\n",
		"a/b.go":  "package b\n\nconst X = 1\n",
		"readme":  "not go\n",
	})
	fails, err := verify.CheckGoFmt(clean)
	log.Assert("clean_err", err == nil, true, err)
	log.Assert("clean_fails", len(fails) == 0, 0, len(fails))
	log.Step("clean", testutil.OutcomeOK, "ok")

	dirty := writeStage(t, map[string]string{
		// Missing newline / bad spacing that go/format rewrites.
		"main.go": "package main\nfunc main(){  }\n",
	})
	fails, err = verify.CheckGoFmt(dirty)
	if err == nil {
		log.Fail("dirty", "expected gofmt failure")
	} else {
		fe, ok := diagnostic.AsFoundryError(err)
		log.Assert("id", ok && fe.ID() == diagnostic.IDVerifyFailed,
			diagnostic.IDVerifyFailed, idOf(fe))
		log.Assert("step_loc", fe.Location().StepID == verify.CheckGofmt,
			verify.CheckGofmt, fe.Location().StepID)
		log.Step("dirty", testutil.OutcomeOK, err.Error())
	}
	log.Assert("fail_main", len(fails) >= 1 && fails[0].Rel == "main.go", "main.go", fails)

	// .git go files ignored.
	stage := writeStage(t, map[string]string{"main.go": "package main\n"})
	if err := os.MkdirAll(filepath.Join(stage, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Intentionally dirty under .git — must not fail.
	if err := os.WriteFile(filepath.Join(stage, ".git", "x.go"), []byte("package x\nfunc f(){  }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = verify.CheckGoFmt(stage)
	log.Assert("git_ignored", err == nil, true, err)

	log.PhaseEnd("gofmt", testutil.OutcomeOK)
}
