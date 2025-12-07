package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gonvenience/ytbx"
	"github.com/homeport/dyff/pkg/dyff"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestMatrix defines comprehensive test cases for all container modes and configurations
var testMatrix = []struct {
	name          string
	templateType  TemplateType
	containerMode types.ContainerMode
	cachePaths    []types.CachePath
	expectedFile  string
}{
	// Controller template test
	{
		name:          "controller-basic",
		templateType:  TemplateTypeController,
		containerMode: types.ContainerModeKubernetes,
		cachePaths:    nil,
		expectedFile:  "controller_basic.yaml",
	},

	// Kubernetes mode tests
	{
		name:          "kubernetes-basic",
		templateType:  TemplateTypeScaleSet,
		containerMode: types.ContainerModeKubernetes,
		cachePaths:    nil,
		expectedFile:  "kubernetes_basic.yaml",
	},
	{
		name:          "kubernetes-with-cache",
		templateType:  TemplateTypeScaleSet,
		containerMode: types.ContainerModeKubernetes,
		cachePaths: []types.CachePath{
			{Source: "/var/lib/docker", Target: "/var/lib/docker"},
		},
		expectedFile: "kubernetes_with_cache.yaml",
	},

	// Docker-in-Docker mode tests
	{
		name:          "dind-basic",
		templateType:  TemplateTypeScaleSet,
		containerMode: types.ContainerModeDinD,
		cachePaths:    nil,
		expectedFile:  "dind_basic.yaml",
	},
	{
		name:          "dind-with-cache",
		templateType:  TemplateTypeScaleSet,
		containerMode: types.ContainerModeDinD,
		cachePaths: []types.CachePath{
			{Source: "/tmp/docker-cache", Target: "/var/lib/docker"},
		},
		expectedFile: "dind_with_cache.yaml",
	},

	// Privileged mode tests
	{
		name:          "privileged-basic",
		templateType:  TemplateTypeScaleSet,
		containerMode: types.ContainerModePrivileged,
		cachePaths:    nil,
		expectedFile:  "privileged_basic.yaml",
	},
	{
		name:          "privileged-single-cache",
		templateType:  TemplateTypeScaleSet,
		containerMode: types.ContainerModePrivileged,
		cachePaths: []types.CachePath{
			{Source: "/var/lib/docker", Target: "/var/lib/docker"},
		},
		expectedFile: "privileged_single_cache.yaml",
	},
	{
		name:          "privileged-multi-cache",
		templateType:  TemplateTypeScaleSet,
		containerMode: types.ContainerModePrivileged,
		cachePaths: []types.CachePath{
			{Source: "/var/lib/docker", Target: "/var/lib/docker"},
			{Source: "/nix/store", Target: "/nix/store"},
		},
		expectedFile: "privileged_multi_cache.yaml",
	},
	{
		name:          "privileged-emptydir-cache",
		templateType:  TemplateTypeScaleSet,
		containerMode: types.ContainerModePrivileged,
		cachePaths: []types.CachePath{
			{Source: "", Target: "/var/lib/docker"}, // Empty source = emptyDir
		},
		expectedFile: "privileged_emptydir_cache.yaml",
	},
}

func TestProcessTemplate(t *testing.T) {
	processor := NewProcessor()

	for _, tc := range testMatrix {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				Installation: &types.RunnerInstallation{
					Name:          "test-runner",
					Repository:    "https://github.com/test/repo",
					AuthValue:     "test-token",
					ContainerMode: tc.containerMode,
					MinRunners:    1,
					MaxRunners:    3,
					CachePaths:    tc.cachePaths,
				},
				InstanceName: "test-runner",
				InstanceNum:  1,
			}

			actualYAML, err := processor.ProcessTemplate(tc.templateType, config)
			require.NoError(t, err, "ProcessTemplate should not return an error")
			require.NotEmpty(t, actualYAML, "Output should not be empty")

			// Compare with expected file
			expectedPath := filepath.Join("testdata", "expected", tc.expectedFile)
			assertYAMLMatchesFile(t, actualYAML, expectedPath)
		})
	}
}

func TestProcessTemplateValidation(t *testing.T) {
	processor := NewProcessor()

	t.Run("nil installation", func(t *testing.T) {
		config := Config{
			Installation: nil,
			InstanceName: "test-runner",
		}
		_, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "installation")
	})

	t.Run("empty instance name", func(t *testing.T) {
		config := Config{
			Installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/test/repo",
				ContainerMode: types.ContainerModeKubernetes,
			},
			InstanceName: "",
		}
		_, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instance name")
	})

	t.Run("empty repository", func(t *testing.T) {
		config := Config{
			Installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "",
				ContainerMode: types.ContainerModeKubernetes,
			},
			InstanceName: "test-runner",
		}
		_, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "repository")
	})

	t.Run("invalid container mode", func(t *testing.T) {
		config := Config{
			Installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/test/repo",
				ContainerMode: types.ContainerMode("invalid"),
			},
			InstanceName: "test-runner",
		}
		_, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "container mode")
	})

	t.Run("invalid template type", func(t *testing.T) {
		config := Config{
			Installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/test/repo",
				ContainerMode: types.ContainerModeKubernetes,
			},
			InstanceName: "test-runner",
		}
		_, err := processor.ProcessTemplate(TemplateType("invalid"), config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown template type")
	})
}

func TestServiceAccountLogic(t *testing.T) {
	processor := NewProcessor()

	testCases := []struct {
		name                  string
		containerMode         types.ContainerMode
		expectedSAType        string
		shouldHaveManagerSA   bool
		shouldHaveManagerRBAC bool
	}{
		{
			// Upstream Helm charts now use kube-mode service account for kubernetes mode
			name:                  "kubernetes-uses-kube-mode",
			containerMode:         types.ContainerModeKubernetes,
			expectedSAType:        "kube-mode",
			shouldHaveManagerSA:   false, // No longer creates separate manager SA
			shouldHaveManagerRBAC: true,  // Manager Role/RoleBinding still exist
		},
		{
			// Upstream Helm charts now use kube-mode service account for privileged mode
			name:                  "privileged-uses-kube-mode",
			containerMode:         types.ContainerModePrivileged,
			expectedSAType:        "kube-mode",
			shouldHaveManagerSA:   false, // No longer creates separate manager SA
			shouldHaveManagerRBAC: true,  // Manager Role/RoleBinding still exist
		},
		{
			name:                  "dind-uses-no-permission",
			containerMode:         types.ContainerModeDinD,
			expectedSAType:        "no-permission",
			shouldHaveManagerSA:   false, // DinD mode uses no-permission SA
			shouldHaveManagerRBAC: true,  // Manager Role/RoleBinding still exist
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				Installation: &types.RunnerInstallation{
					Name:          "test-runner",
					Repository:    "https://github.com/test/repo",
					AuthValue:     "test-token",
					ContainerMode: tc.containerMode,
					MinRunners:    1,
					MaxRunners:    3,
				},
				InstanceName: "test-runner",
				InstanceNum:  1,
			}

			actualYAML, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
			require.NoError(t, err)

			yamlStr := string(actualYAML)

			// Check service account reference in AutoscalingRunnerSet
			expectedSARef := "serviceAccountName: test-runner-gha-rs-" + tc.expectedSAType
			assert.Contains(t, yamlStr, expectedSARef,
				"Expected ServiceAccount reference %s not found", expectedSARef)
		})
	}
}

func TestContainerModeSpecificFeatures(t *testing.T) {
	processor := NewProcessor()

	t.Run("kubernetes-requires-job-container", func(t *testing.T) {
		config := Config{
			Installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/test/repo",
				AuthValue:     "test-token",
				ContainerMode: types.ContainerModeKubernetes,
			},
			InstanceName: "test-runner",
			InstanceNum:  1,
		}

		actualYAML, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
		require.NoError(t, err)

		assert.Contains(t, string(actualYAML), "ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER",
			"Kubernetes mode should set ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER")
	})

	t.Run("dind-has-docker-host", func(t *testing.T) {
		config := Config{
			Installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/test/repo",
				AuthValue:     "test-token",
				ContainerMode: types.ContainerModeDinD,
			},
			InstanceName: "test-runner",
			InstanceNum:  1,
		}

		actualYAML, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
		require.NoError(t, err)

		yamlStr := string(actualYAML)
		assert.Contains(t, yamlStr, "DOCKER_HOST",
			"DinD mode should set DOCKER_HOST environment variable")
		assert.Contains(t, yamlStr, "name: dind",
			"DinD mode should include dind sidecar container")
	})

	t.Run("privileged-has-security-context", func(t *testing.T) {
		config := Config{
			Installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/test/repo",
				AuthValue:     "test-token",
				ContainerMode: types.ContainerModePrivileged,
			},
			InstanceName: "test-runner",
			InstanceNum:  1,
		}

		actualYAML, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
		require.NoError(t, err)

		yamlStr := string(actualYAML)
		assert.Contains(t, yamlStr, "privileged: true",
			"Privileged mode should set privileged: true")
		assert.Contains(t, yamlStr, "ACTIONS_RUNNER_CONTAINER_HOOKS",
			"Privileged mode should set container hooks")
	})
}

func TestControllerOverlayAddsRBACPermissions(t *testing.T) {
	processor := NewProcessor()

	// Process controller template (which applies the overlay)
	config := Config{
		Installation: &types.RunnerInstallation{
			Name:          "test-runner",
			Repository:    "https://github.com/test/repo",
			AuthValue:     "test-token",
			ContainerMode: types.ContainerModeKubernetes,
		},
		InstanceName: "test-runner",
		InstanceNum:  1,
	}

	processedYAML, err := processor.ProcessTemplate(TemplateTypeController, config)
	require.NoError(t, err, "ProcessTemplate should not return an error")
	require.NotEmpty(t, processedYAML, "Output should not be empty")

	yamlStr := string(processedYAML)

	// The controller needs to create roles and rolebindings dynamically for listener pods
	// Verify the ClusterRole has the required RBAC permissions

	// Check that roles resource has create, delete, get verbs
	// The overlay should add these permissions to the arc-controller-gha-rs-controller ClusterRole
	assert.Contains(t, yamlStr, "kind: ClusterRole", "Should contain ClusterRole")

	// Parse YAML to verify specific permissions on roles and rolebindings
	// We need to verify the ClusterRole rules include create/delete for roles and rolebindings
	type Rule struct {
		APIGroups []string `yaml:"apiGroups"`
		Resources []string `yaml:"resources"`
		Verbs     []string `yaml:"verbs"`
	}
	type ClusterRole struct {
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Rules []Rule `yaml:"rules"`
	}

	// Split into documents and find the ClusterRole
	docs := strings.Split(yamlStr, "---")
	var controllerClusterRole *ClusterRole

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		if strings.Contains(doc, "kind: ClusterRole") && strings.Contains(doc, "name: arc-controller-gha-rs-controller") {
			var cr ClusterRole
			err := yaml.Unmarshal([]byte(doc), &cr)
			require.NoError(t, err, "Failed to parse ClusterRole")
			controllerClusterRole = &cr
			break
		}
	}

	require.NotNil(t, controllerClusterRole, "Should find arc-controller-gha-rs-controller ClusterRole")

	// Find rules for roles and rolebindings
	var rolesRule, rolebindingsRule *Rule
	for i := range controllerClusterRole.Rules {
		rule := &controllerClusterRole.Rules[i]
		for _, resource := range rule.Resources {
			if resource == "roles" {
				rolesRule = rule
			}
			if resource == "rolebindings" {
				rolebindingsRule = rule
			}
		}
	}

	require.NotNil(t, rolesRule, "Should have a rule for 'roles' resource")
	require.NotNil(t, rolebindingsRule, "Should have a rule for 'rolebindings' resource")

	// Verify roles has create, delete, get verbs (required for controller to manage listener roles)
	assert.Contains(t, rolesRule.Verbs, "create", "roles rule should have 'create' verb")
	assert.Contains(t, rolesRule.Verbs, "delete", "roles rule should have 'delete' verb")
	assert.Contains(t, rolesRule.Verbs, "get", "roles rule should have 'get' verb")

	// Verify rolebindings has create, delete, get verbs
	assert.Contains(t, rolebindingsRule.Verbs, "create", "rolebindings rule should have 'create' verb")
	assert.Contains(t, rolebindingsRule.Verbs, "delete", "rolebindings rule should have 'delete' verb")
	assert.Contains(t, rolebindingsRule.Verbs, "get", "rolebindings rule should have 'get' verb")
}

func TestControllerOverlayAddsListPermissionToSecrets(t *testing.T) {
	processor := NewProcessor()

	// Process controller template (which applies the overlay)
	config := Config{
		Installation: &types.RunnerInstallation{
			Name:          "test-runner",
			Repository:    "https://github.com/test/repo",
			AuthValue:     "test-token",
			ContainerMode: types.ContainerModeKubernetes,
		},
		InstanceName: "test-runner",
		InstanceNum:  1,
	}

	processedYAML, err := processor.ProcessTemplate(TemplateTypeController, config)
	require.NoError(t, err, "ProcessTemplate should not return an error")
	require.NotEmpty(t, processedYAML, "Output should not be empty")

	yamlStr := string(processedYAML)

	// Verify the manager_listener_role Role has 'list' permission on secrets
	// This is required for the controller to list runner-linked secrets during EphemeralRunner finalization
	assert.Contains(t, yamlStr, "kind: Role", "Should contain Role")

	// Parse YAML to verify specific permissions on secrets resource in the Role
	type Rule struct {
		APIGroups []string `yaml:"apiGroups"`
		Resources []string `yaml:"resources"`
		Verbs     []string `yaml:"verbs"`
	}
	type Role struct {
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Rules []Rule `yaml:"rules"`
	}

	// Split into documents and find the listener Role
	docs := strings.Split(yamlStr, "---")
	var listenerRole *Role

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		if strings.Contains(doc, "kind: Role") && strings.Contains(doc, "arc-controller-gha-rs-controller-listener") {
			var role Role
			err := yaml.Unmarshal([]byte(doc), &role)
			require.NoError(t, err, "Failed to parse listener Role")
			listenerRole = &role
			break
		}
	}

	require.NotNil(t, listenerRole, "Should find arc-controller-gha-rs-controller-listener Role")
	assert.Equal(t, "arc-controller-gha-rs-controller-listener", listenerRole.Metadata.Name)

	// Find the rule for secrets resource
	var secretsRule *Rule
	for i := range listenerRole.Rules {
		rule := &listenerRole.Rules[i]
		for _, resource := range rule.Resources {
			if resource == "secrets" {
				secretsRule = rule
				break
			}
		}
		if secretsRule != nil {
			break
		}
	}

	require.NotNil(t, secretsRule, "Should have a rule for 'secrets' resource")

	// Verify secrets rule has 'list' verb (added by overlay)
	assert.Contains(t, secretsRule.Verbs, "list", "secrets rule should have 'list' verb for EphemeralRunner finalization")

	// Also verify other expected verbs are present
	assert.Contains(t, secretsRule.Verbs, "create", "secrets rule should have 'create' verb")
	assert.Contains(t, secretsRule.Verbs, "delete", "secrets rule should have 'delete' verb")
	assert.Contains(t, secretsRule.Verbs, "get", "secrets rule should have 'get' verb")
	assert.Contains(t, secretsRule.Verbs, "patch", "secrets rule should have 'patch' verb")
	assert.Contains(t, secretsRule.Verbs, "update", "secrets rule should have 'update' verb")
}

func TestGetRawTemplate(t *testing.T) {
	processor := NewProcessor()

	t.Run("controller template", func(t *testing.T) {
		content, err := processor.GetRawTemplate(TemplateTypeController)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
		assert.Contains(t, string(content), "AutoscalingRunnerSet")
	})

	t.Run("scale-set template", func(t *testing.T) {
		content, err := processor.GetRawTemplate(TemplateTypeScaleSet)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
		assert.Contains(t, string(content), "arc-runner")
	})

	t.Run("invalid template type", func(t *testing.T) {
		_, err := processor.GetRawTemplate(TemplateType("invalid"))
		require.Error(t, err)
	})
}

func TestEmbeddedTemplates(t *testing.T) {
	t.Run("GetTemplateFiles", func(t *testing.T) {
		files, err := GetTemplateFiles()
		require.NoError(t, err)
		assert.NotEmpty(t, files)

		// Check required files exist (new structure with base templates)
		requiredFiles := []string{
			"controller/rendered.yaml",
			"scale-set/bases/kubernetes.yaml",
			"scale-set/bases/dind.yaml",
			"scale-set/bases/privileged.yaml",
			"overlay.yaml",
			"values/schema.yaml",
		}
		for _, rf := range requiredFiles {
			_, exists := files[rf]
			assert.True(t, exists, "Expected file %s not found", rf)
		}
	})

	t.Run("GetControllerChart", func(t *testing.T) {
		content, err := GetControllerChart()
		require.NoError(t, err)
		assert.NotEmpty(t, content)
	})

	t.Run("GetScaleSetBase for each mode", func(t *testing.T) {
		modes := []types.ContainerMode{
			types.ContainerModeKubernetes,
			types.ContainerModeDinD,
			types.ContainerModePrivileged,
		}
		for _, mode := range modes {
			content, err := GetScaleSetBase(mode)
			require.NoError(t, err, "Failed to get base template for mode %s", mode)
			assert.NotEmpty(t, content, "Base template for mode %s is empty", mode)
		}
	})

	t.Run("GetUniversalOverlay", func(t *testing.T) {
		content, err := GetUniversalOverlay()
		require.NoError(t, err)
		assert.NotEmpty(t, content)
		assert.Contains(t, content, "@ytt:overlay")
	})
}

func TestTemplateError(t *testing.T) {
	t.Run("basic error", func(t *testing.T) {
		err := NewTemplateError(ErrorTypeSyntax, "test message", nil)
		assert.Equal(t, ErrorTypeSyntax, err.Type)
		assert.Equal(t, "test message", err.Message)
		assert.Contains(t, err.Error(), "syntax")
		assert.Contains(t, err.Error(), "test message")
	})

	t.Run("error with location", func(t *testing.T) {
		err := NewTemplateError(ErrorTypeSyntax, "test message", nil).
			WithTemplate("test.yaml").
			WithLocation(10, 5)
		assert.Contains(t, err.Error(), "test.yaml")
		assert.Contains(t, err.Error(), "line 10")
		assert.Contains(t, err.Error(), "column 5")
	})

	t.Run("verbose error", func(t *testing.T) {
		err := NewTemplateError(ErrorTypeSyntax, "test message", nil).
			WithTemplate("test.yaml").
			WithLocation(10, 5).
			WithYttOutput("ytt error output").
			WithContext(map[string]any{"key": "value"})

		verbose := err.VerboseError()
		assert.Contains(t, verbose, "Template Processing Error")
		assert.Contains(t, verbose, "test.yaml")
		assert.Contains(t, verbose, "ytt error output")
		assert.Contains(t, verbose, "key")
	})
}

// assertYAMLMatchesFile compares actual YAML with expected file content
// If ACCEPT_DIFF=true or UPDATE_SNAPSHOTS=true, it updates the expected file
func assertYAMLMatchesFile(t *testing.T, actual []byte, expectedPath string) {
	t.Helper()

	acceptMode := os.Getenv("ACCEPT_DIFF") == "true" || os.Getenv("UPDATE_SNAPSHOTS") == "true"

	if acceptMode {
		// Create directory if needed
		dir := filepath.Dir(expectedPath)
		require.NoError(t, os.MkdirAll(dir, 0755))

		// Write actual content to expected file
		err := os.WriteFile(expectedPath, actual, 0644)
		require.NoError(t, err, "Failed to write expected file")
		t.Logf("Updated expected file: %s", expectedPath)
		return
	}

	// Read expected file
	expected, err := os.ReadFile(expectedPath)
	if os.IsNotExist(err) {
		t.Fatalf("Expected file not found: %s\nRun with ACCEPT_DIFF=true to create it.\nActual content:\n%s",
			expectedPath, string(actual))
	}
	require.NoError(t, err, "Failed to read expected file")

	// Compare using dyff for nice diff output
	if strings.TrimSpace(string(actual)) != strings.TrimSpace(string(expected)) {
		diff := generateYAMLDiff(t, expected, actual)
		t.Fatalf("YAML content does not match expected file %s\n\nDiff:\n%s\n\nRun with ACCEPT_DIFF=true to update.",
			expectedPath, diff)
	}
}

// generateYAMLDiff generates a human-readable diff between two YAML contents
func generateYAMLDiff(t *testing.T, expected, actual []byte) string {
	t.Helper()

	// Try to use dyff for rich diff
	fromDocs, err := ytbx.LoadDocuments(expected)
	if err != nil {
		return simpleDiff(expected, actual)
	}

	toDocs, err := ytbx.LoadDocuments(actual)
	if err != nil {
		return simpleDiff(expected, actual)
	}

	from := ytbx.InputFile{Location: "expected", Documents: fromDocs}
	to := ytbx.InputFile{Location: "actual", Documents: toDocs}

	report, err := dyff.CompareInputFiles(from, to)
	if err != nil {
		return simpleDiff(expected, actual)
	}

	if len(report.Diffs) == 0 {
		return "No differences found"
	}

	// Format diffs as human-readable output
	var result strings.Builder
	humanReport := dyff.HumanReport{
		Report:               report,
		OmitHeader:           true,
		UseGoPatchPaths:      false,
		MinorChangeThreshold: 0.1,
	}
	if err := humanReport.WriteReport(&result); err != nil {
		return simpleDiff(expected, actual)
	}

	return result.String()
}

// simpleDiff provides a basic line-by-line diff when dyff fails
func simpleDiff(expected, actual []byte) string {
	expectedLines := strings.Split(string(expected), "\n")
	actualLines := strings.Split(string(actual), "\n")

	var result strings.Builder
	result.WriteString("--- Expected\n+++ Actual\n")

	maxLines := len(expectedLines)
	if len(actualLines) > maxLines {
		maxLines = len(actualLines)
	}

	for i := 0; i < maxLines; i++ {
		expectedLine := ""
		actualLine := ""
		if i < len(expectedLines) {
			expectedLine = expectedLines[i]
		}
		if i < len(actualLines) {
			actualLine = actualLines[i]
		}

		if expectedLine != actualLine {
			if expectedLine != "" {
				result.WriteString("- " + expectedLine + "\n")
			}
			if actualLine != "" {
				result.WriteString("+ " + actualLine + "\n")
			}
		}
	}

	return result.String()
}
