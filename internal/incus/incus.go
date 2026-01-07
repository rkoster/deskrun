package incus

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Manager struct{}

type ContainerInfo struct {
	Name   string
	Status string
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) CreateContainer(ctx context.Context, name, image, diskSize string) error {
	args := []string{
		"launch",
		image,
		name,
		"-d", fmt.Sprintf("root,size=%s", diskSize),
		"-c", "security.nesting=true",
	}

	cmd := exec.CommandContext(ctx, "incus", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create container: %w (output: %s)", err, string(output))
	}

	return nil
}

func (m *Manager) DeleteContainer(ctx context.Context, name string) error {
	running, err := m.isRunning(ctx, name)
	if err != nil {
		return err
	}

	if running {
		stopCmd := exec.CommandContext(ctx, "incus", "stop", name, "--force")
		if output, err := stopCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to stop container: %w (output: %s)", err, string(output))
		}
	}

	deleteCmd := exec.CommandContext(ctx, "incus", "delete", name)
	output, err := deleteCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete container: %w (output: %s)", err, string(output))
	}

	return nil
}

func (m *Manager) ContainerExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "incus", "list", "--format=csv", "-c", "n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to list containers: %w (output: %s)", err, string(output))
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}

	return false, nil
}

func (m *Manager) ListContainers(ctx context.Context, prefix string) ([]ContainerInfo, error) {
	cmd := exec.CommandContext(ctx, "incus", "list", "--format=csv", "-c", "ns")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w (output: %s)", err, string(output))
	}

	var containers []ContainerInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		status := strings.TrimSpace(parts[1])

		if prefix == "" || strings.HasPrefix(name, prefix) {
			containers = append(containers, ContainerInfo{
				Name:   name,
				Status: status,
			})
		}
	}

	return containers, nil
}

func (m *Manager) PushContent(ctx context.Context, container, content, remotePath string) error {
	cmd := exec.CommandContext(ctx, "incus", "exec", container, "--", "sh", "-c",
		fmt.Sprintf("cat > %s <<'EOF'\n%s\nEOF", remotePath, content))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push content: %w (output: %s)", err, string(output))
	}

	return nil
}

func (m *Manager) Exec(ctx context.Context, container string, command ...string) (string, error) {
	args := append([]string{"exec", container, "--"}, command...)
	cmd := exec.CommandContext(ctx, "incus", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to execute command: %w (output: %s)", err, string(output))
	}

	return string(output), nil
}

func (m *Manager) WaitForRunning(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		running, err := m.isRunning(ctx, name)
		if err != nil {
			return err
		}

		if running {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return fmt.Errorf("timeout waiting for container to be running")
}

func (m *Manager) isRunning(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "incus", "list", name, "--format=csv", "-c", "s")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to check container status: %w (output: %s)", err, string(output))
	}

	status := strings.TrimSpace(string(output))
	return status == "RUNNING", nil
}
