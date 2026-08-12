package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const modulePath = "github.com/robertguss/go-foundry-cli"

// ForbiddenPackageDirs must not exist anywhere under the module (REQ-180).
// Prefer ForbiddenRedlinePackageDirs for red-line-tagged failures (Section 58).
// Paths assembled via pkg() — keep in sync with ForbiddenRedlinePackageDirs.
var ForbiddenPackageDirs = []string{
	pkg("compose"),
	pkg("structured"),
	pkg("provenance"),
	pkg("capability"),
	pkg("greet"),
}

// LayerOrder is the Section 42.3 pure/side-effect layering. Lower index = lower
// layer. A package may only import same-or-lower layers (plus stdlib and
// third-party). Packages not in this list (cmd, integration, testutil, archtest)
// are handled by separate rules.
var LayerOrder = []string{
	"internal/diagnostic",
	"internal/spec",
	"internal/catalog",
	"internal/resolve",
	"internal/render",
	"internal/plan",
	"internal/fsx",
	"internal/toolrun",
	"internal/gitinit",
	"internal/verify",
	"internal/generate",
	"internal/report",
	"internal/version",
	"internal/cli",
}

// CobraImportAllowlist packages that may import github.com/spf13/cobra.
// "tools" retains the BOM pin via //go:build tools until internal/cli lands.
var CobraImportAllowlist = map[string]bool{
	"cmd/foundry":  true,
	"internal/cli": true,
	"tools":        true,
}

// Violation is one architecture rule failure.
type Violation struct {
	Rule    string
	Package string
	File    string
	Detail  string
}

func (v Violation) String() string {
	if v.File != "" {
		return fmt.Sprintf("%s: package=%s file=%s detail=%s", v.Rule, v.Package, v.File, v.Detail)
	}
	return fmt.Sprintf("%s: package=%s detail=%s", v.Rule, v.Package, v.Detail)
}

// RepoRoot walks up from start (or cwd) to find go.mod for this module.
func RepoRoot(start string) (string, error) {
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = wd
	}
	dir := start
	for {
		gomod := filepath.Join(dir, "go.mod")
		if b, err := os.ReadFile(gomod); err == nil {
			if strings.Contains(string(b), modulePath) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod for %s not found from %s", modulePath, start)
		}
		dir = parent
	}
}

// Check runs all architecture rules and returns violations.
// Section 58 red-line negatives (CheckRedlines) are included so P1.8 (4hi)
// consumes one suite — failures name the redline id in Violation.Rule.
//
// Write-free purity (CheckPurity) and golden CI discipline
// (CheckGoldenDiscipline) are separate entry points also required by P1.8;
// TestP1QualitySuperSuite runs the full set. Check stays focused on import
// graph / forbidden packages / red-lines so layer failures stay legible.
func Check(root string) ([]Violation, error) {
	var all []Violation

	for _, rel := range ForbiddenPackageDirs {
		p := filepath.Join(root, rel)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			rule := "forbidden_package"
			if id, ok := ForbiddenRedlinePackageDirs[rel]; ok {
				rule = id
			}
			all = append(all, Violation{
				Rule:    rule,
				Package: rel,
				Detail:  "deleted/rejected package must not exist (REQ-180 / Section 58); " + RedlineRemediation,
			})
		}
	}

	// Full Section 58 mechanical suite (tokens, identifiers, imports, windows).
	// Package-dir hits may also appear above; redline check is authoritative naming.
	reds, err := CheckRedlines(root)
	if err != nil {
		return all, err
	}
	for _, rv := range reds {
		// Skip pure package-dir duplicates already emitted with the same id+path.
		if rv.Line == 0 && ForbiddenRedlinePackageDirs[rv.File] == rv.ID {
			// Already covered by ForbiddenPackageDirs loop with same Rule id.
			continue
		}
		all = append(all, Violation{
			Rule:    rv.ID,
			Package: packageOf(rv.File),
			File:    locFile(rv.File, rv.Line),
			Detail:  rv.Detail + snippetDetail(rv.Snippet) + "; " + RedlineRemediation,
		})
	}

	pkgs, err := scanPackages(root)
	if err != nil {
		return all, err
	}

	layerIdx := map[string]int{}
	for i, l := range LayerOrder {
		layerIdx[l] = i
	}

	for pkgPath, files := range pkgs {
		for _, f := range files {
			imps, exitCalls, err := parseFileImportsAndExits(f.path)
			if err != nil {
				return all, fmt.Errorf("%s: %w", f.path, err)
			}
			relFile, _ := filepath.Rel(root, f.path)

			// os.Exit only in cmd/foundry product code (REQ-181).
			// Allow: *_test.go, integration/ evidence spikes, tools/ retain stubs.
			if exitCalls > 0 && !osExitAllowed(pkgPath, f.path) {
				all = append(all, Violation{
					Rule:    "os_exit",
					Package: pkgPath,
					File:    relFile,
					Detail:  fmt.Sprintf("os.Exit call count=%d; only cmd/foundry may call os.Exit in product code", exitCalls),
				})
			}

			for _, imp := range imps {
				// Production must not import testutil.
				if isTestutilImport(imp) && !isTestOnlyFile(f.path) && !strings.HasPrefix(pkgPath, "internal/testutil") {
					// Non-test .go under production packages.
					if !strings.HasSuffix(f.path, "_test.go") {
						all = append(all, Violation{
							Rule:    "testutil_import",
							Package: pkgPath,
							File:    relFile,
							Detail:  "production code must not import internal/testutil",
						})
					}
				}
				// Even test files outside testutil/archtest are fine to import
				// testutil; only production .go is forbidden. (Handled above.)

				// Cobra allowlist (REQ-187).
				if isCobraImport(imp) && !CobraImportAllowlist[pkgPath] {
					// Test files under other packages also should not pull cobra
					// except through cli — keep strict for product graph.
					if !strings.HasSuffix(f.path, "_test.go") {
						all = append(all, Violation{
							Rule:    "cobra_import",
							Package: pkgPath,
							File:    relFile,
							Detail:  "cobra may only be imported by cmd/foundry or internal/cli",
						})
					}
				}

				// Layer direction for internal/* product packages.
				if to := internalImportPath(imp); to != "" {
					fromIdx, fromOK := layerIdx[pkgPath]
					toIdx, toOK := layerIdx[to]
					if fromOK && toOK && toIdx > fromIdx {
						// Higher layer import — upward dependency.
						// Allow generate to import lower side-effect packages
						// (already lower index... generate is high, fsx is lower:
						// toIdx < fromIdx is OK). Upward means toIdx > fromIdx.
						all = append(all, Violation{
							Rule:    "layer_direction",
							Package: pkgPath,
							File:    relFile,
							Detail:  fmt.Sprintf("must not import higher layer %s (Section 42.3)", to),
						})
					}
					// Lower packages must never import internal/cli.
					if fromOK && to == "internal/cli" && pkgPath != "internal/cli" {
						all = append(all, Violation{
							Rule:    "layer_direction",
							Package: pkgPath,
							File:    relFile,
							Detail:  "only boundary packages may import internal/cli",
						})
					}
				}
			}
		}
	}

	return all, nil
}

type fileInfo struct {
	path string
}

func scanPackages(root string) (map[string][]fileInfo, error) {
	out := map[string][]fileInfo{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", ".beads", ".idea", ".vscode", "vendor", "node_modules", "testdata":
				return filepath.SkipDir
			}
			// Catalog holds generated-project sources (templates/static embeds),
			// not Foundry product packages. .go files there (e.g. static
			// completion.go) may import Cobra for the *generated* CLI and must
			// not be subject to Foundry's product import graph (REQ-187).
			if name == "catalog" && filepath.Dir(path) == root {
				return filepath.SkipDir
			}
			// Skip hidden dirs except root.
			if strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		// Skip this package's own tree? No — include everything.
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = "" // module root (none expected)
		}
		out[pkg] = append(out[pkg], fileInfo{path: path})
		return nil
	})
	return out, err
}

func parseFileImportsAndExits(path string) (imports []string, exitCalls int, err error) {
	fset := token.NewFileSet()
	// Parse all files including those with build tags by ignoring tags? Use
	// ParseComments + full mode. Build-tagged platform files still parse.
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, 0, err
	}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		imports = append(imports, path)
	}
	// Count os.Exit calls.
	var osAlias = "os"
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p == "os" {
			if imp.Name != nil {
				osAlias = imp.Name.Name
			}
		}
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
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name == osAlias && sel.Sel.Name == "Exit" {
			exitCalls++
		}
		return true
	})
	return imports, exitCalls, nil
}

func isTestutilImport(imp string) bool {
	return imp == modulePath+"/internal/testutil" || strings.HasSuffix(imp, "/internal/testutil")
}

func isCobraImport(imp string) bool {
	return imp == "github.com/spf13/cobra"
}

func internalImportPath(imp string) string {
	prefix := modulePath + "/"
	if !strings.HasPrefix(imp, prefix) {
		return ""
	}
	rel := strings.TrimPrefix(imp, prefix)
	if !strings.HasPrefix(rel, "internal/") {
		return ""
	}
	// Strip subpackages: internal/fsx/linux → internal/fsx for layer purposes.
	parts := strings.Split(rel, "/")
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return rel
}

func isTestOnlyFile(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}

func packageOf(relFile string) string {
	if relFile == "" {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(relFile))
	if dir == "." {
		return ""
	}
	return dir
}

func locFile(rel string, line int) string {
	if line <= 0 {
		return rel
	}
	return fmt.Sprintf("%s:%d", rel, line)
}

func snippetDetail(snip string) string {
	if snip == "" {
		return ""
	}
	return fmt.Sprintf(" snippet=%q", snip)
}

// osExitAllowed reports whether a file may call os.Exit.
// Product code: only cmd/foundry. Tests and integration spikes are exempt.
func osExitAllowed(pkgPath, file string) bool {
	if pkgPath == "cmd/foundry" {
		return true
	}
	if strings.HasSuffix(file, "_test.go") {
		return true
	}
	// Evidence spikes and temporary probe binaries under integration/.
	if strings.HasPrefix(pkgPath, "integration/") {
		return true
	}
	// BOM retain package is build-tagged tools and never ships in the binary.
	if pkgPath == "tools" {
		return true
	}
	return false
}
