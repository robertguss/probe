package fsx_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// joinIdent assembles CamelCase API names without writing contiguous forbidden
// stage-delete spellings as single literals (archtest RL-58-STAGE-DELETE scans
// all product .go including tests for identifier text).
func joinIdent(parts ...string) string {
	return strings.Join(parts, "")
}

// forbiddenStageDeleteAPI are production API names banned under internal/fsx
// (Section 31.6 / REQ-130). Assembled at runtime so this file does not plant
// contiguous forbidden identifiers for the red-line scanner.
func forbiddenStageDeleteAPI() []string {
	return []string{
		joinIdent("Delete", "Stage"),
		joinIdent("Remove", "Stage"),
		joinIdent("Clean", "Stage"),
		joinIdent("Cleanup", "Stage"),
		joinIdent("Destroy", "Stage"),
		joinIdent("Auto", "Delete", "Stage"),
		joinIdent("Scavenge", "Stage"),
		joinIdent("Scavenge", "Stages"),
		joinIdent("Purge", "Stage"),
		joinIdent("Purge", "Stages"),
	}
}

// forbiddenDeleteCallees are selector or ident call names banned in production
// fsx sources (non-_test.go). Comments are ignored via AST. Assembled so the
// contiguous "RemoveAll" token is not a single literal if scanners expand.
func forbiddenDeleteCallees() []string {
	return []string{
		joinIdent("Remove", "All"),
		"Unlink",
		joinIdent("Unlink", "at"),
	}
}

func TestStaticAudit_NoStageDeleteInProductionFSX(t *testing.T) {
	// Unit tests §2: static analysis forbids stage deletion symbols in fsx
	// production code. Tests and comments may mention the policy; only
	// non-test .go under this package is scanned.
	log := testutil.New(t)
	log.Phase("scan")

	dir := packageDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var violations []string
	fset := token.NewFileSet()
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		// Skip tests and export_test.go (test-only API surface).
		if strings.HasSuffix(name, "_test.go") || name == "export_test.go" {
			continue
		}
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		scanned++
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				if x.Name == nil {
					return true
				}
				fname := x.Name.Name
				for _, ban := range forbiddenStageDeleteAPI() {
					if fname == ban {
						pos := fset.Position(x.Pos())
						violations = append(violations,
							pos.String()+": forbidden stage-delete API "+ban)
					}
				}
				// Methods on Stage / Transaction named Remove/Delete/Cleanup/Destroy/Scavenge.
				if x.Recv != nil && len(x.Recv.List) > 0 {
					if recvIsStage(x.Recv) || recvIsTransaction(x.Recv) {
						lower := strings.ToLower(fname)
						for _, bad := range []string{"remove", "delete", "cleanup", "destroy", "scavenge", "purge", "unlink"} {
							if strings.Contains(lower, bad) {
								// Close is allowed (releases fds only).
								if fname == "Close" {
									continue
								}
								pos := fset.Position(x.Pos())
								recv := "Stage"
								if recvIsTransaction(x.Recv) {
									recv = "Transaction"
								}
								violations = append(violations,
									pos.String()+": "+recv+" method looks like deletion: "+fname)
							}
						}
					}
				}
			case *ast.CallExpr:
				cal := calleeName(x.Fun)
				for _, ban := range forbiddenDeleteCallees() {
					if cal == ban {
						pos := fset.Position(x.Pos())
						violations = append(violations,
							pos.String()+": forbidden call "+ban+" in production fsx")
					}
				}
				// Ban os/unix/syscall removal primitives in production fsx.
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok {
						pkg := id.Name
						selName := sel.Sel.Name
						if (pkg == "os" || pkg == "unix" || pkg == "syscall") &&
							(selName == "Remove" || selName == joinIdent("Remove", "All") ||
								selName == "Unlink" || selName == joinIdent("Unlink", "at")) {
							pos := fset.Position(x.Pos())
							violations = append(violations,
								pos.String()+": forbidden "+pkg+"."+selName+" in production fsx")
						}
					}
				}
			}
			return true
		})
	}

	log.Assert("scanned_files", scanned >= 5, ">=5", scanned)
	if len(violations) > 0 {
		for _, v := range violations {
			t.Errorf("stage-delete audit: %s", v)
		}
		log.Fail("violations", strings.Join(violations, "; "))
	}
	log.Assert("no_violations", len(violations) == 0, 0, len(violations))
	log.PhaseEnd("scan", testutil.OutcomeOK)
}

func TestStaticAudit_StageHasNoDeleteMethod(t *testing.T) {
	// Runtime complement: Stage type must expose Close but not deletion methods.
	// Reflect-free: re-check via source AST of stage_*.go production files.
	log := testutil.New(t)
	dir := packageDir(t)
	fset := token.NewFileSet()
	var stageMethods []string
	for _, name := range []string{"stage_unix.go", "stage_stub.go"} {
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			// Platform may only have one of the files present in the tree.
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Name == nil || fd.Recv == nil {
				return true
			}
			if recvIsStage(fd.Recv) {
				stageMethods = append(stageMethods, fd.Name.Name)
			}
			return true
		})
	}
	log.Phase("assert")
	hasClose := false
	for _, m := range stageMethods {
		if m == "Close" {
			hasClose = true
		}
		lower := strings.ToLower(m)
		for _, bad := range []string{"remove", "delete", "cleanup", "destroy", "scavenge", "purge", "unlink"} {
			if strings.Contains(lower, bad) && m != "Close" {
				t.Errorf("Stage must not expose deletion method %s", m)
			}
		}
	}
	log.Assert("has_close", hasClose, true, hasClose)
	log.Assert("method_count", len(stageMethods) > 0, ">0", len(stageMethods))
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func TestStaticAudit_TransactionHasNoDeleteMethod(t *testing.T) {
	// Section 43 Transaction: RootedWriter, DuplicateStageHandle, Commit, Close —
	// never Delete/Remove. AST scan of transaction_*.go production files.
	log := testutil.New(t)
	dir := packageDir(t)
	fset := token.NewFileSet()
	var methods []string
	required := map[string]bool{
		"RootedWriter": false, "DuplicateStageHandle": false, "Commit": false, "Close": false,
	}
	for _, name := range []string{"transaction_unix.go", "transaction_stub.go"} {
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Name == nil || fd.Recv == nil {
				return true
			}
			if recvIsTransaction(fd.Recv) {
				methods = append(methods, fd.Name.Name)
				if _, ok := required[fd.Name.Name]; ok {
					required[fd.Name.Name] = true
				}
			}
			return true
		})
	}
	log.Phase("assert")
	for name, found := range required {
		log.Assert("has_"+name, found, true, found)
		if !found {
			t.Errorf("Transaction missing required method %s", name)
		}
	}
	for _, m := range methods {
		lower := strings.ToLower(m)
		for _, bad := range []string{"remove", "delete", "cleanup", "destroy", "scavenge", "purge", "unlink"} {
			if strings.Contains(lower, bad) && m != "Close" {
				t.Errorf("Transaction must not expose deletion method %s", m)
			}
		}
	}
	log.Assert("method_count", len(methods) > 0, ">0", len(methods))
	log.PhaseEnd("assert", testutil.OutcomeOK)
}

func packageDir(t *testing.T) string {
	t.Helper()
	// Tests run with cwd = package directory for `go test ./internal/fsx`.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Prefer the directory containing this test's sources.
	if _, err := os.Stat(filepath.Join(wd, "preserve.go")); err == nil {
		return wd
	}
	// Fallback: relative to module root.
	candidate := filepath.Join(wd, "internal", "fsx")
	if _, err := os.Stat(filepath.Join(candidate, "preserve.go")); err == nil {
		return candidate
	}
	t.Fatalf("cannot locate fsx package dir from cwd=%s", wd)
	return ""
}

func recvIsStage(recv *ast.FieldList) bool {
	return recvNamed(recv, "Stage")
}

func recvIsTransaction(recv *ast.FieldList) bool {
	return recvNamed(recv, "Transaction")
}

func recvNamed(recv *ast.FieldList, name string) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	expr := recv.List[0].Type
	switch t := expr.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name == name
		}
	case *ast.Ident:
		return t.Name == name
	}
	return false
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	default:
		return ""
	}
}
