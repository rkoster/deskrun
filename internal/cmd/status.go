package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/rkoster/deskrun/internal/cluster"
	"github.com/rkoster/deskrun/internal/config"
	"github.com/rkoster/deskrun/internal/runner"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show status of runner installations",
	Long: `Show the status of runner installations in the kind cluster.

If a name is provided, shows detailed status for that specific runner.
Otherwise, shows status for all runners.

Examples:
  deskrun status           # Show all runners
  deskrun status my-runner # Show specific runner
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Load config
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Setup cluster manager
	clusterConfig := &types.ClusterConfig{
		Name: configMgr.GetConfig().ClusterName,
	}
	clusterMgr := cluster.NewManager(clusterConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if cluster exists
	exists, err := clusterMgr.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster: %w", err)
	}

	if !exists {
		fmt.Printf("Cluster '%s' does not exist\n", clusterConfig.Name)
		return nil
	}

	fmt.Printf("Cluster '%s' is running\n\n", clusterConfig.Name)

	runnerMgr := runner.NewManager(clusterMgr)

	if len(args) == 1 {
		// Show specific runner status
		name := args[0]
		status, err := runnerMgr.Status(ctx, name)
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}
		fmt.Println(status)
	} else {
		// Show all runners
		names, err := runnerMgr.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list runners: %w", err)
		}

		if len(names) == 0 {
			fmt.Println("No runners found in cluster")
			return nil
		}

		fmt.Println("Runners in cluster:")
		for _, name := range names {
			fmt.Printf("  - %s\n", name)
		}
	}

	return nil
}
