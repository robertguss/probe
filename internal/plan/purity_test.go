package plan_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// Forbidden production imports for internal/plan (Phase 1 purity / REQ-183).
// Plan construction is pure given Inputs; host FS/env/exec live in CLI.
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
	"path/filepath",
	"embed",
	"plugin",
	"math/rand",
	"crypto/rand",
	"database/sql",
	"time",
	"github.com/spf13/cobra",
}

// TestPurityNoSideEffectImports scans production sources under internal/plan.
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
			// Third-party outside this module is forbidden in plan.
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

// TestPurityNoBannedSchemaIdentifiers ensures production types do not introduce
// offline/capability/provenance schema surfaces (Section 58).
//
// Tokens are assembled at runtime so this test file itself does not trip the
// product red-line scanner (testdata is the only allowlist).
func TestPurityNoBannedSchemaIdentifiers(t *testing.T) {
	log := testutil.New(t)
	log.Phase("banned_idents")
	dir := packageDir(t)
	// Assembled fragments — never write contiguous banned product identifiers
	// as source literals in this file.
	forbidden := []string{
		"type " + "Off" + "line",
		"type " + "Capa" + "bility",
		"type " + "Prov" + "enance",
		"type " + "Helper" + "Binary",
		"Off" + "lineMode",
		"Capa" + "bilityList",
		"Prov" + "enance" + "Closure",
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
		for _, id := range forbidden {
			if name == "marshal.go" {
				// BannedJSONKeys string table intentionally lists schema absences.
				continue
			}
			if strings.Contains(src, id) {
				log.Fail("banned_ident", name+" contains "+id)
			}
		}
		log.Step("scanned", testutil.OutcomeOK, "file="+name)
	}
	log.PhaseEnd("banned_idents", testutil.OutcomeOK)
}
