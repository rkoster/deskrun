package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/rkoster/deskrun/internal/cluster"
	"github.com/rkoster/deskrun/internal/config"
	"github.com/rkoster/deskrun/internal/runner"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all runner installations",
	Long: `List all configured GitHub Actions runner installations.

This shows all runner installations managed by deskrun, including their
configuration details.

Example:
  deskrun list
  deskrun list --instances
`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().Bool("instances", false, "Show running instances for each installation")
}

func runList(cmd *cobra.Command, args []string) error {
	// Load config
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	installations := configMgr.GetConfig().Installations
	showInstances, _ := cmd.Flags().GetBool("instances")

	if len(installations) == 0 {
		fmt.Println("No runner installations found")
		return nil
	}

	fmt.Printf("Cluster: %s\n\n", configMgr.GetConfig().ClusterName)
	fmt.Println("Runner Installations:")
	fmt.Println(strings.Repeat("-", 80))

	// If showing instances, we need to connect to the cluster
	var runnerMgr *runner.Manager
	var actualInstances []string
	if showInstances {
		// Create cluster configuration
		clusterConfig := &types.ClusterConfig{
			Name: configMgr.GetConfig().ClusterName,
		}
		clusterMgr := cluster.NewManager(clusterConfig)

		exists, err := clusterMgr.Exists(context.Background())
		if err != nil {
			return fmt.Errorf("failed to check cluster: %w", err)
		}
		if !exists {
			fmt.Printf("Note: Cluster '%s' does not exist, cannot show running instances\n\n", configMgr.GetConfig().ClusterName)
		} else {
			runnerMgr = runner.NewManager(clusterMgr)
			actualInstances, err = runnerMgr.List(context.Background())
			if err != nil {
				fmt.Printf("Warning: Failed to get running instances: %v\n\n", err)
				actualInstances = []string{}
			}
		}
	}

	for name, installation := range installations {
		fmt.Printf("\nName:          %s\n", name)
		fmt.Printf("Repository:    %s\n", installation.Repository)
		fmt.Printf("Mode:          %s\n", installation.ContainerMode)

		// Show configured instances
		instances := installation.Instances
		if instances < 1 {
			instances = 1
		}

		// Only show min/max runners for single instance deployments
		// Multiple instances always use min=1, max=1 per instance
		if instances == 1 {
			fmt.Printf("Min Runners:   %d\n", installation.MinRunners)
			fmt.Printf("Max Runners:   %d\n", installation.MaxRunners)
		} else {
			fmt.Printf("Instances:     %d\n", instances)
		}

		fmt.Printf("Auth Type:     %s\n", installation.AuthType)

		if len(installation.Mounts) > 0 {
			fmt.Printf("Mounts:        ")
			for i, mount := range installation.Mounts {
				if i > 0 {
					fmt.Printf("               ")
				}
				if mount.Source != "" {
					if mount.Type != "" && mount.Type != types.MountTypeDirectoryOrCreate {
						fmt.Printf("%s:%s:%s\n", mount.Source, mount.Target, mount.Type)
					} else {
						fmt.Printf("%s:%s\n", mount.Source, mount.Target)
					}
				} else {
					fmt.Printf("%s\n", mount.Target)
				}
			}
		}

		if len(installation.CachePaths) > 0 {
			fmt.Printf("Cache Paths:   ")
			for i, path := range installation.CachePaths {
				if i > 0 {
					fmt.Printf("               ")
				}
				if path.Source != "" {
					fmt.Printf("%s:%s\n", path.Source, path.Target)
				} else {
					fmt.Printf("%s\n", path.Target)
				}
			}
		}

		// Show running instances if requested
		if showInstances && runnerMgr != nil {
			fmt.Printf("Running:       ")

			var runningInstances []string
			if instances == 1 {
				// Single instance - check for exact name match
				for _, instance := range actualInstances {
					if instance == name {
						runningInstances = append(runningInstances, instance)
					}
				}
			} else {
				// Multiple instances - check for numbered suffixes
				for i := 1; i <= instances; i++ {
					expectedName := fmt.Sprintf("%s-%d", name, i)
					for _, instance := range actualInstances {
						if instance == expectedName {
							runningInstances = append(runningInstances, instance)
						}
					}
				}
			}

			if len(runningInstances) == 0 {
				fmt.Printf("None\n")
			} else {
				fmt.Printf("%s\n", strings.Join(runningInstances, ", "))
			}
		}
	}

	return nil
}
