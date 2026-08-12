// Code owned by Foundry extension-path fixture (not generated catalog content).
package cli

import (
	"github.com/example/foundry-smoke-cli/internal/ping"
	"github.com/spf13/cobra"
)

// newPingCmd constructs the ping subcommand. Dependencies are closed over;
// no package-level registration (Section 17.4).
func newPingCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "Print a deterministic pong line (extension-path fixture)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cmd.OutOrStdout().Write([]byte(ping.Format(target) + "\n"))
			return err
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "optional ping target (default localhost)")
	return cmd
}
