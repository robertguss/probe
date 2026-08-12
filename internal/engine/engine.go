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
	"sync"
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
	opts       Options
	environ    map[string]string
	spike      SpikePaths
	jsonOut    bool
	lastResult Result
	helped     bool
	hitMu      sync.Mutex
	lastHitAt  time.Time
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
	e.lastResult = Result{}
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
	if e.lastResult.Envelope.Command == "" {
		return e.ok("help", nil)
	}
	e.writeOutput(e.lastResult)
	return e.lastResult
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
	root.CompletionOptions.DisableDefaultCmd = true
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
	root.AddCommand(e.quickstartCmd(ctx))
	root.AddCommand(e.schemaCmd(ctx))
	root.AddCommand(e.doctorCmd(ctx))
	root.AddCommand(e.initCmd(ctx))
	root.AddCommand(e.authCmd(ctx))
	root.AddCommand(e.hitCmd(ctx))
	root.AddCommand(e.replayCmd(ctx))
	root.AddCommand(e.lastCmd(ctx))
	root.AddCommand(e.findCmd(ctx))
	root.AddCommand(e.noteCmd(ctx))
	root.AddCommand(e.summaryCmd(ctx))
	root.AddCommand(e.promoteCmd(ctx))
	root.AddCommand(e.catalogCmd(ctx))
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
			e.lastResult = e.version(ctx)
			return nil
		},
	}
}

func (e *Engine) quickstartCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "quickstart",
		Short:   "print agent quickstart",
		Example: `  probe quickstart --json`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.lastResult = e.quickstart(ctx)
			return nil
		},
	}
}

func (e *Engine) schemaCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "schema",
		Short:   "describe command tree and JSON envelope",
		Example: `  probe schema --json`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.lastResult = e.schema(ctx)
			return nil
		},
	}
}

func (e *Engine) doctorCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		Short:   "check workspace and catalog readiness (booleans only)",
		Example: `  probe doctor --json`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.lastResult = e.doctor(ctx)
			return nil
		},
	}
}

func (e *Engine) replayCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "replay <NAME|ID>",
		Short:   "replay a saved request by id",
		Example: `  probe replay 001-courses --json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e.lastResult = e.replay(ctx, args[0])
			return nil
		},
	}
}

func (e *Engine) findCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "find <path.hints>",
		Short:   "search recorded exchanges by id/path hint",
		Example: `  probe find courses --json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e.lastResult = e.find(ctx, args[0])
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
			e.lastResult = e.usageError(name, name+" not implemented", firstExampleLine(example))
			return nil
		},
	}
}

func (e *Engine) hitCmd(ctx context.Context) *cobra.Command {
	in := HitInput{Follow: true, Retries: 2, Timeout: 30 * time.Second, MaxWait: 60 * time.Second, MaxBody: 1 << 20}
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
			e.lastResult = e.hit(ctx, in)
			return nil
		},
	}
	cmd.Flags().StringVar(&in.Base, "base", "", "base URL for relative paths")
	cmd.Flags().StringVar(&in.Auth, "auth", "", "auth profile name")
	cmd.Flags().StringArrayVar(&in.Headers, "header", nil, "request header Name:Value (repeatable)")
	cmd.Flags().StringArrayVar(&in.Query, "query", nil, "query key=value (repeatable)")
	cmd.Flags().StringVar(&in.Body, "body", "", "request body string")
	cmd.Flags().StringVar(&in.BodyFile, "file", "", "request body from file, or - for stdin")
	cmd.Flags().StringVar(&in.ContentType, "content-type", "", "Content-Type header")
	cmd.Flags().DurationVar(&in.Timeout, "timeout", 30*time.Second, "HTTP client timeout")
	cmd.Flags().BoolVar(&in.Follow, "follow", true, "follow redirects (default on for GET)")
	cmd.Flags().BoolVar(&noFollow, "no-follow", false, "do not follow redirects")
	cmd.Flags().StringVar(&in.Save, "save", "", "artifact name suffix (default request)")
	cmd.Flags().BoolVar(&in.NoSave, "no-save", false, "do not persist exchange artifacts")
	cmd.Flags().BoolVar(&in.DryRun, "dry-run", false, "print redacted plan only; no network")
	cmd.Flags().StringVar(&in.Fields, "fields", "", "comma-separated response fields to include in data")
	cmd.Flags().Int64Var(&in.MaxBody, "max-body", 1<<20, "max response body bytes to capture")
	cmd.Flags().IntVar(&in.Retries, "retries", 2, "retries for HTTP 429/503")
	cmd.Flags().DurationVar(&in.MaxWait, "max-wait", 60*time.Second, "max total retry wait")
	cmd.Flags().BoolVar(&in.NoRetry, "no-retry", false, "disable retries")
	cmd.Flags().Float64Var(&in.RPS, "rps", 0, "client-side requests-per-second cap")
	cmd.Flags().StringVar(&in.API, "api", "", "optional catalog API name for defaults")
	return cmd
}

func (e *Engine) initCmd(ctx context.Context) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "create .probe/ spike workspace",
		Example: `  probe init
  probe init --dir /tmp/spike --json`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.lastResult = e.initSpike(ctx, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory for .probe workspace (default: ./.probe)")
	return cmd
}

func (e *Engine) lastCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "last",
		Short:   "show the last recorded exchange",
		Example: `  probe last --json`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.lastResult = e.lastExchange(ctx)
			return nil
		},
	}
}

func (e *Engine) noteCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "note",
		Short:   "append a note to .probe/notes.md",
		Example: `  probe note "auth works with staging token" --json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e.lastResult = e.note(ctx, args[0])
			return nil
		},
	}
}

func (e *Engine) summaryCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "summary",
		Short:   "summarize the spike session",
		Example: `  probe summary --json`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.lastResult = e.summary(ctx)
			return nil
		},
	}
}

func (e *Engine) authCmd(ctx context.Context) *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "manage auth profiles (env-name refs only)",
		Example: `  probe auth list --json
  probe auth show canvas --json
  probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Help()
			e.helped = true
			return nil
		},
	}
	auth.AddCommand(e.authSetCmd(ctx))
	auth.AddCommand(e.authListCmd(ctx))
	auth.AddCommand(e.authShowCmd(ctx))
	return auth
}

func (e *Engine) authSetCmd(ctx context.Context) *cobra.Command {
	var typ, tokenEnv, userEnv, passEnv, headerName, valueEnv string
	cmd := &cobra.Command{
		Use:   "set [name]",
		Short: "define an auth profile by env names",
		Example: `  probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json
  probe auth set basic --type basic --user-env API_USER --pass-env API_PASS --json
  probe auth set custom --type header --name X-API-Key --value-env API_KEY --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e.lastResult = e.authSet(ctx, AuthProfile{
				Name:     args[0],
				Type:     AuthType(typ),
				TokenEnv: tokenEnv,
				UserEnv:  userEnv,
				PassEnv:  passEnv,
				Header:   headerName,
				ValueEnv: valueEnv,
			})
			return nil
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

func (e *Engine) authListCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "list auth profiles",
		Example: `  probe auth list --json`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.lastResult = e.authList(ctx)
			return nil
		},
	}
}

func (e *Engine) authShowCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:     "show [name]",
		Short:   "show one auth profile (env names only)",
		Example: `  probe auth show canvas --json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e.lastResult = e.authShow(ctx, args[0])
			return nil
		},
	}
}

func (e *Engine) promoteCmd(ctx context.Context) *cobra.Command {
	in := promoteInput{}
	cmd := &cobra.Command{
		Use:   "promote <api-name>",
		Short: "promote a spike exchange into the catalog",
		Example: `  probe promote canvas --endpoint get-courses --json
  probe promote canvas --request 001-courses --dry-run --json`,
		Args: cobra.ExactArgs(1),
		Long: "Upserts an endpoint into $PROBE_CATALOG/<api>/api.yaml. Hard errors on base_conflict and fixture_exists (no overwrite in v1).",
		RunE: func(_ *cobra.Command, args []string) error {
			in.API = args[0]
			e.lastResult = e.promote(ctx, in)
			return nil
		},
	}
	cmd.Flags().StringVar(&in.Request, "request", "", "saved request id or name (default: last)")
	cmd.Flags().StringVar(&in.Endpoint, "endpoint", "", "endpoint id (default: METHOD-path slug)")
	cmd.Flags().BoolVar(&in.DryRun, "dry-run", false, "preview promote without writing")
	return cmd
}

func (e *Engine) catalogCmd(ctx context.Context) *cobra.Command {
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
	list := &cobra.Command{
		Use:     "list",
		Short:   "list catalog APIs",
		Example: `  probe catalog list --json`,
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			e.lastResult = e.catalogList(ctx)
			return nil
		},
	}
	var endpoint string
	show := &cobra.Command{
		Use:   "show <api>",
		Short: "show one catalog API",
		Example: `  probe catalog show canvas --json
  probe catalog show canvas --endpoint get-courses --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e.lastResult = e.catalogShow(ctx, args[0], endpoint)
			return nil
		},
	}
	show.Flags().StringVar(&endpoint, "endpoint", "", "show a single endpoint id")
	pathCmd := &cobra.Command{
		Use:     "path <api>",
		Short:   "print on-disk path for an API",
		Example: `  probe catalog path canvas --json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e.lastResult = e.catalogPath(ctx, args[0])
			return nil
		},
	}
	cat.AddCommand(list, show, pathCmd)
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
