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

	// Calculate column widths
	maxNamespace := len("Namespace")
	maxName := len("Name")
	maxKind := len("Kind")
	maxOwner := len("Owner")
	maxRs := len("Rs")
	maxRi := len("Ri")

	for _, r := range resources {
		if len(r.Namespace) > maxNamespace {
			maxNamespace = len(r.Namespace)
		}
		if len(r.Name) > maxName {
			maxName = len(r.Name)
		}
		if len(r.Kind) > maxKind {
			maxKind = len(r.Kind)
		}
		if len(r.Owner) > maxOwner {
			maxOwner = len(r.Owner)
		}
		if len(r.ReconcileState) > maxRs {
			maxRs = len(r.ReconcileState)
		}
		// ReconcileInfo can be multi-line, handle first line for width calculation
		firstLine := strings.Split(r.ReconcileInfo, "\n")[0]
		if len(firstLine) > maxRi {
			maxRi = len(firstLine)
		}
	}

	// Print header
	fmt.Printf("%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
		maxNamespace, "Namespace",
		maxName, "Name",
		maxKind, "Kind",
		maxOwner, "Owner",
		maxRs, "Rs",
		"Ri")

	// Print resources
	for _, r := range resources {
		// Handle multi-line reconcile info
		riLines := strings.Split(r.ReconcileInfo, "\n")
		firstRi := "-"
		if len(riLines) > 0 && riLines[0] != "" {
			firstRi = riLines[0]
		}

		// Print first line with all columns
		fmt.Printf("%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
			maxNamespace, r.Namespace,
			maxName, r.Name,
			maxKind, r.Kind,
			maxOwner, r.Owner,
			maxRs, r.ReconcileState,
			firstRi)

		// Print additional reconcile info lines if present
		for i := 1; i < len(riLines); i++ {
			if riLines[i] != "" {
				// Indent to align with Ri column
				indent := maxNamespace + maxName + maxKind + maxOwner + maxRs + 10
				fmt.Printf("%*s%s\n", indent, "", riLines[i])
			}
		}
	}

	// Print footer
	fmt.Printf("\nRs: Reconcile state\n")
	fmt.Printf("Ri: Reconcile information\n")
	fmt.Printf("\n%d resources\n", len(resources))

	return nil
}
