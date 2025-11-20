package runner

import (
	"strings"
	"testing"

	"github.com/rkoster/deskrun/pkg/types"
)

func TestGenerateHelmValues_RunnerGroup(t *testing.T) {
	tests := []struct {
		name        string
		installation *types.RunnerInstallation
		instanceNum int
		wantContains string
		wantNotContains string
	}{
		{
			name: "single instance should not include runnerGroup",
			installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModeKubernetes,
				MinRunners:    1,
				MaxRunners:    1,
				Instances:     1,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
			},
			instanceNum: 0,
			wantNotContains: "runnerGroup:",
		},
		{
			name: "multiple instances should include runnerGroup with base name",
			installation: &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModeKubernetes,
				MinRunners:    1,
				MaxRunners:    1,
				Instances:     3,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
			},
			instanceNum: 1,
			wantContains: `runnerGroup: "test-runner"`,
		},
		{
			name: "multiple instances second instance should have same runnerGroup",
			installation: &types.RunnerInstallation{
				Name:          "my-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModePrivileged,
				MinRunners:    1,
				MaxRunners:    1,
				Instances:     5,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
			},
			instanceNum: 3,
			wantContains: `runnerGroup: "my-runner"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{}
			got, err := m.generateHelmValues(tt.installation, tt.instanceNum)
			if err != nil {
				t.Fatalf("generateHelmValues() error = %v", err)
			}

			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("generateHelmValues() output does not contain %q\nGot:\n%s", tt.wantContains, got)
			}

			if tt.wantNotContains != "" && strings.Contains(got, tt.wantNotContains) {
				t.Errorf("generateHelmValues() output should not contain %q\nGot:\n%s", tt.wantNotContains, got)
			}
		})
	}
}

func TestGenerateHelmValues_AllInstancesSameGroup(t *testing.T) {
	installation := &types.RunnerInstallation{
		Name:          "shared-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: types.ContainerModeKubernetes,
		MinRunners:    1,
		MaxRunners:    1,
		Instances:     3,
		AuthType:      types.AuthTypePAT,
		AuthValue:     "test-token",
	}

	m := &Manager{}
	
	// Generate values for all instances
	var values []string
	for i := 1; i <= installation.Instances; i++ {
		val, err := m.generateHelmValues(installation, i)
		if err != nil {
			t.Fatalf("generateHelmValues() instance %d error = %v", i, err)
		}
		values = append(values, val)
	}

	// All instances should have the same runnerGroup
	expectedGroup := `runnerGroup: "shared-runner"`
	for i, val := range values {
		if !strings.Contains(val, expectedGroup) {
			t.Errorf("Instance %d does not contain expected runnerGroup %q\nGot:\n%s", i+1, expectedGroup, val)
		}
	}
}
