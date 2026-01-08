package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rkoster/deskrun/pkg/types"
)

func TestNewManager(t *testing.T) {
	// Create temporary home directory
	tmpHome, err := os.MkdirTemp("", "deskrun-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpHome)
	})

	// Set HOME environment variable
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if mgr.config == nil {
		t.Error("config is nil")
	}

	if mgr.config.ClusterName != "deskrun" {
		t.Errorf("ClusterName = %v, want deskrun", mgr.config.ClusterName)
	}

	if mgr.config.Installations == nil {
		t.Error("Installations map is nil")
	}
}

func TestAddInstallation(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "deskrun-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpHome)
	})

	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: types.ContainerModeKubernetes,
		MinRunners:    1,
		MaxRunners:    5,
		AuthType:      types.AuthTypePAT,
		AuthValue:     "test-token",
	}

	err = mgr.AddInstallation(installation)
	if err != nil {
		t.Fatalf("AddInstallation() error = %v", err)
	}

	// Verify installation was added
	saved, err := mgr.GetInstallation("test-runner")
	if err != nil {
		t.Fatalf("GetInstallation() error = %v", err)
	}

	if saved.Name != "test-runner" {
		t.Errorf("Name = %v, want test-runner", saved.Name)
	}

	// Try adding duplicate
	err = mgr.AddInstallation(installation)
	if err == nil {
		t.Error("AddInstallation() expected error for duplicate, got nil")
	}
}

func TestRemoveInstallation(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "deskrun-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpHome)
	})

	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: types.ContainerModeKubernetes,
		MinRunners:    1,
		MaxRunners:    5,
		AuthType:      types.AuthTypePAT,
		AuthValue:     "test-token",
	}

	err = mgr.AddInstallation(installation)
	if err != nil {
		t.Fatalf("AddInstallation() error = %v", err)
	}

	err = mgr.RemoveInstallation("test-runner")
	if err != nil {
		t.Fatalf("RemoveInstallation() error = %v", err)
	}

	// Verify removal
	_, err = mgr.GetInstallation("test-runner")
	if err == nil {
		t.Error("GetInstallation() expected error after removal, got nil")
	}

	// Try removing non-existent
	err = mgr.RemoveInstallation("non-existent")
	if err == nil {
		t.Error("RemoveInstallation() expected error for non-existent, got nil")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "deskrun-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpHome)
	})

	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})

	// Create and save config
	mgr1, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	installation := &types.RunnerInstallation{
		Name:          "test-runner",
		Repository:    "https://github.com/owner/repo",
		ContainerMode: types.ContainerModeKubernetes,
		MinRunners:    1,
		MaxRunners:    5,
		AuthType:      types.AuthTypePAT,
		AuthValue:     "test-token",
	}

	err = mgr1.AddInstallation(installation)
	if err != nil {
		t.Fatalf("AddInstallation() error = %v", err)
	}

	// Create new manager and verify it loads the saved config
	mgr2, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	saved, err := mgr2.GetInstallation("test-runner")
	if err != nil {
		t.Fatalf("GetInstallation() error = %v", err)
	}

	if saved.Name != "test-runner" {
		t.Errorf("Name = %v, want test-runner", saved.Name)
	}

	// Verify config file exists
	configPath := filepath.Join(tmpHome, ".deskrun", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}
func TestAddClusterHost(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "deskrun-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpHome)
	})

	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	host := &types.ClusterHost{
		Name:      "test-host",
		Image:     "images:nixos/25.11",
		DiskSize:  "200GiB",
		CreatedAt: "2026-01-08T00:00:00Z",
	}

	err = mgr.AddClusterHost(host)
	if err != nil {
		t.Fatalf("AddClusterHost() error = %v", err)
	}

	saved, err := mgr.GetClusterHost("test-host")
	if err != nil {
		t.Fatalf("GetClusterHost() error = %v", err)
	}

	if saved.Name != "test-host" {
		t.Errorf("Name = %v, want test-host", saved.Name)
	}

	err = mgr.AddClusterHost(host)
	if err == nil {
		t.Error("AddClusterHost() expected error for duplicate, got nil")
	}
}

func TestRemoveClusterHost(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "deskrun-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpHome)
	})

	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	host := &types.ClusterHost{
		Name:      "test-host",
		Image:     "images:nixos/25.11",
		DiskSize:  "200GiB",
		CreatedAt: "2026-01-08T00:00:00Z",
	}

	err = mgr.AddClusterHost(host)
	if err != nil {
		t.Fatalf("AddClusterHost() error = %v", err)
	}

	err = mgr.RemoveClusterHost("test-host")
	if err != nil {
		t.Fatalf("RemoveClusterHost() error = %v", err)
	}

	_, err = mgr.GetClusterHost("test-host")
	if err == nil {
		t.Error("GetClusterHost() expected error after removal, got nil")
	}

	err = mgr.RemoveClusterHost("non-existent")
	if err == nil {
		t.Error("RemoveClusterHost() expected error for non-existent, got nil")
	}
}

func TestGetClusterHost(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "deskrun-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpHome)
	})

	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = mgr.GetClusterHost("non-existent")
	if err == nil {
		t.Error("GetClusterHost() expected error for non-existent host, got nil")
	}

	host := &types.ClusterHost{
		Name:      "test-host",
		Image:     "images:nixos/25.11",
		DiskSize:  "200GiB",
		CreatedAt: "2026-01-08T00:00:00Z",
	}

	err = mgr.AddClusterHost(host)
	if err != nil {
		t.Fatalf("AddClusterHost() error = %v", err)
	}

	retrieved, err := mgr.GetClusterHost("test-host")
	if err != nil {
		t.Fatalf("GetClusterHost() error = %v", err)
	}

	if retrieved.Name != host.Name {
		t.Errorf("Name = %v, want %v", retrieved.Name, host.Name)
	}
	if retrieved.Image != host.Image {
		t.Errorf("Image = %v, want %v", retrieved.Image, host.Image)
	}
	if retrieved.DiskSize != host.DiskSize {
		t.Errorf("DiskSize = %v, want %v", retrieved.DiskSize, host.DiskSize)
	}
	if retrieved.CreatedAt != host.CreatedAt {
		t.Errorf("CreatedAt = %v, want %v", retrieved.CreatedAt, host.CreatedAt)
	}
}
