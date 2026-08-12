package render_test

import (
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestTemplateIfWithoutElseDoesNotPanic guards the typed-nil ElseList walk
// (if with no else branch) used by Core AGENTS.md / README TUI conditionals.
func TestTemplateIfWithoutElseDoesNotPanic(t *testing.T) {
	log := testutil.New(t)
	log.Phase("if_no_else")
	src := []byte("before\n[[ if eq .Archetype \"tui\" -]]TUI-ONLY[[ end -]]\nafter\n")
	for _, arch := range []string{"cli", "tui"} {
		data := render.TemplateData{
			Name: "x", Binary: "x", Module: "example.com/x", Description: "d",
			Archetype: arch, Visibility: "private", FoundryVersion: "dev",
		}
		out, err := render.ExecuteTemplate("if_no_else", src, data, "x.md")
		if err != nil {
			log.Fail("execute_"+arch, err.Error())
		}
		got := string(out)
		if arch == "tui" {
			log.Assert("tui_has_block", strings.Contains(got, "TUI-ONLY"), true, strings.Contains(got, "TUI-ONLY"))
		} else {
			log.Assert("cli_omits_block", !strings.Contains(got, "TUI-ONLY"), true, !strings.Contains(got, "TUI-ONLY"))
		}
		log.Step("arch_"+arch, testutil.OutcomeOK, "bytes="+itoa(len(out)))
	}
	log.PhaseEnd("if_no_else", testutil.OutcomeOK)
}
