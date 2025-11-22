package cluster

import (
	"context"
	"fmt"

	"github.com/rkoster/deskrun/pkg/types"
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

	// Create cluster using kind Go package
	err = m.provider.Create(m.config.Name,
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

// GetKubeconfig returns the kubeconfig context name for the cluster
func (m *Manager) GetKubeconfig() string {
	return fmt.Sprintf("kind-%s", m.config.Name)
}
