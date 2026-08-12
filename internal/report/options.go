package report

// Mode is the global --output mode (Section 13.1).
type Mode string

const (
	// ModeText is plain line-oriented human output (default).
	ModeText Mode = "text"
	// ModeJSON is one deterministic JSON document on stdout (Section 37).
	ModeJSON Mode = "json"
)

// ColorMode is the global --color mode (text only; Section 13.1 / 36.1).
type ColorMode string

const (
	// ColorAuto colors stderr diagnostics only when stderr is a terminal
	// and NO_COLOR is unset.
	ColorAuto ColorMode = "auto"
	// ColorAlways forces color on text diagnostics.
	ColorAlways ColorMode = "always"
	// ColorNever disables color (also forced when NO_COLOR is set).
	ColorNever ColorMode = "never"
)

// Options configures one Encoder. Constructed per invocation; no package
// globals (REQ-188).
type Options struct {
	// Mode is text or json. Default ModeText when empty.
	Mode Mode
	// Quiet suppresses progress and summary lines in text mode (Section 36.3).
	// Never suppresses errors, network disclosure, stage_path, or destination.
	Quiet bool
	// Verbose adds diagnostic detail to stderr in text mode.
	Verbose bool
	// Color controls diagnostic decoration in text mode.
	Color ColorMode
	// NOColor is true when the NO_COLOR environment variable is set (any
	// value). When true, color is disabled regardless of Color mode.
	NOColor bool
	// StderrIsTerminal is true when stderr is a terminal (for ColorAuto).
	StderrIsTerminal bool
}

// normalized returns opts with defaults applied.
func (o Options) normalized() Options {
	out := o
	if out.Mode == "" {
		out.Mode = ModeText
	}
	if out.Color == "" {
		out.Color = ColorAuto
	}
	return out
}

// colorEnabled reports whether ANSI color may be applied to text diagnostics.
func (o Options) colorEnabled() bool {
	o = o.normalized()
	if o.Mode != ModeText {
		return false
	}
	if o.NOColor {
		return false
	}
	switch o.Color {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default: // auto
		return o.StderrIsTerminal
	}
}
