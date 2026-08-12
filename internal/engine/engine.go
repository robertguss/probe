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
	opts      Options
	environ   map[string]string
	lastHitAt time.Time
	run       *invocation
}

// invocation is per-Run command state. It must not outlive Engine.Run.
type invocation struct {
	jsonOut bool
	helped  bool
	last    Result
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
	}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// Run parses args, dispatches a command, writes output, and returns Result.
// Never calls os.Exit.
func (e *Engine) Run(ctx context.Context, args []string) Result {
	inv := &invocation{}
	e.run = inv
	defer func() { e.run = nil }()

	root := e.rootCmd()
	root.SetArgs(args)
	root.SetOut(e.opts.Stdout)
	root.SetErr(e.opts.Stderr)

	err := root.ExecuteContext(ctx)
	if inv.helped {
		return e.ok("help", nil)
	}
	if err != nil {
		msg := err.Error()
		res := e.usageError("probe", msg, "probe --help")
		e.writeOutput(res)
		return res
	}
	if inv.last.Envelope.Command == "" {
		return e.ok("help", nil)
	}
	e.writeOutput(inv.last)
	return inv.last
}

func (e *Engine) finish(res Result) error {
	e.run.last = res
	return nil
}

func (e *Engine) leaf(use, short, example string, args cobra.PositionalArgs, run func(ctx context.Context, args []string) Result) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		Example: example,
		Args:    args,
		RunE: func(cmd *cobra.Command, a []string) error {
			return e.finish(run(cmd.Context(), a))
		},
	}
}

func (e *Engine) rootCmd() *cobra.Command {
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
			e.run.jsonOut = e.opts.JSONDefault || jsonFlag || !isTTY(e.opts.Stdout)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			e.run.helped = true
			return nil
		},
	}
	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "emit JSON envelope on stdout")
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		e.run.helped = true
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

	root.AddCommand(e.versionCmd())
	root.AddCommand(e.quickstartCmd())
	root.AddCommand(e.schemaCmd())
	root.AddCommand(e.doctorCmd())
	root.AddCommand(e.initCmd())
	root.AddCommand(e.authCmd())
	root.AddCommand(e.hitCmd())
	root.AddCommand(e.replayCmd())
	root.AddCommand(e.lastCmd())
	root.AddCommand(e.findCmd())
	root.AddCommand(e.noteCmd())
	root.AddCommand(e.summaryCmd())
	root.AddCommand(e.promoteCmd())
	root.AddCommand(e.catalogCmd())
	return root
}

func (e *Engine) versionCmd() *cobra.Command {
	return e.leaf("version", "print probe version", "  probe version\n  probe version --json", cobra.NoArgs, func(ctx context.Context, _ []string) Result {
		return e.version(ctx)
	})
}

func (e *Engine) quickstartCmd() *cobra.Command {
	return e.leaf("quickstart", "print agent quickstart", "  probe quickstart --json", cobra.NoArgs, func(ctx context.Context, _ []string) Result {
		return e.quickstart(ctx)
	})
}

func (e *Engine) schemaCmd() *cobra.Command {
	return e.leaf("schema", "describe command tree and JSON envelope", "  probe schema --json", cobra.NoArgs, func(ctx context.Context, _ []string) Result {
		return e.schema(ctx)
	})
}

func (e *Engine) doctorCmd() *cobra.Command {
	return e.leaf("doctor", "check workspace and catalog readiness (booleans only)", "  probe doctor --json", cobra.NoArgs, func(ctx context.Context, _ []string) Result {
		return e.doctor(ctx)
	})
}

func (e *Engine) replayCmd() *cobra.Command {
	return e.leaf("replay <NAME|ID>", "replay a saved request by id", "  probe replay 001-courses --json", cobra.ExactArgs(1), func(ctx context.Context, args []string) Result {
		return e.replay(ctx, args[0])
	})
}

func (e *Engine) findCmd() *cobra.Command {
	return e.leaf("find <path.hints>", "search recorded exchanges by id/path hint", "  probe find courses --json", cobra.ExactArgs(1), func(ctx context.Context, args []string) Result {
		return e.find(ctx, args[0])
	})
}

func (e *Engine) hitCmd() *cobra.Command {
	in := HitInput{Follow: true, Retries: defaultRetries, Timeout: defaultTimeout, MaxWait: defaultMaxWait, MaxBody: defaultMaxBody}
	var noFollow bool
	cmd := &cobra.Command{
		Use:   "hit <METHOD> <URL|PATH>",
		Short: "send an HTTP request and record the exchange",
		Example: `  fnox exec -- probe hit GET /api/v1/courses --auth canvas --base https://canvas.test --save courses --json
  probe hit GET https://httpbin.org/get --dry-run --json
  probe hit POST /items --body '{"a":1}' --auth api --json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Method = args[0]
			in.URL = args[1]
			if !cmd.Flags().Changed("follow") && !noFollow {
				in.Follow = strings.EqualFold(args[0], "GET")
			}
			if noFollow {
				in.Follow = false
			}
			return e.finish(e.hit(cmd.Context(), in))
		},
	}
	cmd.Flags().StringVar(&in.Base, "base", "", "base URL for relative paths")
	cmd.Flags().StringVar(&in.Auth, "auth", "", "auth profile name")
	cmd.Flags().StringArrayVar(&in.Headers, "header", nil, "request header Name:Value (repeatable)")
	cmd.Flags().StringArrayVar(&in.Query, "query", nil, "query key=value (repeatable)")
	cmd.Flags().StringVar(&in.Body, "body", "", "request body string")
	cmd.Flags().StringVar(&in.BodyFile, "file", "", "request body from file, or - for stdin")
	cmd.Flags().StringVar(&in.ContentType, "content-type", "", "Content-Type header")
	cmd.Flags().DurationVar(&in.Timeout, "timeout", defaultTimeout, "HTTP client timeout")
	cmd.Flags().BoolVar(&in.Follow, "follow", true, "follow redirects (default on for GET)")
	cmd.Flags().BoolVar(&noFollow, "no-follow", false, "do not follow redirects")
	cmd.Flags().StringVar(&in.Save, "save", "", "artifact name suffix (default request)")
	cmd.Flags().BoolVar(&in.NoSave, "no-save", false, "do not persist exchange artifacts")
	cmd.Flags().BoolVar(&in.DryRun, "dry-run", false, "print redacted plan only; no network")
	cmd.Flags().StringVar(&in.Fields, "fields", "", "comma-separated response fields to include in data")
	cmd.Flags().Int64Var(&in.MaxBody, "max-body", defaultMaxBody, "max response body bytes to capture")
	cmd.Flags().IntVar(&in.Retries, "retries", defaultRetries, "retries for HTTP 429/503")
	cmd.Flags().DurationVar(&in.MaxWait, "max-wait", defaultMaxWait, "max total retry wait")
	cmd.Flags().BoolVar(&in.NoRetry, "no-retry", false, "disable retries")
	cmd.Flags().Float64Var(&in.RPS, "rps", 0, "client-side requests-per-second cap")
	cmd.Flags().StringVar(&in.API, "api", "", "optional catalog API name for defaults")
	return cmd
}

func (e *Engine) initCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "create .probe/ spike workspace",
		Example: `  probe init
  probe init --dir /tmp/spike --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return e.finish(e.initSpike(cmd.Context(), dir))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory for .probe workspace (default: ./.probe)")
	return cmd
}

func (e *Engine) lastCmd() *cobra.Command {
	return e.leaf("last", "show the last recorded exchange", "  probe last --json", cobra.NoArgs, func(ctx context.Context, _ []string) Result {
		return e.lastExchange(ctx)
	})
}

func (e *Engine) noteCmd() *cobra.Command {
	return e.leaf("note", "append a note to .probe/notes.md", "  probe note \"auth works with staging token\" --json", cobra.ExactArgs(1), func(ctx context.Context, args []string) Result {
		return e.note(ctx, args[0])
	})
}

func (e *Engine) summaryCmd() *cobra.Command {
	return e.leaf("summary", "summarize the spike session", "  probe summary --json", cobra.NoArgs, func(ctx context.Context, _ []string) Result {
		return e.summary(ctx)
	})
}

func (e *Engine) authCmd() *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "manage auth profiles (env-name refs only)",
		Example: `  probe auth list --json
  probe auth show canvas --json
  probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			e.run.helped = true
			return nil
		},
	}
	auth.AddCommand(e.authSetCmd())
	auth.AddCommand(e.leaf("list", "list auth profiles", "  probe auth list --json", cobra.NoArgs, func(ctx context.Context, _ []string) Result {
		return e.authList(ctx)
	}))
	auth.AddCommand(e.leaf("show [name]", "show one auth profile (env names only)", "  probe auth show canvas --json", cobra.ExactArgs(1), func(ctx context.Context, args []string) Result {
		return e.authShow(ctx, args[0])
	}))
	return auth
}

func (e *Engine) authSetCmd() *cobra.Command {
	var typ, tokenEnv, userEnv, passEnv, headerName, valueEnv string
	cmd := &cobra.Command{
		Use:   "set [name]",
		Short: "define an auth profile by env names",
		Example: `  probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json
  probe auth set basic --type basic --user-env API_USER --pass-env API_PASS --json
  probe auth set custom --type header --name X-API-Key --value-env API_KEY --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return e.finish(e.authSet(cmd.Context(), AuthProfile{
				Name:     args[0],
				Type:     AuthType(typ),
				TokenEnv: tokenEnv,
				UserEnv:  userEnv,
				PassEnv:  passEnv,
				Header:   headerName,
				ValueEnv: valueEnv,
			}))
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "auth type: bearer|basic|header")
	cmd.Flags().StringVar(&tokenEnv, "token-env", "", "env var name holding bearer token")
	cmd.Flags().StringVar(&userEnv, "user-env", "", "env var name holding basic auth username")
	cmd.Flags().StringVar(&passEnv, "pass-env", "", "env var name holding basic auth password")
	cmd.Flags().StringVar(&headerName, "name", "", "header name for type=header")
	cmd.Flags().StringVar(&valueEnv, "value-env", "", "env var name holding header value")
	return cmd
}

func (e *Engine) promoteCmd() *cobra.Command {
	in := promoteInput{}
	cmd := &cobra.Command{
		Use:   "promote <api-name>",
		Short: "promote a spike exchange into the catalog",
		Example: `  probe promote canvas --endpoint get-courses --json
  probe promote canvas --request 001-courses --dry-run --json`,
		Args: cobra.ExactArgs(1),
		Long: "Upserts an endpoint into $PROBE_CATALOG/<api>/api.yaml. Hard errors on base_conflict and fixture_exists (no overwrite in v1).",
		RunE: func(cmd *cobra.Command, args []string) error {
			in.API = args[0]
			return e.finish(e.promote(cmd.Context(), in))
		},
	}
	cmd.Flags().StringVar(&in.Request, "request", "", "saved request id or name (default: last)")
	cmd.Flags().StringVar(&in.Endpoint, "endpoint", "", "endpoint id (default: METHOD-path slug)")
	cmd.Flags().BoolVar(&in.DryRun, "dry-run", false, "preview promote without writing")
	return cmd
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
			e.run.helped = true
			return nil
		},
	}
	cat.AddCommand(e.leaf("list", "list catalog APIs", "  probe catalog list --json", cobra.NoArgs, func(ctx context.Context, _ []string) Result {
		return e.catalogList(ctx)
	}))
	var endpoint string
	show := &cobra.Command{
		Use:   "show <api>",
		Short: "show one catalog API",
		Example: `  probe catalog show canvas --json
  probe catalog show canvas --endpoint get-courses --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return e.finish(e.catalogShow(cmd.Context(), args[0], endpoint))
		},
	}
	show.Flags().StringVar(&endpoint, "endpoint", "", "show a single endpoint id")
	cat.AddCommand(show)
	cat.AddCommand(e.leaf("path <api>", "print on-disk path for an API", "  probe catalog path canvas --json", cobra.ExactArgs(1), func(ctx context.Context, args []string) Result {
		return e.catalogPath(ctx, args[0])
	}))
	return cat
}

func (e *Engine) writeOutput(res Result) {
	if e.run != nil && e.run.jsonOut {
		enc := json.NewEncoder(e.opts.Stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(res.Envelope)
		return
	}
	if res.Envelope.OK {
		switch res.Envelope.Command {
		case "probe version":
			fmt.Fprintln(e.opts.Stdout, Version)
		case "probe help", "help":
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
