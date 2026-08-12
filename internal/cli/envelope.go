package cli

import (
	"io"
	"os"

	"github.com/robertguss/go-foundry-cli/internal/report"
)

// newEncoder builds a report.Encoder from global flags and streams.
// JSON mode always uses ColorNever; NO_COLOR is honored for text diagnostics.
func newEncoder(stdout, stderr io.Writer, g *globalFlags) *report.Encoder {
	opts := report.Options{
		Mode:    report.ModeText,
		Quiet:   g != nil && g.Quiet,
		Verbose: g != nil && g.Verbose,
		Color:   report.ColorAuto,
		NOColor: os.Getenv("NO_COLOR") != "",
	}
	if g != nil {
		switch g.Output {
		case OutputJSON:
			opts.Mode = report.ModeJSON
			opts.Quiet = false
			opts.Verbose = false
			opts.Color = report.ColorNever
		default:
			opts.Mode = report.ModeText
			switch g.Color {
			case ColorAlways:
				opts.Color = report.ColorAlways
			case ColorNever:
				opts.Color = report.ColorNever
			default:
				opts.Color = report.ColorAuto
			}
		}
	}
	// StderrIsTerminal: best-effort via type assert; tests use buffers (false).
	if f, ok := stderr.(*os.File); ok {
		opts.StderrIsTerminal = isTerminal(f)
	}
	return report.New(stdout, stderr, opts)
}

// isTerminal reports whether f is a character device (cheap, no cgo).
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
