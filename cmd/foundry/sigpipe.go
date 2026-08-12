// SIGPIPE process-boundary setup (SPEC-FOUNDRY-002 Section 36.5 / REQ-158).
//
// Ownership: cmd/foundry/main.go is the sole site that registers the drained
// SIGPIPE subscription (Section 42.2). Child processes retain default SIGPIPE
// disposition because fork/exec resets handlers (SV-04).
package main

import (
	"os"
	"os/signal"
	"syscall"
)

// installDrainedSIGPIPE subscribes to SIGPIPE with a non-blocking drained
// channel so writes to a broken stdout/stderr return EPIPE instead of killing
// the Foundry process. This is what makes the commit-dominates-reporting exit
// contract (Section 31.9 / FND-012) enforceable under real broken pipes.
//
// Safe to call once at process start. The drain goroutine lives for the
// process lifetime. Children started via fork/exec do not inherit this
// subscription and keep conventional SIGPIPE behavior.
func installDrainedSIGPIPE() {
	// Buffer of 1 is enough: Notify never blocks on a full channel when the
	// buffer is non-zero and we continuously drain.
	sigpipe := make(chan os.Signal, 1)
	signal.Notify(sigpipe, syscall.SIGPIPE)
	go func() {
		for range sigpipe {
			// Drained: discard. Go runtime converts subsequent writes to EPIPE.
		}
	}()
}
