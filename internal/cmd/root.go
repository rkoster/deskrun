package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "deskrun",
	Short: "DeskRun: Unlocking Local Compute for GitHub Actions",
	Long: `deskrun is a CLI tool for running GitHub Actions locally using kind clusters.
It provides easy management of local GitHub Actions runners with optimized
configurations based on lessons learned from production deployments.`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags will be added here
}
