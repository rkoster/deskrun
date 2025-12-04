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
	Use:   "status",
	Short: "Show status of runner installations",
	Long: `Show the status of all runner installations in the kind cluster.

Examples:
  deskrun status           # Show all runners
`,
	Args: cobra.NoArgs,
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

	// Show all runners with tree output
	names, err := runnerMgr.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list runners: %w", err)
	}

	if len(names) == 0 {
		fmt.Println("No runners found in cluster")
		return nil
	}

	fmt.Println("Runners in cluster:")
	for i, name := range names {
		if i > 0 {
			fmt.Println() // Add blank line between runners
		}

		// Add runner header
		fmt.Printf("Runner: %s\n", name)

		// Get kapp client to directly call InspectTreeRaw
		kappClient := runnerMgr.GetKappClient()
		treeLines, err := kappClient.InspectTreeRaw(name)
		if err != nil {
			fmt.Printf("Error getting status for %s: %v\n", name, err)
			continue
		}

		// Print the clean tree lines
		for _, line := range treeLines {
			fmt.Println(line)
		}
	}

	return nil
}
