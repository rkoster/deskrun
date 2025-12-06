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

// formatAge ensures age values are always 3 characters by adding leading zeros
func formatAge(age string) string {
	if len(age) >= 3 {
		return age
	}
	// Add leading zero for single digit values
	if len(age) == 2 {
		return "0" + age
	}
	// For very short values, pad appropriately
	if len(age) == 1 {
		return "00" + age
	}
	return age
}

// extractHierarchyInfo extracts hierarchy prefix and actual name from a resource name
// that may contain leading spaces and hierarchy markers (L, L.., etc.)
func extractHierarchyInfo(name string) (hierarchyPrefix, actualName string) {
	// No hierarchy if no leading spaces
	if !strings.HasPrefix(name, " ") {
		return "", name
	}

	trimmed := strings.TrimLeft(name, " ")
	spacesCount := len(name) - len(trimmed)

	// Check for hierarchy markers (L, L.., L..., etc.)
	if strings.HasPrefix(trimmed, "L") {
		parts := strings.SplitN(trimmed, " ", 2)
		if len(parts) == 2 {
			hierarchyMarker := parts[0]
			// Replace L.. with "  L " for better visual alignment
			if hierarchyMarker == "L.." {
				return "  L ", parts[1]
			}
			return hierarchyMarker + " ", parts[1]
		}
	}

	// Fallback to space-based hierarchy
	if spacesCount == 1 {
		return "L ", trimmed
	}
	// For deeper levels, use "  L " pattern
	return "  L ", trimmed
}

// displayResourceTable creates and displays a custom table from kapp JSON output
// Output format:
// 23h [RoleBinding] rubionic-workspace-1-gha-rs-kube-mode
// 23h [AutoscalingRunnerSet] rubionic-workspace-1
// 23h  L [AutoscalingListener] rubionic-workspace-1-6cd58d58-listener
// 22h   L [EphemeralRunner] rubionic-workspace-1-2zgjv-runner-6mckt
//
//	⚠ : Waiting on finalizers: ephemeralrunner.actions.github.com/finalizer
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
		// Extract hierarchy prefix and actual name
		hierarchyPrefix, name := extractHierarchyInfo(r.Name)

		// Format: age hierarchyPrefix [Kind] name
		formattedAge := formatAge(r.Age)
		fmt.Printf("%s %s[%s] %s\n", formattedAge, hierarchyPrefix, r.Kind, name)

		// If there's reconcile info and it's not ok/empty, show it as a warning
		if r.ReconcileInfo != "" && r.ReconcileInfo != "-" {
			// Calculate warning indentation to align with resource name column
			// Base indentation: 3 chars for age + 1 space = 4 chars
			// Plus the length of the hierarchy prefix (e.g., "L ", "  L ")
			// Minus 2 to account for the "⚠ : " prefix characters
			warningIndent := 4 + len(hierarchyPrefix) - 2
			if warningIndent < 0 {
				warningIndent = 0
			}
			warningPrefix := strings.Repeat(" ", warningIndent)

			// Handle multi-line reconcile info
			riLines := strings.Split(r.ReconcileInfo, "\n")
			for _, line := range riLines {
				if line != "" {
					fmt.Printf("%s⚠ : %s\n", warningPrefix, line)
				}
			}
		}
	}

	return nil
}
