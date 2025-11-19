package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rkoster/deskrun/internal/cluster"
	"github.com/rkoster/deskrun/pkg/templates"
	"github.com/rkoster/deskrun/pkg/types"
)

const (
	defaultNamespace = "arc-systems"
	helmChartRepo    = "oci://ghcr.io/actions/actions-runner-controller-charts"
	helmChartName    = "gha-runner-scale-set"
)

// Manager handles runner operations
type Manager struct {
	clusterManager *cluster.Manager
}

// NewManager creates a new runner manager
func NewManager(clusterManager *cluster.Manager) *Manager {
	return &Manager{
		clusterManager: clusterManager,
	}
}

// Install installs a runner scale set
func (m *Manager) Install(ctx context.Context, installation *types.RunnerInstallation) error {
	// Ensure cluster exists
	exists, err := m.clusterManager.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check cluster: %w", err)
	}
	if !exists {
		return fmt.Errorf("cluster does not exist, please create it first")
	}

	// Create namespace
	if err := m.createNamespace(ctx, defaultNamespace); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Create temporary directory for manifests
	tmpDir, err := os.MkdirTemp("/tmp", "deskrun-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate and apply secret
	secretManifest := templates.GenerateGitHubSecretManifest(installation, defaultNamespace)
	secretPath := filepath.Join(tmpDir, "secret.yaml")
	if err := os.WriteFile(secretPath, []byte(secretManifest), 0600); err != nil {
		return fmt.Errorf("failed to write secret manifest: %w", err)
	}

	if err := m.applyManifest(ctx, secretPath); err != nil {
		return fmt.Errorf("failed to apply secret: %w", err)
	}

	// Generate and apply runner scale set
	runnerManifest, err := templates.GenerateRunnerScaleSetManifest(installation, defaultNamespace)
	if err != nil {
		return fmt.Errorf("failed to generate runner manifest: %w", err)
	}

	runnerPath := filepath.Join(tmpDir, "runner.yaml")
	if err := os.WriteFile(runnerPath, []byte(runnerManifest), 0644); err != nil {
		return fmt.Errorf("failed to write runner manifest: %w", err)
	}

	if err := m.applyManifest(ctx, runnerPath); err != nil {
		return fmt.Errorf("failed to apply runner manifest: %w", err)
	}

	return nil
}

// Uninstall removes a runner scale set
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	// Delete runner scale set
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "autoscalingrunnersets", name,
		"-n", defaultNamespace, "--context", m.clusterManager.GetKubeconfig())
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete runner: %w\nOutput: %s", err, string(output))
	}

	// Delete secret
	secretName := fmt.Sprintf("%s-secret", name)
	cmd = exec.CommandContext(ctx, "kubectl", "delete", "secret", secretName,
		"-n", defaultNamespace, "--context", m.clusterManager.GetKubeconfig())
	// Ignore error if secret doesn't exist
	cmd.CombinedOutput()

	return nil
}

// List returns all runner scale sets
func (m *Manager) List(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "autoscalingrunnersets",
		"-n", defaultNamespace, "--context", m.clusterManager.GetKubeconfig(),
		"-o", "jsonpath={.items[*].metadata.name}")

	output, err := cmd.Output()
	if err != nil {
		// If namespace doesn't exist or no resources, return empty list
		return []string{}, nil
	}

	if len(output) == 0 {
		return []string{}, nil
	}

	// Parse output - names are space-separated
	names := []string{}
	current := ""
	for _, b := range output {
		if b == ' ' {
			if current != "" {
				names = append(names, current)
				current = ""
			}
		} else {
			current += string(b)
		}
	}
	if current != "" {
		names = append(names, current)
	}

	return names, nil
}

// Status returns the status of a runner installation
func (m *Manager) Status(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "autoscalingrunnersets", name,
		"-n", defaultNamespace, "--context", m.clusterManager.GetKubeconfig(),
		"-o", "wide")

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	return string(output), nil
}

func (m *Manager) createNamespace(ctx context.Context, namespace string) error {
	manifest := templates.GenerateNamespaceManifest(namespace)

	tmpFile, err := os.CreateTemp("/tmp", "namespace-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if err := os.WriteFile(tmpFile.Name(), []byte(manifest), 0644); err != nil {
		return err
	}

	// Apply with --dry-run=client first to validate, then apply
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", tmpFile.Name(),
		"--context", m.clusterManager.GetKubeconfig())

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore error if namespace already exists
		if !contains(string(output), "AlreadyExists") && !contains(string(output), "already exists") {
			return fmt.Errorf("failed to create namespace: %w\nOutput: %s", err, string(output))
		}
	}

	return nil
}

func (m *Manager) applyManifest(ctx context.Context, manifestPath string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", manifestPath,
		"--context", m.clusterManager.GetKubeconfig())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %w\nOutput: %s", err, string(output))
	}

	return nil
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
