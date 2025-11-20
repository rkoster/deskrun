package cmd

import (
	"testing"

	"github.com/rkoster/deskrun/pkg/types"
)

func TestValidateInstancesAndMaxRunners(t *testing.T) {
	tests := []struct {
		name          string
		instances     int
		maxRunners    int
		containerMode types.ContainerMode
		expectError   bool
		errorContains string
	}{
		{
			name:          "valid: instances=1 maxRunners=5",
			instances:     1,
			maxRunners:    5,
			containerMode: types.ContainerModeKubernetes,
			expectError:   false,
		},
		{
			name:          "valid: instances=3 maxRunners=1",
			instances:     3,
			maxRunners:    1,
			containerMode: types.ContainerModeKubernetes,
			expectError:   false,
		},
		{
			name:          "invalid: instances=5 maxRunners=8",
			instances:     5,
			maxRunners:    8,
			containerMode: types.ContainerModeKubernetes,
			expectError:   true,
			errorContains: "cannot use --instances > 1 with --max-runners > 1",
		},
		{
			name:          "invalid: instances=2 maxRunners=3",
			instances:     2,
			maxRunners:    3,
			containerMode: types.ContainerModeKubernetes,
			expectError:   true,
			errorContains: "cannot use --instances > 1 with --max-runners > 1",
		},
		{
			name:          "valid: cached-privileged-kubernetes with maxRunners=1",
			instances:     3,
			maxRunners:    1,
			containerMode: types.ContainerModePrivileged,
			expectError:   false,
		},
		{
			name:          "invalid: cached-privileged-kubernetes with maxRunners>1",
			instances:     1,
			maxRunners:    5,
			containerMode: types.ContainerModePrivileged,
			expectError:   true,
			errorContains: "cached-privileged-kubernetes mode requires --max-runners=1",
		},
		{
			name:          "invalid: cached-privileged-kubernetes with instances>1 and maxRunners>1",
			instances:     3,
			maxRunners:    5,
			containerMode: types.ContainerModePrivileged,
			expectError:   true,
			errorContains: "cannot use --instances > 1 with --max-runners > 1",
		},
		{
			name:          "valid: dind mode with maxRunners>1",
			instances:     1,
			maxRunners:    5,
			containerMode: types.ContainerModeDinD,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the validation logic directly without saving to config
			err := validateAddParams(tt.instances, tt.maxRunners, tt.containerMode)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func modeToString(mode types.ContainerMode) string {
	switch mode {
	case types.ContainerModeKubernetes:
		return "kubernetes"
	case types.ContainerModePrivileged:
		return "cached-privileged-kubernetes"
	case types.ContainerModeDinD:
		return "dind"
	default:
		return "kubernetes"
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
