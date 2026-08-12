package render_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/render"
)

// FuzzTemplateInvalidNeverPanics feeds arbitrary template source through the
// restricted executor. Every input must either render successfully or return
// a stable FoundryError (render.failed) — never panic (P1.5.b acceptance).
func FuzzTemplateInvalidNeverPanics(f *testing.F) {
	seeds := []string{
		"",
		"plain text only\n",
		"Hello [[.Name]]\n",
		"[[.DoesNotExist]]\n",
		"[[define \"x\"]]y[[end]]\n",
		"[[template \"x\"]]\n",
		"[[block \"x\" .]]y[[end]]\n",
		"[[call .Name]]\n",
		"[[exec \"true\"]]\n",
		"[[join .Name \",\"]]\n",
		"[[quote .Name]]\n",
		"[[if .DistributionEnabled]]yes[[else]]no[[end]]\n",
		"{{.Name}} [[.Name]]\n",
		"[[.Name]] [[.Binary]] [[.Module]]\n",
		"[[" + strings.Repeat("a", 100) + "]]\n",
		"[[",
		"]]",
		"[[/* comment */]]\n",
		"package main\nfunc main() { [[.Binary]] }\n",
		string([]byte{0xff, 0xfe, 0x00}),
		"[[range .Name]]x[[end]]\n",
		"[[with .Name]][[.]][[end]]\n",
		"[[printf \"%s\" .Name]]\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	data := minimalCLITemplateData()

	f.Fuzz(func(t *testing.T, src string) {
		// Bound size and skip invalid UTF-8 thrash (still must not panic).
		if len(src) > 8192 {
			return
		}
		// Drop inputs with many non-printables beyond a small allowance so
		// the fuzzer spends budget on template structure.
		nonPrint := 0
		for _, r := range src {
			if r == utf8.RuneError || (!unicode.IsPrint(r) && r != '\n' && r != '\t' && r != '\r') {
				nonPrint++
			}
		}
		if nonPrint > 64 {
			return
		}

		// Path A: ExecuteTemplate (pure, no catalog).
		out, err := render.ExecuteTemplate("fuzz.tmpl", []byte(src), data, "out.txt")
		if err != nil {
			fe, ok := diagnostic.AsFoundryError(err)
			if !ok {
				t.Fatalf("non-FoundryError: %T %v", err, err)
			}
			if fe.ID() != diagnostic.IDRenderFailed {
				t.Fatalf("unexpected id %s for err %v", fe.ID(), err)
			}
			if out != nil {
				t.Fatalf("expected nil content on error, got %d bytes", len(out))
			}
			return
		}
		// Success: content is a byte slice (possibly empty); no CR left from
		// CRLF normalization path when CR was present is best-effort.
		if out == nil {
			t.Fatal("nil content without error")
		}

		// Path B: also exercise .go formatting path when source looks like Go.
		if strings.Contains(src, "package ") {
			_, _ = render.ExecuteTemplate("fuzz.go.tmpl", []byte(src), data, "main.go")
		}
	})
}

// FuzzTemplateDataFields ensures random safe field values never panic and
// stay deterministic for a fixed template body.
func FuzzTemplateDataFields(f *testing.F) {
	f.Add("name", "bin", "desc")
	f.Add("minimal-cli", "minimal-cli", "hello")
	f.Add("", "", "")
	f.Add("a/b", "c-d", "line\nwith\nbreaks")

	const body = "# [[.Name]]\nbinary=[[.Binary]]\ndesc=[[.Description]]\n"

	f.Fuzz(func(t *testing.T, name, binary, desc string) {
		if len(name)+len(binary)+len(desc) > 4096 {
			return
		}
		for _, s := range []string{name, binary, desc} {
			if !utf8.ValidString(s) {
				return
			}
		}
		data := render.TemplateData{
			Name:        name,
			Binary:      binary,
			Module:      "github.com/example/x",
			Description: desc,
			Archetype:   "cli",
			Visibility:  "private",
		}
		a, err1 := render.ExecuteTemplate("data.tmpl", []byte(body), data, "out.md")
		b, err2 := render.ExecuteTemplate("data.tmpl", []byte(body), data, "out.md")
		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("nondeterministic error: %v vs %v", err1, err2)
		}
		if err1 != nil {
			return
		}
		if string(a) != string(b) {
			t.Fatalf("nondeterministic output\n--- a\n%s\n--- b\n%s", a, b)
		}
		if render.ContentDigest(a) != render.ContentDigest(b) {
			t.Fatal("digest mismatch across identical runs")
		}
	})
}
