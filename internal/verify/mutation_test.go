//go:build unix

package verify_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/verify"
)

func TestMutationSet_OnlyGoModGoSum(t *testing.T) {
	log := testutil.New(t)
	log.Phase("mutation_set")

	stage := writeStage(t, map[string]string{
		"go.mod":  "module m\n\ngo 1.26.0\n",
		"main.go": "package main\n",
	})
	pre, err := verify.SnapshotNonGit(stage)
	if err != nil {
		t.Fatal(err)
	}

	// Allowed: change go.mod and create go.sum.
	if err := os.WriteFile(filepath.Join(stage, "go.mod"), []byte("module m\n\ngo 1.26.0\n\nrequire example.com/x v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "go.sum"), []byte("example.com/x v1.0.0 h1:abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	post, err := verify.SnapshotNonGit(stage)
	if err != nil {
		t.Fatal(err)
	}
	rep := verify.ValidateTidyMutationSet(pre, post)
	log.Assert("allowed_ok", len(rep.ExtraPaths) == 0, 0, rep.ExtraPaths)
	log.Assert("changed_has_gomod", contains(rep.ChangedPaths, "go.mod"), true, rep.ChangedPaths)
	log.Step("allowed", testutil.OutcomeOK, "changed="+strings.Join(rep.ChangedPaths, ","))

	// Extra path fails.
	if err := os.WriteFile(filepath.Join(stage, "evil.txt"), []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	post2, _ := verify.SnapshotNonGit(stage)
	rep2 := verify.ValidateTidyMutationSet(pre, post2)
	log.Assert("extra_fail", contains(rep2.ExtraPaths, "evil.txt"), true, rep2.ExtraPaths)
	log.Step("extra_path", testutil.OutcomeOK, strings.Join(rep2.ExtraPaths, ","))

	// Mutating main.go fails.
	stage3 := writeStage(t, map[string]string{
		"go.mod":  "module m\n\ngo 1.26.0\n",
		"main.go": "package main\n",
	})
	pre3, _ := verify.SnapshotNonGit(stage3)
	if err := os.WriteFile(filepath.Join(stage3, "main.go"), []byte("package main\n// x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	post3, _ := verify.SnapshotNonGit(stage3)
	rep3 := verify.ValidateTidyMutationSet(pre3, post3)
	log.Assert("main_extra", contains(rep3.ExtraPaths, "main.go"), true, rep3.ExtraPaths)

	log.PhaseEnd("mutation_set", testutil.OutcomeOK)
}

func TestPinReparse_ExactAndForbidden(t *testing.T) {
	log := testutil.New(t)
	log.Phase("pin_reparse")

	// Happy path with exact pins.
	stage := writeStage(t, map[string]string{
		"go.mod": `module example.com/demo

go 1.26.0

require (
	github.com/spf13/cobra v1.10.2
	golang.org/x/mod v0.38.0
)
`,
	})
	rep := verify.ReparsePins(stage, []verify.Pin{
		{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
		{Path: "golang.org/x/mod", Version: "v0.38.0"},
	})
	log.Assert("parsed", rep.ParsedOK, true, rep.ParsedOK)
	log.Assert("ok", rep.OK(), true, rep)
	log.Assert("module", rep.ModulePath == "example.com/demo", "example.com/demo", rep.ModulePath)
	log.Step("pins_ok", testutil.OutcomeOK, "module="+rep.ModulePath)

	// Wrong version.
	repBad := verify.ReparsePins(stage, []verify.Pin{
		{Path: "github.com/spf13/cobra", Version: "v1.9.0"},
	})
	log.Assert("wrong_ver", !repBad.OK() && len(repBad.MissingPins) > 0, true, repBad.MissingPins)
	log.Step("wrong_version", testutil.OutcomeOK, strings.Join(repBad.MissingPins, ";"))

	// replace fails closed.
	stageR := writeStage(t, map[string]string{
		"go.mod": `module m

go 1.26.0

require github.com/spf13/cobra v1.10.2

replace github.com/spf13/cobra => ./cobra
`,
	})
	repR := verify.ReparsePins(stageR, nil)
	log.Assert("replace", !repR.OK() && len(repR.ForbiddenDirectives) > 0, true, repR.ForbiddenDirectives)
	log.Step("replace", testutil.OutcomeOK, strings.Join(repR.ForbiddenDirectives, ","))

	// exclude fails closed.
	stageE := writeStage(t, map[string]string{
		"go.mod": `module m

go 1.26.0

exclude golang.org/x/net v0.0.0
`,
	})
	repE := verify.ReparsePins(stageE, nil)
	log.Assert("exclude", !repE.OK(), true, repE.ForbiddenDirectives)
	log.Step("exclude", testutil.OutcomeOK, strings.Join(repE.ForbiddenDirectives, ","))

	// go.work present fails closed.
	stageW := writeStage(t, map[string]string{
		"go.mod":  "module m\n\ngo 1.26.0\n",
		"go.work": "go 1.26.0\n\nuse .\n",
	})
	repW := verify.ReparsePins(stageW, nil)
	log.Assert("workspace", !repW.OK(), true, repW.ForbiddenDirectives)
	foundWS := false
	for _, d := range repW.ForbiddenDirectives {
		if strings.Contains(d, "workspace") {
			foundWS = true
		}
	}
	log.Assert("workspace_token", foundWS, true, repW.ForbiddenDirectives)

	// ValidateMutationAndPins surfaces verify.module_mutation.
	stageX := writeStage(t, map[string]string{
		"go.mod":  "module m\n\ngo 1.26.0\n",
		"main.go": "package main\n",
	})
	pre, _ := verify.SnapshotNonGit(stageX)
	if err := os.WriteFile(filepath.Join(stageX, "side.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	post, _ := verify.SnapshotNonGit(stageX)
	_, err := verify.ValidateMutationAndPins(pre, post, stageX, nil)
	if err == nil {
		log.Fail("module_mutation", "expected error")
	} else {
		fe, ok := diagnostic.AsFoundryError(err)
		log.Assert("id", ok && fe.ID() == diagnostic.IDVerifyModuleMutation,
			diagnostic.IDVerifyModuleMutation, idOf(fe))
		log.Step("module_mutation_err", testutil.OutcomeOK, err.Error())
	}

	log.PhaseEnd("pin_reparse", testutil.OutcomeOK)
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
