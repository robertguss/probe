package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Options configures Engine IO and test hooks.
type Options struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Stdin   io.Reader
	Environ []string

	Getwd func() (string, error)
	Now   func() time.Time
	HTTP  http.RoundTripper

	SpikeDir    string
	CatalogDir  string
	JSONDefault bool
}

// Engine owns process-lifetime probe state and command dispatch.
type Engine struct {
	opts    Options
	environ map[string]string
	jsonOut bool
	last    Result
	helped  bool
}

// New builds an Engine with defaults for missing writers.
func New(opts Options) *Engine {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	environ := make(map[string]string, len(opts.Environ))
	for _, kv := range opts.Environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			environ[k] = v
		}
	}
	return &Engine{
		opts:    opts,
		environ: environ,
		jsonOut: opts.JSONDefault,
	}
}

// Run parses args, dispatches a command, writes output, and returns Result.
// Never calls os.Exit.
func (e *Engine) Run(ctx context.Context, args []string) Result {
	e.last = Result{}
	e.helped = false
	e.jsonOut = e.opts.JSONDefault

	root := e.rootCmd(ctx)
	root.SetArgs(args)
	root.SetOut(e.opts.Stdout)
	root.SetErr(e.opts.Stderr)

	err := root.ExecuteContext(ctx)
	if e.helped {
		return e.ok("help", nil)
	}
	if err != nil {
		msg := err.Error()
		res := e.usageError("probe", msg, "probe --help")
		e.writeOutput(res)
		return res
	}
	if e.last.Envelope.Command == "" {
		return e.ok("help", nil)
	}
	e.writeOutput(e.last)
	return e.last
}

func (e *Engine) rootCmd(ctx context.Context) *cobra.Command {
	var jsonFlag bool
	root := &cobra.Command{
		Use:   "probe",
		Short: "HTTP probe CLI for agents: spike requests, promote fixtures, catalog APIs",
		Long:  "probe records HTTP exchanges under .probe/, promotes redacted fixtures into a catalog, and keeps auth as env-name refs only.",
		Example: `  probe quickstart --json
  probe schema --json
  probe doctor --json`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			e.jsonOut = e.opts.JSONDefault || jsonFlag
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			e.helped = true
			return nil
		},
	}
	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "emit JSON envelope on stdout")
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		e.helped = true
		out := cmd.OutOrStdout()
		if cmd.Long != "" {
			fmt.Fprintln(out, cmd.Long)
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, "Usage:")
		fmt.Fprintf(out, "  %s\n", cmd.UseLine())
		if cmd.HasAvailableSubCommands() {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Available Commands:")
			for _, c := range cmd.Commands() {
				if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
					continue
				}
				fmt.Fprintf(out, "  %-12s %s\n", c.Name(), c.Short)
			}
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Flags:")
		fmt.Fprint(out, cmd.Flags().FlagUsages())
		if cmd.Example != "" {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Examples:")
			fmt.Fprintln(out, cmd.Example)
		}
	})

	root.AddCommand(e.versionCmd(ctx))
	root.AddCommand(e.stubCmd("quickstart", "print agent quickstart", `  probe quickstart --json`))
	root.AddCommand(e.stubCmd("schema", "print JSON schema for envelopes", `  probe schema --json`))
	root.AddCommand(e.stubCmd("doctor", "check workspace and catalog health", `  probe doctor --json`))
	root.AddCommand(e.stubCmd("init", "create .probe/ spike workspace", `  probe init`))
	root.AddCommand(e.authCmd())
	root.AddCommand(e.stubCmd("hit", "send an HTTP request and record the exchange", `  fnox exec -- probe hit GET /api/v1/courses --auth canvas --base https://canvas.test --save courses --json
  probe hit GET https://httpbin.org/get --dry-run --json
  probe hit POST /items --body '{"a":1}' --auth api --json`))
	root.AddCommand(e.stubCmd("replay", "replay a saved request by id", `  probe replay 001-courses --json`))
	root.AddCommand(e.stubCmd("last", "show the last recorded exchange", `  probe last --json`))
	root.AddCommand(e.stubCmd("find", "search recorded exchanges", `  probe find courses --json`))
	root.AddCommand(e.stubCmd("note", "append a note to .probe/notes.md", `  probe note "auth works with staging token"`))
	root.AddCommand(e.stubCmd("summary", "summarize the spike session", `  probe summary --json`))
	root.AddCommand(e.stubCmd("promote", "promote a spike exchange into the catalog", `  probe promote canvas --endpoint get-courses --json
  probe promote canvas --request 001-courses --dry-run --json`))
	root.AddCommand(e.catalogCmd())
	return root
}

func (e *Engine) versionCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print probe version",
		Example: `  probe version
  probe version --json`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.last = e.version(ctx)
			return nil
		},
	}
}

func (e *Engine) stubCmd(name, short, example string) *cobra.Command {
	return &cobra.Command{
		Use:     name,
		Short:   short,
		Example: example,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.last = e.usageError(name, name+" not implemented", firstExampleLine(example))
			return nil
		},
	}
}

func (e *Engine) authCmd() *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "manage auth profiles (env-name refs only)",
		Example: `  probe auth list --json
  probe auth show canvas --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			e.helped = true
			return nil
		},
	}
	auth.AddCommand(e.stubCmd("set", "define an auth profile by env names", `  probe auth set canvas --type bearer --token-env CANVAS_TOKEN`))
	auth.AddCommand(e.stubCmd("list", "list auth profiles", `  probe auth list --json`))
	auth.AddCommand(e.stubCmd("show", "show one auth profile (env names only)", `  probe auth show canvas --json`))
	return auth
}

func (e *Engine) catalogCmd() *cobra.Command {
	cat := &cobra.Command{
		Use:   "catalog",
		Short: "inspect the API catalog",
		Example: `  probe catalog list --json
  probe catalog show canvas --json
  probe catalog show canvas --endpoint get-courses --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			e.helped = true
			return nil
		},
	}
	cat.AddCommand(e.stubCmd("list", "list catalog APIs", `  probe catalog list --json`))
	cat.AddCommand(e.stubCmd("show", "show one catalog API", `  probe catalog show canvas --json
  probe catalog show canvas --endpoint get-courses --json`))
	cat.AddCommand(e.stubCmd("path", "print on-disk path for an API", `  probe catalog path canvas --json`))
	return cat
}

func firstExampleLine(example string) string {
	for _, line := range strings.Split(example, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return "probe --help"
}

func (e *Engine) writeOutput(res Result) {
	if e.jsonOut {
		enc := json.NewEncoder(e.opts.Stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(res.Envelope)
		return
	}
	if res.Envelope.OK {
		switch res.Envelope.Command {
		case "version":
			fmt.Fprintln(e.opts.Stdout, Version)
		case "help":
		default:
			if res.Envelope.Data != nil {
				var buf bytes.Buffer
				enc := json.NewEncoder(&buf)
				enc.SetEscapeHTML(false)
				_ = enc.Encode(res.Envelope.Data)
				fmt.Fprint(e.opts.Stdout, buf.String())
			}
		}
		return
	}
	if res.Envelope.Error != nil {
		fmt.Fprintf(e.opts.Stderr, "Error: %s\n", res.Envelope.Error.Message)
		if res.Envelope.Error.Hint != "" {
			fmt.Fprintf(e.opts.Stderr, "  %s\n", res.Envelope.Error.Hint)
		}
		for _, n := range res.Envelope.Error.Next {
			if n != "" && n != res.Envelope.Error.Hint {
				fmt.Fprintf(e.opts.Stderr, "  %s\n", n)
			}
		}
	}
}
