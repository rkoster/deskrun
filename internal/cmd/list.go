package cmd

import (
	"fmt"
	"strings"

	"github.com/rkoster/deskrun/internal/config"
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
`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	// Load config
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	installations := configMgr.GetConfig().Installations

	if len(installations) == 0 {
		fmt.Println("No runner installations found")
		return nil
	}

	fmt.Printf("Cluster: %s\n\n", configMgr.GetConfig().ClusterName)
	fmt.Println("Runner Installations:")
	fmt.Println(strings.Repeat("-", 80))

	for name, installation := range installations {
		fmt.Printf("\nName:          %s\n", name)
		fmt.Printf("Repository:    %s\n", installation.Repository)
		fmt.Printf("Mode:          %s\n", installation.ContainerMode)
		fmt.Printf("Min Runners:   %d\n", installation.MinRunners)
		fmt.Printf("Max Runners:   %d\n", installation.MaxRunners)
		fmt.Printf("Auth Type:     %s\n", installation.AuthType)

		if len(installation.CachePaths) > 0 {
			fmt.Printf("Cache Paths:   ")
			for i, path := range installation.CachePaths {
				if i > 0 {
					fmt.Printf("               ")
				}
				if path.Source != "" {
					fmt.Printf("%s:%s\n", path.Source, path.Target)
				} else {
					fmt.Printf("%s (auto-generated source)\n", path.Target)
				}
			}
		}
	}

	return nil
}
