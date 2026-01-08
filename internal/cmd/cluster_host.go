package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/rkoster/deskrun/internal/config"
	"github.com/rkoster/deskrun/internal/incus"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/spf13/cobra"
)

var (
	clusterHostName     string
	clusterHostDiskSize string
	clusterHostImage    string
)

var clusterHostCmd = &cobra.Command{
	Use:   "cluster-host",
	Short: "Manage remote Incus cluster hosts",
	Long: `Manage remote Incus cluster hosts for running deskrun on dedicated infrastructure.
	
Cluster hosts are NixOS containers provisioned on Incus with Docker, Kind, and deskrun pre-installed.`,
}

var clusterHostCreateCmd = &cobra.Command{
	Use:   "create [--name <name>] [--disk <size>] [--image <image>]",
	Short: "Create a new cluster host",
	Long: `Create a new Incus container with NixOS pre-configured with Docker, Kind, and deskrun.

The container will be created on the current Incus remote (use 'incus remote switch' to change).

Examples:
  # Create with auto-generated name
  deskrun cluster-host create

  # Create with custom name and disk size
  deskrun cluster-host create --name my-host --disk 300GiB

  # Create with specific NixOS image
  deskrun cluster-host create --image images:nixos/25.11`,
	RunE: runClusterHostCreate,
}

var clusterHostDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a cluster host",
	Long:  `Delete a cluster host container and remove it from configuration.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runClusterHostDelete,
}

var clusterHostListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all cluster hosts",
	Long:  `List all cluster hosts with their status and configuration.`,
	RunE:  runClusterHostList,
}

var clusterHostConfigureCmd = &cobra.Command{
	Use:   "configure <name>",
	Short: "Re-configure a cluster host",
	Long: `Re-apply NixOS configuration to a cluster host.

This is useful after deskrun updates or if the initial configuration failed.`,
	Args: cobra.ExactArgs(1),
	RunE: runClusterHostConfigure,
}

func init() {
	clusterHostCreateCmd.Flags().StringVar(&clusterHostName, "name", "", "Container name (auto-generated if not specified)")
	clusterHostCreateCmd.Flags().StringVar(&clusterHostDiskSize, "disk", "200GiB", "Root disk size")
	clusterHostCreateCmd.Flags().StringVar(&clusterHostImage, "image", "images:nixos/25.11", "NixOS image to use")

	clusterHostCmd.AddCommand(clusterHostCreateCmd)
	clusterHostCmd.AddCommand(clusterHostDeleteCmd)
	clusterHostCmd.AddCommand(clusterHostListCmd)
	clusterHostCmd.AddCommand(clusterHostConfigureCmd)
	rootCmd.AddCommand(clusterHostCmd)
}

func runClusterHostCreate(cmd *cobra.Command, args []string) error {
	name := clusterHostName
	if name == "" {
		randomBytes := make([]byte, 3)
		if _, err := rand.Read(randomBytes); err != nil {
			return fmt.Errorf("failed to generate random name: %w", err)
		}
		name = fmt.Sprintf("deskrun-%s", hex.EncodeToString(randomBytes))
	}

	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if _, err := configMgr.GetClusterHost(name); err == nil {
		return fmt.Errorf("cluster host %s already exists in configuration", name)
	}

	incusMgr := incus.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	exists, err := incusMgr.ContainerExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check if container exists: %w", err)
	}
	if exists {
		return fmt.Errorf("container %s already exists", name)
	}

	fmt.Printf("Creating cluster host '%s'...\n", name)
	fmt.Println("Launching NixOS container...")

	if err := incusMgr.CreateContainer(ctx, name, clusterHostImage, clusterHostDiskSize); err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	fmt.Println("Waiting for container to start...")
	if err := incusMgr.WaitForRunning(ctx, name, 2*time.Minute); err != nil {
		_ = incusMgr.DeleteContainer(ctx, name)
		return fmt.Errorf("container failed to start: %w", err)
	}

	fmt.Println("Configuring NixOS with Docker, Kind, and deskrun...")
	if err := incusMgr.ConfigureNixOS(ctx, name); err != nil {
		return fmt.Errorf("failed to configure NixOS: %w", err)
	}

	host := &types.ClusterHost{
		Name:      name,
		Image:     clusterHostImage,
		DiskSize:  clusterHostDiskSize,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	if err := configMgr.AddClusterHost(host); err != nil {
		return fmt.Errorf("failed to save cluster host to config: %w", err)
	}

	fmt.Printf("\nCluster host '%s' created successfully\n\n", name)
	fmt.Printf("To access: incus exec %s -- bash\n", name)
	fmt.Printf("To run deskrun: incus exec %s -- deskrun --help\n", name)

	return nil
}

func runClusterHostDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if _, err := configMgr.GetClusterHost(name); err != nil {
		fmt.Printf("Warning: cluster host %s not found in configuration\n", name)
	}

	incusMgr := incus.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	exists, err := incusMgr.ContainerExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check if container exists: %w", err)
	}

	if !exists {
		fmt.Printf("Container %s does not exist\n", name)
	} else {
		fmt.Printf("Deleting cluster host '%s'...\n", name)
		if err := incusMgr.DeleteContainer(ctx, name); err != nil {
			return fmt.Errorf("failed to delete container: %w", err)
		}
	}

	if err := configMgr.RemoveClusterHost(name); err != nil {
		fmt.Printf("Note: %v\n", err)
	}

	fmt.Printf("Cluster host '%s' deleted successfully\n", name)
	return nil
}

func runClusterHostList(cmd *cobra.Command, args []string) error {
	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	incusMgr := incus.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allContainers, err := incusMgr.ListContainers(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	configuredHosts := make(map[string]bool)
	for name := range configMgr.GetConfig().ClusterHosts {
		configuredHosts[name] = true
	}

	var containers []incus.ContainerInfo
	for _, container := range allContainers {
		if configuredHosts[container.Name] {
			containers = append(containers, container)
		}
	}

	if len(containers) == 0 && len(configMgr.GetConfig().ClusterHosts) == 0 {
		fmt.Println("No cluster hosts found")
		return nil
	}

	fmt.Printf("%-20s %-10s %-20s %-10s %-20s\n", "NAME", "STATUS", "IMAGE", "DISK", "CREATED")
	fmt.Println("--------------------------------------------------------------------------------------------")

	for _, container := range containers {
		host, err := configMgr.GetClusterHost(container.Name)
		if err != nil {
			fmt.Printf("%-20s %-10s %-20s %-10s %-20s\n",
				container.Name,
				container.Status,
				"N/A",
				"N/A",
				"N/A")
			continue
		}

		createdAt := host.CreatedAt
		if t, err := time.Parse(time.RFC3339, host.CreatedAt); err == nil {
			createdAt = t.Format("2006-01-02 15:04:05")
		}

		fmt.Printf("%-20s %-10s %-20s %-10s %-20s\n",
			host.Name,
			container.Status,
			host.Image,
			host.DiskSize,
			createdAt)
	}

	return nil
}

func runClusterHostConfigure(cmd *cobra.Command, args []string) error {
	name := args[0]

	configMgr, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if _, err := configMgr.GetClusterHost(name); err != nil {
		return fmt.Errorf("cluster host %s not found in configuration", name)
	}

	incusMgr := incus.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	exists, err := incusMgr.ContainerExists(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check if container exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("container %s does not exist", name)
	}

	fmt.Println("Applying NixOS configuration...")
	if err := incusMgr.ConfigureNixOS(ctx, name); err != nil {
		return fmt.Errorf("failed to configure NixOS: %w", err)
	}

	fmt.Println("Configuration applied successfully")
	return nil
}
