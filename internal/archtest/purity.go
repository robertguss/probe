package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WriteFreePackages are Phase-1 product packages that must remain free of
// subprocess starts, network dials, and writable opens on the validate/plan/
// catalog/version product path (REQ-031). cli is checked with a looser import
// policy (os + exec.LookPath only) and a call-pattern ban on StartProcess.
var WriteFreePackages = []string{
	"internal/diagnostic",
	"internal/spec",
	"internal/catalog",
	"internal/resolve",
	"internal/render",
	"internal/plan",
	"internal/report",
	"internal/version",
	"internal/cli",
}

// purityForbiddenImports are banned in pure write-free packages (not cli).
// cli has its own allowlist; diagnostic allows net/url for redaction only.
var purityForbiddenImports = []string{
	"os/exec",
	"os/user",
	"net",
	"net/http",
	"syscall",
	"plugin",
	"database/sql",
	"crypto/rand", // non-determinism in pure planning path
	"math/rand",
}

// purityForbiddenCLIImports remain banned even in internal/cli.
var purityForbiddenCLIImports = []string{
	"net",
	"net/http",
	"syscall",
	"plugin",
	"database/sql",
}

// PurityViolation is one write-free purity failure with counter/evidence.
type PurityViolation struct {
	Package string
	File    string
	Detail  string
	// Evidence is syscall/import/call counter text printed on failure.
	Evidence string
}

func (v PurityViolation) String() string {
	if v.Evidence != "" {
		return fmt.Sprintf("purity: package=%s file=%s detail=%s evidence=%s",
			v.Package, v.File, v.Detail, v.Evidence)
	}
	return fmt.Sprintf("purity: package=%s file=%s detail=%s", v.Package, v.File, v.Detail)
}

// CheckPurity runs static write-free purity analysis for P1 packages (REQ-031).
// Complements P1.8.a e2e strace/tree audits with import and call-graph evidence.
//
// Rules:
//   - Pure packages must not import net/os/exec/syscall/plugin (diagnostic may
//     import net/url for secret redaction only).
//   - internal/cli may import os and os/exec but only for LookPath (tool binary
//     location); exec.Command / Start / Run / Output are forbidden.
//   - foundrydev build-tagged sources are skipped (dev catalog load is not the
//     release write-free surface).
func CheckPurity(root string) ([]PurityViolation, error) {
	var all []PurityViolation
	for _, pkg := range WriteFreePackages {
		dir := filepath.Join(root, filepath.FromSlash(pkg))
		st, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue // package not yet present — no violation
			}
			return all, err
		}
		if !st.IsDir() {
			continue
		}
		vs, err := checkPackagePurity(root, pkg, dir)
		if err != nil {
			return all, err
		}
		all = append(all, vs...)
	}
	return all, nil
}

func checkPackagePurity(root, pkg, dir string) ([]PurityViolation, error) {
	var out []PurityViolation
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		// Skip non-default build tags (foundrydev catalog LoadDir, etc.).
		if skipPurityFile(path) {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return out, err
		}
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return out, fmt.Errorf("parse %s: %w", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		// Import scan.
		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			if v := purityImportViolation(pkg, rel, ip); v != nil {
				out = append(out, *v)
			}
		}

		// CLI call-pattern scan: ban subprocess starts.
		if pkg == "internal/cli" {
			out = append(out, scanCLISubprocessCalls(fset, f, pkg, rel)...)
		}
	}
	return out, nil
}

func purityImportViolation(pkg, rel, ip string) *PurityViolation {
	// Allow diagnostic net/url for redaction (no dial).
	if pkg == "internal/diagnostic" && (ip == "net/url" || strings.HasPrefix(ip, "net/url/")) {
		return nil
	}
	// cli: only the CLI-specific ban list applies for network/plugin.
	if pkg == "internal/cli" {
		for _, bad := range purityForbiddenCLIImports {
			if ip == bad || strings.HasPrefix(ip, bad+"/") {
				return &PurityViolation{
					Package:  pkg,
					File:     rel,
					Detail:   "write-free CLI must not import " + ip,
					Evidence: "import=" + ip,
				}
			}
		}
		// os/exec is allowed only for LookPath (call scan enforces).
		return nil
	}
	// Pure packages: full ban list. Also ban bare "os" for pure planning layers
	// that already enforce it package-locally — but catalog/spec/report may
	// need careful handling:
	//   - catalog production uses embed only (os only under foundrydev, skipped)
	//   - report/version/resolve/render/plan/spec: no os
	//   - diagnostic: no os in production
	for _, bad := range purityForbiddenImports {
		if ip == bad || strings.HasPrefix(ip, bad+"/") {
			return &PurityViolation{
				Package:  pkg,
				File:     rel,
				Detail:   "write-free package must not import " + ip,
				Evidence: "import=" + ip,
			}
		}
	}
	// Ban os in pure layers (read/write FS is CLI boundary only).
	// Exception: none for default-tagged production sources listed here.
	if ip == "os" || strings.HasPrefix(ip, "os/") {
		// os/exec already covered; remaining os/* under pure packages.
		if ip == "os" || (strings.HasPrefix(ip, "os/") && ip != "os/exec") {
			// Allow nothing under pure packages.
			return &PurityViolation{
				Package:  pkg,
				File:     rel,
				Detail:   "write-free pure package must not import " + ip + " (FS/env at CLI boundary)",
				Evidence: "import=" + ip,
			}
		}
	}
	return nil
}

// scanCLISubprocessCalls bans exec.Command / CommandContext and methods that
// start children. LookPath is the only allowed os/exec use on the write-free path.
func scanCLISubprocessCalls(fset *token.FileSet, f *ast.File, pkg, rel string) []PurityViolation {
	execAlias := "exec"
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p != "os/exec" {
			continue
		}
		if imp.Name != nil {
			execAlias = imp.Name.Name
		}
	}
	osAlias := "os"
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p != "os" {
			continue
		}
		if imp.Name != nil {
			osAlias = imp.Name.Name
		}
	}

	var out []PurityViolation
	// Banned selector names on exec package and on *exec.Cmd receivers.
	bannedExecFuncs := map[string]bool{
		"Command":        true,
		"CommandContext": true,
	}
	bannedCmdMethods := map[string]bool{
		"Start":          true,
		"Run":            true,
		"Output":         true,
		"CombinedOutput": true,
	}
	bannedOSFuncs := map[string]bool{
		"StartProcess": true,
	}

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pos := fset.Position(sel.Pos())
		loc := fmt.Sprintf("%s:%d", rel, pos.Line)

		// exec.Command / exec.CommandContext
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == execAlias {
			if bannedExecFuncs[sel.Sel.Name] {
				out = append(out, PurityViolation{
					Package:  pkg,
					File:     loc,
					Detail:   "write-free path must not start subprocesses via exec." + sel.Sel.Name,
					Evidence: "call=exec." + sel.Sel.Name + " count=1",
				})
			}
			// LookPath is allowed — no violation.
		}
		// os.StartProcess
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == osAlias {
			if bannedOSFuncs[sel.Sel.Name] {
				out = append(out, PurityViolation{
					Package:  pkg,
					File:     loc,
					Detail:   "write-free path must not call os." + sel.Sel.Name,
					Evidence: "call=os." + sel.Sel.Name + " count=1",
				})
			}
		}
		// cmd.Start / Run / Output / CombinedOutput — any receiver.
		if bannedCmdMethods[sel.Sel.Name] {
			// Only flag if this file imports os/exec (likely *exec.Cmd).
			// Avoid flagging unrelated Start methods on other types by requiring
			// the selector's package scope to have os/exec imported AND the
			// method name to be used in a CallExpr (already true).
			// Conservative: only when the file imports os/exec.
			hasExec := false
			for _, imp := range f.Imports {
				if strings.Trim(imp.Path.Value, `"`) == "os/exec" {
					hasExec = true
					break
				}
			}
			if hasExec {
				// Skip if X is the exec package itself (already handled).
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == execAlias {
					return true
				}
				out = append(out, PurityViolation{
					Package:  pkg,
					File:     loc,
					Detail:   "write-free path must not invoke Cmd." + sel.Sel.Name + " (subprocess start)",
					Evidence: "call=Cmd." + sel.Sel.Name + " count=1",
				})
			}
		}
		return true
	})
	return out
}

// skipPurityFile reports whether a source file is outside the release
// write-free surface (non-default build tags).
func skipPurityFile(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Check first ~2KB for go:build / +build lines.
	head := string(b)
	if len(head) > 2048 {
		head = head[:2048]
	}
	lines := strings.Split(head, "\n")
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "//") {
			if strings.Contains(trim, "go:build") && strings.Contains(trim, "foundrydev") {
				return true
			}
			if strings.Contains(trim, "+build") && strings.Contains(trim, "foundrydev") {
				return true
			}
			continue
		}
		// Stop at package clause.
		if strings.HasPrefix(trim, "package ") {
			break
		}
	}
	return false
}

// PurityPackagePresence returns which write-free packages exist under root
// (for super-suite logging). Sorted.
func PurityPackagePresence(root string) []string {
	var present []string
	for _, pkg := range WriteFreePackages {
		dir := filepath.Join(root, filepath.FromSlash(pkg))
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			// Require at least one production .go file.
			has := false
			_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					if d != nil && d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
					has = true
					return fs.SkipAll
				}
				return nil
			})
			if has {
				present = append(present, pkg)
			}
		}
	}
	sort.Strings(present)
	return present
}
