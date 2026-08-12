// Command e4spike is a small lifecycle-owned TUI used by E4 PTY/subprocess probes.
//
// It is the promotable shape of a generated TUI main:
//
//	appCtx, stop := signal.NotifyContext(...)
//	defer stop()
//	program := tea.NewProgram(model, tea.WithContext(appCtx), tea.WithoutSignalHandler())
//	...
//	os.Exit(mapExit(...))
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	e4 "github.com/robertguss/go-foundry-cli/integration/hostile/e4"
)

func main() {
	mode := flag.String("mode", "idle", "idle|hold|effect|fail")
	alt := flag.Bool("alt-screen", true, "use alternate screen buffer")
	flag.Parse()

	appCtx, stop := e4.NewLifecycleContext(nil)
	defer stop()

	var mmode e4.ModelMode
	switch *mode {
	case "hold":
		mmode = e4.ModeHold
	case "effect":
		mmode = e4.ModeWithEffect
	case "fail":
		mmode = e4.ModeFailingEffect
	default:
		mmode = e4.ModeIdle
	}

	log := e4.NewProbeLog()
	model := e4.NewSpikeModel(e4.SpikeModelConfig{
		AppCtx:         appCtx,
		Cancel:         stop,
		Log:            log,
		Mode:           mmode,
		EffectDuration: 60 * time.Second,
		UseAltScreen:   *alt,
	})

	// Real stdin/stdout (PTY when launched under creack/pty).
	res := e4.Run(e4.Config{
		AppCtx: appCtx,
		Cancel: stop,
		Model:  model,
		Log:    log,
		// Input/Output nil → bubbletea defaults to os.Stdin/os.Stdout.
	})

	fmt.Fprintf(os.Stderr, "e4spike exit_code=%d quit_class=%s active_effects=%d signal_owner=%s\n",
		res.ExitCode, res.QuitClass, res.ActiveEffects, res.SignalOwner)
	os.Exit(res.ExitCode)
}
