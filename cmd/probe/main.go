package main

import (
	"context"
	"os"

	"github.com/robertguss/probe/internal/engine"
)

func main() {
	eng := engine.New(engine.Options{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Stdin:   os.Stdin,
		Environ: os.Environ(),
	})
	res := eng.Run(context.Background(), os.Args[1:])
	os.Exit(int(res.ExitCode))
}
