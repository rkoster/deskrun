package runner

import (
	"testing"

	"github.com/rkoster/deskrun/pkg/templates"
	"github.com/rkoster/deskrun/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrivilegedContainerTemplateGeneration validates that ytt templates generate
// proper privileged container configuration for workflow-managed Docker
func TestPrivilegedContainerTemplateGeneration(t *testing.T) {
	manager := &Manager{}

	installation := &types.RunnerInstallation{
		Name:          "rubionic-workspace",
		Repository:    "https://github.com/rkoster/rubionic-workspace",
		AuthValue:     "test-token",
		ContainerMode: types.ContainerModePrivileged,
		MinRunners:    1,
		MaxRunners:    1,
		CachePaths: []types.CachePath{
			{Source: "/nix/store", Target: "/nix/store-host"},
			{Source: "/nix/var/nix/daemon-socket", Target: "/nix/var/nix/daemon-socket-host"},
			{Source: "/nvme/docker-cache", Target: "/var/lib/docker"}, // Docker image cache
		},
	}

	// Generate data values for template processing
	dataValues, err := manager.generateYTTDataValues(installation, installation.Name, 0)
	require.NoError(t, err)

	t.Run("data_values_enable_workflow_managed_docker", func(t *testing.T) {
		// Critical settings for Docker daemon installation in workflow
		assert.Contains(t, dataValues, "type: kubernetes-novolume") // Privileged mode uses kubernetes-novolume

		// Should contain basic runner configuration
		assert.Contains(t, dataValues, "githubConfigUrl: https://github.com/rkoster/rubionic-workspace")
		assert.Contains(t, dataValues, "runnerScaleSetName: rubionic-workspace")
		assert.Contains(t, dataValues, "minRunners: 1")
		assert.Contains(t, dataValues, "maxRunners: 1")

		// Should NOT have Docker socket mount (workflow installs Docker)
		assert.NotContains(t, dataValues, "/var/run/docker.sock")
	})

	// Generate hook extension ConfigMap for privileged containers
	hookExtension := manager.generateHookExtensionConfigMap(installation, installation.Name, 0)

	t.Run("hook_extension_enables_docker_daemon", func(t *testing.T) {
		// Security context required for Docker daemon installation
		assert.Contains(t, hookExtension, "privileged: true")
		assert.Contains(t, hookExtension, "runAsUser: 0")  // Root user
		assert.Contains(t, hookExtension, "runAsGroup: 0") // Root group
		assert.Contains(t, hookExtension, "fsGroup: 0")    // Root fs group
		assert.Contains(t, hookExtension, "hostPID: true") // Host PID namespace
		assert.Contains(t, hookExtension, "hostIPC: true") // Host IPC namespace

		// Capabilities required for Docker daemon
		assert.Contains(t, hookExtension, "SYS_ADMIN")  // Mount filesystems, cgroups
		assert.Contains(t, hookExtension, "NET_ADMIN")  // Network configuration
		assert.Contains(t, hookExtension, "SYS_PTRACE") // Process debugging

		// Critical mounts for Docker daemon functionality
		dockerRequiredMounts := []string{
			"mountPath: /sys",           // sysfs for cgroup management
			"mountPath: /proc",          // procfs for process management
			"mountPath: /dev",           // device access
			"mountPath: /dev/pts",       // pseudo-terminals
			"mountPath: /dev/shm",       // shared memory
			"mountPath: /sys/fs/cgroup", // cgroup filesystem
		}

		for _, mount := range dockerRequiredMounts {
			assert.Contains(t, hookExtension, mount,
				"Missing mount %s required for Docker daemon", mount)
		}

		// Host paths for Docker daemon access
		dockerRequiredHostPaths := []string{
			"path: /sys",
			"path: /proc",
			"path: /dev",
			"path: /dev/pts",
			"path: /dev/shm",
			"path: /sys/fs/cgroup",
		}

		for _, hostPath := range dockerRequiredHostPaths {
			assert.Contains(t, hookExtension, hostPath,
				"Missing host path %s required for Docker daemon", hostPath)
		}

		// Verify directory types for mounts
		assert.Contains(t, hookExtension, "type: Directory")

		// Should NOT contain Docker socket (workflow manages Docker)
		assert.NotContains(t, hookExtension, "/var/run/docker.sock")
		assert.NotContains(t, hookExtension, "type: Socket")
	})
}

// TestDockerCachePersistenceTemplates validates Docker image cache configuration
// by testing the full template processing pipeline (via pkg/templates)
func TestDockerCachePersistenceTemplates(t *testing.T) {
	tests := []struct {
		name            string
		cachePath       types.CachePath
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "persistent_docker_cache_with_host_path",
			cachePath: types.CachePath{
				Source: "/nvme/docker-images",
				Target: "/var/lib/docker",
			},
			wantContains: []string{
				// Cache should be mounted in runner container
				"mountPath: /var/lib/docker",
				// Should use hostPath volume
				"path: /nvme/docker-images",
				"type: DirectoryOrCreate",
			},
			wantNotContains: []string{
				// Should NOT be emptyDir
				"emptyDir: {}",
			},
		},
		{
			name: "temporary_docker_cache_with_empty_dir",
			cachePath: types.CachePath{
				Source: "", // Empty source = emptyDir
				Target: "/var/lib/docker",
			},
			wantContains: []string{
				// Cache should be mounted
				"mountPath: /var/lib/docker",
				// Should use emptyDir for empty source
				"emptyDir: {}",
			},
			wantNotContains: []string{
				// Should NOT be hostPath
				"/nvme/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := templates.NewProcessor()

			installation := &types.RunnerInstallation{
				Name:          "docker-workflow",
				Repository:    "https://github.com/test/repo",
				AuthValue:     "test-token",
				ContainerMode: types.ContainerModePrivileged,
				CachePaths:    []types.CachePath{tt.cachePath},
			}

			config := templates.Config{
				Installation: installation,
				InstanceName: installation.Name,
				InstanceNum:  0,
			}

			result, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
			require.NoError(t, err)

			resultStr := string(result)

			for _, want := range tt.wantContains {
				assert.Contains(t, resultStr, want,
					"Docker cache template missing: %s", want)
			}

			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, resultStr, notWant,
					"Docker cache template should not contain: %s", notWant)
			}
		})
	}
}

// TestMultiInstanceDockerCapabilities validates that all instances in multi-instance
// deployments have consistent Docker daemon capabilities
func TestMultiInstanceDockerCapabilities(t *testing.T) {
	manager := &Manager{}

	installation := &types.RunnerInstallation{
		Name:          "rubionic-workspace",
		Repository:    "https://github.com/rkoster/rubionic-workspace",
		AuthValue:     "test-token",
		ContainerMode: types.ContainerModePrivileged,
		Instances:     3,
		CachePaths: []types.CachePath{
			{Source: "/nix/store", Target: "/nix/store-host"},
			{Source: "/nvme/docker-cache", Target: "/var/lib/docker"},
		},
	}

	// Test all instances have identical Docker capabilities
	for i := 1; i <= installation.Instances; i++ {
		instanceName := installation.Name + "-" + string(rune('0'+i))

		t.Run("instance_"+instanceName, func(t *testing.T) {
			hookExtension := manager.generateHookExtensionConfigMap(installation, instanceName, i)

			// Each instance must support Docker daemon installation
			dockerRequirements := []string{
				"privileged: true",
				"SYS_ADMIN",
				"NET_ADMIN",
				"SYS_PTRACE",
				"hostPID: true",
				"hostIPC: true",
				"mountPath: /sys",
				"mountPath: /proc",
				"mountPath: /dev",
			}

			for _, req := range dockerRequirements {
				assert.Contains(t, hookExtension, req,
					"Instance %s missing Docker requirement: %s", instanceName, req)
			}

			// Verify instance-specific naming in ConfigMap
			expectedConfigMapName := "privileged-hook-extension-" + instanceName
			assert.Contains(t, hookExtension, expectedConfigMapName)
		})
	}
}
