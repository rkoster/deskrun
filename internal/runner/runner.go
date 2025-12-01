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

	"carvel.dev/kapp/pkg/kapp/cmd"
	"github.com/cppforlife/go-cli-ui/ui"
	"github.com/rkoster/deskrun/arcembedded"
	"github.com/rkoster/deskrun/internal/cluster"
	deskruntypes "github.com/rkoster/deskrun/pkg/types"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultNamespace       = "arc-systems"
	arcControllerNamespace = "arc-systems"
	arcControllerAppName   = "arc-controller"
	// kapp app naming for runner scale sets
	kappAppLabelKey = "kapp.k14s.io/app"
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

// applyConfigMapFromYAML applies a ConfigMap from YAML string
func (m *Manager) applyConfigMapFromYAML(ctx context.Context, yamlContent string) error {
	clientset, err := m.getKubernetesClient()
	if err != nil {
		return err
	}

	// Use Kubernetes deserializer to properly parse the YAML
	decode := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer().Decode
	obj, _, err := decode([]byte(yamlContent), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to decode ConfigMap YAML: %w", err)
	}

	// Type assert to ConfigMap
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return fmt.Errorf("decoded object is not a ConfigMap, got %T", obj)
	}

	// Ensure namespace is set (fallback to default if not parsed correctly)
	if configMap.Namespace == "" {
		configMap.Namespace = defaultNamespace
	}

	// Ensure name is set
	if configMap.Name == "" {
		return fmt.Errorf("ConfigMap name is empty after parsing YAML")
	}

	// Try to create or update the ConfigMap
	_, err = clientset.CoreV1().ConfigMaps(configMap.Namespace).Get(ctx, configMap.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Create the ConfigMap
			_, err = clientset.CoreV1().ConfigMaps(configMap.Namespace).Create(ctx, configMap, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create ConfigMap: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get ConfigMap: %w", err)
		}
	} else {
		// Update the ConfigMap
		_, err = clientset.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ConfigMap: %w", err)
		}
	}

	return nil
}

// applyManifestFromHelm applies a Helm-generated manifest
func (m *Manager) applyManifestFromHelm(ctx context.Context, manifestData string) error {
	dynamicClient, err := m.getDynamicClient()
	if err != nil {
		return err
	}

	// Split the manifest by "---" to handle multiple resources
	resources := strings.Split(manifestData, "---")
	appliedCount := 0
	for _, resource := range resources {
		resource = strings.TrimSpace(resource)
		if resource == "" {
			continue
		}

		// Parse the resource
		var obj unstructured.Unstructured
		if err := yaml.Unmarshal([]byte(resource), &obj); err != nil {
			return fmt.Errorf("failed to unmarshal resource: %w\nResource:\n%s", err, resource)
		}

		if obj.GetKind() == "" {
			continue // Skip empty resources
		}
		appliedCount++

		// Get the GVR for the resource
		gvr := schema.GroupVersionResource{
			Group:    obj.GroupVersionKind().Group,
			Version:  obj.GroupVersionKind().Version,
			Resource: strings.ToLower(obj.GetKind()) + "s",
		}

		// Apply the resource
		_, err = dynamicClient.Resource(gvr).Namespace(obj.GetNamespace()).Create(ctx, &obj, metav1.CreateOptions{})
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			// Try to update if creation failed
			_, err = dynamicClient.Resource(gvr).Namespace(obj.GetNamespace()).Update(ctx, &obj, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetKind(), obj.GetName(), err)
			}
		}
	}

	if appliedCount == 0 {
		return fmt.Errorf("no resources were applied from manifest")
	}

	return nil
}

// patchAutoscalingRunnerSet patches an AutoscalingRunnerSet resource
func (m *Manager) patchAutoscalingRunnerSet(ctx context.Context, namespace, name string, minRunners, maxRunners int) error {
	dynamicClient, err := m.getDynamicClient()
	if err != nil {
		return err
	}

	// Define the GVR for AutoscalingRunnerSet
	gvr := schema.GroupVersionResource{
		Group:    "actions.github.com",
		Version:  "v1alpha1",
		Resource: "autoscalingrunnersets",
	}

	// Create the patch
	patchData := []byte(fmt.Sprintf(`{"spec":{"minRunners":%d,"maxRunners":%d}}`, minRunners, maxRunners))

	// Apply the patch
	_, err = dynamicClient.Resource(gvr).Namespace(namespace).Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch AutoscalingRunnerSet: %w", err)
	}

	return nil
}

// getAutoscalingRunnerSetStatus gets the status of an AutoscalingRunnerSet
func (m *Manager) getAutoscalingRunnerSetStatus(ctx context.Context, namespace, name string) (string, error) {
	dynamicClient, err := m.getDynamicClient()
	if err != nil {
		return "", err
	}

	// Define the GVR for AutoscalingRunnerSet
	gvr := schema.GroupVersionResource{
		Group:    "actions.github.com",
		Version:  "v1alpha1",
		Resource: "autoscalingrunnersets",
	}

	// Get the resource
	obj, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get AutoscalingRunnerSet: %w", err)
	}

	// Format the output
	status := fmt.Sprintf("NAME: %s\nNAMESPACE: %s\n", obj.GetName(), obj.GetNamespace())
	if spec, found, _ := unstructured.NestedMap(obj.Object, "spec"); found {
		if minRunners, found, _ := unstructured.NestedInt64(spec, "minRunners"); found {
			status += fmt.Sprintf("MIN_RUNNERS: %d\n", minRunners)
		}
		if maxRunners, found, _ := unstructured.NestedInt64(spec, "maxRunners"); found {
			status += fmt.Sprintf("MAX_RUNNERS: %d\n", maxRunners)
		}
	}

	return status, nil
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

// executeYTT executes ytt CLI to process templates with data values
func (m *Manager) executeYTT(templateDir string, dataValuesPath string) (string, error) {
	cmd := exec.Command("ytt",
		"-f", templateDir,
		"--data-values-file", dataValuesPath,
	)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ytt failed: %w\nstderr: %s", err, stderr.String())
	}
	
	return stdout.String(), nil
}

// executeKappDeploy deploys resources using kapp as a Go package
func (m *Manager) executeKappDeploy(appName string, manifestPath string) error {
	kubeconfig := m.clusterManager.GetKubeconfig()
	
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()
	
	// Create the kapp command
	kappCmd := cmd.NewDefaultKappCmd(confUI)
	
	// Set the command args
	kappCmd.SetArgs([]string{
		"deploy",
		"-a", appName,
		"-f", manifestPath,
		"--kubeconfig-context", kubeconfig,
		"-n", defaultNamespace,
		"-y", // auto-confirm
	})
	
	// Capture output
	kappCmd.SetOut(&outBuf)
	kappCmd.SetErr(&errBuf)
	
	// Execute the command
	if err := kappCmd.Execute(); err != nil {
		return fmt.Errorf("kapp deploy failed: %w\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}
	
	return nil
}

// executeKappDelete deletes an app using kapp as a Go package
func (m *Manager) executeKappDelete(appName string) error {
	kubeconfig := m.clusterManager.GetKubeconfig()
	
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()
	
	// Create the kapp command
	kappCmd := cmd.NewDefaultKappCmd(confUI)
	
	// Set the command args
	kappCmd.SetArgs([]string{
		"delete",
		"-a", appName,
		"--kubeconfig-context", kubeconfig,
		"-n", defaultNamespace,
		"-y", // auto-confirm
	})
	
	// Capture output
	kappCmd.SetOut(&outBuf)
	kappCmd.SetErr(&errBuf)
	
	// Execute the command
	if err := kappCmd.Execute(); err != nil {
		return fmt.Errorf("kapp delete failed: %w\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}
	
	return nil
}

// executeKappList lists all kapp apps using kapp as a Go package
func (m *Manager) executeKappList() ([]string, error) {
	kubeconfig := m.clusterManager.GetKubeconfig()
	
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()
	
	// Create the kapp command
	kappCmd := cmd.NewDefaultKappCmd(confUI)
	
	// Set the command args
	kappCmd.SetArgs([]string{
		"list",
		"--kubeconfig-context", kubeconfig,
		"-n", defaultNamespace,
		"--json",
	})
	
	// Capture output
	kappCmd.SetOut(&outBuf)
	kappCmd.SetErr(&errBuf)
	
	// Execute the command
	if err := kappCmd.Execute(); err != nil {
		// If namespace doesn't exist, return empty list
		if strings.Contains(errBuf.String(), "not found") {
			return []string{}, nil
		}
		return nil, fmt.Errorf("kapp list failed: %w\nstderr: %s", err, errBuf.String())
	}
	
	// Parse JSON output to extract app names
	var result struct {
		Tables []struct {
			Rows []struct {
				Name string `yaml:"name"`
			} `yaml:"rows"`
		} `yaml:"tables"`
	}
	
	if err := yaml.Unmarshal(outBuf.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse kapp list output: %w", err)
	}
	
	var names []string
	if len(result.Tables) > 0 {
		for _, row := range result.Tables[0].Rows {
			// Exclude the ARC controller from the list
			if row.Name != arcControllerAppName {
				names = append(names, row.Name)
			}
		}
	}
	
	return names, nil
}

// executeKappInspect inspects a kapp app using kapp as a Go package
func (m *Manager) executeKappInspect(appName string) (string, error) {
	kubeconfig := m.clusterManager.GetKubeconfig()
	
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()
	
	// Create the kapp command
	kappCmd := cmd.NewDefaultKappCmd(confUI)
	
	// Set the command args
	kappCmd.SetArgs([]string{
		"inspect",
		"-a", appName,
		"--kubeconfig-context", kubeconfig,
		"-n", defaultNamespace,
		"--json",
	})
	
	// Capture output
	kappCmd.SetOut(&outBuf)
	kappCmd.SetErr(&errBuf)
	
	// Execute the command
	if err := kappCmd.Execute(); err != nil {
		return "", fmt.Errorf("kapp inspect failed: %w\nstderr: %s", err, errBuf.String())
	}
	
	return outBuf.String(), nil
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

// installInstance installs a single runner scale set instance
func (m *Manager) installInstance(ctx context.Context, installation *deskruntypes.RunnerInstallation, instanceName string, instanceNum int) error {
	// Create temporary directory for templates and manifests
	tmpDir, err := os.MkdirTemp("/tmp", "deskrun-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("  Installing runner scale set '%s'...\n", instanceName)

	// Write embedded templates to temp directory
	templateDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return fmt.Errorf("failed to create template dir: %w", err)
	}
	
	templateFiles, err := arcembedded.GetTemplateFiles()
	if err != nil {
		return fmt.Errorf("failed to get embedded templates: %w", err)
	}
	for filename, content := range templateFiles {
		filePath := filepath.Join(templateDir, filename)
		fileDir := filepath.Dir(filePath)
		if err := os.MkdirAll(fileDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", filename, err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write template file %s: %w", filename, err)
		}
	}

	// For privileged container mode, generate and apply the hook extension ConfigMap
	if installation.ContainerMode == deskruntypes.ContainerModePrivileged {
		fmt.Printf("  Applying hook extension ConfigMap for privileged mode...\n")
		hookExtension := m.generateHookExtensionConfigMap(installation, instanceName, instanceNum)
		
		// Apply the ConfigMap using Kubernetes client
		if err := m.applyConfigMapFromYAML(ctx, hookExtension); err != nil {
			return fmt.Errorf("failed to apply hook extension ConfigMap: %w", err)
		}
	}

	// Generate ytt data values
	dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")
	dataValuesContent, err := m.generateYTTDataValues(installation, instanceName, instanceNum)
	if err != nil {
		return fmt.Errorf("failed to generate ytt data values: %w", err)
	}

	if err := os.WriteFile(dataValuesPath, []byte(dataValuesContent), 0644); err != nil {
		return fmt.Errorf("failed to write ytt data values: %w", err)
	}

	// Execute ytt to process templates
	processedYAML, err := m.executeYTT(templateDir, dataValuesPath)
	if err != nil {
		return fmt.Errorf("failed to process templates with ytt: %w", err)
	}

	// Write processed YAML to file for kapp
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(processedYAML), 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Deploy using kapp
	appName := instanceName
	if err := m.executeKappDeploy(appName, manifestPath); err != nil {
		return fmt.Errorf("failed to deploy with kapp: %w", err)
	}

	// Wait for the AutoscalingRunnerSet CRD to be available
	if err := m.waitForCRD(ctx, "autoscalingrunnersets.actions.github.com"); err != nil {
		return fmt.Errorf("failed to wait for ARS CRD: %w", err)
	}

	fmt.Printf("  Instance '%s' installed successfully\n", instanceName)
	return nil
}

// Uninstall removes a runner scale set
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	// Uninstall using kapp delete
	if err := m.executeKappDelete(name); err != nil {
		return fmt.Errorf("failed to uninstall runner: %w", err)
	}

	return nil
}

// List returns all runner scale sets
func (m *Manager) List(ctx context.Context) ([]string, error) {
	// List kapp apps
	names, err := m.executeKappList()
	if err != nil {
		// If namespace doesn't exist, return empty list
		return []string{}, nil
	}

	return names, nil
}

// Status returns the status of a runner installation
func (m *Manager) Status(ctx context.Context, name string) (string, error) {
	// Get kapp app status
	inspectOutput, err := m.executeKappInspect(name)
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	statusStr := fmt.Sprintf("NAME: %s\nNAMESPACE: %s\nMANAGED BY: kapp\n\n", name, defaultNamespace)

	// Also get the AutoscalingRunnerSet status
	arsStatus, err := m.getAutoscalingRunnerSetStatus(ctx, defaultNamespace, name)
	if err != nil {
		// Just show kapp status if getting ARS status fails
		return statusStr + inspectOutput, nil
	}

	return fmt.Sprintf("%s\nKubernetes Resources:\n%s\n\nkapp Details:\n%s", statusStr, arsStatus, inspectOutput), nil
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

func (m *Manager) applyManifest(ctx context.Context, manifestPath string) error {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	return m.applyManifestFromHelm(ctx, string(content))
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
	case deskruntypes.ContainerModePrivileged:
		// For privileged mode, we need to parse the generated YAML string into a map
		containerModeYAML := m.generatePrivilegedContainerMode(installation, instanceName, instanceNum)
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
	case deskruntypes.ContainerModeDinD:
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
func (m *Manager) generatePrivilegedContainerMode(installation *deskruntypes.RunnerInstallation, instanceName string, instanceNum int) string {
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

	// Add volume mounts for hook extension only
	// Cache volumes are only needed for job containers and are defined in the hook extension
	config += "\n      volumeMounts:"
	config += "\n      - name: hook-extension"
	config += "\n        mountPath: /etc/hooks"
	config += "\n        readOnly: true"

	// Define only the hook-extension volume
	// Cache volumes are only needed for job containers and are defined in the hook extension
	configMapName := fmt.Sprintf("privileged-hook-extension-%s", instanceName)
	config += "\n    volumes:"
	config += "\n    - name: hook-extension"
	config += "\n      configMap:"
	config += fmt.Sprintf("\n        name: %s", configMapName)
	config += "\n        defaultMode: 0755"

	return config
}

// generateHookExtensionConfigMap generates the hook extension ConfigMap YAML for privileged container mode
// This ConfigMap contains the PodSpec patch that ARC applies to job containers
// IMPORTANT: The hook extension should ONLY patch the job container with security context and privileged mounts.
// It should NOT redefine volumes that are already in the runner template (like "work").
// The hook extension is a JSON patch that gets merged with the existing pod spec.
func (m *Manager) generateHookExtensionConfigMap(installation *deskruntypes.RunnerInstallation, instanceName string, instanceNum int) string {
	// Build the PodSpec patch that will be applied to job pods
	// This patch adds privileged context and capabilities only to job containers
	// The "$job" placeholder targets the job container created by the runner
	// NOTE: We do NOT include "work" volume or "cgroup2" (which may not exist) in the patch
	configMapName := fmt.Sprintf("privileged-hook-extension-%s", instanceName)
	hookExtension := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s`, configMapName, defaultNamespace) + `
data:
  content: |
    spec:
      hostPID: true
      hostIPC: true
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

	// Add cache path volume mounts to job container
	if len(installation.CachePaths) > 0 {
		for i := range installation.CachePaths {
			hookExtension += fmt.Sprintf("\n        - name: cache-%d\n          mountPath: %s",
				i, installation.CachePaths[i].Target)
		}
	}

	// Add volume definitions (system mounts + cache volumes needed by job container)
	// Cache volumes are only needed for job containers, not the runner container
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

	// Add cache path volumes to hook extension
	if len(installation.CachePaths) > 0 {
		for i, path := range installation.CachePaths {
			hostPath := path.Source
			if hostPath == "" {
				// Generate instance-specific cache path for multi-instance setups
				if instanceNum > 0 {
					hostPath = fmt.Sprintf("/tmp/github-runner-cache/%s-%d/cache-%d", installation.Name, instanceNum, i)
				} else {
					hostPath = fmt.Sprintf("/tmp/github-runner-cache/%s/cache-%d", installation.Name, i)
				}
			}
			hookExtension += fmt.Sprintf("\n      - name: cache-%d\n        hostPath:\n          path: %s\n          type: DirectoryOrCreate",
				i, hostPath)
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
	defer os.RemoveAll(tmpDir)

	// Write embedded controller templates to temp directory
	templateDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return fmt.Errorf("failed to create template dir: %w", err)
	}
	
	// For the controller, we only need the controller chart rendered YAML
	controllerYAML, err := arcembedded.GetControllerChart()
	if err != nil {
		return fmt.Errorf("failed to get controller chart: %w", err)
	}
	controllerPath := filepath.Join(templateDir, "controller.yaml")
	if err := os.WriteFile(controllerPath, []byte(controllerYAML), 0644); err != nil {
		return fmt.Errorf("failed to write controller template: %w", err)
	}

	// Deploy controller using kapp (no ytt processing needed for controller - it's pre-rendered)
	appName := arcControllerAppName
	if err := m.executeKappDeploy(appName, controllerPath); err != nil {
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
