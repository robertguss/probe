package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
	"github.com/spf13/cobra"
)

// Note: validate and plan both call root.runPurePipeline (single code path).

// sharedSpecFlags holds --spec / --dest for validate, plan, generate.
type sharedSpecFlags struct {
	Spec string
	Dest string
}

// planGenerateFlags adds --verify for plan and generate.
type planGenerateFlags struct {
	sharedSpecFlags
	Verify string
}

func newValidateCmd(r *root) *cobra.Command {
	f := &sharedSpecFlags{}
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a Project Specification through the complete pure pipeline",
		Long: strings.TrimSpace(`
validate executes the complete pure pipeline (strict parse, field validation,
catalog resolution, plan construction, non-binding destination observation)
and discards the plan. A specification that validates is exactly one that plans.

Requires an explicit --spec <path|->. There is no implicit foundry.toml discovery.
Use --spec - to read the specification from stdin (UTF-8, 1 MiB cap).

Optional --dest overrides the specification destination under the same rules;
observation is read-only and non-binding.

Examples:
  foundry validate --spec foundry.toml
  foundry validate --spec - < foundry.toml
  foundry validate --spec foundry.toml --dest /tmp/demo-cli
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCancelled(cmd.Context()); err != nil {
				return err
			}
			// Same pure pipeline as plan; discard plan body (REQ-032 / Section 13.3).
			out, err := r.runPurePipeline(cmd.InOrStdin(), f.Spec, f.Dest, VerifyDefault)
			if err != nil {
				return err
			}
			result, text := formatValidateSuccess(out)
			return r.emitTextOrJSON(cmd, "validate", result, text)
		},
	}
	cmd.Flags().StringVar(&f.Spec, "spec", "", "path to Project Specification, or - for stdin (required)")
	cmd.Flags().StringVar(&f.Dest, "dest", "", "override specification destination (optional)")
	_ = cmd.MarkFlagRequired("spec")
	return cmd
}

func newPlanCmd(r *root) *cobra.Command {
	f := &planGenerateFlags{Verify: VerifyDefault}
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Authoritative dry run: print the complete Generation Plan (no side effects)",
		Long: strings.TrimSpace(`
plan is the authoritative dry run. It produces the Generation Plan without
filesystem writes, subprocesses, or network access.

Text mode prints a human-readable summary: file paths, dependencies, verify
checks, network-capable steps, and git init policy. --verbose expands digests,
tools, and external-step argv. --output json emits the complete versioned plan
document (machine-complete).

There is no separate dry run command or flag — use plan to inspect exactly what
generate would execute with the same flags.

--verify default|strict selects which verification step list the plan records
so the inspected plan is byte-equal to the plan generate executes.

Requires an explicit --spec <path|->. Use --spec - for stdin.

Examples:
  foundry plan --spec foundry.toml
  foundry plan --spec foundry.toml --verbose
  foundry plan --spec foundry.toml --verify strict --output json
  foundry plan --spec - < foundry.toml
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCancelled(cmd.Context()); err != nil {
				return err
			}
			// Authoritative dry run: full pure pipeline; emit plan (REQ-033).
			out, err := r.runPurePipeline(cmd.InOrStdin(), f.Spec, f.Dest, f.Verify)
			if err != nil {
				return err
			}
			// JSON result is the complete Generation Plan (schema 1).
			// Plan implements json.Marshaler with sealed canonical bytes.
			// Text mode: multi-section summary; --verbose expands digests/argv.
			return r.emitTextOrJSON(cmd, "plan", out.Plan, formatPlanSuccess(out, r.g.Verbose))
		},
	}
	cmd.Flags().StringVar(&f.Spec, "spec", "", "path to Project Specification, or - for stdin (required)")
	cmd.Flags().StringVar(&f.Dest, "dest", "", "override specification destination (optional)")
	cmd.Flags().StringVar(&f.Verify, "verify", VerifyDefault, "verification step list: default|strict")
	_ = cmd.MarkFlagRequired("spec")
	return cmd
}

func newGenerateCmd(r *root) *cobra.Command {
	f := &planGenerateFlags{Verify: VerifyDefault}
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Execute the Generation Plan (staging, verify, place destination)",
		Long: strings.TrimSpace(`
generate is the only command that writes a Generated Project. It constructs the
same Generation Plan as plan (authoritative dry run), discloses network-capable
steps, then stages, verifies, and places the destination exclusively (no
overwrite). Optional isolated git init runs in staging; Foundry never creates
git commits or configures git identity — commit yourself after generate.

Use plan first to inspect the Generation Plan without side effects.
--verify default|strict and --dest match plan so the executed plan is
byte-equal to the inspected plan under the same flags.

Requires an explicit --spec <path|->. Progress lines use stable Section 29.2
stage names; --quiet suppresses progress and summary but never errors,
network disclosure, or preserved-stage locations.

Examples:
  foundry plan --spec foundry.toml
  foundry generate --spec foundry.toml
  foundry generate --spec foundry.toml --verify strict
  foundry generate --spec foundry.toml --quiet
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.runGenerate(cmd, f)
		},
	}
	cmd.Flags().StringVar(&f.Spec, "spec", "", "path to Project Specification, or - for stdin (required)")
	cmd.Flags().StringVar(&f.Dest, "dest", "", "override specification destination (optional)")
	cmd.Flags().StringVar(&f.Verify, "verify", VerifyDefault, "verification step list: default|strict")
	_ = cmd.MarkFlagRequired("spec")
	return cmd
}

func newCatalogCmd(r *root) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Inspect the embedded catalog",
		Long:  "Render embedded catalog metadata only (no network, no writes, no subprocesses).",
		// catalog with no subcommand → help
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCatalogListCmd(r), newCatalogShowCmd(r))
	return cmd
}

// catalogListUnit is one stable row in catalog list JSON (REQ-035).
// Field names are the contract surface for goldens and tooling.
type catalogListUnit struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Description  string `json:"description"`
	ManifestPath string `json:"manifest_path"`
}

// catalogListResult is the stable JSON result for `catalog list`.
// catalog_digest matches `foundry version` so agents can cross-check embeds.
type catalogListResult struct {
	CatalogDigest string            `json:"catalog_digest"`
	Units         []catalogListUnit `json:"units"`
}

// catalogShowFile is one [[files]] contribution in catalog show JSON.
type catalogShowFile struct {
	Path   string `json:"path"`
	Render string `json:"render"`
	Source string `json:"source"`
	Mode   string `json:"mode"`
}

// catalogShowDep is one [[dependencies]] contribution in catalog show JSON.
type catalogShowDep struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Scope   string `json:"scope"`
}

// catalogShowResult is the stable JSON result for `catalog show <id>`.
// Unknown IDs never reach this type (catalog.invalid with available set).
type catalogShowResult struct {
	CatalogDigest        string            `json:"catalog_digest"`
	ID                   string            `json:"id"`
	Kind                 string            `json:"kind"`
	Schema               int               `json:"schema"`
	Description          string            `json:"description"`
	ManifestPath         string            `json:"manifest_path"`
	CompatibleArchetypes []string          `json:"compatible_archetypes,omitempty"`
	RequiresVisibility   string            `json:"requires_visibility,omitempty"`
	Files                []catalogShowFile `json:"files"`
	Dependencies         []catalogShowDep  `json:"dependencies"`
}

func newCatalogListCmd(r *root) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List embedded catalog units (core, archetypes, profiles)",
		Long: strings.TrimSpace(`
catalog list prints every validated unit in the embedded catalog sorted by id
(including core, archetypes, and catalogued profiles such as distribution).

Catalog presence is not the same as generation-ready selection: the only
selectable optional profile is distribution (Phase 4), which requires
visibility=public and a github.com/<owner>/<name> module path. Recipes
(configuration, local-persistence) are never generation-selectable.

Write-free: no network, no filesystem writes, no subprocesses. Output is
derived only from the embedded catalog.

Examples:
  foundry catalog list
  foundry catalog list --output json
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCancelled(cmd.Context()); err != nil {
				return err
			}
			c, err := r.loadCatalog()
			if err != nil {
				return err
			}
			rows := c.List()
			out := make([]catalogListUnit, 0, len(rows))
			var b strings.Builder
			for _, u := range rows {
				out = append(out, catalogListUnit{
					ID:           u.ID,
					Kind:         string(u.Kind),
					Description:  u.Description,
					ManifestPath: u.ManifestPath,
				})
				fmt.Fprintf(&b, "%s\t%s\t%s\n", u.ID, u.Kind, u.Description)
			}
			result := catalogListResult{
				CatalogDigest: string(c.Digest()),
				Units:         out,
			}
			return r.emitTextOrJSON(cmd, "catalog list", result, strings.TrimRight(b.String(), "\n"))
		},
	}
}

func newCatalogShowCmd(r *root) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one embedded catalog unit by exact id",
		Long: strings.TrimSpace(`
catalog show prints one unit's validated manifest metadata (including core).
IDs are exact only — no did-you-mean. Unknown ids fail closed with
catalog.invalid naming the sorted available set.

Write-free: no network, no filesystem writes, no subprocesses.

Examples:
  foundry catalog show core
  foundry catalog show cli
  foundry catalog show tui
  foundry catalog show distribution
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCancelled(cmd.Context()); err != nil {
				return err
			}
			c, err := r.loadCatalog()
			if err != nil {
				return err
			}
			id := args[0]
			m, err := c.Show(id)
			if err != nil {
				return err
			}
			result := catalogShowFromManifest(m, string(c.Digest()))
			return r.emitTextOrJSON(cmd, "catalog show", result, formatCatalogShowText(result))
		},
	}
}

// catalogShowFromManifest builds the stable show result from a validated unit.
func catalogShowFromManifest(m *catalog.Manifest, digest string) catalogShowResult {
	files := make([]catalogShowFile, 0, len(m.Files))
	for _, f := range m.Files {
		files = append(files, catalogShowFile{
			Path:   f.Path,
			Render: string(f.Render),
			Source: f.Source,
			Mode:   f.Mode,
		})
	}
	deps := make([]catalogShowDep, 0, len(m.Dependencies))
	for _, d := range m.Dependencies {
		deps = append(deps, catalogShowDep{
			Module:  d.Module,
			Version: d.Version,
			Scope:   string(d.Scope),
		})
	}
	// Defensive copies so result is free of shared slices.
	var compat []string
	if len(m.CompatibleArchetypes) > 0 {
		compat = append([]string(nil), m.CompatibleArchetypes...)
	}
	return catalogShowResult{
		CatalogDigest:        digest,
		ID:                   m.ID,
		Kind:                 string(m.Kind),
		Schema:               m.Schema,
		Description:          m.Description,
		ManifestPath:         m.Path,
		CompatibleArchetypes: compat,
		RequiresVisibility:   m.RequiresVisibility,
		Files:                files,
		Dependencies:         deps,
	}
}

// formatCatalogShowText is the human text form for catalog show.
func formatCatalogShowText(r catalogShowResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "id=%s kind=%s schema=%d\n", r.ID, r.Kind, r.Schema)
	fmt.Fprintf(&b, "manifest_path=%s\n", r.ManifestPath)
	fmt.Fprintf(&b, "catalog_digest=%s\n", r.CatalogDigest)
	fmt.Fprintf(&b, "description=%s\n", r.Description)
	if len(r.CompatibleArchetypes) > 0 {
		fmt.Fprintf(&b, "compatible_archetypes=%s\n", strings.Join(r.CompatibleArchetypes, ","))
	}
	if r.RequiresVisibility != "" {
		fmt.Fprintf(&b, "requires_visibility=%s\n", r.RequiresVisibility)
	}
	if len(r.Files) > 0 {
		b.WriteString("files:\n")
		for _, f := range r.Files {
			fmt.Fprintf(&b, "  %s render=%s source=%s mode=%s\n", f.Path, f.Render, f.Source, f.Mode)
		}
	}
	if len(r.Dependencies) > 0 {
		b.WriteString("dependencies:\n")
		for _, d := range r.Dependencies {
			fmt.Fprintf(&b, "  %s %s scope=%s\n", d.Module, d.Version, d.Scope)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func newVersionCmd(r *root) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print Foundry version, commit, Go toolchain, and catalog digest",
		Long: strings.TrimSpace(`
version reports semantic version, VCS commit (when stamped), Go runtime
version, the embedded catalog SHA-256 digest, and the catalog Go pin plus
FOUNDRY_GO_BIN guidance for generate (REQ-035 / REQ-162).

Metadata comes from Go build settings (not linker-flag string injection).
For a fuller toolchain probe, use foundry doctor.

Examples:
  foundry version
  foundry version --output json
`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCancelled(cmd.Context()); err != nil {
				return err
			}
			info := r.versionInfo()
			pin := toolrun.DefaultPinnedGoTag
			envBin := strings.TrimSpace(os.Getenv(toolrun.EnvFoundryGoBin))
			envLabel := "(unset)"
			if envBin != "" {
				envLabel = envBin
			}
			text := fmt.Sprintf(
				"foundry %s\ncommit: %s\ngo: %s\ncatalog_digest: %s\ncatalog_go_pin: %s\nFOUNDRY_GO_BIN: %s\nnote: generate requires exact %s; run foundry doctor or set FOUNDRY_GO_BIN if PATH differs",
				info.Version, emptyDash(info.Commit), info.Go, emptyDash(info.CatalogDigest),
				pin, envLabel, pin,
			)
			return r.emitTextOrJSON(cmd, "version", info, text)
		},
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// emitTextOrJSON writes result according to global --output via internal/report
// (REQ-186: report owns all human/JSON formatting).
func (r *root) emitTextOrJSON(cmd *cobra.Command, command string, result any, text string) error {
	enc := newEncoder(cmd.OutOrStdout(), cmd.ErrOrStderr(), r.g)
	return enc.Success(command, result, text)
}
