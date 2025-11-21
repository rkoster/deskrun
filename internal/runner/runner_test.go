package runner

import (
	"strings"
	"testing"

	"github.com/rkoster/deskrun/pkg/types"
)

func TestGenerateHelmValues_RepositoryLevel(t *testing.T) {
	tests := []struct {
		name            string
		installation    *types.RunnerInstallation
		instanceNum     int
		wantContains    string
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
			instanceNum:     0,
			wantNotContains: "runnerGroup:",
		},
		{
			name: "multi-instance setup should not include runnerGroup",
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
			instanceNum:     1,
			wantNotContains: "runnerGroup:",
		},
		{
			name: "runnerScaleSetName should be set to instance name",
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
			instanceNum:  3,
			wantContains: `runnerScaleSetName: "my-runner-3"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{}
			instanceName := tt.installation.Name
			if tt.instanceNum > 0 {
				instanceName = tt.installation.Name + "-" + string(rune('0'+tt.instanceNum))
			}
			got, err := m.generateHelmValues(tt.installation, instanceName, tt.instanceNum)
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

func TestGenerateHelmValues_NoRunnerGroupForRepoLevel(t *testing.T) {
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
		instanceName := installation.Name + "-" + string(rune('0'+i))
		val, err := m.generateHelmValues(installation, instanceName, i)
		if err != nil {
			t.Fatalf("generateHelmValues() instance %d error = %v", i, err)
		}
		values = append(values, val)
	}

	// Repository-level runners should never have runnerGroup set
	unexpectedGroup := "runnerGroup:"
	for i, val := range values {
		if strings.Contains(val, unexpectedGroup) {
			t.Errorf("Instance %d should not contain %q (runnerGroup is organization-only)\nGot:\n%s", i+1, unexpectedGroup, val)
		}
	}
}

func TestGenerateHelmValues_MinMaxRunners(t *testing.T) {
	tests := []struct {
		name           string
		minRunners     int
		maxRunners     int
		wantMinRunners string
		wantMaxRunners string
	}{
		{
			name:           "default min/max",
			minRunners:     1,
			maxRunners:     5,
			wantMinRunners: "minRunners: 1",
			wantMaxRunners: "maxRunners: 5",
		},
		{
			name:           "zero min runners",
			minRunners:     0,
			maxRunners:     10,
			wantMinRunners: "minRunners: 0",
			wantMaxRunners: "maxRunners: 10",
		},
		{
			name:           "high max runners",
			minRunners:     2,
			maxRunners:     100,
			wantMinRunners: "minRunners: 2",
			wantMaxRunners: "maxRunners: 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModeKubernetes,
				MinRunners:    tt.minRunners,
				MaxRunners:    tt.maxRunners,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
			}

			m := &Manager{}
			got, err := m.generateHelmValues(installation, "test-runner-1", 1)
			if err != nil {
				t.Fatalf("generateHelmValues() error = %v", err)
			}

			if !strings.Contains(got, tt.wantMinRunners) {
				t.Errorf("generateHelmValues() output does not contain %q\nGot:\n%s", tt.wantMinRunners, got)
			}

			if !strings.Contains(got, tt.wantMaxRunners) {
				t.Errorf("generateHelmValues() output does not contain %q\nGot:\n%s", tt.wantMaxRunners, got)
			}
		})
	}
}
