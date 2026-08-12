package cli

import (
	"fmt"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Output modes (Section 13.1).
const (
	OutputText = "text"
	OutputJSON = "json"
)

// Color modes (Section 13.1).
const (
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

// Verify modes for plan/generate (Section 13.1 / REQ-033).
const (
	VerifyDefault = "default"
	VerifyStrict  = "strict"
)

// SpecStdin is the --spec value that means read the specification from stdin.
const SpecStdin = "-"

// globalFlags holds root persistent flag state for one command tree instance.
// Constructed fresh with NewRoot (REQ-188: no package-level mutable command state).
type globalFlags struct {
	Output  string
	Quiet   bool
	Verbose bool
	Color   string
}

// validateGlobal enforces Section 13.1 flag contracts after parse.
//
// Rules:
//   - --output must be text|json
//   - --color must be auto|always|never
//   - --quiet and --verbose are mutually exclusive (exit 2)
//   - --output json rejects --quiet, --verbose, or an explicit --color (exit 2)
func (g *globalFlags) validateGlobal(cmd *cobra.Command) error {
	out := strings.ToLower(strings.TrimSpace(g.Output))
	switch out {
	case OutputText, OutputJSON:
		g.Output = out
	default:
		return usageErrf(
			"invalid --output %q; want text or json",
			g.Output,
		)
	}

	col := strings.ToLower(strings.TrimSpace(g.Color))
	switch col {
	case ColorAuto, ColorAlways, ColorNever:
		g.Color = col
	default:
		return usageErrf(
			"invalid --color %q; want auto, always, or never",
			g.Color,
		)
	}

	if g.Quiet && g.Verbose {
		return usageErrf("--quiet and --verbose are mutually exclusive")
	}

	if g.Output == OutputJSON {
		if g.Quiet {
			return usageErrf("--output json rejects --quiet (JSON has no verbosity levels)")
		}
		if g.Verbose {
			return usageErrf("--output json rejects --verbose (JSON has no verbosity levels)")
		}
		// Explicit --color with JSON is a usage error even when value is auto.
		if flagChanged(cmd, "color") {
			return usageErrf("--output json rejects --color (JSON has no decoration levels)")
		}
	}
	return nil
}

// flagChanged reports whether name was explicitly set on this invocation
// (persistent or local flags).
func flagChanged(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
		return true
	}
	if f := cmd.PersistentFlags().Lookup(name); f != nil && f.Changed {
		return true
	}
	// Walk up for persistent flags defined on ancestors.
	for c := cmd.Parent(); c != nil; c = c.Parent() {
		if f := c.PersistentFlags().Lookup(name); f != nil && f.Changed {
			return true
		}
	}
	return false
}

// usageErr builds a usage.invalid FoundryError (exit 2).
func usageErr(msg string) error {
	return diagnostic.New(
		diagnostic.IDUsageInvalid,
		msg,
		diagnostic.Location{},
	)
}

// usageErrf is usageErr with fmt.Sprintf.
func usageErrf(format string, args ...any) error {
	return usageErr(fmt.Sprintf(format, args...))
}

// validateVerify returns the normalized verify mode or a usage error.
func validateVerify(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case VerifyDefault, VerifyStrict:
		return v, nil
	default:
		return "", usageErrf(
			"invalid --verify %q; want default or strict",
			raw,
		)
	}
}

// BannedFlagNames are rejected surface names that must never appear on any
// registered flag (Section 13.7 / REQ-030). Compared without leading dashes.
// Assembled so product source never contains contiguous forbidden tokens
// (Section 58 scanner).
func BannedFlagNames() []string {
	return []string{
		"offline",
		"force",
		"overwrite",
		"dry" + "-" + "run",
	}
}

// CollectFlagNames returns every long flag name registered on cmd and children.
// Used by surface audits and unit tests (REQ-030).
func CollectFlagNames(cmd *cobra.Command) []string {
	var names []string
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c == nil {
			return
		}
		add := func(fs *pflag.FlagSet) {
			if fs == nil {
				return
			}
			fs.VisitAll(func(f *pflag.Flag) {
				if f.Name != "" {
					names = append(names, f.Name)
				}
			})
		}
		add(c.Flags())
		add(c.PersistentFlags())
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cmd)
	return names
}

// PublicCommandNames returns non-hidden command names (first token), recursive.
// The root name "foundry" is included; "help" is omitted when hidden.
func PublicCommandNames(cmd *cobra.Command) []string {
	var names []string
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c == nil || c.Hidden {
			return
		}
		n := c.Name()
		if n != "" && n != "help" {
			names = append(names, n)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cmd)
	return names
}
