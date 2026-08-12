package render_test

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

// Forbidden production imports for internal/render (Phase 1 purity / REQ-184).
// Render produces bytes/inventory only; no destination FS, network, or exec.
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
	"path/filepath", // FS-facing; render uses path only
	"embed",
	"plugin",
	"math/rand",
	"crypto/rand",
	"database/sql",
	"time", // no clock in pure render
	"github.com/spf13/cobra",
	// Deleted Stage-4 emitters / frameworks (FND-015).
	"gopkg.in/yaml.v3",
	"gopkg.in/yaml.v2",
	"sigs.k8s.io/yaml",
}

// TestPurityNoSideEffectImports scans production sources under internal/render.
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
			// Third-party: only golang.org/x/mod/* is admitted outside this module.
			if strings.HasPrefix(p, "github.com/") &&
				!strings.HasPrefix(p, "github.com/robertguss/go-foundry-cli/") {
				log.Fail("third_party", name+" imports "+p)
			}
			if !isStdlibOrAdmitted(p) {
				log.Fail("non_admitted_import", name+" imports "+p)
			}
			// No upward layer imports (plan/fsx/cli/…).
			if strings.HasPrefix(p, "github.com/robertguss/go-foundry-cli/internal/") {
				rest := strings.TrimPrefix(p, "github.com/robertguss/go-foundry-cli/internal/")
				switch {
				case rest == "catalog", rest == "diagnostic",
					strings.HasPrefix(rest, "catalog/"),
					strings.HasPrefix(rest, "diagnostic/"):
					// allowed lower layers
				default:
					log.Fail("upward_import", name+" imports "+p)
				}
			}
		}
	}
	log.Assert("has_prod_files", prodFiles > 0, true, prodFiles > 0)
	log.Step("prod_files", testutil.OutcomeOK, "n="+itoa(prodFiles))
	log.PhaseEnd("purity_scan", testutil.OutcomeOK)
}

// TestPurityNoDeletedEmitters bans Stage-4 token language / typed YAML emitter
// identifiers in production sources (FND-015 / REQ-096).
func TestPurityNoDeletedEmitters(t *testing.T) {
	log := testutil.New(t)
	log.Phase("no_deleted_emitters")

	dir := packageDir(t)
	// type/func declaration prefixes for deleted machinery.
	forbiddenDecls := []string{
		"type TokenSubst",
		"type TokenLanguage",
		"type YAMLEmitter",
		"type GoReleaserEmitter",
		"type GitignoreEmitter",
		"type DependabotEmitter",
		"type WorkflowEmitter",
		"func EmitYAML",
		"func EmitGoReleaser",
		"func EmitWorkflow",
		"func SubstituteTokens",
		"func TokenReplace",
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
		for _, decl := range forbiddenDecls {
			if strings.Contains(src, decl) {
				log.Fail("deleted_emitter", name+" contains "+decl)
			}
		}
		// Mechanism surface must not invent a fourth product mechanism constant.
		if strings.Contains(src, `Mechanism = "yaml"`) ||
			strings.Contains(src, `Mechanism = "token"`) ||
			strings.Contains(src, `Mechanism = "workflow"`) {
			log.Fail("extra_mechanism", name)
		}
		log.Step("scanned", testutil.OutcomeOK, "file="+name)
	}
	log.PhaseEnd("no_deleted_emitters", testutil.OutcomeOK)
}

// isStdlibOrAdmitted reports whether import path p is stdlib, this module,
// or the admitted golang.org/x/mod dependency for typed go.mod generation.
func isStdlibOrAdmitted(p string) bool {
	if p == "" {
		return false
	}
	// This module.
	if strings.HasPrefix(p, "github.com/robertguss/go-foundry-cli/") {
		return true
	}
	// Admitted external dependency for REQ-095.
	if p == "golang.org/x/mod" || strings.HasPrefix(p, "golang.org/x/mod/") {
		return true
	}
	// golang.org other than x/mod is not admitted.
	if strings.HasPrefix(p, "golang.org/") {
		return false
	}
	// Other domain third-party (github already filtered; catch gopkg, etc.).
	if strings.Contains(p, ".") {
		// stdlib has no dots (except rare historical); third-party domains do.
		// path/filepath is stdlib but banned separately. encoding/json is ok.
		// Heuristic: if first element contains a dot, it is a domain import.
		first, _, _ := strings.Cut(p, "/")
		if strings.Contains(first, ".") {
			return false
		}
	}
	return true
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
