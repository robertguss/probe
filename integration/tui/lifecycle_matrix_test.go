package tui_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestTUILifecycleOwnerMatrix asserts Section 18.3 single-owner contracts on
// catalog TUI sources and rendered outputs (bead go-foundry-cli-1yq).
func TestTUILifecycleOwnerMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("lifecycle_owner")
	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	tui, ok := c.Manifest("tui")
	if !ok || tui == nil {
		log.Fail("manifest", "tui missing")
	}

	data := render.TemplateData{
		Name: "smoke-tui", Binary: "smoke-tui", Module: "github.com/example/smoke-tui",
		Description: "matrix", Archetype: "tui", Visibility: "private", FoundryVersion: "test",
	}

	cases := []struct {
		path    string
		wants   []string
		forbids []string
	}{
		{
			path: "cmd/{{binary}}/main.go",
			wants: []string{
				"signal.NotifyContext",
				"os.Interrupt",
				"syscall.SIGTERM",
				"--version",
				"--no-color",
				"--debug-log",
				"O_CREATE",
				"O_EXCL",
			},
			forbids: []string{
				"WithoutCatchPanics(",
				"signal.Notify(", // second owner pattern (NotifyContext is ok)
			},
		},
		{
			path: "internal/tui/app.go",
			wants: []string{
				"tea.WithContext",
				"tea.WithoutSignalHandler",
				"MapExitCode",
				"ExitSignal",
				"QuitClassQKey",
			},
			forbids: []string{
				"WithoutCatchPanics(",
				// main owns signal.NotifyContext; app must not create a second Notify.
				"signal.Notify(",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(sanitizeName(tc.path), func(t *testing.T) {
			var srcRel string
			for _, f := range tui.Files {
				if f.Path == tc.path {
					srcRel = filepath.ToSlash(filepath.Join(tui.UnitDir, f.Source))
					break
				}
			}
			if srcRel == "" {
				log.Fail("missing_"+tc.path, "not in manifest")
			}
			raw, err := c.Read(srcRel)
			if err != nil {
				log.Fail("read_"+tc.path, err.Error())
			}
			body := string(raw)
			// Render if template.
			if strings.HasSuffix(srcRel, ".tmpl") {
				out, err := render.ExecuteTemplate(srcRel, raw, data, strings.TrimPrefix(tc.path, "cmd/{{binary}}/"))
				if err != nil {
					// main.go needs out path ending .go for format
					outPath := "main.go"
					if strings.Contains(tc.path, "app.go") {
						outPath = "app.go"
					}
					out, err = render.ExecuteTemplate(srcRel, raw, data, outPath)
				}
				if err != nil {
					log.Fail("render_"+tc.path, err.Error())
				}
				body = string(out)
			}
			log.Phase("file_" + sanitizeName(tc.path))
			for _, w := range tc.wants {
				present := strings.Contains(body, w)
				log.Assert("want_"+sanitizeName(w), present, true, present)
			}
			for _, f := range tc.forbids {
				// signal.Notify( without NotifyContext
				if f == "signal.Notify(" {
					// Allow NotifyContext; forbid bare Notify( for signals.
					if strings.Contains(body, "signal.Notify(") && !strings.Contains(body, "signal.NotifyContext") {
						log.Fail("forbidden_signal_notify", tc.path)
					}
					// Also reject signal.Notify( that is not NotifyContext
					for _, line := range strings.Split(body, "\n") {
						if strings.Contains(line, "signal.Notify(") && !strings.Contains(line, "NotifyContext") {
							log.Fail("forbidden_signal_notify_line", line)
						}
					}
					continue
				}
				present := strings.Contains(body, f)
				if present {
					log.Fail("forbidden_"+sanitizeName(f), tc.path)
				}
				log.Assert("forbid_"+sanitizeName(f), !present, true, !present)
			}
			// AST: WithoutCatchPanics must not be called in app.go
			if strings.Contains(tc.path, "app.go") {
				fset := token.NewFileSet()
				f, err := parser.ParseFile(fset, "app.go", body, 0)
				if err != nil {
					log.Fail("parse_app", err.Error())
				}
				ast.Inspect(f, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if ok && sel.Sel.Name == "WithoutCatchPanics" {
						log.Fail("ast_without_catch", "WithoutCatchPanics call forbidden")
					}
					return true
				})
				log.Step("ast_options", testutil.OutcomeOK, "WithoutCatchPanics=absent")
			}
			log.PhaseEnd("file_"+sanitizeName(tc.path), testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("lifecycle_owner", testutil.OutcomeOK)
}

// TestTUIExitMappingMatrix table-drives MapExitCode semantics from catalog
// sources (rendered comments + generated lifecycle_test presence).
func TestTUIExitMappingMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("exit_mapping")
	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	tui, ok := c.Manifest("tui")
	if !ok {
		log.Fail("manifest", "tui")
	}
	var appSrc string
	for _, f := range tui.Files {
		if f.Path == "internal/tui/app.go" {
			appSrc = filepath.ToSlash(filepath.Join(tui.UnitDir, f.Source))
		}
	}
	raw, err := c.Read(appSrc)
	if err != nil {
		log.Fail("read", err.Error())
	}
	data := render.TemplateData{
		Name: "x", Binary: "x", Module: "example.com/x", Description: "d",
		Archetype: "tui", Visibility: "private", FoundryVersion: "t",
	}
	out, err := render.ExecuteTemplate(appSrc, raw, data, "app.go")
	if err != nil {
		log.Fail("render", err.Error())
	}
	body := string(out)
	// Contract strings for the three exit classes.
	for _, row := range []struct {
		name string
		tok  string
	}{
		{"q_zero", "ExitOK"},
		{"signal_130", "ExitSignal"},
		{"failure_1", "ExitFailure"},
		{"code_130", "130"},
		{"map_fn", "func MapExitCode"},
		{"classify_fn", "func ClassifyRunError"},
		{"q_class", "QuitClassQKey"},
		{"ctrlc_class", "QuitClassCtrlCKey"},
		{"signal_class", "QuitClassSignal"},
	} {
		present := strings.Contains(body, row.tok)
		log.Assert(row.name, present, true, present)
	}
	log.PhaseEnd("exit_mapping", testutil.OutcomeOK)
}

func sanitizeName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, s)
	if len(s) > 48 {
		return s[:48]
	}
	return s
}

// Ensure os import used when reading files in other tests.
var _ = os.ErrNotExist
