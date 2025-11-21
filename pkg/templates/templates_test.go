package templates

import (
	"strings"
	"testing"

	"github.com/rkoster/deskrun/pkg/types"
)

func TestGenerateRunnerScaleSetManifest_Standard(t *testing.T) {
	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: types.ContainerModeKubernetes,
		MinRunners:    1,
		MaxRunners:    5,
		AuthType:      types.AuthTypePAT,
		AuthValue:     "test-token",
	}

	manifest, err := GenerateRunnerScaleSetManifest(installation, "arc-systems")
	if err != nil {
		t.Fatalf("GenerateRunnerScaleSetManifest() error = %v", err)
	}

	// Check for required fields
	requiredFields := []string{
		"apiVersion: actions.github.com/v1alpha1",
		"kind: AutoscalingRunnerSet",
		"name: test-runner",
		"namespace: arc-systems",
		"githubConfigUrl: https://github.com/owner/repo",
		"minRunners: 1",
		"maxRunners: 5",
		"githubConfigSecret: test-runner-secret",
	}

	for _, field := range requiredFields {
		if !strings.Contains(manifest, field) {
			t.Errorf("Manifest missing field: %s", field)
		}
	}
}

func TestGenerateRunnerScaleSetManifest_Privileged(t *testing.T) {
	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: types.ContainerModePrivileged,
		MinRunners:    1,
		MaxRunners:    5,
		CachePaths: []types.CachePath{
			{MountPath: "/nix/store", HostPath: ""},
		},
		AuthType:  types.AuthTypePAT,
		AuthValue: "test-token",
	}

	manifest, err := GenerateRunnerScaleSetManifest(installation, "arc-systems")
	if err != nil {
		t.Fatalf("GenerateRunnerScaleSetManifest() error = %v", err)
	}

	// Check for privileged-specific fields
	privilegedFields := []string{
		"privileged: true",
		"SYS_ADMIN",
		"NET_ADMIN",
		"SYSTEMD_IGNORE_CHROOT",
		"runAsUser: 0",
	}

	for _, field := range privilegedFields {
		if !strings.Contains(manifest, field) {
			t.Errorf("Manifest missing privileged field: %s", field)
		}
	}

	// Check for cache paths
	if !strings.Contains(manifest, "/nix/store") {
		t.Error("Manifest missing cache path")
	}
}

func TestGenerateRunnerScaleSetManifest_DinD(t *testing.T) {
	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: types.ContainerModeDinD,
		MinRunners:    1,
		MaxRunners:    5,
		AuthType:      types.AuthTypePAT,
		AuthValue:     "test-token",
	}

	manifest, err := GenerateRunnerScaleSetManifest(installation, "arc-systems")
	if err != nil {
		t.Fatalf("GenerateRunnerScaleSetManifest() error = %v", err)
	}

	// Check for DinD-specific fields
	dindFields := []string{
		"DOCKER_HOST",
		"tcp://localhost:2376",
		"name: dind",
		"image: docker:dind",
	}

	for _, field := range dindFields {
		if !strings.Contains(manifest, field) {
			t.Errorf("Manifest missing DinD field: %s", field)
		}
	}
}

func TestGenerateRunnerScaleSetManifest_InvalidMode(t *testing.T) {
	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: "invalid-mode",
		MinRunners:    1,
		MaxRunners:    5,
		AuthType:      types.AuthTypePAT,
		AuthValue:     "test-token",
	}

	_, err := GenerateRunnerScaleSetManifest(installation, "arc-systems")
	if err == nil {
		t.Error("GenerateRunnerScaleSetManifest() expected error for invalid mode, got nil")
	}
}

func TestGenerateGitHubSecretManifest_PAT(t *testing.T) {
	installation := &types.RunnerInstallation{
		Name:      "test-runner",
		AuthType:  types.AuthTypePAT,
		AuthValue: "ghp_test_token",
	}

	manifest := GenerateGitHubSecretManifest(installation, "arc-systems")

	requiredFields := []string{
		"apiVersion: v1",
		"kind: Secret",
		"name: test-runner-secret",
		"namespace: arc-systems",
		"github_token: ghp_test_token",
	}

	for _, field := range requiredFields {
		if !strings.Contains(manifest, field) {
			t.Errorf("Secret manifest missing field: %s", field)
		}
	}
}

func TestGenerateGitHubSecretManifest_GitHubApp(t *testing.T) {
	installation := &types.RunnerInstallation{
		Name:      "test-runner",
		AuthType:  types.AuthTypeGitHubApp,
		AuthValue: "private-key-content",
	}

	manifest := GenerateGitHubSecretManifest(installation, "arc-systems")

	requiredFields := []string{
		"apiVersion: v1",
		"kind: Secret",
		"name: test-runner-secret",
		"namespace: arc-systems",
		"github_app_private_key: private-key-content",
	}

	for _, field := range requiredFields {
		if !strings.Contains(manifest, field) {
			t.Errorf("Secret manifest missing field: %s", field)
		}
	}
}

func TestGenerateNamespaceManifest(t *testing.T) {
	manifest := GenerateNamespaceManifest("test-namespace")

	requiredFields := []string{
		"apiVersion: v1",
		"kind: Namespace",
		"name: test-namespace",
	}

	for _, field := range requiredFields {
		if !strings.Contains(manifest, field) {
			t.Errorf("Namespace manifest missing field: %s", field)
		}
	}
}

func TestGenerateVolumeMounts(t *testing.T) {
	cachePaths := []types.CachePath{
		{MountPath: "/nix/store", HostPath: ""},
		{MountPath: "/root/.cache", HostPath: ""},
	}

	result := generateVolumeMounts(cachePaths)

	if !strings.Contains(result, "/nix/store") {
		t.Error("Volume mounts missing /nix/store")
	}
	if !strings.Contains(result, "/root/.cache") {
		t.Error("Volume mounts missing /root/.cache")
	}
	if !strings.Contains(result, "/home/runner/_work") {
		t.Error("Volume mounts missing work directory")
	}
}

func TestGenerateVolumes(t *testing.T) {
	cachePaths := []types.CachePath{
		{MountPath: "/nix/store", HostPath: "/custom/path"},
		{MountPath: "/root/.cache", HostPath: ""},
	}

	result := generateVolumes(cachePaths, "test-installation")

	if !strings.Contains(result, "/custom/path") {
		t.Error("Volumes missing custom host path")
	}
	if !strings.Contains(result, "/tmp/github-runner-cache/test-installation/cache-1") {
		t.Error("Volumes missing auto-generated host path")
	}
	if !strings.Contains(result, "DirectoryOrCreate") {
		t.Error("Volumes missing DirectoryOrCreate type")
	}
}

func TestGenerateVolumes_Empty(t *testing.T) {
	result := generateVolumes([]types.CachePath{}, "test-installation")
	if result != "" {
		t.Errorf("Expected empty result for no cache paths, got: %s", result)
	}
}
