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

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Deploy all configured runners to the cluster",
	Long: `Deploy all configured runner installations to the kind cluster.

This command idempotently:
- Creates the kind cluster if it doesn't exist
- Installs the ARC controller if it's not installed
- Deploys all configured runner scale sets
- Updates existing runners if their configuration has changed

This is the command to run after adding or modifying runner configurations
with 'deskrun add' or 'deskrun remove'.

Example:
  deskrun up
`,
	RunE: runUp,
}

func init() {
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	// Load config
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	installations := configMgr.GetConfig().Installations

	if len(installations) == 0 {
		fmt.Println("No runner installations configured")
		fmt.Println("\nTo add a runner, run:")
		fmt.Println("  deskrun add <name> --repository <url> --auth-type pat --auth-value <token>")
		return nil
	}

	// Detect available nix mounts
	nixStore, nixSocket := cluster.DetectNixMounts()

	// Detect docker socket if available
	dockerSocket := cluster.DetectDockerSocket()

	// Setup cluster manager
	clusterConfig := &types.ClusterConfig{
		Name:         configMgr.GetConfig().ClusterName,
		NixStore:     nixStore,
		NixSocket:    nixSocket,
		DockerSocket: dockerSocket,
	}
	clusterMgr := cluster.NewManager(clusterConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Check if cluster exists, create if needed
	exists, err := clusterMgr.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster: %w", err)
	}

	if !exists {
		fmt.Printf("Creating kind cluster '%s'...\n", clusterConfig.Name)
		if err := clusterMgr.Create(ctx); err != nil {
			return fmt.Errorf("failed to create cluster: %w", err)
		}
		fmt.Println("Cluster created successfully")
	} else {
		fmt.Printf("Using existing cluster '%s'\n", clusterConfig.Name)
	}

	// Setup runner manager
	runnerMgr := runner.NewManager(clusterMgr)

	// Get list of currently deployed runners
	deployedRunners, err := runnerMgr.List(ctx)
	if err != nil {
		fmt.Printf("Warning: failed to list deployed runners: %v\n", err)
		deployedRunners = []string{}
	}

	// Create a map of deployed runners for quick lookup
	deployedMap := make(map[string]bool)
	for _, name := range deployedRunners {
		deployedMap[name] = true
	}

	// Install/update configured runners
	fmt.Println("\nDeploying configured runners...")
	for name, installation := range installations {
		if deployedMap[name] {
			fmt.Printf("  Updating runner '%s'...\n", name)
			// For now, we'll uninstall and reinstall to update
			if err := runnerMgr.Uninstall(ctx, name); err != nil {
				fmt.Printf("  Warning: failed to uninstall runner '%s': %v\n", name, err)
			}
		} else {
			fmt.Printf("  Installing runner '%s'...\n", name)
		}

		if err := runnerMgr.Install(ctx, installation); err != nil {
			fmt.Printf("  Error: failed to install runner '%s': %v\n", name, err)
			continue
		}
		fmt.Printf("  ✓ Runner '%s' deployed\n", name)
	}

	// Remove runners that are deployed but not in config
	fmt.Println("\nCleaning up removed runners...")
	for _, name := range deployedRunners {
		if _, exists := installations[name]; !exists {
			fmt.Printf("  Removing runner '%s'...\n", name)
			if err := runnerMgr.Uninstall(ctx, name); err != nil {
				fmt.Printf("  Warning: failed to remove runner '%s': %v\n", name, err)
			} else {
				fmt.Printf("  ✓ Runner '%s' removed\n", name)
			}
		}
	}

	fmt.Println("\nDeployment complete!")
	return nil
}
