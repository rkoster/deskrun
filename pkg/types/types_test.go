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
			{Target: "/nix/store", Source: "/tmp/nix"},
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
			{Target: "/nix/store", Source: ""},
			{Target: "/var/lib/docker", Source: ""},
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

func TestMountTypeConstants(t *testing.T) {
	tests := []struct {
		name      string
		mountType MountType
		want      string
	}{
		{
			name:      "directory or create",
			mountType: MountTypeDirectoryOrCreate,
			want:      "DirectoryOrCreate",
		},
		{
			name:      "directory",
			mountType: MountTypeDirectory,
			want:      "Directory",
		},
		{
			name:      "socket",
			mountType: MountTypeSocket,
			want:      "Socket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.mountType) != tt.want {
				t.Errorf("MountType = %v, want %v", tt.mountType, tt.want)
			}
		})
	}
}

func TestMount(t *testing.T) {
	tests := []struct {
		name   string
		mount  Mount
		verify func(*testing.T, Mount)
	}{
		{
			name: "directory or create mount with auto-generated source",
			mount: Mount{
				Source: "",
				Target: "/nix/store",
				Type:   MountTypeDirectoryOrCreate,
			},
			verify: func(t *testing.T, m Mount) {
				if m.Target != "/nix/store" {
					t.Errorf("Target = %v, want /nix/store", m.Target)
				}
				if m.Type != MountTypeDirectoryOrCreate {
					t.Errorf("Type = %v, want DirectoryOrCreate", m.Type)
				}
			},
		},
		{
			name: "socket mount",
			mount: Mount{
				Source: "/var/run/docker.sock",
				Target: "/var/run/docker.sock",
				Type:   MountTypeSocket,
			},
			verify: func(t *testing.T, m Mount) {
				if m.Source != "/var/run/docker.sock" {
					t.Errorf("Source = %v, want /var/run/docker.sock", m.Source)
				}
				if m.Target != "/var/run/docker.sock" {
					t.Errorf("Target = %v, want /var/run/docker.sock", m.Target)
				}
				if m.Type != MountTypeSocket {
					t.Errorf("Type = %v, want Socket", m.Type)
				}
			},
		},
		{
			name: "directory mount with explicit source",
			mount: Mount{
				Source: "/host/path",
				Target: "/container/path",
				Type:   MountTypeDirectory,
			},
			verify: func(t *testing.T, m Mount) {
				if m.Source != "/host/path" {
					t.Errorf("Source = %v, want /host/path", m.Source)
				}
				if m.Target != "/container/path" {
					t.Errorf("Target = %v, want /container/path", m.Target)
				}
				if m.Type != MountTypeDirectory {
					t.Errorf("Type = %v, want Directory", m.Type)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.mount)
		})
	}
}

func TestRunnerInstallationWithMounts(t *testing.T) {
	installation := &RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: ContainerModeKubernetes,
		MinRunners:    1,
		MaxRunners:    5,
		Instances:     1,
		Mounts: []Mount{
			{
				Source: "/var/run/docker.sock",
				Target: "/var/run/docker.sock",
				Type:   MountTypeSocket,
			},
			{
				Source: "/tmp/cache",
				Target: "/cache",
				Type:   MountTypeDirectoryOrCreate,
			},
		},
		AuthType:  AuthTypePAT,
		AuthValue: "ghp_test",
	}

	if installation.Name != "test-runner" {
		t.Errorf("Name = %v, want test-runner", installation.Name)
	}
	if len(installation.Mounts) != 2 {
		t.Errorf("Mounts length = %v, want 2", len(installation.Mounts))
	}
	if installation.Mounts[0].Type != MountTypeSocket {
		t.Errorf("First mount Type = %v, want Socket", installation.Mounts[0].Type)
	}
	if installation.Mounts[1].Type != MountTypeDirectoryOrCreate {
		t.Errorf("Second mount Type = %v, want DirectoryOrCreate", installation.Mounts[1].Type)
	}
}
