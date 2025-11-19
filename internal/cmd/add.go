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

var (
	addRepository string
	addMode       string
	addMinRunners int
	addMaxRunners int
	addAuthType   string
	addAuthValue  string
	addCachePaths []string
)

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new runner installation",
	Long: `Add a new GitHub Actions runner installation to the kind cluster.

The installation will be configured with the specified container mode and
authentication credentials. Use different container modes based on your needs:

- kubernetes: Standard mode for simple repositories
- cached-privileged-kubernetes: For repositories needing systemd, nested Docker, or Nix
- dind: Docker-in-Docker mode for full Docker access

Examples:
  # Add a standard runner
  deskrun add my-runner --repository https://github.com/owner/repo --auth-type pat --auth-value ghp_xxx

  # Add a privileged runner with Nix cache
  deskrun add nix-runner \
    --repository https://github.com/owner/repo \
    --mode cached-privileged-kubernetes \
    --cache /nix/store \
    --auth-type pat --auth-value ghp_xxx

  # Add a DinD runner
  deskrun add dind-runner \
    --repository https://github.com/owner/repo \
    --mode dind \
    --auth-type pat --auth-value ghp_xxx
`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	addCmd.Flags().StringVarP(&addRepository, "repository", "r", "", "GitHub repository URL (required)")
	addCmd.Flags().StringVarP(&addMode, "mode", "m", "kubernetes", "Container mode (kubernetes, cached-privileged-kubernetes, dind)")
	addCmd.Flags().IntVar(&addMinRunners, "min-runners", 1, "Minimum number of runners")
	addCmd.Flags().IntVar(&addMaxRunners, "max-runners", 5, "Maximum number of runners")
	addCmd.Flags().StringVar(&addAuthType, "auth-type", "pat", "Authentication type (pat, github-app)")
	addCmd.Flags().StringVar(&addAuthValue, "auth-value", "", "Authentication value (PAT token or GitHub App private key)")
	addCmd.Flags().StringSliceVar(&addCachePaths, "cache", []string{}, "Cache paths to mount (can be specified multiple times)")

	addCmd.MarkFlagRequired("repository")
	addCmd.MarkFlagRequired("auth-value")

	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate container mode
	var containerMode types.ContainerMode
	switch addMode {
	case "kubernetes":
		containerMode = types.ContainerModeKubernetes
	case "cached-privileged-kubernetes":
		containerMode = types.ContainerModePrivileged
	case "dind":
		containerMode = types.ContainerModeDinD
	default:
		return fmt.Errorf("invalid container mode: %s", addMode)
	}

	// Validate auth type
	var authType types.AuthType
	switch addAuthType {
	case "pat":
		authType = types.AuthTypePAT
	case "github-app":
		authType = types.AuthTypeGitHubApp
	default:
		return fmt.Errorf("invalid auth type: %s", addAuthType)
	}

	// Create cache paths
	cachePaths := []types.CachePath{}
	for _, path := range addCachePaths {
		cachePaths = append(cachePaths, types.CachePath{
			MountPath: path,
			HostPath:  "", // Will be auto-generated
		})
	}

	// Create installation
	installation := &types.RunnerInstallation{
		Name:          name,
		Repository:    addRepository,
		ContainerMode: containerMode,
		MinRunners:    addMinRunners,
		MaxRunners:    addMaxRunners,
		CachePaths:    cachePaths,
		AuthType:      authType,
		AuthValue:     addAuthValue,
	}

	// Load config
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if cluster exists, create if needed
	clusterConfig := &types.ClusterConfig{
		Name: configMgr.GetConfig().ClusterName,
	}
	clusterMgr := cluster.NewManager(clusterConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

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
	}

	// Install runner
	fmt.Printf("Installing runner '%s'...\n", name)
	runnerMgr := runner.NewManager(clusterMgr)
	if err := runnerMgr.Install(ctx, installation); err != nil {
		return fmt.Errorf("failed to install runner: %w", err)
	}

	// Save to config
	if err := configMgr.AddInstallation(installation); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Runner '%s' installed successfully\n", name)
	return nil
}
