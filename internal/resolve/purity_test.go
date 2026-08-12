package resolve_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// Forbidden production imports for internal/resolve (REQ-098 purity).
// Tests may import testing and testutil; production .go files must not.
var forbiddenProdImports = []string{
	"os",
	"os/exec",
	"os/user",
	"net",
	"net/http",
	"net/url",
	"syscall",
	"log",
	"log/slog",
	"io/ioutil",
	"path/filepath", // FS-facing; resolve uses path only
	"embed",
	"plugin",
	"runtime",
	"time", // no clock side effects in pure resolve
	"math/rand",
	"crypto/rand",
	"database/sql",
	"github.com/spf13/cobra",
}

// TestPurityNoSideEffectImports scans production sources under internal/resolve
// for forbidden imports (acceptance: purity tests).
func TestPurityNoSideEffectImports(t *testing.T) {
	log := testutil.New(t)
	log.Phase("purity_scan")

	dir := packageDir(t)
	log.Fixture("package", dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fail("readdir", err.Error())
	}

	fset := token.NewFileSet()
	var prodFiles int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		prodFiles++
		path := filepath.Join(dir, name)
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			log.Fail("parse_"+name, err.Error())
		}
		log.Step("scan", testutil.OutcomeOK, "file="+name)
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenProdImports {
				if p == bad || strings.HasPrefix(p, bad+"/") {
					log.Fail("forbidden_import", name+" imports "+p)
				}
			}
			// No third-party beyond catalog/diagnostic/spec module paths.
			if strings.HasPrefix(p, "github.com/") &&
				!strings.HasPrefix(p, "github.com/robertguss/go-foundry-cli/") {
				log.Fail("third_party", name+" imports "+p)
			}
		}
	}
	log.Assert("has_prod_files", prodFiles > 0, true, prodFiles > 0)
	log.Step("prod_files", testutil.OutcomeOK, "n="+itoa(prodFiles))
	log.PhaseEnd("purity_scan", testutil.OutcomeOK)
}

// TestPurityNoGraphIdentifiers ensures FND-009 / Section 23.3 absences:
// no requires/conflicts/capability/provenance graph machinery tokens as
// declared type or function names in production sources.
func TestPurityNoGraphIdentifiers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("flat_semantics")

	dir := packageDir(t)
	forbiddenIdents := []string{
		"Capability",
		"Provenance",
		"RequiresEdge",
		"ConflictGraph",
		"TopologicalSort",
		"DFS",
		"CycleDetect",
		"ProviderRegistry",
		"HelperBinary",
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fail("readdir", err.Error())
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			log.Fail("read", err.Error())
		}
		src := string(b)
		for _, id := range forbiddenIdents {
			// Word-boundary-ish: avoid matching comments that document absence.
			// Fail only on type/func declarations.
			for _, prefix := range []string{"type " + id, "func " + id, "func ("} {
				if prefix == "func (" {
					// skip
					continue
				}
				if strings.Contains(src, prefix) {
					log.Fail("graph_ident", name+" contains "+prefix)
				}
			}
		}
		log.Step("scanned", testutil.OutcomeOK, "file="+name)
	}
	log.PhaseEnd("flat_semantics", testutil.OutcomeOK)
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}
