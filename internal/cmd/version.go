package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Version is the current version of deskrun
	Version = "0.1.0"
	// GitCommit is the git commit hash
	GitCommit = "dev"
	// BuildDate is the build date
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version number of deskrun and additional build information.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("deskrun version %s\n", Version)
		fmt.Printf("Git commit: %s\n", GitCommit)
		fmt.Printf("Build date: %s\n", BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
