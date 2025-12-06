package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rkoster/deskrun/internal/cluster"
	"github.com/rkoster/deskrun/internal/config"
	"github.com/rkoster/deskrun/internal/kapp"
	"github.com/rkoster/deskrun/internal/runner"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show status of runner installations",
	Long: `Show the status of runner installations in the kind cluster.

Examples:
  deskrun status           # Show all runners
  deskrun status my-runner # Show status for specific runner
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

	// Determine which runners to show
	var names []string
	if len(args) > 0 {
		// Show specific runner
		names = []string{args[0]}
	} else {
		// Show all runners
		var err error
		names, err = runnerMgr.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list runners: %w", err)
		}

		if len(names) == 0 {
			fmt.Println("No runners found in cluster")
			return nil
		}
	}

	// Get kapp client once
	kappClient := kapp.NewClient(clusterMgr.GetKubeconfig(), "arc-systems")

	// Display status for each runner using the same logic
	for i, name := range names {
		if i > 0 {
			fmt.Println() // Add blank line between runners
		}

		// Add runner header
		fmt.Printf("Runner: %s\n", name)

		// Get JSON output from kapp
		inspectOutput, err := kappClient.InspectJSON(name)
		if err != nil {
			fmt.Printf("Error getting status for %s: %v\n", name, err)
			continue
		}

		// Display resources in custom table format
		if err := displayResourceTable(inspectOutput); err != nil {
			fmt.Printf("Error displaying resources for %s: %v\n", name, err)
		}
	}

	return nil
}

// displayResourceTable creates and displays a custom table from kapp JSON output
// Output format:
// 23h [RoleBinding] rubionic-workspace-1-gha-rs-kube-mode
// 23h [AutoscalingRunnerSet] rubionic-workspace-1
// 23h  L [AutoscalingListener] rubionic-workspace-1-6cd58d58-listener
// 22h  L.. [EphemeralRunner] rubionic-workspace-1-2zgjv-runner-6mckt
//        ⚠ | Waiting on finalizers: ephemeralrunner.actions.github.com/finalizer
func displayResourceTable(output *kapp.KappInspectOutput) error {
	if len(output.Tables) == 0 {
		return fmt.Errorf("no tables in kapp output")
	}

	// Get the resources table (usually the first table)
	table := output.Tables[0]
	resources := table.Rows

	if len(resources) == 0 {
		fmt.Println("No resources found")
		return nil
	}

	// Print resources in the requested format
	for _, r := range resources {
		// Extract hierarchy prefix from name (L, L.., etc.)
		name := r.Name
		hierarchyPrefix := ""
		
		// Check if name starts with spaces (indicating hierarchy)
		if strings.HasPrefix(name, " ") {
			// Count leading spaces and extract hierarchy markers
			trimmed := strings.TrimLeft(name, " ")
			spacesCount := len(name) - len(trimmed)
			
			// In tree output, hierarchy is indicated by leading spaces
			// Typically: 1 space = L, 2 spaces = L.., etc.
			if spacesCount > 0 {
				hierarchyPrefix = " L"
				if spacesCount > 1 {
					hierarchyPrefix += strings.Repeat(".", spacesCount-1)
				}
				hierarchyPrefix += " "
			}
			name = trimmed
		}

		// Format: age hierarchyPrefix [Kind] name
		fmt.Printf("%s %s[%s] %s\n", r.Age, hierarchyPrefix, r.Kind, name)

		// If there's reconcile info and it's not ok/empty, show it as a warning
		if r.ReconcileInfo != "" && r.ReconcileInfo != "-" {
			// Handle multi-line reconcile info
			riLines := strings.Split(r.ReconcileInfo, "\n")
			for _, line := range riLines {
				if line != "" {
					fmt.Printf("       ⚠ | %s\n", line)
				}
			}
		}
	}

	return nil
}
