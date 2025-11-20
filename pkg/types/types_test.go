package types

import "testing"

func TestContainerModeConstants(t *testing.T) {
	tests := []struct {
		name string
		mode ContainerMode
		want string
	}{
		{
			name: "kubernetes mode",
			mode: ContainerModeKubernetes,
			want: "kubernetes",
		},
		{
			name: "dind mode",
			mode: ContainerModeDinD,
			want: "dind",
		},
		{
			name: "privileged mode",
			mode: ContainerModePrivileged,
			want: "cached-privileged-kubernetes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.mode) != tt.want {
				t.Errorf("ContainerMode = %v, want %v", tt.mode, tt.want)
			}
		})
	}
}

func TestAuthTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		authType AuthType
		want     string
	}{
		{
			name:     "github app auth",
			authType: AuthTypeGitHubApp,
			want:     "github-app",
		},
		{
			name:     "pat auth",
			authType: AuthTypePAT,
			want:     "pat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.authType) != tt.want {
				t.Errorf("AuthType = %v, want %v", tt.authType, tt.want)
			}
		})
	}
}

func TestRunnerInstallation(t *testing.T) {
	installation := &RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: ContainerModeKubernetes,
		MinRunners:    1,
		MaxRunners:    5,
		Instances:     1,
		CachePaths: []CachePath{
			{MountPath: "/nix/store", HostPath: "/tmp/nix"},
		},
		AuthType:  AuthTypePAT,
		AuthValue: "ghp_test",
	}

	if installation.Name != "test-runner" {
		t.Errorf("Name = %v, want test-runner", installation.Name)
	}
	if installation.ContainerMode != ContainerModeKubernetes {
		t.Errorf("ContainerMode = %v, want kubernetes", installation.ContainerMode)
	}
	if installation.Instances != 1 {
		t.Errorf("Instances = %v, want 1", installation.Instances)
	}
	if len(installation.CachePaths) != 1 {
		t.Errorf("CachePaths length = %v, want 1", len(installation.CachePaths))
	}
}

func TestRunnerInstallationMultipleInstances(t *testing.T) {
	installation := &RunnerInstallation{
		Name:          "multi-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: ContainerModePrivileged,
		MinRunners:    0,
		MaxRunners:    1,
		Instances:     3,
		CachePaths: []CachePath{
			{MountPath: "/nix/store", HostPath: ""},
			{MountPath: "/var/lib/docker", HostPath: ""},
		},
		AuthType:  AuthTypePAT,
		AuthValue: "ghp_test",
	}

	if installation.Instances != 3 {
		t.Errorf("Instances = %v, want 3", installation.Instances)
	}
	if len(installation.CachePaths) != 2 {
		t.Errorf("CachePaths length = %v, want 2", len(installation.CachePaths))
	}
	if installation.MaxRunners != 1 {
		t.Errorf("MaxRunners = %v, want 1 for multi-instance setup", installation.MaxRunners)
	}
}

func TestClusterConfig(t *testing.T) {
	config := &ClusterConfig{
		Name:    "test-cluster",
		Network: "test-network",
	}

	if config.Name != "test-cluster" {
		t.Errorf("Name = %v, want test-cluster", config.Name)
	}
	if config.Network != "test-network" {
		t.Errorf("Network = %v, want test-network", config.Network)
	}
}
