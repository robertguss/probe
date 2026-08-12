package gitinit_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/gitinit"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestSurface_OnlyInitSubcommand locks PlannedArgs shape (REQ-160).
func TestSurface_OnlyInitSubcommand(t *testing.T) {
	log := testutil.New(t)
	args := gitinit.PlannedArgs("main", "/tmp/foundry-git-template-XYZ")
	log.Assert("len", len(args) == 4, 4, len(args))
	log.Assert("sub", args[0] == "init", "init", args[0])
	log.Assert("branch", args[1] == "--initial-branch=main", "--initial-branch=main", args[1])
	log.Assert("template", strings.HasPrefix(args[2], "--template="), true, args[2])
	log.Assert("dot", args[3] == ".", ".", args[3])

	argv := gitinit.PlannedArgv("main", "/tmp/foundry-git-template-XYZ")
	log.Assert("argv0", argv[0] == "git", "git", argv[0])
	log.Assert("sub_of", gitinit.SubcommandOf(argv) == "init", "init", gitinit.SubcommandOf(argv))
	log.Assert("assert", gitinit.AssertOnlyInit(argv, "main", "/tmp/foundry-git-template-XYZ") == "",
		"", gitinit.AssertOnlyInit(argv, "main", "/tmp/foundry-git-template-XYZ"))
}

// TestSurface_ForbiddenListCoversDEC004 ensures critical verbs are banned.
func TestSurface_ForbiddenListCoversDEC004(t *testing.T) {
	log := testutil.New(t)
	required := []string{"commit", "push", "remote", "tag", "config", "checkout", "add"}
	for _, r := range required {
		log.Assert("forbidden_"+r, gitinit.IsForbiddenSubcommand(r), true, false)
	}
	log.Assert("init_ok", !gitinit.IsForbiddenSubcommand("init"), false, false)
}

// TestSurface_ProductionNoOsExec proves product .go files do not import os/exec
// (REQ-185: toolrun is the only subprocess-starting package).
func TestSurface_ProductionNoOsExec(t *testing.T) {
	log := testutil.New(t)
	dir := "."
	// Resolve package dir from this test file.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// When run as package, cwd is the package dir.
	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(wd, name)
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p == "os/exec" {
				log.Fail("os_exec_import", name)
			}
		}
		log.Step("file", testutil.OutcomeOK, name)
	}
	_ = dir
}

// TestSurface_NoCommitPushInSource scans product sources for argv construction
// of forbidden verbs (defense in depth beyond PlannedArgs).
func TestSurface_NoCommitPushInSource(t *testing.T) {
	log := testutil.New(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}
	// Tokens that must not appear as git subcommand string literals in product code.
	// Allow "commit" in comments/docs via checking quoted forms only.
	bannedQuoted := []string{
		`"commit"`,
		`"push"`,
		`"remote"`,
		`"tag"`,
		`"checkout"`,
		`"config"`,
		`"add"`,
		`"clone"`,
		`"pull"`,
		`"fetch"`,
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// surface.go intentionally lists ForbiddenGitSubcommands — allow that file
		// for the banned list definition only.
		if name == "surface.go" || name == "doc.go" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(wd, name))
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, q := range bannedQuoted {
			if strings.Contains(src, q) {
				log.Fail("banned_literal", name+": "+q)
			}
		}
	}
	log.Assert("planned_is_init", gitinit.OnlyGitSubcommand == "init", "init", gitinit.OnlyGitSubcommand)
}
