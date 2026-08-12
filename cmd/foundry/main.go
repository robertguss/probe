// Command foundry is the process boundary for the Foundry CLI.
//
// This file is the only permitted os.Exit site (REQ-181) and registers the
// drained SIGPIPE handler required by Section 36.5 so a broken pipe cannot
// kill the process mid-transaction.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/robertguss/go-foundry-cli/internal/cli"
)

func main() {
	// Section 36.5 / REQ-158 / FND-012: drained SIGPIPE so broken stdout/stderr
	// surfaces as EPIPE (commit-dominates-reporting is then enforceable).
	installDrainedSIGPIPE()

	// Single signal owner for cancellation (Section 13.6 / REQ-036): SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	code := run(ctx, os.Args[1:], cli.Streams{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	})
	os.Exit(code)
}

// run is the sole error-to-exit translation point (REQ-181). streams is
// injectable (bead go-foundry-cli-wet.2.5) so unit tests can exercise the
// process entry point end-to-end with buffers instead of swapping the
// os.Stdout/os.Stderr globals or spawning a real subprocess.
func run(ctx context.Context, args []string, streams cli.Streams) int {
	return cli.Run(ctx, args, streams, cli.Options{})
}
