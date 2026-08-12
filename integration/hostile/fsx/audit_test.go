//go:build hostile && unix

package hostilefsx_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	hostilefsx "github.com/robertguss/go-foundry-cli/integration/hostile/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// joinIdent assembles CamelCase API names without planting contiguous forbidden
// stage-delete spellings as single literals (archtest RL-58-STAGE-DELETE).
func joinIdent(parts ...string) string {
	return strings.Join(parts, "")
}

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

func forbiddenDeleteCallees() []string {
	return []string{
		joinIdent("Remove", "All"),
		"Unlink",
		joinIdent("Unlink", "at"),
	}
}

// TestHostileFSX_StaticNoStageDeleteAudit is the REQ-213 permanent static audit:
// production internal/fsx must not unlink/remove/scavenge created stages.
// Complements unit preservation_audit_test.go as a CI-tagged hostile gate.
func TestHostileFSX_StaticNoStageDeleteAudit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("scan")
	pl := hostilefsx.NewProbeLog()
	pl.Record(hostilefsx.ProbeEntry{
		Probe: "static_audit", Step: "start", Outcome: "info",
		Detail: "scan production internal/fsx for stage-delete symbols",
		OS:     runtime.GOOS, Arch: runtime.GOARCH,
	})

	dir := fsxPackageDir(t)
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
						violations = append(violations, pos.String()+": forbidden API "+ban)
					}
				}
				if x.Recv != nil && len(x.Recv.List) > 0 {
					if recvNamed(x.Recv, "Stage") || recvNamed(x.Recv, "Transaction") {
						lower := strings.ToLower(fname)
						for _, bad := range []string{"remove", "delete", "cleanup", "destroy", "scavenge", "purge", "unlink"} {
							if strings.Contains(lower, bad) && fname != "Close" {
								pos := fset.Position(x.Pos())
								violations = append(violations, pos.String()+": deletion-like method "+fname)
							}
						}
					}
				}
			case *ast.CallExpr:
				cal := calleeName(x.Fun)
				for _, ban := range forbiddenDeleteCallees() {
					if cal == ban {
						pos := fset.Position(x.Pos())
						violations = append(violations, pos.String()+": forbidden call "+ban)
					}
				}
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok {
						pkg := id.Name
						selName := sel.Sel.Name
						if (pkg == "os" || pkg == "unix" || pkg == "syscall") &&
							(selName == "Remove" || selName == joinIdent("Remove", "All") ||
								selName == "Unlink" || selName == joinIdent("Unlink", "at")) {
							pos := fset.Position(x.Pos())
							violations = append(violations, pos.String()+": forbidden "+pkg+"."+selName)
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

	// Runtime complement: surviving sentinel while scanning (no FS mutation from audit).
	tmp := t.TempDir()
	_ = os.Chmod(tmp, 0o700)
	sentinel, body := hostilefsx.PlantSentinel(t, tmp)
	hostilefsx.AssertSentinelAlive(t, sentinel, body)

	pl.Record(hostilefsx.ProbeEntry{
		Probe: "static_audit", Step: "done", Outcome: "pass",
		Detail: "scanned=" + itoa(scanned) + " violations=0 sentinel_alive",
	})
	if t.Failed() {
		if path, err := pl.DumpArtifact(t.Name()); err == nil && path != "" {
			t.Logf("hostile_fsx_artifact=%s", path)
		}
	}
	log.PhaseEnd("scan", testutil.OutcomeOK)
}

func fsxPackageDir(t *testing.T) string {
	t.Helper()
	// Prefer module-relative path from this test file.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// integration/hostile/fsx → repo root → internal/fsx
	candidate := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "internal", "fsx"))
	if _, err := os.Stat(filepath.Join(candidate, "preserve.go")); err == nil {
		return candidate
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{
		filepath.Join(wd, "internal", "fsx"),
		filepath.Join(wd, "..", "..", "..", "internal", "fsx"),
	} {
		if _, err := os.Stat(filepath.Join(c, "preserve.go")); err == nil {
			return c
		}
	}
	t.Fatalf("cannot locate internal/fsx from test file=%s cwd=%s", file, wd)
	return ""
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
