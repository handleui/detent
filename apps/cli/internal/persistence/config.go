package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/detent/cli/schema"
)

// --- File paths ---

const (
	detentDirName    = ".detent"
	globalConfigFile = "config.json"
	localConfigFile  = "detent.json"

	// SchemaURL is the JSON Schema reference for config files.
	// Uses relative path since schema is written alongside config.
	SchemaURL = "./schema.json"

	// DetentHomeEnv overrides ~/.detent for testing.
	DetentHomeEnv = "DETENT_HOME"
)

// cachedDetentDir stores the computed detent directory to avoid repeated os.UserHomeDir calls.
var (
	cachedDetentDir   string
	cachedDetentDirMu sync.RWMutex
)

// --- Structs ---

// TrustedRepo stores trust information for a repository.
// The key in TrustedRepos map is the first commit SHA (immutable identifier).
type TrustedRepo struct {
	RemoteURL string    `json:"remote_url,omitempty"`
	TrustedAt time.Time `json:"trusted_at"`
}

// GlobalConfig is the user's global settings (~/.detent/config.json).
// This is the raw structure that gets persisted to disk.
type GlobalConfig struct {
	Schema           string                 `json:"$schema,omitempty"`
	APIKey           string                 `json:"api_key,omitempty"`
	Model            string                 `json:"model,omitempty"`
	BudgetPerRunUSD  *float64               `json:"budget_per_run_usd,omitempty"`
	BudgetMonthlyUSD *float64               `json:"budget_monthly_usd,omitempty"`
	TimeoutMins      *int                   `json:"timeout_mins,omitempty"`
	TrustedRepos     map[string]TrustedRepo `json:"trusted_repos,omitempty"`
	SpendHistory     map[string]float64     `json:"spend_history,omitempty"` // key is YYYY-MM
}

// LocalConfig is per-repository settings (detent.json in repo root).
// This overrides global config for the specific project.
type LocalConfig struct {
	Schema          string   `json:"$schema,omitempty"`
	Model           string   `json:"model,omitempty"`
	BudgetPerRunUSD *float64 `json:"budget_per_run_usd,omitempty"`
	TimeoutMins     *int     `json:"timeout_mins,omitempty"`
	Commands        []string `json:"commands,omitempty"` // Extra allowed commands
}

// Config is the merged, resolved config used by the application.
// Values are resolved from: env var > local config > global config > defaults.
type Config struct {
	// Resolved settings
	APIKey           string
	Model            string
	BudgetPerRunUSD  float64
	BudgetMonthlyUSD float64
	TimeoutMins      int

	// Aggregated allowlists (from local config)
	ExtraCommands []string

	// Internal references for mutation
	global   *GlobalConfig
	local    *LocalConfig
	repoRoot string // For saving
}

// --- Defaults ---

const (
	// DefaultModel is the default Claude model for AI healing.
	DefaultModel = "claude-sonnet-4-5"
	// DefaultBudgetPerRunUSD is the default max spend per healing run.
	DefaultBudgetPerRunUSD = 1.00
	// DefaultTimeoutMins is the default max time per healing run.
	DefaultTimeoutMins = 10

	minTimeoutMins     = 1
	maxTimeoutMins     = 60
	minBudgetUSD       = 0.0
	maxBudgetUSD       = 100.0
	maxBudgetMonthlyUSD = 1000.0
	modelPrefix        = "claude-"
)

// --- Value Source Tracking ---

// ValueSource indicates where a configuration value originated.
type ValueSource int

// Value sources indicate where configuration values originated.
const (
	SourceDefault ValueSource = iota // SourceDefault indicates the value is a hardcoded default.
	SourceGlobal                     // SourceGlobal indicates the value comes from ~/.detent/config.json.
	SourceLocal                      // SourceLocal indicates the value comes from detent.json.
	SourceEnv                        // SourceEnv indicates the value comes from an environment variable.
)

// String returns the display name for a value source.
func (s ValueSource) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceGlobal:
		return "global"
	case SourceLocal:
		return "local"
	case SourceEnv:
		return "env"
	}
	return "unknown"
}

// ConfigValue holds a resolved value with its source.
type ConfigValue[T any] struct {
	Value  T
	Source ValueSource
}

// ConfigWithSources provides resolved values with source information.
// Used by the TUI to show where each value came from.
type ConfigWithSources struct {
	APIKey           ConfigValue[string]
	Model            ConfigValue[string]
	BudgetPerRunUSD  ConfigValue[float64]
	BudgetMonthlyUSD ConfigValue[float64]
	TimeoutMins      ConfigValue[int]
	MonthlySpend     float64

	// Read-only (local config only)
	ExtraCommands []string

	// Internal references
	Global   *GlobalConfig
	Local    *LocalConfig
	RepoRoot string
}

// --- Path helpers ---

// GetDetentDir returns the global detent directory path (~/.detent).
// If DETENT_HOME is set, uses that instead (for testing).
// Results are cached to avoid repeated os.UserHomeDir calls.
// This function is safe for concurrent use.
func GetDetentDir() (string, error) {
	// DETENT_HOME override always checked (allows dynamic test changes)
	if override := os.Getenv(DetentHomeEnv); override != "" {
		return filepath.Clean(override), nil
	}

	// Check cache with read lock first
	cachedDetentDirMu.RLock()
	cached := cachedDetentDir
	cachedDetentDirMu.RUnlock()
	if cached != "" {
		return cached, nil
	}

	// Compute and cache with write lock
	cachedDetentDirMu.Lock()
	defer cachedDetentDirMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have set it)
	if cachedDetentDir != "" {
		return cachedDetentDir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	cachedDetentDir = filepath.Join(home, detentDirName)
	return cachedDetentDir, nil
}

// GetConfigPath returns the path to the global config file.
func GetConfigPath() (string, error) {
	dir, err := GetDetentDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, globalConfigFile), nil
}

// --- Loading ---

// Load loads global + local config, merges them, and returns the resolved Config.
// repoRoot is the directory to look for detent.json (pass "" for global-only).
func Load(repoRoot string) (*Config, error) {
	global, err := loadGlobal()
	if err != nil {
		return nil, fmt.Errorf("global config: %w", err)
	}

	var local *LocalConfig
	if repoRoot != "" {
		local, err = loadLocal(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("local config: %w", err)
		}
	}

	return merge(global, local, repoRoot), nil
}

// LoadWithSources loads config and tracks the source of each value.
// Used by the TUI to display where values originated.
func LoadWithSources(repoRoot string) (*ConfigWithSources, error) {
	global, err := loadGlobal()
	if err != nil {
		return nil, fmt.Errorf("global config: %w", err)
	}

	var local *LocalConfig
	if repoRoot != "" {
		local, err = loadLocal(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("local config: %w", err)
		}
	}

	return mergeWithSources(global, local, repoRoot), nil
}

// mergeWithSources combines global and local config, tracking value sources.
func mergeWithSources(global *GlobalConfig, local *LocalConfig, repoRoot string) *ConfigWithSources {
	c := &ConfigWithSources{
		Model:            ConfigValue[string]{Value: DefaultModel, Source: SourceDefault},
		BudgetPerRunUSD:  ConfigValue[float64]{Value: DefaultBudgetPerRunUSD, Source: SourceDefault},
		BudgetMonthlyUSD: ConfigValue[float64]{Value: 0, Source: SourceDefault}, // 0 means unlimited
		TimeoutMins:      ConfigValue[int]{Value: DefaultTimeoutMins, Source: SourceDefault},
		APIKey:           ConfigValue[string]{Value: "", Source: SourceDefault},
		Global:           global,
		Local:            local,
		RepoRoot:         repoRoot,
	}

	// Apply global config
	if global != nil {
		if global.APIKey != "" {
			c.APIKey = ConfigValue[string]{Value: global.APIKey, Source: SourceGlobal}
		}
		if global.Model != "" {
			if strings.HasPrefix(global.Model, modelPrefix) {
				c.Model = ConfigValue[string]{Value: global.Model, Source: SourceGlobal}
			}
		}
		if global.BudgetPerRunUSD != nil {
			c.BudgetPerRunUSD = ConfigValue[float64]{
				Value:  clampBudget(*global.BudgetPerRunUSD),
				Source: SourceGlobal,
			}
		}
		if global.BudgetMonthlyUSD != nil {
			c.BudgetMonthlyUSD = ConfigValue[float64]{
				Value:  clampMonthlyBudget(*global.BudgetMonthlyUSD),
				Source: SourceGlobal,
			}
		}
		if global.TimeoutMins != nil {
			c.TimeoutMins = ConfigValue[int]{
				Value:  clampTimeout(*global.TimeoutMins),
				Source: SourceGlobal,
			}
		}
		// Calculate monthly spend from history
		c.MonthlySpend = global.SpendHistory[currentMonth()]
	}

	// Apply local config (overrides global for per-run budget)
	if local != nil {
		if local.Model != "" {
			if strings.HasPrefix(local.Model, modelPrefix) {
				c.Model = ConfigValue[string]{Value: local.Model, Source: SourceLocal}
			}
		}
		if local.BudgetPerRunUSD != nil {
			c.BudgetPerRunUSD = ConfigValue[float64]{
				Value:  clampBudget(*local.BudgetPerRunUSD),
				Source: SourceLocal,
			}
		}
		if local.TimeoutMins != nil {
			c.TimeoutMins = ConfigValue[int]{
				Value:  clampTimeout(*local.TimeoutMins),
				Source: SourceLocal,
			}
		}
		c.ExtraCommands = local.Commands
	}

	// Environment variable overrides everything for API key
	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		c.APIKey = ConfigValue[string]{Value: envKey, Source: SourceEnv}
	}

	return c
}

// loadGlobal loads the global config from ~/.detent/config.json.
func loadGlobal() (*GlobalConfig, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// Read file directly - os.ReadFile handles non-existence check efficiently
	// #nosec G304 - path is derived from user's home directory
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}
		return nil, fmt.Errorf("reading: %w", err)
	}

	if len(data) == 0 {
		return &GlobalConfig{}, nil
	}

	var cfg GlobalConfig
	if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
		return nil, fmt.Errorf("parsing: %w", unmarshalErr)
	}

	return &cfg, nil
}

// loadLocal loads the local config from detent.json in the given directory.
func loadLocal(dir string) (*LocalConfig, error) {
	// Clean path to prevent traversal attacks
	path := filepath.Clean(filepath.Join(dir, localConfigFile))

	// Read file directly - os.ReadFile handles non-existence check efficiently
	// #nosec G304 - path is constructed from repoRoot parameter
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No local config is fine
		}
		return nil, fmt.Errorf("reading: %w", err)
	}

	if len(data) == 0 {
		return nil, nil
	}

	var cfg LocalConfig
	if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
		return nil, fmt.Errorf("parsing: %w", unmarshalErr)
	}

	return &cfg, nil
}

// merge combines global and local config with proper precedence.
func merge(global *GlobalConfig, local *LocalConfig, repoRoot string) *Config {
	c := &Config{
		Model:            DefaultModel,
		BudgetPerRunUSD:  DefaultBudgetPerRunUSD,
		BudgetMonthlyUSD: 0, // 0 means unlimited
		TimeoutMins:      DefaultTimeoutMins,
		global:           global,
		local:            local,
		repoRoot:         repoRoot,
	}

	// Apply global config
	if global != nil {
		if global.APIKey != "" {
			c.APIKey = global.APIKey
		}
		if global.Model != "" {
			if strings.HasPrefix(global.Model, modelPrefix) {
				c.Model = global.Model
			} else {
				fmt.Fprintf(os.Stderr, "warning: ignoring invalid model %q (must start with %q)\n", global.Model, modelPrefix)
			}
		}
		if global.BudgetPerRunUSD != nil {
			c.BudgetPerRunUSD = clampBudget(*global.BudgetPerRunUSD)
		}
		if global.BudgetMonthlyUSD != nil {
			c.BudgetMonthlyUSD = clampMonthlyBudget(*global.BudgetMonthlyUSD)
		}
		if global.TimeoutMins != nil {
			c.TimeoutMins = clampTimeout(*global.TimeoutMins)
		}
	}

	// Apply local config (overrides global for per-run budget)
	if local != nil {
		if local.Model != "" {
			if strings.HasPrefix(local.Model, modelPrefix) {
				c.Model = local.Model
			} else {
				fmt.Fprintf(os.Stderr, "warning: ignoring invalid model %q (must start with %q)\n", local.Model, modelPrefix)
			}
		}
		if local.BudgetPerRunUSD != nil {
			c.BudgetPerRunUSD = clampBudget(*local.BudgetPerRunUSD)
		}
		if local.TimeoutMins != nil {
			c.TimeoutMins = clampTimeout(*local.TimeoutMins)
		}
		c.ExtraCommands = local.Commands
	}

	// Environment variable overrides everything for API key
	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		c.APIKey = envKey
	}

	return c
}

// --- Clamping helpers ---

func clampBudget(value float64) float64 {
	if value < minBudgetUSD {
		return minBudgetUSD
	}
	if value > maxBudgetUSD {
		return maxBudgetUSD
	}
	return value
}

func clampMonthlyBudget(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > maxBudgetMonthlyUSD {
		return maxBudgetMonthlyUSD
	}
	return value
}

func clampTimeout(value int) int {
	if value < minTimeoutMins {
		return minTimeoutMins
	}
	if value > maxTimeoutMins {
		return maxTimeoutMins
	}
	return value
}

// --- Saving ---

// SaveGlobal persists the global config to disk.
func (c *Config) SaveGlobal() error {
	if c.global == nil {
		c.global = &GlobalConfig{}
	}

	// Set schema
	c.global.Schema = SchemaURL

	dir, err := GetDetentDir()
	if err != nil {
		return err
	}

	// Create directory if needed
	// #nosec G301 - 0700 is intentionally restrictive
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("creating config directory: %w", mkdirErr)
	}

	data, marshalErr := json.MarshalIndent(c.global, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshaling: %w", marshalErr)
	}
	// Append newline for proper file ending
	data = append(data, '\n')

	path := filepath.Join(dir, globalConfigFile)
	// #nosec G306 - 0600 is intentionally restrictive
	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("writing: %w", writeErr)
	}

	// Write schema file alongside config for IDE support
	schemaPath := filepath.Join(dir, "schema.json")
	// #nosec G306 - 0644 is fine for schema file
	if schemaErr := os.WriteFile(schemaPath, []byte(schema.JSON), 0o644); schemaErr != nil {
		return fmt.Errorf("writing schema: %w", schemaErr)
	}

	return nil
}

// SetAPIKey updates the API key in global config and saves.
func (c *Config) SetAPIKey(key string) error {
	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	c.global.APIKey = key
	c.APIKey = key
	return c.SaveGlobal()
}

// SetBudgetMonthlyUSD updates the monthly budget in global config and saves.
func (c *Config) SetBudgetMonthlyUSD(budget float64) error {
	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	c.global.BudgetMonthlyUSD = &budget
	c.BudgetMonthlyUSD = budget
	return c.SaveGlobal()
}

// SaveLocal persists the local config to disk (detent.json).
func (c *Config) SaveLocal() error {
	if c.repoRoot == "" {
		return fmt.Errorf("no repository root set")
	}
	if c.local == nil {
		c.local = &LocalConfig{}
	}

	// Set schema
	c.local.Schema = SchemaURL

	data, marshalErr := json.MarshalIndent(c.local, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshaling: %w", marshalErr)
	}
	// Append newline for proper file ending
	data = append(data, '\n')

	path := filepath.Clean(filepath.Join(c.repoRoot, localConfigFile))
	// #nosec G306 - 0644 is appropriate for project config files
	if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil {
		return fmt.Errorf("writing: %w", writeErr)
	}

	return nil
}

// SaveLocalWithSources saves local config from a ConfigWithSources struct.
func SaveLocalWithSources(cfg *ConfigWithSources) error {
	if cfg.RepoRoot == "" {
		return fmt.Errorf("no repository root set")
	}
	if cfg.Local == nil {
		cfg.Local = &LocalConfig{}
	}

	// Set schema
	cfg.Local.Schema = SchemaURL

	data, marshalErr := json.MarshalIndent(cfg.Local, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshaling: %w", marshalErr)
	}
	// Append newline for proper file ending
	data = append(data, '\n')

	path := filepath.Clean(filepath.Join(cfg.RepoRoot, localConfigFile))
	// #nosec G306 - 0644 is appropriate for project config files
	if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil {
		return fmt.Errorf("writing: %w", writeErr)
	}

	return nil
}

// --- Trust helpers ---

// IsTrustedRepo checks if a repository is trusted by its first commit SHA.
func (c *Config) IsTrustedRepo(firstCommitSHA string) bool {
	if c.global == nil || c.global.TrustedRepos == nil {
		return false
	}
	_, ok := c.global.TrustedRepos[firstCommitSHA]
	return ok
}

// TrustRepo marks a repository as trusted and saves the config.
func (c *Config) TrustRepo(firstCommitSHA, remoteURL string) error {
	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	if c.global.TrustedRepos == nil {
		c.global.TrustedRepos = make(map[string]TrustedRepo)
	}

	c.global.TrustedRepos[firstCommitSHA] = TrustedRepo{
		RemoteURL: remoteURL,
		TrustedAt: time.Now(),
	}
	return c.SaveGlobal()
}

// --- Spend tracking helpers ---

// currentMonth returns the current month in YYYY-MM format.
func currentMonth() string {
	return time.Now().Format("2006-01")
}

// GetMonthlySpend returns the total spend for the current month.
func (c *Config) GetMonthlySpend() float64 {
	if c.global == nil || c.global.SpendHistory == nil {
		return 0
	}
	return c.global.SpendHistory[currentMonth()]
}

// RecordSpend adds the given amount to the current month's spend and saves to disk.
// NOTE: This operation is not atomic across processes. If multiple CLI instances
// run concurrently, there's a race condition where spend data could be lost.
// This is acceptable for budget tracking (best-effort) but should not be relied
// upon for strict financial controls.
func (c *Config) RecordSpend(amount float64) error {
	if amount < 0 {
		return nil // Ignore negative amounts
	}
	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	if c.global.SpendHistory == nil {
		c.global.SpendHistory = make(map[string]float64)
	}

	month := currentMonth()
	c.global.SpendHistory[month] += amount
	c.pruneSpendHistory()
	return c.SaveGlobal()
}

// RemainingMonthlyBudget returns the remaining monthly budget.
// Returns -1 if the monthly budget is unlimited (0).
func (c *Config) RemainingMonthlyBudget() float64 {
	if c.BudgetMonthlyUSD == 0 {
		return -1 // Unlimited
	}
	remaining := c.BudgetMonthlyUSD - c.GetMonthlySpend()
	if remaining < 0 {
		return 0
	}
	return remaining
}

// pruneSpendHistory removes entries older than 3 months to keep the config file small.
func (c *Config) pruneSpendHistory() {
	if c.global == nil || c.global.SpendHistory == nil {
		return
	}

	// Calculate the cutoff month (3 months ago).
	// Use local time consistently with currentMonth() to avoid timezone mismatches.
	// Go's time.Date normalizes month values, so Month()-2 works correctly
	// even for January/February (becomes November/December of previous year).
	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, now.Location()).Format("2006-01")

	for month := range c.global.SpendHistory {
		if month < cutoff {
			delete(c.global.SpendHistory, month)
		}
	}
}

// --- Command helpers ---

// MatchesCommand checks if a command is allowed by local config.
// Supports exact matches and wildcard patterns (e.g., "bun run *").
func (c *Config) MatchesCommand(cmd string) bool {
	for _, pattern := range c.ExtraCommands {
		if cmd == pattern {
			return true
		}
		if strings.HasSuffix(pattern, " *") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(cmd, prefix) {
				return true
			}
		}
	}
	return false
}

// AddCommand adds a command to local config and saves.
func (c *Config) AddCommand(cmd string) error {
	if c.local == nil {
		c.local = &LocalConfig{}
	}
	// Check if already exists
	for _, existing := range c.local.Commands {
		if existing == cmd {
			return nil
		}
	}
	c.local.Commands = append(c.local.Commands, cmd)
	c.ExtraCommands = c.local.Commands
	return c.SaveLocal()
}

// MaskAPIKey returns a masked version of an API key for safe display.
// Shows only the last 4 characters prefixed with dots.
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}
