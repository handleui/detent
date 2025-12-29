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
	globalConfigFile = "detent.json"

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

	// schemaWritten tracks if schema.json has been written this session.
	// The schema is static, so we only need to write once per session.
	schemaWritten   bool
	schemaWrittenMu sync.Mutex
)

// --- Structs ---

// TrustedRepo stores trust information for a repository.
// The key in TrustedRepos map is the first commit SHA (immutable identifier).
type TrustedRepo struct {
	RemoteURL string    `json:"remote_url,omitempty"`
	TrustedAt time.Time `json:"trusted_at"`
}

// GlobalConfig is the user's global settings (~/.detent/detent.json).
// This is the raw structure that gets persisted to disk.
type GlobalConfig struct {
	Schema           string                 `json:"$schema,omitempty"`
	APIKey           string                 `json:"api_key,omitempty"`
	Model            string                 `json:"model,omitempty"`
	BudgetPerRunUSD  *float64               `json:"budget_per_run_usd,omitempty"`
	BudgetMonthlyUSD *float64               `json:"budget_monthly_usd,omitempty"`
	TimeoutMins      *int                   `json:"timeout_mins,omitempty"`
	TrustedRepos     map[string]TrustedRepo `json:"trusted_repos,omitempty"`
	AllowedCommands  map[string][]string    `json:"allowed_commands,omitempty"` // key is first commit SHA
}

// Config is the merged, resolved config used by the application.
// Values are resolved from: env var > global config > defaults.
type Config struct {
	// Resolved settings (from global config)
	APIKey           string
	Model            string
	BudgetPerRunUSD  float64
	BudgetMonthlyUSD float64
	TimeoutMins      int

	// Internal references for mutation
	global *GlobalConfig
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
	SourceGlobal                     // SourceGlobal indicates the value comes from ~/.detent/detent.json.
	SourceEnv                        // SourceEnv indicates the value comes from an environment variable.
)

// String returns the display name for a value source.
func (s ValueSource) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceGlobal:
		return "global"
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

	// Internal reference for saving
	Global *GlobalConfig
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

// Load loads the global config, returning the resolved Config.
func Load() (*Config, error) {
	global, err := loadGlobal()
	if err != nil {
		return nil, fmt.Errorf("global config: %w", err)
	}

	return merge(global), nil
}

// LoadWithSources loads config and tracks the source of each value.
// Used by the TUI to display where values originated.
func LoadWithSources() (*ConfigWithSources, error) {
	global, err := loadGlobal()
	if err != nil {
		return nil, fmt.Errorf("global config: %w", err)
	}

	return mergeWithSources(global), nil
}

// mergeInternal combines global config with defaults, tracking value sources.
// This is the single implementation used by both merge() and mergeWithSources().
func mergeInternal(global *GlobalConfig) *ConfigWithSources {
	c := &ConfigWithSources{
		Model:            ConfigValue[string]{Value: DefaultModel, Source: SourceDefault},
		BudgetPerRunUSD:  ConfigValue[float64]{Value: DefaultBudgetPerRunUSD, Source: SourceDefault},
		BudgetMonthlyUSD: ConfigValue[float64]{Value: 0, Source: SourceDefault}, // 0 means unlimited
		TimeoutMins:      ConfigValue[int]{Value: DefaultTimeoutMins, Source: SourceDefault},
		APIKey:           ConfigValue[string]{Value: "", Source: SourceDefault},
		Global:           global,
	}

	// Apply global config
	if global != nil {
		if global.APIKey != "" {
			c.APIKey = ConfigValue[string]{Value: global.APIKey, Source: SourceGlobal}
		}
		if global.Model != "" {
			if strings.HasPrefix(global.Model, modelPrefix) {
				c.Model = ConfigValue[string]{Value: global.Model, Source: SourceGlobal}
			} else {
				fmt.Fprintf(os.Stderr, "warning: ignoring invalid model %q (must start with %q)\n", global.Model, modelPrefix)
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
	}

	// Environment variable overrides everything for API key
	if envKey := os.Getenv("ANTHROPIC_API_KEY"); envKey != "" {
		c.APIKey = ConfigValue[string]{Value: envKey, Source: SourceEnv}
	}

	return c
}

// mergeWithSources combines global config with defaults, tracking value sources.
func mergeWithSources(global *GlobalConfig) *ConfigWithSources {
	return mergeInternal(global)
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

// merge combines global config with defaults.
// Uses mergeInternal and extracts just the values.
func merge(global *GlobalConfig) *Config {
	src := mergeInternal(global)
	return &Config{
		APIKey:           src.APIKey.Value,
		Model:            src.Model.Value,
		BudgetPerRunUSD:  src.BudgetPerRunUSD.Value,
		BudgetMonthlyUSD: src.BudgetMonthlyUSD.Value,
		TimeoutMins:      src.TimeoutMins.Value,
		global:           global,
	}
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

// ensureSchemaWritten writes the schema.json file if not already written this session.
// This avoids redundant I/O since the schema is static.
func ensureSchemaWritten(dir string) error {
	schemaWrittenMu.Lock()
	defer schemaWrittenMu.Unlock()

	if schemaWritten {
		return nil
	}

	schemaPath := filepath.Join(dir, "schema.json")

	// Check if schema file already exists with correct content
	// #nosec G304 - path is derived from user's home directory
	existing, err := os.ReadFile(schemaPath)
	if err == nil && string(existing) == schema.JSON {
		schemaWritten = true
		return nil
	}

	// Write only if missing or different
	// #nosec G306 - 0644 is fine for schema file
	if err := os.WriteFile(schemaPath, []byte(schema.JSON), 0o644); err != nil {
		return fmt.Errorf("writing schema: %w", err)
	}

	schemaWritten = true
	return nil
}

// saveGlobalConfig is the shared implementation for persisting GlobalConfig to disk.
func saveGlobalConfig(global *GlobalConfig) error {
	global.Schema = SchemaURL

	dir, err := GetDetentDir()
	if err != nil {
		return err
	}

	// Create directory if needed
	// #nosec G301 - 0700 is intentionally restrictive
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("creating config directory: %w", mkdirErr)
	}

	data, marshalErr := json.MarshalIndent(global, "", "  ")
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

	// Write schema file alongside config for IDE support (only once per session)
	return ensureSchemaWritten(dir)
}

// SaveGlobal persists the global config to disk.
func (c *Config) SaveGlobal() error {
	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	return saveGlobalConfig(c.global)
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

// SaveGlobalFromSources saves the global config from a ConfigWithSources struct.
func SaveGlobalFromSources(cfg *ConfigWithSources) error {
	if cfg.Global == nil {
		cfg.Global = &GlobalConfig{}
	}
	return saveGlobalConfig(cfg.Global)
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

// --- Command helpers ---

// GetAllowedCommands returns the allowed commands for a repo by its first commit SHA.
func (c *Config) GetAllowedCommands(repoSHA string) []string {
	if c.global == nil || c.global.AllowedCommands == nil {
		return nil
	}
	return c.global.AllowedCommands[repoSHA]
}

// dangerousPatterns are shell metacharacters and patterns that could enable command injection.
// These are blocked even in wildcard-matched commands.
var dangerousPatterns = []string{
	";", "&&", "||", "|", ">", "<", ">>", "<<",
	"$(", "`", "${", "\\", "\n", "\r", "\x00",
}

// MatchesCommand checks if a command is in the repo's allowlist.
// Supports exact matches and wildcard patterns (e.g., "bun run *").
// Wildcard patterns only match the immediate argument after the prefix,
// and reject commands containing shell metacharacters.
func (c *Config) MatchesCommand(repoSHA, cmd string) bool {
	commands := c.GetAllowedCommands(repoSHA)
	for _, pattern := range commands {
		if cmd == pattern {
			return true
		}
		if strings.HasSuffix(pattern, " *") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(cmd, prefix) {
				// Extract the part matched by the wildcard
				suffix := strings.TrimPrefix(cmd, prefix)
				// Reject if suffix contains dangerous patterns
				if containsDangerousPattern(suffix) {
					continue
				}
				// Reject if suffix contains spaces (wildcard should match single argument)
				if strings.Contains(strings.TrimSpace(suffix), " ") {
					continue
				}
				return true
			}
		}
	}
	return false
}

// containsDangerousPattern checks if a string contains shell metacharacters.
func containsDangerousPattern(s string) bool {
	for _, pattern := range dangerousPatterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}
	return false
}

// ValidateAllowedCommand checks if a command pattern is safe to add to the allowlist.
// Returns an error describing why the command is unsafe.
func ValidateAllowedCommand(cmd string) error {
	if cmd == "" {
		return fmt.Errorf("command cannot be empty")
	}

	// Normalize whitespace
	normalized := strings.Join(strings.Fields(cmd), " ")
	if normalized != cmd {
		return fmt.Errorf("command contains irregular whitespace")
	}

	// Check for dangerous patterns that should never be allowed
	for _, pattern := range dangerousPatterns {
		if strings.Contains(cmd, pattern) {
			return fmt.Errorf("command contains dangerous pattern: %q", pattern)
		}
	}

	// Check for null bytes and control characters
	for i, r := range cmd {
		if r < 32 && r != ' ' && r != '\t' {
			return fmt.Errorf("command contains control character at position %d", i)
		}
	}

	// Wildcard must only appear at the end and preceded by space
	if strings.Contains(cmd, "*") {
		if !strings.HasSuffix(cmd, " *") {
			return fmt.Errorf("wildcard (*) must appear only at the end as ' *'")
		}
		if strings.Count(cmd, "*") > 1 {
			return fmt.Errorf("only one wildcard (*) is allowed")
		}
	}

	return nil
}

// AddAllowedCommand adds a command to a repo's allowlist and saves.
// Returns an error if the command pattern is unsafe.
func (c *Config) AddAllowedCommand(repoSHA, cmd string) error {
	// Validate command before adding
	if err := ValidateAllowedCommand(cmd); err != nil {
		return fmt.Errorf("invalid command pattern: %w", err)
	}

	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	if c.global.AllowedCommands == nil {
		c.global.AllowedCommands = make(map[string][]string)
	}

	// Check if already exists
	for _, existing := range c.global.AllowedCommands[repoSHA] {
		if existing == cmd {
			return nil
		}
	}
	c.global.AllowedCommands[repoSHA] = append(c.global.AllowedCommands[repoSHA], cmd)
	return c.SaveGlobal()
}

// RemoveAllowedCommand removes a command from a repo's allowlist and saves.
func (c *Config) RemoveAllowedCommand(repoSHA, cmd string) error {
	if c.global == nil || c.global.AllowedCommands == nil {
		return nil
	}

	commands := c.global.AllowedCommands[repoSHA]
	for i, existing := range commands {
		if existing == cmd {
			c.global.AllowedCommands[repoSHA] = append(commands[:i], commands[i+1:]...)
			return c.SaveGlobal()
		}
	}
	return nil
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

// FormatBudget formats a budget value for display.
// Returns "unlimited" for 0, otherwise returns "$X.XX".
func FormatBudget(usd float64) string {
	if usd == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("$%.2f", usd)
}

// FormatBudgetRaw formats a budget value for editing.
// Returns "0" for 0, otherwise returns the numeric value as a string.
func FormatBudgetRaw(usd float64) string {
	if usd == 0 {
		return "0"
	}
	return fmt.Sprintf("%.2f", usd)
}

// NewConfigWithDefaults creates a new Config with default values and empty GlobalConfig.
// Use this when you need a fresh config that doesn't inherit existing settings.
func NewConfigWithDefaults() *Config {
	return &Config{
		Model:            DefaultModel,
		BudgetPerRunUSD:  DefaultBudgetPerRunUSD,
		BudgetMonthlyUSD: 0,
		TimeoutMins:      DefaultTimeoutMins,
		global:           &GlobalConfig{},
	}
}

// GetGlobal returns the underlying GlobalConfig for direct access.
// Returns nil if no global config is loaded.
func (c *Config) GetGlobal() *GlobalConfig {
	return c.global
}

// SetAPIKeyValue sets the API key without saving.
// Use SaveGlobal() after to persist changes.
func (c *Config) SetAPIKeyValue(key string) {
	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	c.global.APIKey = key
	c.APIKey = key
}

// SetTrustedRepos sets the trusted repos map without saving.
// Use SaveGlobal() after to persist changes.
func (c *Config) SetTrustedRepos(repos map[string]TrustedRepo) {
	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	c.global.TrustedRepos = repos
}

// SetAllowedCommands sets the allowed commands map without saving.
// Use SaveGlobal() after to persist changes.
func (c *Config) SetAllowedCommands(commands map[string][]string) {
	if c.global == nil {
		c.global = &GlobalConfig{}
	}
	c.global.AllowedCommands = commands
}
