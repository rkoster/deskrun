package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rkoster/deskrun/pkg/types"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
)

// Manager handles kind cluster operations
type Manager struct {
	config   *types.ClusterConfig
	provider *cluster.Provider
}

// NewManager creates a new cluster manager
func NewManager(config *types.ClusterConfig) *Manager {
	return &Manager{
		config:   config,
		provider: cluster.NewProvider(),
	}
}

// Exists checks if the cluster exists
func (m *Manager) Exists(ctx context.Context) (bool, error) {
	clusters, err := m.provider.List()
	if err != nil {
		return false, fmt.Errorf("failed to list clusters: %w", err)
	}

	// Check if our cluster is in the list
	for _, name := range clusters {
		if name == m.config.Name {
			return true, nil
		}
	}

	return false, nil
}

// Create creates a new kind cluster
func (m *Manager) Create(ctx context.Context) error {
	exists, err := m.Exists(ctx)
	if err != nil {
		return err
	}

	if exists {
		return fmt.Errorf("cluster %s already exists", m.config.Name)
	}

	// Build kind configuration with nix mounts
	kindConfig := m.buildKindConfig()

	// Create cluster using kind Go package with custom config
	err = m.provider.Create(m.config.Name,
		cluster.CreateWithV1Alpha4Config(kindConfig),
		cluster.CreateWithWaitForReady(0), // Use default wait time
	)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	return nil
}

// Delete deletes the kind cluster
func (m *Manager) Delete(ctx context.Context) error {
	exists, err := m.Exists(ctx)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("cluster %s does not exist", m.config.Name)
	}

	// Delete cluster using kind Go package
	err = m.provider.Delete(m.config.Name, "")
	if err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	return nil
}

// DetectNixMounts detects available nix mounts on the host system
func DetectNixMounts() (*types.ClusterMount, *types.ClusterMount) {
	var nixStore, nixSocket *types.ClusterMount

	// Check for /nix/store
	if _, err := os.Stat("/nix/store"); err == nil {
		nixStore = &types.ClusterMount{
			HostPath:      "/nix/store",
			ContainerPath: "/nix/store",
		}
	}

	// Check for nix daemon socket
	nixSocketPaths := []string{
		"/nix/var/nix/daemon-socket/socket",
		"/var/run/nix/daemon-socket/socket",
	}

	for _, socketPath := range nixSocketPaths {
		if _, err := os.Stat(socketPath); err == nil {
			nixSocket = &types.ClusterMount{
				HostPath:      socketPath,
				ContainerPath: "/nix/var/nix/daemon-socket/socket",
			}
			break
		}
	}

	return nixStore, nixSocket
}

// DetectDeskrunCache detects the host deskrun cache directory and creates mount config
func DetectDeskrunCache() *types.ClusterMount {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get home directory for deskrun cache: %v\n", err)
		return nil
	}

	deskrunCachePath := filepath.Join(homeDir, ".cache", "deskrun")

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(deskrunCachePath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create deskrun cache directory at %s: %v\n", deskrunCachePath, err)
		return nil
	}

	return &types.ClusterMount{
		HostPath:      deskrunCachePath,
		ContainerPath: "/host-cache/deskrun",
	}
}

// DetectDockerSocket detects if Docker socket is available on the host system
func DetectDockerSocket() *types.ClusterMount {
	dockerSocketPath := "/var/run/docker.sock"

	// Check if Docker socket exists
	if _, err := os.Stat(dockerSocketPath); err == nil {
		return &types.ClusterMount{
			HostPath:      dockerSocketPath,
			ContainerPath: dockerSocketPath,
		}
	}

	return nil
}

// buildKindConfig creates a kind cluster configuration with nix and cache mounts
func (m *Manager) buildKindConfig() *v1alpha4.Cluster {
	config := &v1alpha4.Cluster{
		TypeMeta: v1alpha4.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "kind.x-k8s.io/v1alpha4",
		},
		Name: m.config.Name,
	}

	// Add a single node configuration
	node := v1alpha4.Node{
		Role: v1alpha4.ControlPlaneRole,
	}

	// Add nix mounts if available
	var extraMounts []v1alpha4.Mount

	if m.config.NixStore != nil {
		extraMounts = append(extraMounts, v1alpha4.Mount{
			HostPath:      m.config.NixStore.HostPath,
			ContainerPath: m.config.NixStore.ContainerPath,
			Readonly:      true,
		})
	}

	if m.config.NixSocket != nil {
		// Create the directory for the socket if it doesn't exist
		socketDir := filepath.Dir(m.config.NixSocket.ContainerPath)
		extraMounts = append(extraMounts, v1alpha4.Mount{
			HostPath:      filepath.Dir(m.config.NixSocket.HostPath),
			ContainerPath: socketDir,
		})
	}

	if m.config.DeskrunCache != nil {
		extraMounts = append(extraMounts, v1alpha4.Mount{
			HostPath:      m.config.DeskrunCache.HostPath,
			ContainerPath: m.config.DeskrunCache.ContainerPath,
		})
	}

	if m.config.DockerSocket != nil {
		extraMounts = append(extraMounts, v1alpha4.Mount{
			HostPath:      m.config.DockerSocket.HostPath,
			ContainerPath: m.config.DockerSocket.ContainerPath,
		})
	}

	if len(extraMounts) > 0 {
		node.ExtraMounts = extraMounts
	}

	config.Nodes = []v1alpha4.Node{node}
	return config
}

// GetKubeconfig returns the kubeconfig context name for the cluster
func (m *Manager) GetKubeconfig() string {
	return fmt.Sprintf("kind-%s", m.config.Name)
}
