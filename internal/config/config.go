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

	// First, try to unmarshal into a temporary structure that can handle both old and new formats
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Check if we need to migrate from old format
	needsMigration := false
	if installations, ok := rawConfig["installations"].(map[string]interface{}); ok {
		for _, installation := range installations {
			if instMap, ok := installation.(map[string]interface{}); ok {
				if cachePaths, ok := instMap["CachePaths"].([]interface{}); ok {
					for _, cachePath := range cachePaths {
						if cpMap, ok := cachePath.(map[string]interface{}); ok {
							// Check if it has old field names
							if _, hasMountPath := cpMap["MountPath"]; hasMountPath {
								needsMigration = true
								break
							}
							if _, hasHostPath := cpMap["HostPath"]; hasHostPath {
								needsMigration = true
								break
							}
						}
					}
				}
			}
			if needsMigration {
				break
			}
		}
	}

	if needsMigration {
		// Migrate old format to new format
		if err := m.migrateConfig(data); err != nil {
			return fmt.Errorf("failed to migrate config: %w", err)
		}
		// Save the migrated config
		return m.Save()
	}

	// Parse as new format
	m.config = &Config{}
	if err := json.Unmarshal(data, m.config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if m.config.Installations == nil {
		m.config.Installations = make(map[string]*types.RunnerInstallation)
	}

	return nil
}

// migrateConfig migrates old config format to new format
func (m *Manager) migrateConfig(data []byte) error {
	// Define old CachePath structure
	type OldCachePath struct {
		MountPath string `json:"MountPath"`
		HostPath  string `json:"HostPath"`
	}

	// Define old RunnerInstallation structure
	type OldRunnerInstallation struct {
		Name          string              `json:"Name"`
		Repository    string              `json:"Repository"`
		ContainerMode types.ContainerMode `json:"ContainerMode"`
		MinRunners    int                 `json:"MinRunners"`
		MaxRunners    int                 `json:"MaxRunners"`
		Instances     int                 `json:"Instances"`
		CachePaths    []OldCachePath      `json:"CachePaths"`
		AuthType      types.AuthType      `json:"AuthType"`
		AuthValue     string              `json:"AuthValue"`
	}

	// Define old Config structure
	type OldConfig struct {
		ClusterName   string                            `json:"cluster_name"`
		Installations map[string]*OldRunnerInstallation `json:"installations"`
	}

	// Parse as old format
	var oldConfig OldConfig
	if err := json.Unmarshal(data, &oldConfig); err != nil {
		return fmt.Errorf("failed to parse old config format: %w", err)
	}

	// Convert to new format
	m.config = &Config{
		ClusterName:   oldConfig.ClusterName,
		Installations: make(map[string]*types.RunnerInstallation),
	}

	for name, oldInstallation := range oldConfig.Installations {
		// Convert cache paths from old to new format
		var newCachePaths []types.CachePath
		for _, oldPath := range oldInstallation.CachePaths {
			newCachePaths = append(newCachePaths, types.CachePath{
				Target: oldPath.MountPath,
				Source: oldPath.HostPath,
			})
		}

		// Create new installation
		m.config.Installations[name] = &types.RunnerInstallation{
			Name:          oldInstallation.Name,
			Repository:    oldInstallation.Repository,
			ContainerMode: oldInstallation.ContainerMode,
			MinRunners:    oldInstallation.MinRunners,
			MaxRunners:    oldInstallation.MaxRunners,
			Instances:     oldInstallation.Instances,
			CachePaths:    newCachePaths,
			AuthType:      oldInstallation.AuthType,
			AuthValue:     oldInstallation.AuthValue,
		}
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
