package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rkoster/deskrun/internal/cluster"
	"github.com/rkoster/deskrun/internal/kapp"
	"github.com/rkoster/deskrun/pkg/templates"
	deskruntypes "github.com/rkoster/deskrun/pkg/types"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultNamespace       = "arc-systems"
	arcControllerNamespace = "arc-systems"
	arcControllerAppName   = "arc-controller"
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

// getKappClient returns a kapp client configured for the current cluster
func (m *Manager) getKappClient() *kapp.Client {
	return kapp.NewClient(m.clusterManager.GetKubeconfig(), defaultNamespace)
}

// customWarningHandler is a warning handler that filters out unrecognized format warnings
// It implements the rest.WarningHandler interface
type customWarningHandler struct{}

func (h customWarningHandler) HandleWarningHeader(code int, agent string, text string) {
	// Filter out unrecognized format warnings to reduce noise
	if strings.Contains(text, "unrecognized format") &&
		(strings.Contains(text, "int32") || strings.Contains(text, "int64")) {
		return // Skip these warnings
	}
	// For other warnings, print them normally (this mimics the default behavior)
	fmt.Printf("Warning: %s\n", text)
}

// getKubernetesClient creates a Kubernetes clientset
func (m *Manager) getKubernetesClient() (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: m.clusterManager.GetKubeconfig(),
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Set custom warning handler to filter out unrecognized format warnings
	// This cast ensures we use the rest package
	config.WarningHandler = rest.WarningHandler(customWarningHandler{})

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset, nil
}

// getDynamicClient creates a dynamic Kubernetes client
func (m *Manager) getDynamicClient() (dynamic.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: m.clusterManager.GetKubeconfig(),
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Set custom warning handler to filter out unrecognized format warnings
	config.WarningHandler = rest.WarningHandler(customWarningHandler{})

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return dynamicClient, nil
}

// crdExists checks if a CRD exists
func (m *Manager) crdExists(ctx context.Context, crdName string) (bool, error) {
	dynamicClient, err := m.getDynamicClient()
	if err != nil {
		return false, err
	}

	// Define the GVR for CRDs
	gvr := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	// Try to get the CRD
	_, err = dynamicClient.Resource(gvr).Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// waitForCRD waits for a CRD to be established
func (m *Manager) waitForCRD(ctx context.Context, crdName string) error {
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		exists, err := m.crdExists(ctx, crdName)
		if err != nil {
			return false, err
		}
		return exists, nil
	})
}

// Install installs a runner scale set
func (m *Manager) Install(ctx context.Context, installation *deskruntypes.RunnerInstallation) error {
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

// installInstance installs a single runner scale set instance using the unified template processing package
func (m *Manager) installInstance(ctx context.Context, installation *deskruntypes.RunnerInstallation, instanceName string, instanceNum int) error {
	// Create temporary directory for manifests
	tmpDir, err := os.MkdirTemp("/tmp", "deskrun-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	fmt.Printf("  Installing runner scale set '%s'...\n", instanceName)

	// Use the unified template processing package (ytt Go library, no shell execution)
	processor := templates.NewProcessor()
	config := templates.Config{
		Installation: installation,
		InstanceName: instanceName,
		InstanceNum:  instanceNum,
		Namespace:    defaultNamespace,
	}

	processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
	if err != nil {
		// Check if it's a TemplateError with verbose information
		if templateErr, ok := err.(*templates.TemplateError); ok {
			return fmt.Errorf("failed to process template: %s", templateErr.VerboseError())
		}
		return fmt.Errorf("failed to process template: %w", err)
	}

	// Write processed YAML to file for kapp
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, processedYAML, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Deploy using kapp
	kappClient := m.getKappClient()
	appName := instanceName
	if err := kappClient.Deploy(appName, manifestPath); err != nil {
		return fmt.Errorf("failed to deploy with kapp: %w", err)
	}

	fmt.Printf("  Instance '%s' installed successfully\n", instanceName)
	return nil
}

// Uninstall removes a runner scale set
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	// Uninstall using kapp delete
	kappClient := m.getKappClient()
	if err := kappClient.Delete(name); err != nil {
		return fmt.Errorf("failed to uninstall runner: %w", err)
	}

	return nil
}

// List returns all runner scale sets
func (m *Manager) List(ctx context.Context) ([]string, error) {
	// List kapp apps since the status command uses kapp inspect
	kappClient := m.getKappClient()
	appNames, err := kappClient.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list kapp apps: %w", err)
	}

	// Filter out the controller app to only show runner apps
	var runnerNames []string
	for _, name := range appNames {
		if name != arcControllerAppName {
			runnerNames = append(runnerNames, name)
		}
	}

	return runnerNames, nil
}

func (m *Manager) createNamespace(ctx context.Context, namespace string) error {
	clientset, err := m.getKubernetesClient()
	if err != nil {
		return err
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	_, err = clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	return nil
}

// generateYTTDataValues generates ytt data values for the runner scale set
func (m *Manager) generateYTTDataValues(installation *deskruntypes.RunnerInstallation, instanceName string, instanceNum int) (string, error) {
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
	if installation.AuthType == deskruntypes.AuthTypePAT {
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
	case deskruntypes.ContainerModeKubernetes:
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
	case deskruntypes.ContainerModeDinD:
		containerModeConfig = map[string]interface{}{
			"type": "dind",
		}
	case deskruntypes.ContainerModePrivileged:
		// cached-privileged-kubernetes mode using kubernetes-novolume
		containerModeConfig = map[string]interface{}{
			"type": "kubernetes-novolume",
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

func (m *Manager) ensureARCController(ctx context.Context) error {
	// Check if CRDs are already installed
	exists, err := m.crdExists(ctx, "autoscalingrunnersets.actions.github.com")
	if err != nil {
		return fmt.Errorf("failed to check CRD: %w", err)
	}
	if exists {
		// CRDs already exist, controller is likely installed
		return nil
	}

	// CRDs don't exist, install the controller
	fmt.Println("Installing GitHub Actions Runner Controller...")

	// Create temporary directory for controller templates
	tmpDir, err := os.MkdirTemp("/tmp", "deskrun-controller-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Get controller template using the unified template package
	// ProcessTemplate applies the overlay which adds required RBAC permissions
	processor := templates.NewProcessor()
	config := templates.Config{
		Installation: &deskruntypes.RunnerInstallation{
			Name:          "arc-controller",
			Repository:    "https://github.com/placeholder",
			ContainerMode: deskruntypes.ContainerModeKubernetes,
		},
		InstanceName: "arc-controller",
		InstanceNum:  1,
	}
	controllerYAML, err := processor.ProcessTemplate(templates.TemplateTypeController, config)
	if err != nil {
		return fmt.Errorf("failed to get controller chart: %w", err)
	}

	// Write to temp file for kapp
	controllerPath := filepath.Join(tmpDir, "controller.yaml")
	if err := os.WriteFile(controllerPath, controllerYAML, 0644); err != nil {
		return fmt.Errorf("failed to write controller template: %w", err)
	}

	// Deploy controller using kapp (no ytt processing needed for controller - it's pre-rendered)
	appName := arcControllerAppName
	kappClient := m.getKappClient()
	if err := kappClient.Deploy(appName, controllerPath); err != nil {
		// Check if already installed
		if strings.Contains(err.Error(), "already exists") {
			fmt.Println("Controller already installed")
			return nil
		}
		return fmt.Errorf("failed to install ARC controller: %w", err)
	}

	fmt.Println("ARC controller installed successfully")

	// Wait for CRDs to be ready
	fmt.Println("Waiting for CRDs to be ready...")
	if err := m.waitForCRD(ctx, "autoscalingrunnersets.actions.github.com"); err != nil {
		return fmt.Errorf("timeout waiting for CRDs to be ready: %w", err)
	}

	fmt.Println("CRDs are ready")
	return nil
}
