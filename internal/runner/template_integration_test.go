package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rkoster/deskrun/internal/kapp"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullYttProcessing tests the complete ytt processing pipeline
func TestFullYttProcessing(t *testing.T) {
	// Check if ytt is available
	if _, err := exec.LookPath("ytt"); err != nil {
		t.Skip("ytt not available, skipping integration test")
	}

	manager := &Manager{}

	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/test/repo",
		AuthValue:     "test-token",
		ContainerMode: types.ContainerModePrivileged,
		MinRunners:    1,
		MaxRunners:    3,
		CachePaths: []types.CachePath{
			{Target: "/nix/store", Source: "/nix/store"},
			{Target: "/var/lib/docker", Source: "/tmp/test-cache"},
		},
	}

	tmpDir := t.TempDir()

	// Setup template directory
	templateDir, err := manager.setupYttTemplateDir(installation, tmpDir)
	require.NoError(t, err)

	// Create data values file
	dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")
	err = manager.createDataValuesFile(installation, "test-runner", 0, dataValuesPath)
	require.NoError(t, err)

	// Process with ytt
	client := kapp.NewClient("", "arc-systems")
	result, err := client.ProcessTemplate(templateDir, dataValuesPath)

	if err != nil {
		t.Logf("YTT processing failed: %v", err)

		// Read the generated files for debugging
		scaleSetPath := filepath.Join(templateDir, "scale-set.yaml")
		if content, readErr := os.ReadFile(scaleSetPath); readErr == nil {
			t.Logf("Generated scale-set.yaml content:\n%s", string(content))
		}

		if content, readErr := os.ReadFile(dataValuesPath); readErr == nil {
			t.Logf("Data values content:\n%s", string(content))
		}

		require.NoError(t, err, "YTT processing should succeed")
	}

	// Validate the result
	assert.NotEmpty(t, result)

	// Check that ytt expressions have been resolved
	assert.NotContains(t, result, "#@ data.values", "All ytt expressions should be resolved")

	// Check that values have been substituted correctly
	assert.Contains(t, result, "test-runner-gha-rs-github-secret")
	assert.Contains(t, result, "test-runner-gha-rs-no-permission")
	assert.Contains(t, result, "test-runner-gha-rs-manager")
	assert.Contains(t, result, "https://github.com/test/repo")

	// Check for proper base64 encoding of auth value
	assert.Contains(t, result, "github_token:")

	// Verify container mode specific configurations
	assert.Contains(t, result, "privileged: true")
	assert.Contains(t, result, "ACTIONS_RUNNER_CONTAINER_HOOKS")

	// Check cache path volumes are configured
	assert.Contains(t, result, "/nix/store")
	assert.Contains(t, result, "/tmp/test-cache")

	t.Logf("YTT processing completed successfully. Output length: %d", len(result))
}

// TestYttProcessingForDifferentContainerModes tests all container modes
func TestYttProcessingForDifferentContainerModes(t *testing.T) {
	if _, err := exec.LookPath("ytt"); err != nil {
		t.Skip("ytt not available, skipping integration test")
	}

	testCases := []struct {
		name          string
		containerMode types.ContainerMode
		expectStrings []string
		rejectStrings []string
	}{
		{
			name:          "kubernetes mode",
			containerMode: types.ContainerModeKubernetes,
			expectStrings: []string{
				"ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER",
				"value: \"true\"",
			},
			rejectStrings: []string{
				"privileged: true",
				"DOCKER_HOST",
				"ACTIONS_RUNNER_CONTAINER_HOOKS",
			},
		},
		{
			name:          "dind mode",
			containerMode: types.ContainerModeDinD,
			expectStrings: []string{
				"DOCKER_HOST",
				"tcp://localhost:2375",
				"docker:dind",
				"emptyDir: {}",
			},
			rejectStrings: []string{
				"ACTIONS_RUNNER_CONTAINER_HOOKS",
			},
		},
		{
			name:          "cached-privileged-kubernetes mode",
			containerMode: types.ContainerModePrivileged,
			expectStrings: []string{
				"privileged: true",
				"ACTIONS_RUNNER_CONTAINER_HOOKS",
				"ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER",
				"value: \"false\"",
				"fsGroup: 123",
			},
			rejectStrings: []string{
				"DOCKER_HOST",
				"docker:dind",
				"emptyDir: {}",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			manager := &Manager{}

			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/test/repo",
				AuthValue:     "test-token",
				ContainerMode: tc.containerMode,
				CachePaths: []types.CachePath{
					{Target: "/tmp/cache", Source: "/host/cache"},
				},
			}

			tmpDir := t.TempDir()

			templateDir, err := manager.setupYttTemplateDir(installation, tmpDir)
			require.NoError(t, err)

			dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")
			err = manager.createDataValuesFile(installation, "test-runner", 0, dataValuesPath)
			require.NoError(t, err)

			client := kapp.NewClient("", "arc-systems")
			result, err := client.ProcessTemplate(templateDir, dataValuesPath)
			require.NoError(t, err, "YTT processing for %s mode should succeed", tc.containerMode)

			// Check expected strings
			for _, expectStr := range tc.expectStrings {
				assert.Contains(t, result, expectStr, "Should contain %s for %s mode", expectStr, tc.containerMode)
			}

			// Check rejected strings
			for _, rejectStr := range tc.rejectStrings {
				assert.NotContains(t, result, rejectStr, "Should not contain %s for %s mode", rejectStr, tc.containerMode)
			}
		})
	}
}

// TestYttSyntaxErrors tests that we catch common ytt syntax errors
func TestYttSyntaxErrors(t *testing.T) {
	if _, err := exec.LookPath("ytt"); err != nil {
		t.Skip("ytt not available, skipping syntax error test")
	}

	tmpDir := t.TempDir()

	// Create a template with intentional syntax errors
	templateDir := filepath.Join(tmpDir, "templates")
	err := os.MkdirAll(templateDir, 0755)
	require.NoError(t, err)

	// Template with invalid ytt syntax (missing + operator)
	invalidTemplate := `
#@ load("@ytt:data", "data")
---
apiVersion: v1
kind: Secret
metadata:
  name: #@ data.values.installation.name-invalid-syntax
data:
  token: #@ data.values.installation.authValue
`

	templatePath := filepath.Join(templateDir, "invalid.yaml")
	err = os.WriteFile(templatePath, []byte(invalidTemplate), 0644)
	require.NoError(t, err)

	// Create valid data values
	dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")
	dataValuesContent := `
installation:
  name: test
  authValue: token
`
	err = os.WriteFile(dataValuesPath, []byte(dataValuesContent), 0644)
	require.NoError(t, err)

	// Process with ytt - should fail
	client := kapp.NewClient("", "arc-systems")
	_, err = client.ProcessTemplate(templateDir, dataValuesPath)

	// Should fail due to invalid syntax
	assert.Error(t, err, "YTT should reject invalid syntax")
	assert.Contains(t, strings.ToLower(err.Error()), "undefined", "Error should mention undefined variable")
}

// TestTemplateFileGeneration validates that the generated template files are correct
func TestTemplateFileGeneration(t *testing.T) {
	manager := &Manager{}

	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		ContainerMode: types.ContainerModePrivileged,
	}

	tmpDir := t.TempDir()
	templateDir, err := manager.setupYttTemplateDir(installation, tmpDir)
	require.NoError(t, err)

	// Check scale-set.yaml
	scaleSetPath := filepath.Join(templateDir, "scale-set.yaml")
	content, err := os.ReadFile(scaleSetPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Verify no broken syntax from string replacements
	brokenPatterns := []string{
		"data.values.installation.name-gha-rs", // Missing + operator
		"name-gha-rs-github-secret",            // Partial replacement
		"#@ name",                              // Invalid expression
	}

	for _, pattern := range brokenPatterns {
		assert.NotContains(t, contentStr, pattern,
			"Template should not contain broken pattern: %s", pattern)
	}

	// Verify correct patterns exist
	correctPatterns := []string{
		"#@ data.values.installation.name + \"-gha-rs-github-secret\"",
		"#@ data.values.installation.name + \"-gha-rs-no-permission\"",
		"#@ data.values.installation.repository",
	}

	foundPatterns := 0
	for _, pattern := range correctPatterns {
		if strings.Contains(contentStr, pattern) {
			foundPatterns++
		}
	}

	assert.Greater(t, foundPatterns, 0, "Template should contain at least some correct ytt expressions")
}
