package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/rkoster/deskrun/internal/cluster"
	"github.com/rkoster/deskrun/internal/config"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/spf13/cobra"
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage the kind cluster",
	Long:  `Manage the kind cluster used for running GitHub Actions runners.`,
}

var clusterCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create the kind cluster",
	Long:  `Create a new kind cluster for running GitHub Actions runners.`,
	RunE:  runClusterCreate,
}

var clusterDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete the kind cluster",
	Long:  `Delete the kind cluster and all associated resources.`,
	RunE:  runClusterDelete,
}

var clusterStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check cluster status",
	Long:  `Check if the kind cluster exists and is running.`,
	RunE:  runClusterStatus,
}

func init() {
	clusterCmd.AddCommand(clusterCreateCmd)
	clusterCmd.AddCommand(clusterDeleteCmd)
	clusterCmd.AddCommand(clusterStatusCmd)
	rootCmd.AddCommand(clusterCmd)
}

func runClusterCreate(cmd *cobra.Command, args []string) error {
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Detect available nix mounts
	nixStore, nixSocket := cluster.DetectNixMounts()

	// Log what we found
	if nixStore != nil {
		fmt.Printf("Detected Nix store at: %s\n", nixStore.HostPath)
	}
	if nixSocket != nil {
		fmt.Printf("Detected Nix daemon socket at: %s\n", nixSocket.HostPath)
	}

	clusterConfig := &types.ClusterConfig{
		Name:      configMgr.GetConfig().ClusterName,
		NixStore:  nixStore,
		NixSocket: nixSocket,
	}
	clusterMgr := cluster.NewManager(clusterConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	exists, err := clusterMgr.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster: %w", err)
	}

	if exists {
		fmt.Printf("Cluster '%s' already exists\n", clusterConfig.Name)
		return nil
	}

	fmt.Printf("Creating kind cluster '%s'", clusterConfig.Name)
	if nixStore != nil || nixSocket != nil {
		fmt.Print(" with Nix support")
	}
	fmt.Println("...")

	if err := clusterMgr.Create(ctx); err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	fmt.Printf("Cluster '%s' created successfully", clusterConfig.Name)
	if nixStore != nil || nixSocket != nil {
		fmt.Print(" with Nix bind mounts configured")
	}
	fmt.Println()
	return nil
}

func runClusterDelete(cmd *cobra.Command, args []string) error {
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	clusterConfig := &types.ClusterConfig{
		Name: configMgr.GetConfig().ClusterName,
	}
	clusterMgr := cluster.NewManager(clusterConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	exists, err := clusterMgr.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster: %w", err)
	}

	if !exists {
		fmt.Printf("Cluster '%s' does not exist\n", clusterConfig.Name)
		return nil
	}

	fmt.Printf("Deleting kind cluster '%s'...\n", clusterConfig.Name)
	if err := clusterMgr.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	fmt.Printf("Cluster '%s' deleted successfully\n", clusterConfig.Name)
	return nil
}

func runClusterStatus(cmd *cobra.Command, args []string) error {
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	clusterConfig := &types.ClusterConfig{
		Name: configMgr.GetConfig().ClusterName,
	}
	clusterMgr := cluster.NewManager(clusterConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	exists, err := clusterMgr.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster: %w", err)
	}

	if exists {
		fmt.Printf("Cluster '%s' is running\n", clusterConfig.Name)
		fmt.Printf("Kubeconfig context: %s\n", clusterMgr.GetKubeconfig())
	} else {
		fmt.Printf("Cluster '%s' does not exist\n", clusterConfig.Name)
	}

	return nil
}
