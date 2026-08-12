package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/version"
	"github.com/spf13/cobra"
)

// Streams are the process I/O handles. Tests inject buffers; production uses os.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Options configures one command-tree construction (no package globals).
type Options struct {
	// Version is optional; empty fields are filled from version.Read().
	Version version.Info
	// Catalog is optional; when nil, version/catalog commands Load() the embed.
	Catalog *catalog.Catalog
	// SkipCatalogLoad prevents embed load (tests that only exercise flags/help).
	SkipCatalogLoad bool

	// PipelineHost injects tool binaries + Section 34.2 host env for the pure
	// validate/plan path. Zero fields are filled from ambient capture so unit
	// tests can pin host-independent plan_sha256 / goldens.
	PipelineHost PipelineHost
	// WorkingDir, when non-empty, replaces os.Getwd for destination abs resolution.
	WorkingDir string
	// ReadFile, when non-nil, replaces os.ReadFile for --spec path reads (tests).
	ReadFile func(path string) ([]byte, error)
	// ObserveDestination, when non-nil, replaces Lstat-based destination
	// observation so tests can pin observation without touching the real FS.
	ObserveDestination func(authored string) (plan.DestinationInfo, error)

	// GenerateStages, when non-nil, replaces production generate stage funcs
	// (unit tests inject failures/cancel/fake commit without touching fsx).
	// Unset stages use generate.NopStage.
	GenerateStages map[generate.StageID]generate.StageFunc
}

// root holds per-tree state shared by subcommands.
type root struct {
	opts Options
	g    *globalFlags
	// lastCommand is the CommandPath leaf updated in PersistentPreRunE.
	lastCommand string
	// catalogMemo is loaded once per Execute when needed.
	catalogMemo *catalog.Catalog
	catalogErr  error
	catalogDone bool
}

// NewRoot constructs a fresh root command tree (Section 17.4).
// Dependencies are closed over; nothing is registered in init.
func NewRoot(opts Options) *cobra.Command {
	r := newRootState(opts)
	return r.command()
}

func newRootState(opts Options) *root {
	return &root{
		opts:        opts,
		g:           &globalFlags{Output: OutputText, Color: ColorAuto},
		catalogMemo: opts.Catalog,
		catalogDone: opts.Catalog != nil,
	}
}

func (r *root) command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "foundry",
		Short: "Deterministic Go project generator (CLI/TUI) from a TOML specification",
		Long: strings.TrimSpace(`
Foundry generates complete, verified Go CLI and TUI repositories from a
declarative Project Specification (foundry.toml).

Command surface:
  init       write a Project Spec TOML to an explicit --out path
  validate   complete pure pipeline; discard plan
  plan       authoritative dry run — Generation Plan, no side effects
  generate   execute the plan (staging, verify, exclusive filesystem place)
  catalog    list or show embedded catalog units
  doctor     check Go pin / FOUNDRY_GO_BIN guidance (advisory)
  version    report version, commit, Go toolchain, catalog digest

plan is the authoritative dry run: inspect the Generation Plan before any
project write. There is no separate dry run command or flag.

The specification path is always explicit via --spec <path|-> (no implicit
working-directory discovery). Stdin is selected with --spec -. init requires
an explicit --out path (also no discovery).
`),
		SilenceErrors: true,
		SilenceUsage:  true,
		// No subcommand → deterministic help, exit 0 (Section 17.4).
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	pf := cmd.PersistentFlags()
	pf.StringVar(&r.g.Output, "output", OutputText, "output mode: text|json")
	pf.BoolVar(&r.g.Quiet, "quiet", false, "suppress non-essential human output (text mode only)")
	pf.BoolVar(&r.g.Verbose, "verbose", false, "diagnostic detail on stderr (text mode only)")
	pf.StringVar(&r.g.Color, "color", ColorAuto, "color mode: auto|always|never (text mode only)")

	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		r.lastCommand = trimFoundryPrefix(c.CommandPath())
		return r.g.validateGlobal(c)
	}

	cmd.AddCommand(
		newInitCmd(r),
		newValidateCmd(r),
		newPlanCmd(r),
		newGenerateCmd(r),
		newCatalogCmd(r),
		newDoctorCmd(r),
		newVersionCmd(r),
	)

	// Disable the default completion command (Section 13.7: deferred).
	cmd.CompletionOptions.DisableDefaultCmd = true
	// Keep --help / -h flags. Hide the `help` verb from Available Commands.
	// Cobra's default usage template special-cases Name=="help", so override
	// that clause.
	cmd.SetHelpCommand(&cobra.Command{
		Use:    "help [command]",
		Hidden: true,
		Short:  "Help about any command",
		Run: func(c *cobra.Command, args []string) {
			target := c.Root()
			if len(args) > 0 {
				if found, _, err := target.Find(args); err == nil && found != nil {
					target = found
				}
			}
			_ = target.Help()
		},
	})
	cmd.SetUsageTemplate(hiddenHelpUsageTemplate())

	return cmd
}

// hiddenHelpUsageTemplate is cobra's default usage template with the
// `(eq .Name "help")` special-case removed so a Hidden help verb does not
// appear under Available Commands.
func hiddenHelpUsageTemplate() string {
	// Derived from cobra.Command.UsageTemplate(); keep layout stable for goldens.
	return `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
}

// Run executes the CLI with args (excluding the program name) and returns the
// process exit code (0/1/2/130). It is the sole error-to-exit translation used
// by cmd/foundry (REQ-181).
func Run(ctx context.Context, args []string, streams Streams, opts Options) int {
	if streams.In == nil {
		streams.In = os.Stdin
	}
	if streams.Out == nil {
		streams.Out = os.Stdout
	}
	if streams.Err == nil {
		streams.Err = os.Stderr
	}
	if ctx == nil {
		ctx = context.Background()
	}

	state := newRootState(opts)
	rootCmd := state.command()
	rootCmd.SetArgs(args)
	rootCmd.SetIn(streams.In)
	rootCmd.SetOut(streams.Out)
	rootCmd.SetErr(streams.Err)

	err := rootCmd.ExecuteContext(ctx)
	if err == nil {
		return diagnostic.ExitSuccess
	}
	// generate (and similar) may finish encoding themselves and return exitWith.
	if ee, ok := asExitError(err); ok {
		return ee.code
	}
	err = normalizeCLIError(err)

	cmdName := state.lastCommand
	if cmdName == "" {
		cmdName = "foundry"
	}
	enc := newEncoder(streams.Out, streams.Err, state.g)

	// Cancellation (Section 13.6 / 38.1) and all other failures go through
	// report so agent fields (error_id, remediation, stage_path) are stable.
	code, _ := enc.Failure(cmdName, err, "")
	return code
}

// normalizeCLIError maps cobra/pflag errors to usage.invalid FoundryErrors.
func normalizeCLIError(err error) error {
	if err == nil {
		return nil
	}
	if diagnostic.IsCancelled(err) {
		return err
	}
	if _, ok := diagnostic.AsFoundryError(err); ok {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return diagnostic.ErrCancelled
	}
	// Cobra/pflag usage messages (unknown command/flag, missing required, …).
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "invalid command line"
	}
	return usageErr(msg)
}

func trimFoundryPrefix(path string) string {
	path = strings.TrimSpace(path)
	if path == "foundry" || path == "" {
		return ""
	}
	return strings.TrimPrefix(path, "foundry ")
}

// loadCatalog returns the embedded (or injected) catalog for version/list/show.
func (r *root) loadCatalog() (*catalog.Catalog, error) {
	if r.catalogDone {
		return r.catalogMemo, r.catalogErr
	}
	r.catalogDone = true
	if r.opts.SkipCatalogLoad {
		r.catalogErr = diagnostic.New(
			diagnostic.IDCatalogInvalid,
			"catalog load skipped",
			diagnostic.PathLocation("catalog"),
		)
		return nil, r.catalogErr
	}
	if r.opts.Catalog != nil {
		r.catalogMemo = r.opts.Catalog
		return r.catalogMemo, nil
	}
	c, err := catalog.Load()
	if err != nil {
		r.catalogErr = err
		return nil, err
	}
	r.catalogMemo = c
	return c, nil
}

// versionInfo builds version.Info with catalog digest when available.
func (r *root) versionInfo() version.Info {
	info := r.opts.Version
	base := version.Read()
	if info.Version == "" {
		info.Version = base.Version
	}
	if info.Commit == "" {
		info.Commit = base.Commit
	}
	if info.Go == "" {
		info.Go = base.Go
	}
	if info.CatalogDigest == "" {
		if c, err := r.loadCatalog(); err == nil && c != nil {
			info.CatalogDigest = string(c.Digest())
		}
	}
	return info
}

// checkCancelled returns a cancellation error when ctx is done.
func checkCancelled(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.Canceled) {
			return diagnostic.ErrCancelled
		}
		return ctx.Err()
	default:
		return nil
	}
}

// requireSpec validates that --spec was provided (non-empty). The value may be
// a filesystem path or SpecStdin ("-"). Body reading is owned by pipeline beads.
func requireSpec(spec string) error {
	if strings.TrimSpace(spec) == "" {
		return usageErr("--spec is required (path or - for stdin); there is no implicit foundry.toml discovery")
	}
	return nil
}
