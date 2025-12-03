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
)

// TestMatrix defines comprehensive test cases for all container modes and configurations
var testMatrix = []struct {
	name          string
	containerMode types.ContainerMode
	cachePaths    []types.CachePath
	expectedFile  string
}{
	// Kubernetes mode tests
	{
		name:          "kubernetes-basic",
		containerMode: types.ContainerModeKubernetes,
		cachePaths:    nil,
		expectedFile:  "kubernetes_basic.yaml",
	},
	{
		name:          "kubernetes-with-cache",
		containerMode: types.ContainerModeKubernetes,
		cachePaths: []types.CachePath{
			{Source: "/var/lib/docker", Target: "/var/lib/docker"},
		},
		expectedFile: "kubernetes_with_cache.yaml",
	},

	// Docker-in-Docker mode tests
	{
		name:          "dind-basic",
		containerMode: types.ContainerModeDinD,
		cachePaths:    nil,
		expectedFile:  "dind_basic.yaml",
	},
	{
		name:          "dind-with-cache",
		containerMode: types.ContainerModeDinD,
		cachePaths: []types.CachePath{
			{Source: "/tmp/docker-cache", Target: "/var/lib/docker"},
		},
		expectedFile: "dind_with_cache.yaml",
	},

	// Privileged mode tests
	{
		name:          "privileged-basic",
		containerMode: types.ContainerModePrivileged,
		cachePaths:    nil,
		expectedFile:  "privileged_basic.yaml",
	},
	{
		name:          "privileged-single-cache",
		containerMode: types.ContainerModePrivileged,
		cachePaths: []types.CachePath{
			{Source: "/var/lib/docker", Target: "/var/lib/docker"},
		},
		expectedFile: "privileged_single_cache.yaml",
	},
	{
		name:          "privileged-multi-cache",
		containerMode: types.ContainerModePrivileged,
		cachePaths: []types.CachePath{
			{Source: "/var/lib/docker", Target: "/var/lib/docker"},
			{Source: "/nix/store", Target: "/nix/store"},
		},
		expectedFile: "privileged_multi_cache.yaml",
	},
	{
		name:          "privileged-emptydir-cache",
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

			actualYAML, err := processor.ProcessTemplate(TemplateTypeScaleSet, config)
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
			name:                  "kubernetes-uses-manager",
			containerMode:         types.ContainerModeKubernetes,
			expectedSAType:        "manager",
			shouldHaveManagerSA:   true,
			shouldHaveManagerRBAC: true,
		},
		{
			name:                  "privileged-uses-manager",
			containerMode:         types.ContainerModePrivileged,
			expectedSAType:        "manager",
			shouldHaveManagerSA:   true,
			shouldHaveManagerRBAC: true,
		},
		{
			name:                  "dind-uses-no-permission",
			containerMode:         types.ContainerModeDinD,
			expectedSAType:        "no-permission",
			shouldHaveManagerSA:   false, // DinD mode removes manager SA
			shouldHaveManagerRBAC: false,
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

		// Check required files exist
		requiredFiles := []string{
			"controller/rendered.yaml",
			"scale-set/rendered.yaml",
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

	t.Run("GetScaleSetChart", func(t *testing.T) {
		content, err := GetScaleSetChart()
		require.NoError(t, err)
		assert.NotEmpty(t, content)
	})

	t.Run("GetUniversalOverlay", func(t *testing.T) {
		content, err := GetUniversalOverlay()
		require.NoError(t, err)
		assert.NotEmpty(t, content)
		assert.Contains(t, content, "@ytt:overlay")
	})

	t.Run("GetOverlay for each mode", func(t *testing.T) {
		overlays := []string{
			"container-mode-dind.yaml",
			"container-mode-kubernetes.yaml",
			"container-mode-privileged.yaml",
		}
		for _, overlay := range overlays {
			content, err := GetOverlay(overlay)
			require.NoError(t, err, "Failed to get overlay %s", overlay)
			assert.NotEmpty(t, content, "Overlay %s is empty", overlay)
		}
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
