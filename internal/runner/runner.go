package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rkoster/deskrun/internal/cluster"
	"github.com/rkoster/deskrun/pkg/templates"
	"github.com/rkoster/deskrun/pkg/types"
)

const (
	defaultNamespace        = "arc-systems"
	arcControllerNamespace  = "arc-systems"
	arcControllerRelease    = "arc-controller"
	arcControllerChartRepo  = "oci://ghcr.io/actions/actions-runner-controller-charts"
	arcControllerChartName  = "gha-runner-scale-set-controller"
	runnerScaleSetChartRepo = "oci://ghcr.io/actions/actions-runner-controller-charts"
	runnerScaleSetChartName = "gha-runner-scale-set"
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

	// Ensure ARC controller is installed
	if err := m.ensureARCController(ctx); err != nil {
		return fmt.Errorf("failed to ensure ARC controller: %w", err)
	}

	// Create temporary directory for Helm values
	tmpDir, err := os.MkdirTemp("/tmp", "deskrun-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Install runner scale set using Helm
	fmt.Printf("Installing runner scale set '%s'...\n", installation.Name)

	// Prepare Helm values
	valuesPath := filepath.Join(tmpDir, "values.yaml")
	valuesContent, err := m.generateHelmValues(installation)
	if err != nil {
		return fmt.Errorf("failed to generate helm values: %w", err)
	}

	if err := os.WriteFile(valuesPath, []byte(valuesContent), 0644); err != nil {
		return fmt.Errorf("failed to write helm values: %w", err)
	}

	// Install using Helm
	cmd := exec.CommandContext(ctx, "helm", "install", installation.Name,
		fmt.Sprintf("%s/%s", runnerScaleSetChartRepo, runnerScaleSetChartName),
		"--namespace", defaultNamespace,
		"--values", valuesPath,
		"--kube-context", m.clusterManager.GetKubeconfig(),
		"--wait")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if already installed
		if contains(string(output), "already exists") || contains(string(output), "cannot re-use") {
			return fmt.Errorf("runner %s already exists, please remove it first: %w\nOutput: %s", installation.Name, err, string(output))
		}
		return fmt.Errorf("failed to install runner scale set: %w\nOutput: %s", err, string(output))
	}

	fmt.Println("Runner scale set installed successfully")
	return nil
}

// Uninstall removes a runner scale set
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	// Uninstall using Helm
	cmd := exec.CommandContext(ctx, "helm", "uninstall", name,
		"--namespace", defaultNamespace,
		"--kube-context", m.clusterManager.GetKubeconfig())

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if already uninstalled
		if contains(string(output), "not found") || contains(string(output), "no release found") {
			return fmt.Errorf("runner %s not found: %w\nOutput: %s", name, err, string(output))
		}
		return fmt.Errorf("failed to uninstall runner: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// List returns all runner scale sets
func (m *Manager) List(ctx context.Context) ([]string, error) {
	// List Helm releases in the namespace
	cmd := exec.CommandContext(ctx, "helm", "list",
		"--namespace", defaultNamespace,
		"--kube-context", m.clusterManager.GetKubeconfig(),
		"--short")

	output, err := cmd.Output()
	if err != nil {
		// If namespace doesn't exist or no releases, return empty list
		return []string{}, nil
	}

	if len(output) == 0 {
		return []string{}, nil
	}

	// Parse output - names are line-separated
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	names := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != arcControllerRelease {
			// Exclude the ARC controller from the list
			names = append(names, line)
		}
	}

	return names, nil
}

// Status returns the status of a runner installation
func (m *Manager) Status(ctx context.Context, name string) (string, error) {
	// Get Helm release status
	cmd := exec.CommandContext(ctx, "helm", "status", name,
		"--namespace", defaultNamespace,
		"--kube-context", m.clusterManager.GetKubeconfig())

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	// Also get the AutoscalingRunnerSet status
	cmd = exec.CommandContext(ctx, "kubectl", "get", "autoscalingrunnersets", name,
		"-n", defaultNamespace, "--context", m.clusterManager.GetKubeconfig(),
		"-o", "wide")

	k8sOutput, err := cmd.Output()
	if err != nil {
		// Just show Helm status if kubectl fails
		return string(output), nil
	}

	return fmt.Sprintf("%s\n\nKubernetes Resources:\n%s", string(output), string(k8sOutput)), nil
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

// generateHelmValues generates Helm values for the runner scale set
func (m *Manager) generateHelmValues(installation *types.RunnerInstallation) (string, error) {
	// Determine authentication method
	var githubConfigSecret string
	if installation.AuthType == types.AuthTypePAT {
		githubConfigSecret = fmt.Sprintf(`githubConfigSecret:
  github_token: "%s"`, installation.AuthValue)
	} else {
		githubConfigSecret = fmt.Sprintf(`githubConfigSecret:
  github_app_id: ""
  github_app_installation_id: ""
  github_app_private_key: |
    %s`, installation.AuthValue)
	}

	// Build container mode configuration
	var containerModeConfig string
	switch installation.ContainerMode {
	case types.ContainerModeKubernetes:
		containerModeConfig = `containerMode:
  type: "kubernetes"`
	case types.ContainerModePrivileged:
		containerModeConfig = m.generatePrivilegedContainerMode(installation)
	case types.ContainerModeDinD:
		containerModeConfig = `containerMode:
  type: "dind"`
	default:
		return "", fmt.Errorf("unsupported container mode: %s", installation.ContainerMode)
	}

	values := fmt.Sprintf(`githubConfigUrl: "%s"
minRunners: %d
maxRunners: %d

%s

%s
`, installation.Repository, installation.MinRunners, installation.MaxRunners, githubConfigSecret, containerModeConfig)

	return values, nil
}

// generatePrivilegedContainerMode generates the privileged container mode configuration
func (m *Manager) generatePrivilegedContainerMode(installation *types.RunnerInstallation) string {
	config := `containerMode:
  type: "kubernetes"
template:
  spec:
    securityContext:
      runAsUser: 0
      runAsGroup: 0
      fsGroup: 0
    containers:
    - name: runner
      securityContext:
        privileged: true
        runAsUser: 0
        runAsGroup: 0
        allowPrivilegeEscalation: true
        readOnlyRootFilesystem: false
        capabilities:
          add:
            - SYS_ADMIN
            - NET_ADMIN
            - SYS_PTRACE
            - SYS_CHROOT
            - SETFCAP
            - SETPCAP
            - NET_RAW
            - IPC_LOCK
            - SYS_RESOURCE
            - MKNOD
            - AUDIT_WRITE
            - AUDIT_CONTROL
      env:
      - name: SYSTEMD_IGNORE_CHROOT
        value: "1"`

	// Add volume mounts if cache paths are specified
	if len(installation.CachePaths) > 0 {
		config += "\n      volumeMounts:"
		config += "\n      - name: work"
		config += "\n        mountPath: /home/runner/_work"
		for i, path := range installation.CachePaths {
			config += fmt.Sprintf("\n      - name: cache-%d", i)
			config += fmt.Sprintf("\n        mountPath: %s", path.MountPath)
		}

		config += "\n    volumes:"
		config += "\n    - name: work"
		config += "\n      emptyDir: {}"
		for i, path := range installation.CachePaths {
			hostPath := path.HostPath
			if hostPath == "" {
				hostPath = fmt.Sprintf("/tmp/github-runner-cache/%s/cache-%d", installation.Name, i)
			}
			config += fmt.Sprintf("\n    - name: cache-%d", i)
			config += "\n      hostPath:"
			config += fmt.Sprintf("\n        path: %s", hostPath)
			config += "\n        type: DirectoryOrCreate"
		}
	}

	return config
}

// ensureARCController checks if the ARC controller is installed and installs it if needed
func (m *Manager) ensureARCController(ctx context.Context) error {
	// Check if CRDs are already installed
	cmd := exec.CommandContext(ctx, "kubectl", "get", "crd", "autoscalingrunnersets.actions.github.com",
		"--context", m.clusterManager.GetKubeconfig())
	if err := cmd.Run(); err == nil {
		// CRDs already exist, controller is likely installed
		return nil
	}

	// CRDs don't exist, install the controller
	fmt.Println("Installing GitHub Actions Runner Controller...")

	// Install controller using helm
	cmd = exec.CommandContext(ctx, "helm", "install", arcControllerRelease,
		fmt.Sprintf("%s/%s", arcControllerChartRepo, arcControllerChartName),
		"--namespace", arcControllerNamespace,
		"--create-namespace",
		"--kube-context", m.clusterManager.GetKubeconfig(),
		"--wait")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if already installed
		if contains(string(output), "already exists") || contains(string(output), "cannot re-use") {
			fmt.Println("Controller already installed")
			return nil
		}
		return fmt.Errorf("failed to install ARC controller: %w\nOutput: %s", err, string(output))
	}

	fmt.Println("ARC controller installed successfully")

	// Wait a moment for CRDs to be ready
	fmt.Println("Waiting for CRDs to be ready...")
	for i := 0; i < 30; i++ {
		cmd = exec.CommandContext(ctx, "kubectl", "wait", "--for", "condition=established",
			"--timeout=10s", "crd/autoscalingrunnersets.actions.github.com",
			"--context", m.clusterManager.GetKubeconfig())
		if err := cmd.Run(); err == nil {
			fmt.Println("CRDs are ready")
			return nil
		}
		// Wait a bit and retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
			// Continue to next iteration
		}
	}

	return fmt.Errorf("timeout waiting for CRDs to be ready")
}
