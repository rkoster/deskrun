package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rkoster/deskrun/pkg/types"
)

const (
	configDirName  = ".deskrun"
	configFileName = "config.json"
)

// Config represents the deskrun configuration
type Config struct {
	ClusterName   string                               `json:"cluster_name"`
	Installations map[string]*types.RunnerInstallation `json:"installations"`
}

// Manager handles configuration persistence
type Manager struct {
	configPath string
	config     *Config
}

// NewManager creates a new configuration manager
func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, configDirName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, configFileName)

	m := &Manager{
		configPath: configPath,
	}

	if err := m.Load(); err != nil {
		// If config doesn't exist, initialize with empty config
		if os.IsNotExist(err) {
			m.config = &Config{
				ClusterName:   "deskrun",
				Installations: make(map[string]*types.RunnerInstallation),
			}
			return m, nil
		}
		return nil, err
	}

	return m, nil
}

// Load loads the configuration from disk
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}

	m.config = &Config{}
	if err := json.Unmarshal(data, m.config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if m.config.Installations == nil {
		m.config.Installations = make(map[string]*types.RunnerInstallation)
	}

	return nil
}

// Save saves the configuration to disk
func (m *Manager) Save() error {
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// AddInstallation adds a runner installation to the config
func (m *Manager) AddInstallation(installation *types.RunnerInstallation) error {
	if m.config.Installations[installation.Name] != nil {
		return fmt.Errorf("installation %s already exists", installation.Name)
	}

	m.config.Installations[installation.Name] = installation
	return m.Save()
}

// RemoveInstallation removes a runner installation from the config
func (m *Manager) RemoveInstallation(name string) error {
	if m.config.Installations[name] == nil {
		return fmt.Errorf("installation %s does not exist", name)
	}

	delete(m.config.Installations, name)
	return m.Save()
}

// GetInstallation gets a runner installation by name
func (m *Manager) GetInstallation(name string) (*types.RunnerInstallation, error) {
	installation := m.config.Installations[name]
	if installation == nil {
		return nil, fmt.Errorf("installation %s not found", name)
	}
	return installation, nil
}
