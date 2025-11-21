package cmd

import (
	"fmt"

	"github.com/rkoster/deskrun/internal/config"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a runner installation from configuration",
	Long: `Remove a GitHub Actions runner installation from the deskrun configuration.

This is a config-only operation. After removing a runner, you need to run 'deskrun up'
to apply the changes to the cluster, or use 'deskrun down' to remove all runners.

Example:
  deskrun remove my-runner
  deskrun up
`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Load config
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if installation exists
	_, err = configMgr.GetInstallation(name)
	if err != nil {
		return fmt.Errorf("installation not found: %w", err)
	}

	// Remove from config
	if err := configMgr.RemoveInstallation(name); err != nil {
		return fmt.Errorf("failed to remove from config: %w", err)
	}

	fmt.Printf("Runner '%s' removed from configuration\n", name)
	fmt.Println("\nTo apply this change to the cluster, run:")
	fmt.Println("  deskrun up")
	fmt.Println("\nOr to remove all runners from the cluster, run:")
	fmt.Println("  deskrun down")
	return nil
}
