// Code owned by repo-map dogfood overlay (not generated catalog content).
package cli

import (
	"fmt"
	"strings"

	"github.com/robertguss/repo-map/internal/inventory"
	"github.com/spf13/cobra"
)

// newInventoryCmd constructs the inventory subcommand. Dependencies are closed
// over; no package-level registration (Section 17.4 / REQ-065).
func newInventoryCmd() *cobra.Command {
	var (
		output   string
		maxDepth int
	)
	cmd := &cobra.Command{
		Use:   "inventory [path]",
		Short: "List files under a path as text or JSON",
		Long: strings.TrimSpace(`
Walk a directory and print a repository inventory.

Text mode prints one relative path per line (directories end with "/").
JSON mode prints a stable array of {path, bytes, dir} objects.
The .git directory is always skipped.
`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			entries, err := inventory.Walk(inventory.Options{
				Root:     root,
				MaxDepth: maxDepth,
			})
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(output)) {
			case "", "text":
				_, err = cmd.OutOrStdout().Write([]byte(inventory.FormatText(entries)))
				return err
			case "json":
				body, err := inventory.FormatJSON(entries)
				if err != nil {
					return err
				}
				_, err = cmd.OutOrStdout().Write([]byte(body))
				return err
			default:
				return fmt.Errorf("unknown --output %q (want text or json)", output)
			}
		},
	}
	cmd.Flags().StringVar(&output, "output", "text", "output format: text|json")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 0, "maximum directory depth (0 = unlimited)")
	return cmd
}
