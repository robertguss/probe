package tui_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/render"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestTUITooSmallViewMatrix locks Section 18.4 / 18.9 minimum dimensions and
// deterministic too-small notice in catalog view sources.
func TestTUITooSmallViewMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("too_small")
	c, err := catalog.Load()
	if err != nil {
		log.Fail("load", err.Error())
	}
	tui, ok := c.Manifest("tui")
	if !ok {
		log.Fail("manifest", "tui")
	}
	var viewSrc, stateSrc, viewTestSrc string
	for _, f := range tui.Files {
		switch f.Path {
		case "internal/tui/view.go":
			viewSrc = filepath.ToSlash(filepath.Join(tui.UnitDir, f.Source))
		case "internal/tui/state.go":
			stateSrc = filepath.ToSlash(filepath.Join(tui.UnitDir, f.Source))
		case "internal/tui/view_test.go":
			viewTestSrc = filepath.ToSlash(filepath.Join(tui.UnitDir, f.Source))
		}
	}
	viewRaw, _ := c.Read(viewSrc)
	stateRaw, _ := c.Read(stateSrc)
	testRaw, _ := c.Read(viewTestSrc)
	view := string(viewRaw)
	state := string(stateRaw)
	tests := string(testRaw)

	mw := strings.Contains(state, "MinWidth")
	mh := strings.Contains(state, "MinHeight")
	ts := strings.Contains(view, "terminal too small")
	vt := strings.Contains(tests, "TooSmall") || strings.Contains(tests, "too small")
	nc := strings.Contains(view, "noColor") || strings.Contains(view, "NoColor") || strings.Contains(string(stateRaw), "NoColor")
	hv := strings.Contains(view, "help.View") || strings.Contains(view, "m.help")
	log.Assert("min_width", mw, true, mw)
	log.Assert("min_height", mh, true, mh)
	log.Assert("too_small_notice", ts, true, ts)
	log.Assert("view_test_too_small", vt, true, vt)
	log.Assert("no_color_path", nc, true, nc)
	log.Assert("help_view", hv, true, hv)
	log.PhaseEnd("too_small", testutil.OutcomeOK)
	_ = render.TemplateData{} // keep render import for future render-based view checks
}
