package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

const (
	detentDirName  = ".detent"
	configFileName = "config.yaml"
)

// GlobalConfig holds user-level settings for detent.
// Stored in ~/.detent/config.yaml
type GlobalConfig struct {
	AnthropicAPIKey string `yaml:"anthropic_api_key,omitempty"`

	// Heal settings
	Heal HealConfig `yaml:"heal,omitempty"`
}

// HealConfig contains settings for the heal command.
type HealConfig struct {
	Model       string  `yaml:"model,omitempty"`       // Model: claude-sonnet-4-5, claude-opus-4-5, claude-haiku-4-5
	TimeoutMins int     `yaml:"timeout_mins,omitempty"` // Total timeout in minutes (default: 10)
	BudgetUSD   float64 `yaml:"budget_usd,omitempty"`   // Max spend per run in USD (default: 1.00, 0 = unlimited)
	Verbose     bool    `yaml:"verbose,omitempty"`      // Show tool calls as they happen
}

// HealConfig validation bounds.
const (
	minTimeoutMins = 1
	maxTimeoutMins = 60
	minBudgetUSD   = 0.0
	maxBudgetUSD   = 100.0

	defaultBudgetUSD = 1.00

	modelPrefix = "claude-"
)

// DefaultHealConfig returns the default heal configuration.
func DefaultHealConfig() HealConfig {
	return HealConfig{
		Model:       "claude-sonnet-4-5",
		TimeoutMins: 10,
		BudgetUSD:   defaultBudgetUSD,
		Verbose:     false,
	}
}

// WithDefaults returns a HealConfig with defaults applied for zero/invalid values.
// Values outside valid bounds are clamped to the nearest bound.
// Invalid model names are reset to the default.
func (h HealConfig) WithDefaults() HealConfig {
	defaults := DefaultHealConfig()
	if h.Model == "" || !strings.HasPrefix(h.Model, modelPrefix) {
		h.Model = defaults.Model
	}
	h.TimeoutMins = clampInt(h.TimeoutMins, minTimeoutMins, maxTimeoutMins, defaults.TimeoutMins)
	h.BudgetUSD = clampFloat(h.BudgetUSD, minBudgetUSD, maxBudgetUSD, defaults.BudgetUSD)
	// Verbose is a bool, no clamping needed
	return h
}

// clampFloat clamps a value to [minVal, maxVal] range.
// 0 is preserved (means unlimited budget).
// Negative values are clamped to 0.
func clampFloat(value, minVal, maxVal, _ float64) float64 {
	if value < 0 {
		return 0
	}
	return max(minVal, min(value, maxVal))
}

// clampInt clamps a value to [minVal, maxVal] range, using defaultVal if value is <= 0.
func clampInt(value, minVal, maxVal, defaultVal int) int {
	if value <= 0 {
		return defaultVal
	}
	return max(minVal, min(value, maxVal))
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
// If the file does not exist, creates it with default values.
func LoadGlobalConfig() (*GlobalConfig, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// #nosec G304 - configPath is derived from user's home directory, not user input
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create config with defaults
			cfg := &GlobalConfig{
				Heal: DefaultHealConfig(),
			}
			// Try to save it (ignore errors - config dir might not exist yet)
			_ = SaveGlobalConfig(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg GlobalConfig
	if unmarshalErr := yaml.Unmarshal(data, &cfg); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", unmarshalErr)
	}

	return &cfg, nil
}

// ResolveAPIKey returns the API key from config or environment.
// Config takes precedence (if user explicitly sets it, respect that).
// Returns empty string if no key is found.
func ResolveAPIKey(configKey string) string {
	if configKey != "" {
		return configKey
	}
	return os.Getenv("ANTHROPIC_API_KEY")
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
