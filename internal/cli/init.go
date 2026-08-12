package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/spf13/cobra"
)

// initFlags holds flags for foundry init (flags only; never prompts).
type initFlags struct {
	Out         string
	Name        string
	Module      string
	Archetype   string
	Description string
	Destination string
	Visibility  string
	Profiles    []string
}

// initResult is the stable JSON payload for foundry init success.
type initResult struct {
	Out       string   `json:"out"`
	Name      string   `json:"name"`
	Module    string   `json:"module"`
	Archetype string   `json:"archetype"`
	Next      []string `json:"next_steps"`
}

func newInitCmd(r *root) *cobra.Command {
	f := &initFlags{
		Archetype:  "cli",
		Visibility: "private",
	}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a valid Project Spec TOML to an explicit path (flags only)",
		Long: strings.TrimSpace(`
init writes one Project Specification TOML to --out. It does not discover a
default path, does not prompt, and does not generate a project. Destination
must not already exist (no overwrite).

Required: --out, --name, --module. Optional: --archetype (cli|tui, default cli),
--description, --destination (default ./<name>), --visibility (private|public),
--profile (repeatable; only "distribution" is selectable today).

The written file is validated with the same field contract as validate before
it is written. Next step: foundry validate --spec <out> && foundry plan …

Examples:
  foundry init --out ./my-cli.toml --name my-cli --module github.com/you/my-cli
  foundry init --out ./app.toml --name app --module github.com/you/app --archetype tui
  foundry init --out ./pub.toml --name pub --module github.com/you/pub \
    --visibility public --profile distribution
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCancelled(cmd.Context()); err != nil {
				return err
			}
			res, text, err := runInit(f)
			if err != nil {
				return err
			}
			return r.emitTextOrJSON(cmd, "init", res, text)
		},
	}
	cmd.Flags().StringVar(&f.Out, "out", "", "path to write the Project Spec TOML (required; must not exist)")
	cmd.Flags().StringVar(&f.Name, "name", "", "project name (kebab-case; required)")
	cmd.Flags().StringVar(&f.Module, "module", "", "Go module path (final segment must equal name; required)")
	cmd.Flags().StringVar(&f.Archetype, "archetype", "cli", "archetype: cli|tui")
	cmd.Flags().StringVar(&f.Description, "description", "", "one-line description (default: derived from name)")
	cmd.Flags().StringVar(&f.Destination, "destination", "", "generate destination (default: ./<name>)")
	cmd.Flags().StringVar(&f.Visibility, "visibility", "private", "visibility: private|public")
	cmd.Flags().StringArrayVar(&f.Profiles, "profile", nil, "optional profile id (repeatable; distribution only today)")
	_ = cmd.MarkFlagRequired("out")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("module")
	return cmd
}

func runInit(f *initFlags) (initResult, string, error) {
	if f == nil {
		return initResult{}, "", usageErr("init flags are required")
	}
	outPath := strings.TrimSpace(f.Out)
	if outPath == "" {
		return initResult{}, "", usageErr("--out is required (explicit path; no default discovery)")
	}
	if outPath == "-" {
		return initResult{}, "", usageErr("--out must be a filesystem path (stdout redirect is not supported; no implicit discovery)")
	}
	name := strings.TrimSpace(f.Name)
	module := strings.TrimSpace(f.Module)
	archetype := strings.ToLower(strings.TrimSpace(f.Archetype))
	visibility := strings.ToLower(strings.TrimSpace(f.Visibility))
	desc := strings.TrimSpace(f.Description)
	dest := strings.TrimSpace(f.Destination)
	if desc == "" {
		desc = fmt.Sprintf("%s — Foundry-generated %s project", name, archetype)
	}
	if dest == "" {
		dest = "./" + name
	}
	if archetype != "cli" && archetype != "tui" {
		return initResult{}, "", usageErrf("invalid --archetype %q; want cli or tui", f.Archetype)
	}
	if visibility != "private" && visibility != "public" {
		return initResult{}, "", usageErrf("invalid --visibility %q; want private or public", f.Visibility)
	}

	body := renderSpecTOML(name, module, desc, archetype, dest, visibility, f.Profiles)
	// Validate with the same Decode+Validate path as validate/plan.
	raw, err := spec.Decode(outPath, []byte(body))
	if err != nil {
		return initResult{}, "", err
	}
	if _, err := spec.Validate(raw); err != nil {
		return initResult{}, "", err
	}

	abs, err := filepath.Abs(outPath)
	if err != nil {
		return initResult{}, "", diagnostic.New(
			diagnostic.IDUsageInvalid,
			fmt.Sprintf("cannot resolve --out path: %v", err),
			diagnostic.PathLocation(outPath),
		)
	}
	if _, err := os.Lstat(abs); err == nil {
		return initResult{}, "", diagnostic.New(
			diagnostic.IDUsageInvalid,
			fmt.Sprintf("--out already exists: %s (refusing to overwrite)", abs),
			diagnostic.PathLocation(abs),
		).WithRemediation("Choose a new --out path, or move/remove the existing file yourself.")
	} else if !os.IsNotExist(err) {
		return initResult{}, "", diagnostic.New(
			diagnostic.IDUsageInvalid,
			fmt.Sprintf("cannot stat --out: %v", err),
			diagnostic.PathLocation(abs),
		)
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return initResult{}, "", diagnostic.New(
			diagnostic.IDUsageInvalid,
			fmt.Sprintf("cannot create parent directory for --out: %v", err),
			diagnostic.PathLocation(abs),
		)
	}
	// O_CREATE|O_EXCL: never overwrite even under races.
	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return initResult{}, "", diagnostic.New(
			diagnostic.IDUsageInvalid,
			fmt.Sprintf("cannot create --out: %v", err),
			diagnostic.PathLocation(abs),
		).WithRemediation("Choose a new --out path that does not already exist.")
	}
	defer file.Close()
	if _, err := file.WriteString(body); err != nil {
		return initResult{}, "", diagnostic.New(
			diagnostic.IDUsageInvalid,
			fmt.Sprintf("failed writing --out: %v", err),
			diagnostic.PathLocation(abs),
		)
	}

	res := initResult{
		Out:       abs,
		Name:      name,
		Module:    module,
		Archetype: archetype,
		Next: []string{
			fmt.Sprintf("foundry validate --spec %s", abs),
			fmt.Sprintf("foundry plan --spec %s", abs),
			fmt.Sprintf("foundry generate --spec %s", abs),
		},
	}
	text := fmt.Sprintf(
		"init: wrote %s\nnext: foundry validate --spec %s && foundry plan --spec %s",
		abs, abs, abs,
	)
	return res, text, nil
}

func renderSpecTOML(name, module, desc, archetype, dest, visibility string, profiles []string) string {
	var b strings.Builder
	b.WriteString("# Project Specification written by foundry init.\n")
	b.WriteString("# Edit as needed, then: foundry validate --spec <this-file>\n")
	fmt.Fprintf(&b, "schema = 1\n")
	fmt.Fprintf(&b, "name = %q\n", name)
	fmt.Fprintf(&b, "module = %q\n", module)
	fmt.Fprintf(&b, "description = %q\n", desc)
	fmt.Fprintf(&b, "archetype = %q\n", archetype)
	fmt.Fprintf(&b, "destination = %q\n", dest)
	fmt.Fprintf(&b, "visibility = %q\n", visibility)
	if len(profiles) == 0 {
		b.WriteString("profiles = []\n")
	} else {
		b.WriteString("profiles = [")
		for i, p := range profiles {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q", strings.TrimSpace(p))
		}
		b.WriteString("]\n")
	}
	b.WriteString("\n[git]\n")
	b.WriteString("init = true\n")
	b.WriteString("initial_branch = \"main\"\n")
	return b.String()
}
