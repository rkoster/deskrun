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

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a runner installation",
	Long: `Remove a GitHub Actions runner installation from the kind cluster.

This will delete the runner scale set and associated resources from the cluster,
and remove the installation from the deskrun configuration.

Example:
  deskrun remove my-runner
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

	// Setup cluster manager
	clusterConfig := &types.ClusterConfig{
		Name: configMgr.GetConfig().ClusterName,
	}
	clusterMgr := cluster.NewManager(clusterConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Check if cluster exists
	exists, err := clusterMgr.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster: %w", err)
	}

	if exists {
		// Uninstall runner from cluster
		fmt.Printf("Removing runner '%s' from cluster...\n", name)
		runnerMgr := runner.NewManager(clusterMgr)
		if err := runnerMgr.Uninstall(ctx, name); err != nil {
			fmt.Printf("Warning: failed to remove runner from cluster: %v\n", err)
		} else {
			fmt.Println("Runner removed from cluster")
		}
	} else {
		fmt.Println("Cluster does not exist, skipping cluster cleanup")
	}

	// Remove from config
	if err := configMgr.RemoveInstallation(name); err != nil {
		return fmt.Errorf("failed to remove from config: %w", err)
	}

	fmt.Printf("Runner '%s' removed successfully\n", name)
	return nil
}
