package runner

import (
	"bytes"
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

	// Ensure the ARS resource is created by applying the manifest via kubectl
	// The Helm SDK doesn't always immediately apply resources, so we ensure they're created
	getManifest := exec.CommandContext(ctx, "helm", "get", "manifest", instanceName,
		"-n", defaultNamespace)
	manifestData, err := getManifest.Output()
	if err != nil {
		return fmt.Errorf("failed to get helm manifest: %w", err)
	}

	applyCmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-",
		"--context", m.clusterManager.GetKubeconfig())
	applyCmd.Stdin = bytes.NewReader(manifestData)
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply ARS manifest: %w", err)
	}

	// Patch the ARS resource to set minRunners and maxRunners
	// The Helm chart doesn't expose these values, so we need to patch after install
	patchJSON := fmt.Sprintf(`{"spec":{"minRunners":%d,"maxRunners":%d}}`,
		installation.MinRunners, installation.MaxRunners)
	patchCmd := exec.CommandContext(ctx, "kubectl", "patch", "autoscalingrunnersets",
		"-n", defaultNamespace, instanceName, "--type", "merge", "-p", patchJSON,
		"--context", m.clusterManager.GetKubeconfig())
	if err := patchCmd.Run(); err != nil {
		return fmt.Errorf("failed to patch ARS minRunners/maxRunners: %w", err)
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
	// Build the values map
	values := map[string]interface{}{
		"githubConfigUrl":    installation.Repository,
		"minRunners":         installation.MinRunners,
		"maxRunners":         installation.MaxRunners,
		"runnerScaleSetName": instanceName,
		// Note: GitHub automatically assigns the "self-hosted" label to all self-hosted runners,
		// including ephemeral runners created by ARC. This happens on the GitHub side and doesn't
		// need to be explicitly configured. The ARC Helm chart does not support the runnerLabels field,
		// so we don't add it here.
	}

	// Determine authentication method
	if installation.AuthType == types.AuthTypePAT {
		values["githubConfigSecret"] = map[string]interface{}{
			"github_token": installation.AuthValue,
		}
	} else {
		values["githubConfigSecret"] = map[string]interface{}{
			"github_app_id":              "",
			"github_app_installation_id": "",
			"github_app_private_key":     installation.AuthValue,
		}
	}

	// Build container mode configuration
	var containerModeConfig map[string]interface{}
	switch installation.ContainerMode {
	case types.ContainerModeKubernetes:
		// Standard kubernetes mode with simple template (no hook extension)
		containerModeConfig = map[string]interface{}{
			"type": "kubernetes",
			// Add storage configuration for kubernetes mode - must be inside containerMode
			"kubernetesModeWorkVolumeClaim": map[string]interface{}{
				"accessModes":      []string{"ReadWriteOnce"},
				"storageClassName": "standard",
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"storage": "1Gi",
					},
				},
			},
		}
		// Add simple template without hook extension
		values["template"] = map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []map[string]interface{}{
					{
						"name":    "runner",
						"image":   "ghcr.io/actions/actions-runner:latest",
						"command": []string{"/home/runner/run.sh"},
						"env": []map[string]interface{}{
							{
								"name":  "ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER",
								"value": "true",
							},
						},
					},
				},
			},
		}
	case types.ContainerModePrivileged:
		// For privileged mode, we need to parse the generated YAML string into a map
		containerModeYAML := m.generatePrivilegedContainerMode(installation, instanceNum)
		// The function returns the full "containerMode: ..." structure, so parse it into a temp object
		tempConfig := make(map[string]interface{})
		if err := yaml.Unmarshal([]byte(containerModeYAML), &tempConfig); err != nil {
			return "", fmt.Errorf("failed to parse privileged container mode YAML: %w", err)
		}
		// Extract the containerMode value from the parsed YAML
		if cm, ok := tempConfig["containerMode"]; ok {
			if cmMap, ok := cm.(map[string]interface{}); ok {
				containerModeConfig = cmMap
			} else {
				return "", fmt.Errorf("containerMode is not a map: %T", cm)
			}
		} else {
			return "", fmt.Errorf("containerMode not found in generated YAML")
		}
		// Also add template and other top-level fields from the privileged config
		if template, ok := tempConfig["template"]; ok {
			// For privileged mode, we need to merge both containerMode and template at the top level
			// Reconstruct containerModeConfig to include the template
			if tempCM, ok := tempConfig["containerMode"].(map[string]interface{}); ok {
				containerModeConfig = tempCM
			}
			values["template"] = template
		}
	case types.ContainerModeDinD:
		containerModeConfig = map[string]interface{}{
			"type": "dind",
		}
	default:
		return "", fmt.Errorf("unsupported container mode: %s", installation.ContainerMode)
	}
	values["containerMode"] = containerModeConfig

	// Convert to YAML
	yamlData, err := yaml.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to marshal values to YAML: %w", err)
	}

	return string(yamlData), nil
}

// generatePrivilegedContainerMode generates the containerMode configuration for privileged kubernetes mode
// using ARC's hook extension pattern to inject privileged context into job containers only.
// Uses kubernetes-novolume mode to avoid PVC complications - ephemeral storage is handled by the hook extension.
func (m *Manager) generatePrivilegedContainerMode(installation *types.RunnerInstallation, instanceNum int) string {
	config := `containerMode:
  type: "kubernetes-novolume"
template:
  spec:
    securityContext:
      runAsUser: 1001
      runAsGroup: 1001
      fsGroup: 1001
    containers:
    - name: runner
      image: "ghcr.io/actions/actions-runner:latest"
      command: ["/home/runner/run.sh"]
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: false
        runAsNonRoot: true
        runAsUser: 1001
        runAsGroup: 1001
      env:
      - name: ACTIONS_RUNNER_CONTAINER_HOOKS
        value: "/home/runner/k8s-novolume/index.js"
      - name: ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE
        value: "/etc/hooks/content"
      - name: ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER
        value: "false"`

	// Add volume mounts for hook extension
	config += "\n      volumeMounts:"
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
// IMPORTANT: The hook extension should ONLY patch the job container with security context and privileged mounts.
// It should NOT redefine volumes that are already in the runner template (like "work").
// The hook extension is a JSON patch that gets merged with the existing pod spec.
func (m *Manager) generateHookExtensionConfigMap(installation *types.RunnerInstallation, instanceName string) string {
	// Build the PodSpec patch that will be applied to job pods
	// This patch adds privileged context and capabilities only to job containers
	// The "$job" placeholder targets the job container created by the runner
	// NOTE: We do NOT include "work" volume or "cgroup2" (which may not exist) in the patch
	hookExtension := `apiVersion: v1
kind: ConfigMap
metadata:
  name: privileged-hook-extension
  namespace: ` + defaultNamespace + `
data:
  content: |
     spec:
       securityContext:
         runAsUser: 0
         runAsGroup: 0
         fsGroup: 0
       containers:
       - name: "$job"
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
         - name: sys
           mountPath: /sys
         - name: cgroup
           mountPath: /sys/fs/cgroup
           mountPropagation: Bidirectional
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

	// Add volume definitions (only for host path mounts, not work which is already in runner template)
	hookExtension += `
       volumes:
       - name: sys
         hostPath:
           path: /sys
           type: Directory
       - name: cgroup
         hostPath:
           path: /sys/fs/cgroup
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
