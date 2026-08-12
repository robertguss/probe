package e4

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// StaticCheckResult is the outcome of source-level option verification.
type StaticCheckResult struct {
	WithContext          bool
	WithoutSignalHandler bool
	WithoutCatchPanics   bool // must be false (absent) for CONFIRMS
	FilesScanned         []string
	Detail               string
}

// Pass reports whether the source satisfies FND-010 static checks.
func (r StaticCheckResult) Pass() bool {
	return r.WithContext && r.WithoutSignalHandler && !r.WithoutCatchPanics
}

// StaticCheckOptions scans Go source under dir for the normative lifecycle options.
// It confirms RequiredProgramOptions / Run wiring references WithContext and
// WithoutSignalHandler, and that WithoutCatchPanics is never called.
func StaticCheckOptions(dir string) (StaticCheckResult, error) {
	var res StaticCheckResult
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "testdata" || base == "vendor" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Skip the spike subprocess main if it only re-exports — still scan it.
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		res.FilesScanned = append(res.FilesScanned, path)
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := callName(call)
			switch name {
			case "WithContext", "tea.WithContext":
				res.WithContext = true
			case "WithoutSignalHandler", "tea.WithoutSignalHandler":
				res.WithoutSignalHandler = true
			case "WithoutCatchPanics", "tea.WithoutCatchPanics":
				res.WithoutCatchPanics = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		return res, err
	}
	parts := []string{}
	if res.WithContext {
		parts = append(parts, "WithContext=present")
	} else {
		parts = append(parts, "WithContext=MISSING")
	}
	if res.WithoutSignalHandler {
		parts = append(parts, "WithoutSignalHandler=present")
	} else {
		parts = append(parts, "WithoutSignalHandler=MISSING")
	}
	if res.WithoutCatchPanics {
		parts = append(parts, "WithoutCatchPanics=PRESENT(forbidden)")
	} else {
		parts = append(parts, "WithoutCatchPanics=absent(ok)")
	}
	res.Detail = strings.Join(parts, "; ")
	return res, nil
}

func callName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if id, ok := fun.X.(*ast.Ident); ok {
			return id.Name + "." + fun.Sel.Name
		}
		return fun.Sel.Name
	default:
		return ""
	}
}
