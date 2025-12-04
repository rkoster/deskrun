package runner

import (
	"strings"
	"testing"

	"github.com/rkoster/deskrun/pkg/templates"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullYttProcessing tests the complete ytt processing pipeline using the unified template package
func TestFullYttProcessing(t *testing.T) {
	processor := templates.NewProcessor()

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

	config := templates.Config{
		Installation: installation,
		InstanceName: "test-runner",
		InstanceNum:  0,
	}

	result, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
	require.NoError(t, err, "YTT processing should succeed")

	resultStr := string(result)

	// Validate the result
	assert.NotEmpty(t, resultStr)

	// Check that ytt expressions have been resolved
	assert.NotContains(t, resultStr, "#@ data.values", "All ytt expressions should be resolved")

	// Check that values have been substituted correctly
	assert.Contains(t, resultStr, "test-runner-gha-rs-github-secret")
	// Privileged mode uses kube-mode service account (for kubernetes hooks)
	assert.Contains(t, resultStr, "test-runner-gha-rs-kube-mode")
	assert.Contains(t, resultStr, "test-runner-gha-rs-manager")
	assert.Contains(t, resultStr, "https://github.com/test/repo")

	// Check for proper base64 encoding of auth value
	assert.Contains(t, resultStr, "github_token:")

	// Verify container mode specific configurations
	assert.Contains(t, resultStr, "ACTIONS_RUNNER_CONTAINER_HOOKS")

	// Cache paths SHOULD be mounted in privileged runner container (this is the whole point)
	// Verify that runner container can access cache paths directly
	assert.Contains(t, resultStr, "ACTIONS_RUNNER_CONTAINER_HOOKS")

	// Verify privileged runner has direct access to cache resources
	assert.Contains(t, resultStr, "privileged: true", "Runner should be privileged for direct host access")
	assert.Contains(t, resultStr, "k8s-novolume/index.js", "Should use novolume hooks for privileged runner")

	// Verify hook extension is configured for job container privileges
	assert.Contains(t, resultStr, "privileged-hook-extension-test-runner")

	t.Logf("YTT processing completed successfully. Output length: %d", len(resultStr))
}

// TestYttProcessingForDifferentContainerModes tests all container modes using the unified template package
func TestYttProcessingForDifferentContainerModes(t *testing.T) {
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
				// Upstream kubernetes mode includes container hooks
				"ACTIONS_RUNNER_CONTAINER_HOOKS",
				"/home/runner/k8s/index.js",
			},
			rejectStrings: []string{
				"privileged: true",
				"DOCKER_HOST",
				// Should not have novolume hooks
				"k8s-novolume",
			},
		},
		{
			name:          "dind mode",
			containerMode: types.ContainerModeDinD,
			expectStrings: []string{
				"DOCKER_HOST",
				"unix:///var/run/docker.sock",
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
				// Runner container security context (PRIVILEGED)
				"privileged: true",
				"allowPrivilegeEscalation: true",
				"runAsNonRoot: false",
				// Hook extension environment variables
				"ACTIONS_RUNNER_CONTAINER_HOOKS",
				"/home/runner/k8s-novolume/index.js",
				"ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE",
				"/etc/hooks/content",
				"ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER",
				"value: \"false\"",
				// Hook extension volume
				"hook-extension",
				"privileged-hook-extension-",
				// Pod-level security context
				"fsGroup: 123",
			},
			rejectStrings: []string{
				// Should NOT have dind sidecar (that's a different mode)
				"docker:dind",
				"DOCKER_HOST",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			processor := templates.NewProcessor()

			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/test/repo",
				AuthValue:     "test-token",
				ContainerMode: tc.containerMode,
				CachePaths: []types.CachePath{
					{Target: "/tmp/cache", Source: "/host/cache"},
				},
			}

			config := templates.Config{
				Installation: installation,
				InstanceName: "test-runner",
				InstanceNum:  0,
			}

			result, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
			require.NoError(t, err, "YTT processing for %s mode should succeed", tc.containerMode)

			resultStr := string(result)

			// Check expected strings
			for _, expectStr := range tc.expectStrings {
				assert.Contains(t, resultStr, expectStr, "Should contain %s for %s mode", expectStr, tc.containerMode)
			}

			// Check rejected strings
			for _, rejectStr := range tc.rejectStrings {
				assert.NotContains(t, resultStr, rejectStr, "Should not contain %s for %s mode", rejectStr, tc.containerMode)
			}
		})
	}
}

// TestYttSyntaxErrors tests that we catch common ytt syntax errors
func TestYttSyntaxErrors(t *testing.T) {
	processor := templates.NewProcessor()

	// Test with invalid container mode (should fail validation)
	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/test/repo",
		AuthValue:     "test-token",
		ContainerMode: types.ContainerMode("invalid-mode"),
	}

	config := templates.Config{
		Installation: installation,
		InstanceName: "test-runner",
		InstanceNum:  0,
	}

	_, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
	assert.Error(t, err, "Should reject invalid container mode")
	assert.Contains(t, strings.ToLower(err.Error()), "invalid", "Error should mention invalid mode")
}

// TestTemplateValidation validates that the processor validates input correctly
func TestTemplateValidation(t *testing.T) {
	processor := templates.NewProcessor()

	testCases := []struct {
		name        string
		config      templates.Config
		expectError string
	}{
		{
			name: "empty instance name",
			config: templates.Config{
				Installation: &types.RunnerInstallation{
					Name:          "test",
					Repository:    "https://github.com/test/repo",
					ContainerMode: types.ContainerModeKubernetes,
				},
				InstanceName: "",
			},
			expectError: "instance name",
		},
		{
			name: "nil installation",
			config: templates.Config{
				Installation: nil,
				InstanceName: "test",
			},
			expectError: "installation",
		},
		{
			name: "empty repository",
			config: templates.Config{
				Installation: &types.RunnerInstallation{
					Name:          "test",
					Repository:    "",
					ContainerMode: types.ContainerModeKubernetes,
				},
				InstanceName: "test",
			},
			expectError: "repository",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, tc.config)
			assert.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), tc.expectError)
		})
	}
}
