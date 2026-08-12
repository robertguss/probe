// Package tui holds Phase 3 lifecycle / PTY / debug-log matrices for the
// generated TUI archetype (Section 18.3–18.8, bead go-foundry-cli-1yq).
//
// PTY restoration smoke tests live in files with build tag tui_pty so the
// default Linux CI unit job stays free of flaky terminal races; enable with:
//
//	go test -tags=tui_pty ./integration/tui/
//
// Default (always-on) matrices cover single lifecycle owner static options,
// debug-log safe-basename exclusive-create, exit mapping, and too-small view.
package tui
