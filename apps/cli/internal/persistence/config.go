package persistence

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

const (
	detentDirName  = ".detent"
	configFileName = "config.yaml"
)

// GlobalConfig holds user-level settings for detent.
// Stored in ~/.detent/config.yaml
type GlobalConfig struct {
	Model           string  `yaml:"model,omitempty"`
	CostLimitUSD    float64 `yaml:"cost_limit_usd,omitempty"`
	AnthropicAPIKey string  `yaml:"anthropic_api_key,omitempty"`
}

// GetDetentDir returns the global detent directory path (~/.detent).
// This directory contains user configuration and can be shared with other components.
func GetDetentDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, detentDirName), nil
}

// GetConfigPath returns the path to the global config file (~/.detent/config.yaml).
func GetConfigPath() (string, error) {
	dir, err := GetDetentDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// LoadGlobalConfig loads the global configuration from ~/.detent/config.yaml.
// Returns an empty config if the file does not exist.
func LoadGlobalConfig() (*GlobalConfig, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// #nosec G304 - configPath is derived from user's home directory, not user input
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg GlobalConfig
	if unmarshalErr := yaml.Unmarshal(data, &cfg); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", unmarshalErr)
	}

	return &cfg, nil
}

// SaveGlobalConfig saves the global configuration to ~/.detent/config.yaml.
// Creates the ~/.detent directory if it does not exist.
func SaveGlobalConfig(cfg *GlobalConfig) error {
	dir, err := GetDetentDir()
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	// #nosec G301 - 0700 permissions are intentionally restrictive (owner-only)
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("failed to create config directory: %w", mkdirErr)
	}

	data, marshalErr := yaml.Marshal(cfg)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal config: %w", marshalErr)
	}

	configPath := filepath.Join(dir, configFileName)
	// #nosec G306 - 0600 permissions are intentionally restrictive (owner read/write only)
	if writeErr := os.WriteFile(configPath, data, 0o600); writeErr != nil {
		return fmt.Errorf("failed to write config file: %w", writeErr)
	}

	return nil
}
