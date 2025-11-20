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
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
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

// getHelmConfig creates a Helm action configuration
func (m *Manager) getHelmConfig(namespace string) (*action.Configuration, error) {
	settings := cli.New()
	settings.KubeContext = m.clusterManager.GetKubeconfig()

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, "", func(format string, v ...interface{}) {
		// Suppress Helm's default logger
	}); err != nil {
		return nil, err
	}

	// Configure registry client for OCI
	registryClient, err := registry.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create registry client: %w", err)
	}
	actionConfig.RegistryClient = registryClient

	return actionConfig, nil
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

	// If instances > 1, create multiple separate runner scale sets
	instances := installation.Instances
	if instances < 1 {
		instances = 1
	}

	if instances == 1 {
		// Single instance - use the installation name as-is
		return m.installInstance(ctx, installation, installation.Name, 0)
	}

	// Multiple instances - create separate scale sets with numbered suffixes
	fmt.Printf("Installing %d runner scale set instances for '%s'...\n", instances, installation.Name)
	for i := 1; i <= instances; i++ {
		instanceName := fmt.Sprintf("%s-%d", installation.Name, i)
		if err := m.installInstance(ctx, installation, instanceName, i); err != nil {
			return fmt.Errorf("failed to install instance %d: %w", i, err)
		}
	}

	fmt.Printf("All %d instances installed successfully\n", instances)
	return nil
}

// installInstance installs a single runner scale set instance
func (m *Manager) installInstance(ctx context.Context, installation *types.RunnerInstallation, instanceName string, instanceNum int) error {
	// Create temporary directory for Helm values
	tmpDir, err := os.MkdirTemp("/tmp", "deskrun-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("  Installing runner scale set '%s'...\n", instanceName)

	// For privileged container mode, generate and apply the hook extension ConfigMap
	if installation.ContainerMode == types.ContainerModePrivileged {
		fmt.Printf("  Applying hook extension ConfigMap for privileged mode...\n")
		hookExtension := m.generateHookExtensionConfigMap(installation, instanceName)
		hookExtensionPath := filepath.Join(tmpDir, "hook-extension.yaml")
		if err := os.WriteFile(hookExtensionPath, []byte(hookExtension), 0644); err != nil {
			return fmt.Errorf("failed to write hook extension: %w", err)
		}

		// Apply the ConfigMap using kubectl
		cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", hookExtensionPath,
			"--context", m.clusterManager.GetKubeconfig())
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to apply hook extension ConfigMap: %w", err)
		}
	}

	// Prepare Helm values with instance-specific cache paths
	valuesPath := filepath.Join(tmpDir, "values.yaml")
	valuesContent, err := m.generateHelmValues(installation, instanceName, instanceNum)
	if err != nil {
		return fmt.Errorf("failed to generate helm values: %w", err)
	}

	if err := os.WriteFile(valuesPath, []byte(valuesContent), 0644); err != nil {
		return fmt.Errorf("failed to write helm values: %w", err)
	}

	// Install using Helm SDK
	actionConfig, err := m.getHelmConfig(defaultNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm config: %w", err)
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = instanceName
	client.Namespace = defaultNamespace
	client.Wait = true
	client.Timeout = 5 * time.Minute
	client.CreateNamespace = false // We create it separately

	// Load the chart from OCI registry
	chartPath := fmt.Sprintf("%s/%s", runnerScaleSetChartRepo, runnerScaleSetChartName)

	chartRef, err := client.ChartPathOptions.LocateChart(chartPath, cli.New())
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}

	chart, err := loader.Load(chartRef)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Load values from file
	vals := map[string]interface{}{}
	if valuesContent, err := os.ReadFile(valuesPath); err == nil {
		// Parse YAML values
		if err := yaml.Unmarshal(valuesContent, &vals); err != nil {
			return fmt.Errorf("failed to parse values: %w", err)
		}
	}

	_, err = client.Run(chart, vals)
	if err != nil {
		return fmt.Errorf("failed to install runner scale set: %w", err)
	}

	fmt.Printf("  Instance '%s' installed successfully\n", instanceName)
	return nil
}

// Uninstall removes a runner scale set
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	// Uninstall using Helm SDK
	actionConfig, err := m.getHelmConfig(defaultNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm config: %w", err)
	}

	client := action.NewUninstall(actionConfig)
	client.Timeout = 2 * time.Minute

	_, err = client.Run(name)
	if err != nil {
		return fmt.Errorf("failed to uninstall runner: %w", err)
	}

	return nil
}

// List returns all runner scale sets
func (m *Manager) List(ctx context.Context) ([]string, error) {
	// List Helm releases using SDK
	actionConfig, err := m.getHelmConfig(defaultNamespace)
	if err != nil {
		return []string{}, nil // Return empty list if config fails
	}

	client := action.NewList(actionConfig)
	releases, err := client.Run()
	if err != nil {
		// If namespace doesn't exist or no releases, return empty list
		return []string{}, nil
	}

	names := []string{}
	for _, release := range releases {
		// Exclude the ARC controller from the list
		if release.Name != arcControllerRelease {
			names = append(names, release.Name)
		}
	}

	return names, nil
}

// Status returns the status of a runner installation
func (m *Manager) Status(ctx context.Context, name string) (string, error) {
	// Get Helm release status using SDK
	actionConfig, err := m.getHelmConfig(defaultNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to create helm config: %w", err)
	}

	client := action.NewStatus(actionConfig)
	release, err := client.Run(name)
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	statusStr := fmt.Sprintf("NAME: %s\nNAMESPACE: %s\nSTATUS: %s\nREVISION: %d\n",
		release.Name, release.Namespace, release.Info.Status, release.Version)

	// Also get the AutoscalingRunnerSet status
	cmd := exec.CommandContext(ctx, "kubectl", "get", "autoscalingrunnersets", name,
		"-n", defaultNamespace, "--context", m.clusterManager.GetKubeconfig(),
		"-o", "wide")

	k8sOutput, err := cmd.Output()
	if err != nil {
		// Just show Helm status if kubectl fails
		return statusStr, nil
	}

	return fmt.Sprintf("%s\nKubernetes Resources:\n%s", statusStr, string(k8sOutput)), nil
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
func (m *Manager) generateHelmValues(installation *types.RunnerInstallation, instanceName string, instanceNum int) (string, error) {
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
		containerModeConfig = m.generatePrivilegedContainerMode(installation, instanceNum)
	case types.ContainerModeDinD:
		containerModeConfig = `containerMode:
  type: "dind"`
	default:
		return "", fmt.Errorf("unsupported container mode: %s", installation.ContainerMode)
	}

	// For repository-level runners, use the default runner group
	// For multi-instance setups, all instances share the same base runner group
	runnerGroupConfig := ""
	if installation.Instances > 1 {
		runnerGroupConfig = fmt.Sprintf(`runnerGroup: "%s"`, installation.Name)
	}

	// Add runner labels so workflows can target these runners
	// Use the instance name to ensure unique ServiceAccount names across instances
	runnerLabels := fmt.Sprintf(`runnerScaleSetName: "%s"`, instanceName)

	values := fmt.Sprintf(`githubConfigUrl: "%s"
minRunners: %d
maxRunners: %d
%s
%s

%s

%s
`, installation.Repository, installation.MinRunners, installation.MaxRunners,
		runnerGroupConfig, githubConfigSecret, runnerLabels, containerModeConfig)

	return values, nil
}

// generatePrivilegedContainerMode generates the containerMode configuration for privileged kubernetes mode
// using ARC's hook extension pattern to inject privileged context into job containers only
func (m *Manager) generatePrivilegedContainerMode(installation *types.RunnerInstallation, instanceNum int) string {
	config := `containerMode:
  type: "kubernetes"
template:
  spec:
    securityContext:
      runAsUser: 1001
      runAsGroup: 1001
      fsGroup: 1001
    containers:
    - name: runner
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: false
        runAsNonRoot: true
        runAsUser: 1001
        runAsGroup: 1001
      env:
      - name: ACTIONS_RUNNER_CONTAINER_HOOKS
        value: "/home/runner/k8s/index.js"
      - name: ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE
        value: "/etc/hooks/hook-extension.yaml"
      - name: ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER
        value: "false"`

	// Add volume mounts for work directory and cache paths
	config += "\n      volumeMounts:"
	config += "\n      - name: work"
	config += "\n        mountPath: /home/runner/_work"
	config += "\n      - name: hook-extension"
	config += "\n        mountPath: /etc/hooks"
	config += "\n        readOnly: true"

	if len(installation.CachePaths) > 0 {
		for i, path := range installation.CachePaths {
			config += fmt.Sprintf("\n      - name: cache-%d", i)
			config += fmt.Sprintf("\n        mountPath: %s", path.MountPath)
		}
	}

	// Define volumes
	config += "\n    volumes:"
	config += "\n    - name: work"
	config += "\n      emptyDir: {}"
	config += "\n    - name: hook-extension"
	config += "\n      configMap:"
	config += "\n        name: privileged-hook-extension"
	config += "\n        defaultMode: 0755"

	if len(installation.CachePaths) > 0 {
		for i, path := range installation.CachePaths {
			hostPath := path.HostPath
			if hostPath == "" {
				// Generate instance-specific cache path for multi-instance setups
				if instanceNum > 0 {
					hostPath = fmt.Sprintf("/tmp/github-runner-cache/%s-%d/cache-%d", installation.Name, instanceNum, i)
				} else {
					hostPath = fmt.Sprintf("/tmp/github-runner-cache/%s/cache-%d", installation.Name, i)
				}
			}
			config += fmt.Sprintf("\n    - name: cache-%d", i)
			config += "\n      hostPath:"
			config += fmt.Sprintf("\n        path: %s", hostPath)
			config += "\n        type: DirectoryOrCreate"
		}
	}

	return config
}

// generateHookExtensionConfigMap generates the hook extension ConfigMap YAML for privileged container mode
// This ConfigMap contains the PodSpec patch that ARC applies to job containers
func (m *Manager) generateHookExtensionConfigMap(installation *types.RunnerInstallation, instanceName string) string {
	// Build the PodSpec patch that will be applied to job containers
	// This patch adds privileged context and capabilities only to job containers
	hookExtension := `apiVersion: v1
kind: ConfigMap
metadata:
  name: privileged-hook-extension
  namespace: ` + defaultNamespace + `
data:
  hook-extension.yaml: |
    version: 1
    container:
      image: "{{ $.job.container.image }}"
      securityContext:
        privileged: true
        runAsUser: 0
        runAsGroup: 0
        allowPrivilegeEscalation: true
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
      volumeMounts:
        - name: work
          mountPath: /home/runner/_work
        - name: sys
          mountPath: /sys
        - name: cgroup
          mountPath: /sys/fs/cgroup
        - name: cgroup2
          mountPath: /sys/fs/cgroup2
        - name: proc
          mountPath: /proc
        - name: dev
          mountPath: /dev
        - name: dev-pts
          mountPath: /dev/pts
        - name: shm
          mountPath: /dev/shm`

	// Add cache path volume mounts
	if len(installation.CachePaths) > 0 {
		for _, path := range installation.CachePaths {
			hookExtension += fmt.Sprintf("\n        - name: cache-%s\n          mountPath: %s",
				sanitizeVolumeName(path.MountPath), path.MountPath)
		}
	}

	// Add volume definitions
	hookExtension += `
    volumes:
      - name: work
        emptyDir: {}
      - name: sys
        hostPath:
          path: /sys
          type: Directory
      - name: cgroup
        hostPath:
          path: /sys/fs/cgroup
          type: Directory
      - name: cgroup2
        hostPath:
          path: /sys/fs/cgroup2
          type: Directory
      - name: proc
        hostPath:
          path: /proc
          type: Directory
      - name: dev
        hostPath:
          path: /dev
          type: Directory
      - name: dev-pts
        hostPath:
          path: /dev/pts
          type: Directory
      - name: shm
        hostPath:
          path: /dev/shm
          type: Directory`

	// Add cache path volumes
	if len(installation.CachePaths) > 0 {
		for _, path := range installation.CachePaths {
			hookExtension += fmt.Sprintf("\n      - name: cache-%s\n        hostPath:\n          path: %s\n          type: DirectoryOrCreate",
				sanitizeVolumeName(path.MountPath), path.HostPath)
		}
	}

	return hookExtension
}

// sanitizeVolumeName sanitutes a mount path to a valid Kubernetes volume name
func sanitizeVolumeName(path string) string {
	// Replace forward slashes and dots with hyphens, convert to lowercase
	name := strings.ToLower(strings.Trim(path, "/"))
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, ".", "-")
	return name
}
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

	// Install controller using Helm SDK
	actionConfig, err := m.getHelmConfig(arcControllerNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm config: %w", err)
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = arcControllerRelease
	client.Namespace = arcControllerNamespace
	client.CreateNamespace = true
	client.Wait = true
	client.Timeout = 5 * time.Minute

	// Locate and load the chart from OCI registry
	chartPath := fmt.Sprintf("%s/%s", arcControllerChartRepo, arcControllerChartName)
	chartRef, err := client.ChartPathOptions.LocateChart(chartPath, cli.New())
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}

	chart, err := loader.Load(chartRef)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Install with empty values
	_, err = client.Run(chart, nil)
	if err != nil {
		// Check if already installed
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "cannot re-use") {
			fmt.Println("Controller already installed")
			return nil
		}
		return fmt.Errorf("failed to install ARC controller: %w", err)
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
