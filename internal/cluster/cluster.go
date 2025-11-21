package cluster

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/rkoster/deskrun/pkg/types"
)

// Manager handles kind cluster operations
type Manager struct {
	config *types.ClusterConfig
}

// NewManager creates a new cluster manager
func NewManager(config *types.ClusterConfig) *Manager {
	return &Manager{
		config: config,
	}
}

// Exists checks if the cluster exists
func (m *Manager) Exists(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		// If kind returns error, no clusters exist
		return false, nil
	}

	// Check if our cluster is in the list
	clusterName := m.config.Name
	clusters := string(output)

	// Simple check if cluster name appears in output
	return contains(clusters, clusterName), nil
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

	// Create cluster with config
	args := []string{"create", "cluster", "--name", m.config.Name}

	cmd := exec.CommandContext(ctx, "kind", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w\nOutput: %s", err, string(output))
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

	cmd := exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", m.config.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete cluster: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// GetKubeconfig returns the kubeconfig context name for the cluster
func (m *Manager) GetKubeconfig() string {
	return fmt.Sprintf("kind-%s", m.config.Name)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
