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

func (m *Manager) CreateContainer(ctx context.Context, name, image, diskSize, storagePool string) error {
	if name == "" {
		return fmt.Errorf("container name cannot be empty")
	}
	if strings.ContainsAny(name, " /\\:@#$%^&*()[]{}!?'\"<>,;|`~+=") {
		return fmt.Errorf("container name contains invalid characters: %s", name)
	}
	if image == "" {
		return fmt.Errorf("image cannot be empty")
	}
	if diskSize == "" {
		return fmt.Errorf("disk size cannot be empty")
	}
	if !strings.HasSuffix(diskSize, "GiB") && !strings.HasSuffix(diskSize, "GB") &&
		!strings.HasSuffix(diskSize, "MiB") && !strings.HasSuffix(diskSize, "MB") {
		return fmt.Errorf("disk size must end with GiB, GB, MiB, or MB: %s", diskSize)
	}

	// Ensure the default bridge network exists
	if err := m.ensureNetwork(ctx); err != nil {
		return fmt.Errorf("failed to ensure network: %w", err)
	}

	args := []string{
		"launch",
		image,
		name,
		"-d", fmt.Sprintf("root,size=%s", diskSize),
		"-n", "incusbr0",
		"-c", "security.nesting=true",
		"-c", "security.privileged=true",
	}

	// Add storage pool if specified
	if storagePool != "" {
		args = append(args, "-s", storagePool)
	}

	cmd := exec.CommandContext(ctx, "incus", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create container: %w (output: %s)", err, string(output))
	}

	// Add /dev/kmsg device for kubelet
	addDeviceCmd := exec.CommandContext(ctx, "incus", "config", "device", "add", name, "kmsg", "unix-char", "source=/dev/kmsg", "path=/dev/kmsg")
	output, err = addDeviceCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add kmsg device: %w (output: %s)", err, string(output))
	}

	return nil
}

func (m *Manager) ensureNetwork(ctx context.Context) error {
	// Check if incusbr0 network exists
	cmd := exec.CommandContext(ctx, "incus", "network", "show", "incusbr0")
	if err := cmd.Run(); err == nil {
		return nil // Network already exists
	}

	// Create the bridge network
	cmd = exec.CommandContext(ctx, "incus", "network", "create", "incusbr0", "--type=bridge")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create network: %w (output: %s)", err, string(output))
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

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return false, nil
	}

	lines := strings.Split(outputStr, "\n")
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
	// Use stdin redirection to avoid shell injection vulnerabilities
	cmd := exec.CommandContext(ctx, "incus", "exec", container, "--", "sh", "-c", `cat > "$1"`, "sh", remotePath)
	cmd.Stdin = strings.NewReader(content)
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

func (m *Manager) WaitForNetwork(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try to ping a well-known DNS server to check network connectivity
		_, err := m.Exec(ctx, name, "timeout", "2", "ping", "-c", "1", "1.1.1.1")
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return fmt.Errorf("timeout waiting for network connectivity")
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

func (m *Manager) PushConfigFile(ctx context.Context, containerName, configPath string) error {
	// Create .deskrun directory in container
	if _, err := m.Exec(ctx, containerName, "mkdir", "-p", "/root/.deskrun"); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Use incus file push to copy the config file
	cmd := exec.CommandContext(ctx, "incus", "file", "push", configPath, fmt.Sprintf("%s/root/.deskrun/config.json", containerName))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push config file: %w (output: %s)", err, string(output))
	}

	return nil
}

// ListStoragePools returns a list of available storage pool names
func (m *Manager) ListStoragePools(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "incus", "storage", "list", "--format=csv", "-c", "n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list storage pools: %w (output: %s)", err, string(output))
	}

	var pools []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		pools = append(pools, strings.TrimSpace(line))
	}

	return pools, nil
}

// DetectStorageDriver returns the driver type for a given storage pool
func (m *Manager) DetectStorageDriver(ctx context.Context, poolName string) (string, error) {
	cmd := exec.CommandContext(ctx, "incus", "storage", "show", poolName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to show storage pool: %w (output: %s)", err, string(output))
	}

	// Parse the output to find the driver line
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "driver:") {
			driver := strings.TrimSpace(strings.TrimPrefix(line, "driver:"))
			return driver, nil
		}
	}

	return "", fmt.Errorf("could not find driver in storage pool info")
}

// CreateStoragePool creates a new storage pool
func (m *Manager) CreateStoragePool(ctx context.Context, name, driver, size string) error {
	args := []string{"storage", "create", name, driver}
	
	// Add size parameter for zfs and btrfs
	if size != "" && (driver == "zfs" || driver == "btrfs") {
		args = append(args, fmt.Sprintf("size=%s", size))
	}

	cmd := exec.CommandContext(ctx, "incus", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create storage pool: %w (output: %s)", err, string(output))
	}

	return nil
}

// EnsureGoodStoragePool ensures a storage pool suitable for Docker workloads exists
// Returns the name of the storage pool to use
func (m *Manager) EnsureGoodStoragePool(ctx context.Context) (string, error) {
	pools, err := m.ListStoragePools(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list storage pools: %w", err)
	}

	// First pass: Look for existing good pools (zfs or dir)
	for _, pool := range pools {
		driver, err := m.DetectStorageDriver(ctx, pool)
		if err != nil {
			continue // Skip pools we can't inspect
		}

		if driver == "zfs" || driver == "dir" {
			return pool, nil
		}
	}

	// Second pass: Check if default pool is good enough
	for _, pool := range pools {
		if pool == "default" {
			driver, err := m.DetectStorageDriver(ctx, pool)
			if err == nil && driver != "btrfs" {
				// Default pool is not btrfs, so it's probably okay
				return pool, nil
			}
		}
	}

	// No suitable pool found, create one
	// Prefer ZFS, fallback to dir if ZFS creation fails
	poolName := "deskrun-pool"
	
	// Try ZFS first
	err = m.CreateStoragePool(ctx, poolName, "zfs", "100GB")
	if err == nil {
		return poolName, nil
	}

	// ZFS failed, try dir as fallback
	err = m.CreateStoragePool(ctx, poolName, "dir", "")
	if err != nil {
		return "", fmt.Errorf("failed to create storage pool (tried zfs and dir): %w", err)
	}

	return poolName, nil
}
