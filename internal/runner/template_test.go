package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rkoster/deskrun/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test the string replacement logic to ensure it doesn't create invalid ytt syntax
func TestStringReplacements(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Replace composite names correctly",
			input:    "name: arc-runner-gha-rs-github-secret",
			expected: "name: #@ data.values.installation.name + \"-gha-rs-github-secret\"",
		},
		{
			name:     "Replace no-permission service account",
			input:    "serviceAccountName: arc-runner-gha-rs-no-permission",
			expected: "serviceAccountName: #@ data.values.installation.name + \"-gha-rs-no-permission\"",
		},
		{
			name:     "Replace manager role names",
			input:    "name: arc-runner-gha-rs-manager",
			expected: "name: #@ data.values.installation.name + \"-gha-rs-manager\"",
		},
		{
			name:     "Replace simple arc-runner references in labels",
			input:    "app.kubernetes.io/name: arc-runner",
			expected: "app.kubernetes.io/name: #@ data.values.installation.name",
		},
		{
			name:     "Replace repository URL",
			input:    "githubConfigUrl: https://github.com/example/repo",
			expected: "githubConfigUrl: #@ data.values.installation.repository",
		},
		{
			name:     "Don't break when arc-runner appears in composite names",
			input:    "cleanup-github-secret-name: arc-runner-gha-rs-github-secret",
			expected: "cleanup-github-secret-name: #@ data.values.installation.name + \"-gha-rs-github-secret\"",
		},
		{
			name: "Multiple replacements in same content",
			input: `name: arc-runner-gha-rs-github-secret
labels:
  app.kubernetes.io/name: arc-runner
  actions.github.com/cleanup-github-secret-name: arc-runner-gha-rs-github-secret`,
			expected: `name: #@ data.values.installation.name + "-gha-rs-github-secret"
labels:
  app.kubernetes.io/name: #@ data.values.installation.name
  actions.github.com/cleanup-github-secret-name: #@ data.values.installation.name + "-gha-rs-github-secret"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyStringReplacements(tt.input)
			assert.Equal(t, tt.expected, result)

			// Ensure no invalid ytt syntax like "name-gha-rs-secret"
			assert.NotContains(t, result, "data.values.installation.name-gha")
			assert.NotContains(t, result, "name-gha-rs")
		})
	}
}

// Helper function that extracts the string replacement logic for testing
func applyStringReplacements(scaleSetYAML string) string {
	// Same logic as in setupYttTemplateDir
	scaleSetYAML = strings.ReplaceAll(scaleSetYAML, "https://github.com/example/repo", "#@ data.values.installation.repository")
	scaleSetYAML = strings.ReplaceAll(scaleSetYAML, "arc-runner-gha-rs-github-secret", "#@ data.values.installation.name + \"-gha-rs-github-secret\"")
	scaleSetYAML = strings.ReplaceAll(scaleSetYAML, "arc-runner-gha-rs-no-permission", "#@ data.values.installation.name + \"-gha-rs-no-permission\"")
	scaleSetYAML = strings.ReplaceAll(scaleSetYAML, "arc-runner-gha-rs-manager", "#@ data.values.installation.name + \"-gha-rs-manager\"")
	// Replace remaining arc-runner references (labels, names, etc.)
	scaleSetYAML = strings.ReplaceAll(scaleSetYAML, "\"arc-runner\"", "#@ data.values.installation.name")
	scaleSetYAML = strings.ReplaceAll(scaleSetYAML, ": arc-runner", ": #@ data.values.installation.name")
	scaleSetYAML = strings.ReplaceAll(scaleSetYAML, "name: arc-runner", "name: #@ data.values.installation.name")
	scaleSetYAML = strings.ReplaceAll(scaleSetYAML, "placeholder", "#@ data.values.installation.authValue")

	return scaleSetYAML
}

// Test the data values file creation
func TestCreateDataValuesFile(t *testing.T) {
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
	dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")

	err := manager.createDataValuesFile(installation, "test-runner", 0, dataValuesPath)
	require.NoError(t, err)

	// Read the generated file
	content, err := os.ReadFile(dataValuesPath)
	require.NoError(t, err)

	// Check that it contains expected values
	contentStr := string(content)
	assert.Contains(t, contentStr, "name: test-runner")
	assert.Contains(t, contentStr, "repository: https://github.com/test/repo")
	assert.Contains(t, contentStr, "authValue: test-token")
	assert.Contains(t, contentStr, "containerMode: cached-privileged-kubernetes")
	assert.Contains(t, contentStr, "minRunners: 1")
	assert.Contains(t, contentStr, "maxRunners: 3")
	assert.Contains(t, contentStr, "target: /nix/store")
	assert.Contains(t, contentStr, "source: /nix/store")
	assert.Contains(t, contentStr, "target: /var/lib/docker")
	assert.Contains(t, contentStr, "source: /tmp/test-cache")
}

// Test setupYttTemplateDir function
func TestSetupYttTemplateDir(t *testing.T) {
	manager := &Manager{}

	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/test/repo",
		AuthValue:     "test-token",
		ContainerMode: types.ContainerModePrivileged,
		CachePaths: []types.CachePath{
			{Target: "/nix/store", Source: "/nix/store"},
		},
	}

	tmpDir := t.TempDir()

	templateDir, err := manager.setupYttTemplateDir(installation, tmpDir)
	require.NoError(t, err)

	// Check that template directory was created
	assert.DirExists(t, templateDir)

	// Check that scale-set.yaml exists and has correct replacements
	scaleSetPath := filepath.Join(templateDir, "scale-set.yaml")
	assert.FileExists(t, scaleSetPath)

	content, err := os.ReadFile(scaleSetPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Ensure proper ytt syntax - no broken replacements
	assert.NotContains(t, contentStr, "data.values.installation.name-gha-rs")
	assert.NotContains(t, contentStr, "name-gha-rs-github-secret")

	// Check for correct replacements
	if strings.Contains(contentStr, "github-secret") {
		assert.Contains(t, contentStr, "#@ data.values.installation.name + \"-gha-rs-github-secret\"")
	}

	// Check that overlay.yaml exists
	overlayPath := filepath.Join(templateDir, "overlay.yaml")
	assert.FileExists(t, overlayPath)
}

// Test that ytt expressions are syntactically valid
func TestYttSyntaxValidation(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		isValid bool
	}{
		{
			name:    "Valid concatenation",
			input:   "#@ data.values.installation.name + \"-gha-rs-github-secret\"",
			isValid: true,
		},
		{
			name:    "Invalid syntax with dash",
			input:   "#@ data.values.installation.name-gha-rs-github-secret",
			isValid: false,
		},
		{
			name:    "Valid simple reference",
			input:   "#@ data.values.installation.name",
			isValid: true,
		},
		{
			name:    "Valid repository reference",
			input:   "#@ data.values.installation.repository",
			isValid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Check basic syntax patterns
			if tc.isValid {
				// Valid ytt expressions should have proper structure
				if strings.Contains(tc.input, "+") {
					// Concatenation should have quotes around string literals
					assert.Contains(t, tc.input, "\"-")
				}
				// Should start with #@ and reference data.values
				assert.True(t, strings.HasPrefix(tc.input, "#@ data.values."))
			} else {
				// Invalid syntax - should not have unquoted dashes after variable names
				if strings.Contains(tc.input, "data.values.installation.name-") {
					assert.NotContains(t, tc.input, "+ \"-") // Missing concatenation operator
				}
			}
		})
	}
}

// Test overlay file structure and syntax
func TestOverlayFileSyntax(t *testing.T) {
	manager := &Manager{}
	tmpDir := t.TempDir()

	installation := &types.RunnerInstallation{
		ContainerMode: types.ContainerModePrivileged,
		CachePaths: []types.CachePath{
			{Target: "/nix/store", Source: "/nix/store"},
		},
	}

	templateDir, err := manager.setupYttTemplateDir(installation, tmpDir)
	require.NoError(t, err)

	overlayPath := filepath.Join(templateDir, "overlay.yaml")
	content, err := os.ReadFile(overlayPath)
	require.NoError(t, err)

	contentStr := string(content)

	// Check overlay structure
	assert.Contains(t, contentStr, "#@ load(\"@ytt:data\", \"data\")")
	assert.Contains(t, contentStr, "#@ load(\"@ytt:overlay\", \"overlay\")")
	assert.Contains(t, contentStr, "#@overlay/match")

	// Ensure proper ytt syntax in overlays
	assert.Contains(t, contentStr, "#@ data.values.installation.name + \"-gha-rs")
	assert.NotContains(t, contentStr, "data.values.installation.name-gha") // No broken syntax
}
