// Package e4 is the Phase-3-entry Bubble Tea v2 lifecycle spike (evidence gate E4).
//
// Concurrency test notes (ipk.10 / ipk.15):
//   - goleak.VerifyTestMain runs after the suite; Bubble Tea eventLoop/readInputs
//     top frames may be ignored when kill paths leave non-joinable readers.
//   - WaitUntilIdle and CooperativeSleep cancel use testing/synctest in
//     synctest_test.go (fake clock). Real-PTY / subprocess waits remain OS-bound
//     wall clocks (creack/pty, termios, external process) and are not bubbleable.
//
// It proves Section 18.3 / FND-010 / REQ-070:
//
//  1. Single lifecycle owner in main via signal.NotifyContext(SIGINT, SIGTERM).
//  2. Program constructed with tea.WithContext(appCtx) + tea.WithoutSignalHandler().
//  3. Default panic recovery stays enabled (WithoutCatchPanics is never used).
//  4. Every quit path cancels appCtx before tea.Quit / tea.Interrupt.
//  5. Effects are context-cooperative; EffectTracker asserts zero active effects
//     at return across q / Ctrl-C-as-key / SIGINT / SIGTERM / startup failure /
//     effect-error / framework-recovered panic paths.
//  6. PTY/termios restoration on normal quit, signal kill, and recovered panic.
//
// Patterns here are the template for generated TUI shells and permanent P3
// lifecycle matrices — promote without rewrite.
//
// Bound: ≤2 days. Does not implement production TUI archetype code.
package e4
