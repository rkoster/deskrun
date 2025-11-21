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

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Remove all ARC runners from the cluster",
	Long: `Remove all GitHub Actions runner installations from the kind cluster.

This command removes all deployed runner scale sets from the cluster.
The runner configurations remain in deskrun's config and can be redeployed
with 'deskrun up'.

To also delete the configuration, use 'deskrun remove' before running 'down',
or delete individual runners with 'deskrun remove <name>'.

Example:
  deskrun down
`,
	RunE: runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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

	// Setup runner manager
	runnerMgr := runner.NewManager(clusterMgr)

	// Get list of currently deployed runners
	fmt.Println("Finding deployed runners...")
	deployedRunners, err := runnerMgr.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list deployed runners: %w", err)
	}

	if len(deployedRunners) == 0 {
		fmt.Println("No runners deployed in cluster")
		return nil
	}

	fmt.Printf("Found %d runner(s) to remove\n\n", len(deployedRunners))

	// Remove all deployed runners
	for _, name := range deployedRunners {
		fmt.Printf("Removing runner '%s'...\n", name)
		if err := runnerMgr.Uninstall(ctx, name); err != nil {
			fmt.Printf("  Warning: failed to remove runner '%s': %v\n", name, err)
		} else {
			fmt.Printf("  ✓ Runner '%s' removed\n", name)
		}
	}

	fmt.Println("\nAll runners removed from cluster")
	fmt.Println("\nNote: Runner configurations are still saved.")
	fmt.Println("To redeploy, run: deskrun up")
	return nil
}
