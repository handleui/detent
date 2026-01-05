package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTestConfig creates a Config with the given trusted repos for testing
func createTestConfig(trustedRepos map[string]TrustedRepo) *Config {
	return &Config{
		Model:           DefaultModel,
		BudgetPerRunUSD: DefaultBudgetPerRunUSD,
		TimeoutMins:     DefaultTimeoutMins,
		global: &GlobalConfig{
			TrustedRepos: trustedRepos,
		},
	}
}

// TestIsTrustedRepo tests the IsTrustedRepo method
func TestIsTrustedRepo(t *testing.T) {
	tests := []struct {
		name           string
		trustedRepos   map[string]TrustedRepo
		firstCommitSHA string
		want           bool
	}{
		{
			name:           "nil map returns false",
			trustedRepos:   nil,
			firstCommitSHA: "abc123",
			want:           false,
		},
		{
			name:           "empty map returns false",
			trustedRepos:   map[string]TrustedRepo{},
			firstCommitSHA: "abc123",
			want:           false,
		},
		{
			name: "found returns true",
			trustedRepos: map[string]TrustedRepo{
				"abc123": {
					RemoteURL: "github.com/user/repo",
					TrustedAt: time.Now(),
				},
			},
			firstCommitSHA: "abc123",
			want:           true,
		},
		{
			name: "not found returns false",
			trustedRepos: map[string]TrustedRepo{
				"def456": {
					RemoteURL: "github.com/other/repo",
					TrustedAt: time.Now(),
				},
			},
			firstCommitSHA: "abc123",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createTestConfig(tt.trustedRepos)
			got := cfg.IsTrustedRepo(tt.firstCommitSHA)
			if got != tt.want {
				t.Errorf("IsTrustedRepo(%q) = %v, want %v", tt.firstCommitSHA, got, tt.want)
			}
		})
	}
}

// TestTrustRepo tests the TrustRepo method
func TestTrustRepo(t *testing.T) {
	t.Run("new trust creates entry", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		err := cfg.TrustRepo("abc123", "github.com/user/repo")
		if err != nil {
			t.Fatalf("TrustRepo() error = %v", err)
		}

		if !cfg.IsTrustedRepo("abc123") {
			t.Error("Expected repo to be trusted after TrustRepo()")
		}

		repo := cfg.global.TrustedRepos["abc123"]
		if repo.RemoteURL != "github.com/user/repo" {
			t.Errorf("RemoteURL = %v, want %v", repo.RemoteURL, "github.com/user/repo")
		}
		if repo.TrustedAt.IsZero() {
			t.Error("TrustedAt should not be zero")
		}
	})

	t.Run("re-trust updates timestamp", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		oldTime := time.Now().Add(-time.Hour)
		cfg := createTestConfig(map[string]TrustedRepo{
			"abc123": {
				RemoteURL: "github.com/user/repo",
				TrustedAt: oldTime,
			},
		})

		err := cfg.TrustRepo("abc123", "github.com/user/repo-updated")
		if err != nil {
			t.Fatalf("TrustRepo() error = %v", err)
		}

		repo := cfg.global.TrustedRepos["abc123"]
		if repo.RemoteURL != "github.com/user/repo-updated" {
			t.Errorf("RemoteURL = %v, want github.com/user/repo-updated", repo.RemoteURL)
		}
		if !repo.TrustedAt.After(oldTime) {
			t.Error("TrustedAt should be updated")
		}
	})

	t.Run("saves to disk", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		err := cfg.TrustRepo("abc123", "github.com/user/repo")
		if err != nil {
			t.Fatalf("TrustRepo() error = %v", err)
		}

		// Verify config file was created (DETENT_HOME is the .detent dir equivalent)
		configPath := filepath.Join(tmpDir, "detent.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("Config file was not created")
		}

		// Reload config and verify
		loadedCfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if !loadedCfg.IsTrustedRepo("abc123") {
			t.Error("Loaded config should have trusted repo")
		}
	})
}

// TestMatchesCommand tests the MatchesCommand helper
func TestMatchesCommand(t *testing.T) {
	repoSHA := "test-repo-sha"
	cfg := &Config{
		global: &GlobalConfig{
			AllowedCommands: map[string]RepoCommands{
				repoSHA: {Commands: []string{"bun run typecheck", "pnpm exec playwright *", "make deploy"}},
			},
		},
	}

	if !cfg.MatchesCommand(repoSHA, "bun run typecheck") {
		t.Error("Expected exact match for 'bun run typecheck'")
	}
	if !cfg.MatchesCommand(repoSHA, "pnpm exec playwright test") {
		t.Error("Expected wildcard match for 'pnpm exec playwright test'")
	}
	if !cfg.MatchesCommand(repoSHA, "make deploy") {
		t.Error("Expected match for 'make deploy'")
	}
	if cfg.MatchesCommand(repoSHA, "npm run test") {
		t.Error("Expected no match for 'npm run test'")
	}
	if cfg.MatchesCommand("other-repo", "bun run typecheck") {
		t.Error("Expected no match for different repo SHA")
	}
}

// TestConfigMerge tests the config merge precedence
func TestConfigMerge(t *testing.T) {
	t.Run("global config applied", func(t *testing.T) {
		global := &GlobalConfig{
			Model:           "claude-sonnet-4-5",
			BudgetPerRunUSD: float64Ptr(2.0),
			TimeoutMins:     intPtr(15),
		}

		cfg := merge(global)

		if cfg.Model != "claude-sonnet-4-5" {
			t.Errorf("Model = %v, want claude-sonnet-4-5", cfg.Model)
		}
		if cfg.BudgetPerRunUSD != 2.0 {
			t.Errorf("BudgetPerRunUSD = %v, want 2.0", cfg.BudgetPerRunUSD)
		}
		if cfg.TimeoutMins != 15 {
			t.Errorf("TimeoutMins = %v, want 15", cfg.TimeoutMins)
		}
	})

	t.Run("defaults used when global nil", func(t *testing.T) {
		cfg := merge(nil)

		if cfg.Model != DefaultModel {
			t.Errorf("Model = %v, want %v", cfg.Model, DefaultModel)
		}
		if cfg.BudgetPerRunUSD != DefaultBudgetPerRunUSD {
			t.Errorf("BudgetPerRunUSD = %v, want %v", cfg.BudgetPerRunUSD, DefaultBudgetPerRunUSD)
		}
		if cfg.TimeoutMins != DefaultTimeoutMins {
			t.Errorf("TimeoutMins = %v, want %v", cfg.TimeoutMins, DefaultTimeoutMins)
		}
	})
}

func float64Ptr(v float64) *float64 { return &v }
func intPtr(v int) *int             { return &v }

// TestLoadGlobal_MalformedJSON tests loading malformed JSON config files
func TestLoadGlobal_MalformedJSON(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		checkResult func(*testing.T, *Config)
	}{
		{
			name:    "invalid JSON - missing closing brace",
			content: `{"model": "claude-sonnet-4-5"`,
			wantErr: true,
		},
		{
			name:    "truncated JSON",
			content: `{"model": "claude-son`,
			wantErr: true,
		},
		{
			name:    "wrong type for budget - string instead of number",
			content: `{"budget_per_run_usd": "five"}`,
			wantErr: true,
		},
		{
			name:    "wrong type for timeout - string instead of number",
			content: `{"timeout_mins": "ten"}`,
			wantErr: true,
		},
		{
			name:    "null values are valid",
			content: `{"model": null, "budget_per_run_usd": null, "timeout_mins": null}`,
			wantErr: false,
			checkResult: func(t *testing.T, cfg *Config) {
				if cfg.Model != DefaultModel {
					t.Errorf("Model = %v, want %v", cfg.Model, DefaultModel)
				}
				if cfg.BudgetPerRunUSD != DefaultBudgetPerRunUSD {
					t.Errorf("BudgetPerRunUSD = %v, want %v", cfg.BudgetPerRunUSD, DefaultBudgetPerRunUSD)
				}
				if cfg.TimeoutMins != DefaultTimeoutMins {
					t.Errorf("TimeoutMins = %v, want %v", cfg.TimeoutMins, DefaultTimeoutMins)
				}
			},
		},
		{
			name:    "empty object is valid",
			content: `{}`,
			wantErr: false,
			checkResult: func(t *testing.T, cfg *Config) {
				if cfg.Model != DefaultModel {
					t.Errorf("Model = %v, want %v", cfg.Model, DefaultModel)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			// Use DETENT_HOME to bypass the cached directory path
			t.Setenv(DetentHomeEnv, tmpDir)

			// Create config file directly in tmpDir (DETENT_HOME points to .detent dir equivalent)
			configPath := filepath.Join(tmpDir, "detent.json")
			if err := os.WriteFile(configPath, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if tt.checkResult != nil {
				tt.checkResult(t, cfg)
			}
		})
	}
}

// TestConfigClamping tests boundary value clamping for budget and timeout
func TestConfigClamping(t *testing.T) {
	tests := []struct {
		name            string
		budget          *float64
		timeout         *int
		wantBudget      float64
		wantTimeout     int
	}{
		{
			name:        "negative budget clamps to 0",
			budget:      float64Ptr(-5.0),
			timeout:     intPtr(10),
			wantBudget:  0.0,
			wantTimeout: 10,
		},
		{
			name:        "negative timeout clamps to 1",
			budget:      float64Ptr(1.0),
			timeout:     intPtr(-10),
			wantBudget:  1.0,
			wantTimeout: 1,
		},
		{
			name:        "zero timeout clamps to 1",
			budget:      float64Ptr(1.0),
			timeout:     intPtr(0),
			wantBudget:  1.0,
			wantTimeout: 1,
		},
		{
			name:        "budget at max stays at max",
			budget:      float64Ptr(100.0),
			timeout:     intPtr(10),
			wantBudget:  100.0,
			wantTimeout: 10,
		},
		{
			name:        "timeout at max stays at max",
			budget:      float64Ptr(1.0),
			timeout:     intPtr(60),
			wantBudget:  1.0,
			wantTimeout: 60,
		},
		{
			name:        "budget over max clamps to max",
			budget:      float64Ptr(500.0),
			timeout:     intPtr(10),
			wantBudget:  100.0,
			wantTimeout: 10,
		},
		{
			name:        "timeout over max clamps to max",
			budget:      float64Ptr(1.0),
			timeout:     intPtr(120),
			wantBudget:  1.0,
			wantTimeout: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global := &GlobalConfig{
				BudgetPerRunUSD: tt.budget,
				TimeoutMins:     tt.timeout,
			}

			cfg := merge(global)

			if cfg.BudgetPerRunUSD != tt.wantBudget {
				t.Errorf("BudgetPerRunUSD = %v, want %v", cfg.BudgetPerRunUSD, tt.wantBudget)
			}
			if cfg.TimeoutMins != tt.wantTimeout {
				t.Errorf("TimeoutMins = %v, want %v", cfg.TimeoutMins, tt.wantTimeout)
			}
		})
	}
}

// TestModelValidation tests model prefix validation
func TestModelValidation(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantModel string
	}{
		{
			name:      "valid claude model accepted",
			model:     "claude-sonnet-4-5",
			wantModel: "claude-sonnet-4-5",
		},
		{
			name:      "valid claude-opus model accepted",
			model:     "claude-opus-4-5",
			wantModel: "claude-opus-4-5",
		},
		{
			name:      "invalid prefix falls back to default",
			model:     "gpt-4",
			wantModel: DefaultModel,
		},
		{
			name:      "empty model uses default",
			model:     "",
			wantModel: DefaultModel,
		},
		{
			name:      "case sensitivity - uppercase prefix rejected",
			model:     "Claude-sonnet-4-5",
			wantModel: DefaultModel,
		},
		{
			name:      "case sensitivity - mixed case rejected",
			model:     "CLAUDE-sonnet-4-5",
			wantModel: DefaultModel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global := &GlobalConfig{
				Model: tt.model,
			}

			cfg := merge(global)

			if cfg.Model != tt.wantModel {
				t.Errorf("Model = %v, want %v", cfg.Model, tt.wantModel)
			}
		})
	}
}

// TestEnvOverride tests ANTHROPIC_API_KEY environment variable overrides
func TestEnvOverride(t *testing.T) {
	t.Run("env overrides global config api_key", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Use DETENT_HOME to bypass the cached directory path
		t.Setenv(DetentHomeEnv, tmpDir)
		t.Setenv("ANTHROPIC_API_KEY", "env-key-12345")

		// Create global config with api_key (DETENT_HOME points to .detent dir equivalent)
		configPath := filepath.Join(tmpDir, "detent.json")
		content := `{"api_key": "global-key-67890"}`
		if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.APIKey != "env-key-12345" {
			t.Errorf("APIKey = %v, want env-key-12345", cfg.APIKey)
		}
	})
}

// TestMaskAPIKey_EdgeCases tests edge cases for API key masking
func TestMaskAPIKey_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string returns empty",
			input:    "",
			expected: "",
		},
		{
			name:     "1 char returns ****",
			input:    "a",
			expected: "****",
		},
		{
			name:     "2 chars returns ****",
			input:    "ab",
			expected: "****",
		},
		{
			name:     "3 chars returns ****",
			input:    "abc",
			expected: "****",
		},
		{
			name:     "4 chars returns ****",
			input:    "abcd",
			expected: "****",
		},
		{
			name:     "5 chars returns **** plus last 4",
			input:    "abcde",
			expected: "****bcde",
		},
		{
			name:     "longer key shows last 4",
			input:    "sk-ant-api03-1234567890",
			expected: "****7890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskAPIKey(tt.input)
			if result != tt.expected {
				t.Errorf("MaskAPIKey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGetDetentDir_Concurrent tests that GetDetentDir is safe for concurrent use.
// Run with -race flag to verify there are no race conditions.
func TestGetDetentDir_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(DetentHomeEnv, tmpDir)

	const numGoroutines = 100
	done := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			dir, err := GetDetentDir()
			if err != nil {
				t.Errorf("GetDetentDir() error = %v", err)
			}
			if dir == "" {
				t.Error("GetDetentDir() returned empty string")
			}
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestAllowedCommands tests the allowed commands management
func TestAllowedCommands(t *testing.T) {
	repoSHA := "test-repo-sha"

	t.Run("add command", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		if err := cfg.AddAllowedCommand(repoSHA, "", "bun test"); err != nil {
			t.Fatalf("AddAllowedCommand() error = %v", err)
		}

		commands := cfg.GetAllowedCommands(repoSHA)
		if len(commands) != 1 || commands[0] != "bun test" {
			t.Errorf("GetAllowedCommands() = %v, want [bun test]", commands)
		}
	})

	t.Run("remove command", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := &Config{
			global: &GlobalConfig{
				AllowedCommands: map[string]RepoCommands{
					repoSHA: {Commands: []string{"bun test", "npm run lint"}},
				},
			},
		}

		if err := cfg.RemoveAllowedCommand(repoSHA, "bun test"); err != nil {
			t.Fatalf("RemoveAllowedCommand() error = %v", err)
		}

		commands := cfg.GetAllowedCommands(repoSHA)
		if len(commands) != 1 || commands[0] != "npm run lint" {
			t.Errorf("GetAllowedCommands() = %v, want [npm run lint]", commands)
		}
	})

	t.Run("get empty for unknown repo", func(t *testing.T) {
		cfg := createTestConfig(nil)
		commands := cfg.GetAllowedCommands("unknown-sha")
		if len(commands) != 0 {
			t.Errorf("GetAllowedCommands() = %v, want empty", commands)
		}
	})

	t.Run("no duplicate commands", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		_ = cfg.AddAllowedCommand(repoSHA, "", "bun test")
		_ = cfg.AddAllowedCommand(repoSHA, "", "bun test")

		commands := cfg.GetAllowedCommands(repoSHA)
		if len(commands) != 1 {
			t.Errorf("GetAllowedCommands() = %v, want 1 entry (no duplicates)", commands)
		}
	})

	t.Run("remove nonexistent command", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := &Config{
			global: &GlobalConfig{
				AllowedCommands: map[string]RepoCommands{
					repoSHA: {Commands: []string{"bun test"}},
				},
			},
		}

		// Should not error when removing a command that doesn't exist
		if err := cfg.RemoveAllowedCommand(repoSHA, "nonexistent"); err != nil {
			t.Fatalf("RemoveAllowedCommand() error = %v", err)
		}

		commands := cfg.GetAllowedCommands(repoSHA)
		if len(commands) != 1 || commands[0] != "bun test" {
			t.Errorf("GetAllowedCommands() = %v, want [bun test]", commands)
		}
	})

	t.Run("remove from nil config", func(t *testing.T) {
		cfg := &Config{global: nil}

		// Should not error or panic with nil global config
		err := cfg.RemoveAllowedCommand(repoSHA, "any")
		if err != nil {
			t.Fatalf("RemoveAllowedCommand() error = %v", err)
		}
	})

	t.Run("remove from nil AllowedCommands map", func(t *testing.T) {
		cfg := &Config{global: &GlobalConfig{}}

		// Should not error or panic with nil AllowedCommands
		err := cfg.RemoveAllowedCommand(repoSHA, "any")
		if err != nil {
			t.Fatalf("RemoveAllowedCommand() error = %v", err)
		}
	})

	t.Run("add multiple commands to same repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		_ = cfg.AddAllowedCommand(repoSHA, "", "bun test")
		_ = cfg.AddAllowedCommand(repoSHA, "", "npm run lint")
		_ = cfg.AddAllowedCommand(repoSHA, "", "go build ./...")

		commands := cfg.GetAllowedCommands(repoSHA)
		if len(commands) != 3 {
			t.Errorf("GetAllowedCommands() len = %d, want 3", len(commands))
		}
	})

	t.Run("add commands to different repos", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		_ = cfg.AddAllowedCommand("repo1", "", "cmd1")
		_ = cfg.AddAllowedCommand("repo2", "", "cmd2")

		commands1 := cfg.GetAllowedCommands("repo1")
		commands2 := cfg.GetAllowedCommands("repo2")

		if len(commands1) != 1 || commands1[0] != "cmd1" {
			t.Errorf("GetAllowedCommands('repo1') = %v, want [cmd1]", commands1)
		}
		if len(commands2) != 1 || commands2[0] != "cmd2" {
			t.Errorf("GetAllowedCommands('repo2') = %v, want [cmd2]", commands2)
		}
	})
}

// TestMatchesCommand_EdgeCases tests edge cases for command matching
func TestMatchesCommand_EdgeCases(t *testing.T) {
	repoSHA := "test-repo"

	tests := []struct {
		name     string
		patterns []string
		cmd      string
		want     bool
	}{
		{
			name:     "exact match",
			patterns: []string{"npm test"},
			cmd:      "npm test",
			want:     true,
		},
		{
			name:     "no match",
			patterns: []string{"npm test"},
			cmd:      "npm run",
			want:     false,
		},
		{
			name:     "wildcard match",
			patterns: []string{"npm run *"},
			cmd:      "npm run test",
			want:     true,
		},
		{
			name:     "wildcard rejects multiple args for security",
			patterns: []string{"npm run *"},
			cmd:      "npm run test --watch",
			want:     false,
		},
		{
			name:     "wildcard no match before prefix",
			patterns: []string{"npm run *"},
			cmd:      "npm test",
			want:     false,
		},
		{
			name:     "empty command list",
			patterns: []string{},
			cmd:      "anything",
			want:     false,
		},
		{
			name:     "multiple patterns first matches",
			patterns: []string{"npm test", "npm run *"},
			cmd:      "npm test",
			want:     true,
		},
		{
			name:     "multiple patterns second matches",
			patterns: []string{"npm test", "npm run *"},
			cmd:      "npm run lint",
			want:     true,
		},
		{
			name:     "pattern without space before wildcard",
			patterns: []string{"npm*"},
			cmd:      "npm test",
			want:     false,
		},
		{
			name:     "command shorter than prefix",
			patterns: []string{"npm run *"},
			cmd:      "npm",
			want:     false,
		},
		{
			name:     "wildcard rejects semicolon injection",
			patterns: []string{"npm run *"},
			cmd:      "npm run test; rm -rf /",
			want:     false,
		},
		{
			name:     "wildcard rejects && injection",
			patterns: []string{"npm run *"},
			cmd:      "npm run test && malicious",
			want:     false,
		},
		{
			name:     "wildcard rejects pipe injection",
			patterns: []string{"npm run *"},
			cmd:      "npm run test | cat",
			want:     false,
		},
		{
			name:     "wildcard rejects subshell injection",
			patterns: []string{"npm run *"},
			cmd:      "npm run $(whoami)",
			want:     false,
		},
		{
			name:     "wildcard rejects backtick injection",
			patterns: []string{"npm run *"},
			cmd:      "npm run `whoami`",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				global: &GlobalConfig{
					AllowedCommands: map[string]RepoCommands{
						repoSHA: {Commands: tt.patterns},
					},
				},
			}

			got := cfg.MatchesCommand(repoSHA, tt.cmd)
			if got != tt.want {
				t.Errorf("MatchesCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// TestMatchesCommand_NilSafety tests nil safety of MatchesCommand
func TestMatchesCommand_NilSafety(t *testing.T) {
	t.Run("nil global config", func(t *testing.T) {
		cfg := &Config{global: nil}
		if cfg.MatchesCommand("repo", "cmd") {
			t.Error("Expected false for nil global config")
		}
	})

	t.Run("nil AllowedCommands map", func(t *testing.T) {
		cfg := &Config{global: &GlobalConfig{}}
		if cfg.MatchesCommand("repo", "cmd") {
			t.Error("Expected false for nil AllowedCommands map")
		}
	})
}

// TestJobOverrides tests the job override management
func TestJobOverrides(t *testing.T) {
	repoSHA := "test-repo-sha"

	t.Run("set job override to run", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		if err := cfg.SetJobOverride(repoSHA, "", "deploy", JobStateRun); err != nil {
			t.Fatalf("SetJobOverride() error = %v", err)
		}

		state := cfg.GetJobState(repoSHA, "deploy")
		if state != JobStateRun {
			t.Errorf("GetJobState() = %q, want %q", state, JobStateRun)
		}
	})

	t.Run("set job override to skip", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		if err := cfg.SetJobOverride(repoSHA, "", "release", JobStateSkip); err != nil {
			t.Fatalf("SetJobOverride() error = %v", err)
		}

		state := cfg.GetJobState(repoSHA, "release")
		if state != JobStateSkip {
			t.Errorf("GetJobState() = %q, want %q", state, JobStateSkip)
		}
	})

	t.Run("invalid state returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		err := cfg.SetJobOverride(repoSHA, "", "job", "invalid")
		if err == nil {
			t.Error("Expected error for invalid state")
		}
	})

	t.Run("empty job ID returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		err := cfg.SetJobOverride(repoSHA, "", "", JobStateRun)
		if err == nil {
			t.Error("Expected error for empty job ID")
		}
	})

	t.Run("clear job override", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := &Config{
			global: &GlobalConfig{
				JobOverrides: map[string]RepoJobOverrides{
					repoSHA: {Jobs: map[string]string{"deploy": JobStateRun, "release": JobStateSkip}},
				},
			},
		}

		if err := cfg.ClearJobOverride(repoSHA, "deploy"); err != nil {
			t.Fatalf("ClearJobOverride() error = %v", err)
		}

		state := cfg.GetJobState(repoSHA, "deploy")
		if state != "" {
			t.Errorf("GetJobState() = %q, want empty (auto)", state)
		}

		// Other override should still be there
		state = cfg.GetJobState(repoSHA, "release")
		if state != JobStateSkip {
			t.Errorf("GetJobState() = %q, want %q", state, JobStateSkip)
		}
	})

	t.Run("get empty for unknown repo", func(t *testing.T) {
		cfg := createTestConfig(nil)
		overrides := cfg.GetJobOverrides("unknown-sha")
		if overrides != nil {
			t.Errorf("GetJobOverrides() = %v, want nil", overrides)
		}
	})

	t.Run("get empty state for unknown job", func(t *testing.T) {
		cfg := &Config{
			global: &GlobalConfig{
				JobOverrides: map[string]RepoJobOverrides{
					repoSHA: {Jobs: map[string]string{"deploy": JobStateRun}},
				},
			},
		}

		state := cfg.GetJobState(repoSHA, "unknown")
		if state != "" {
			t.Errorf("GetJobState() = %q, want empty", state)
		}
	})

	t.Run("multiple repos with different overrides", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		_ = cfg.SetJobOverride("repo1", "", "deploy", JobStateRun)
		_ = cfg.SetJobOverride("repo2", "", "deploy", JobStateSkip)

		state1 := cfg.GetJobState("repo1", "deploy")
		state2 := cfg.GetJobState("repo2", "deploy")

		if state1 != JobStateRun {
			t.Errorf("GetJobState('repo1', 'deploy') = %q, want %q", state1, JobStateRun)
		}
		if state2 != JobStateSkip {
			t.Errorf("GetJobState('repo2', 'deploy') = %q, want %q", state2, JobStateSkip)
		}
	})

	t.Run("overwrite existing override", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		_ = cfg.SetJobOverride(repoSHA, "", "deploy", JobStateRun)
		_ = cfg.SetJobOverride(repoSHA, "", "deploy", JobStateSkip)

		state := cfg.GetJobState(repoSHA, "deploy")
		if state != JobStateSkip {
			t.Errorf("GetJobState() = %q, want %q", state, JobStateSkip)
		}
	})

	t.Run("set job overrides batch", func(t *testing.T) {
		cfg := createTestConfig(nil)
		overrides := map[string]string{
			"deploy":  JobStateRun,
			"release": JobStateSkip,
		}
		cfg.SetJobOverrides(repoSHA, "", overrides)

		result := cfg.GetJobOverrides(repoSHA)
		if len(result) != 2 {
			t.Errorf("GetJobOverrides() len = %d, want 2", len(result))
		}
		if result["deploy"] != JobStateRun {
			t.Errorf("GetJobOverrides()['deploy'] = %q, want %q", result["deploy"], JobStateRun)
		}
		if result["release"] != JobStateSkip {
			t.Errorf("GetJobOverrides()['release'] = %q, want %q", result["release"], JobStateSkip)
		}
	})

	t.Run("set empty overrides clears repo", func(t *testing.T) {
		cfg := &Config{
			global: &GlobalConfig{
				JobOverrides: map[string]RepoJobOverrides{
					repoSHA: {Jobs: map[string]string{"deploy": JobStateRun}},
				},
			},
		}
		cfg.SetJobOverrides(repoSHA, "", nil)

		result := cfg.GetJobOverrides(repoSHA)
		if result != nil {
			t.Errorf("GetJobOverrides() = %v, want nil", result)
		}
	})

	t.Run("clear from nil config", func(t *testing.T) {
		cfg := &Config{global: nil}

		err := cfg.ClearJobOverride(repoSHA, "any")
		if err != nil {
			t.Fatalf("ClearJobOverride() error = %v", err)
		}
	})

	t.Run("clear from nil JobOverrides map", func(t *testing.T) {
		cfg := &Config{global: &GlobalConfig{}}

		err := cfg.ClearJobOverride(repoSHA, "any")
		if err != nil {
			t.Fatalf("ClearJobOverride() error = %v", err)
		}
	})

	t.Run("clear last override cleans up empty map", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := &Config{
			global: &GlobalConfig{
				JobOverrides: map[string]RepoJobOverrides{
					repoSHA: {Jobs: map[string]string{"deploy": JobStateRun}},
				},
			},
		}

		if err := cfg.ClearJobOverride(repoSHA, "deploy"); err != nil {
			t.Fatalf("ClearJobOverride() error = %v", err)
		}

		if _, exists := cfg.global.JobOverrides[repoSHA]; exists {
			t.Error("Expected empty repo map to be cleaned up")
		}
	})

	t.Run("persists to disk", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		if err := cfg.SetJobOverride(repoSHA, "", "deploy", JobStateRun); err != nil {
			t.Fatalf("SetJobOverride() error = %v", err)
		}

		configPath := filepath.Join(tmpDir, "detent.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("Config file was not created")
		}

		loadedCfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if loadedCfg.GetJobState(repoSHA, "deploy") != JobStateRun {
			t.Error("Loaded config should have job override")
		}
	})
}

// TestGetJobState_NilSafety tests nil safety of GetJobState
func TestGetJobState_NilSafety(t *testing.T) {
	t.Run("nil global config", func(t *testing.T) {
		cfg := &Config{global: nil}
		if state := cfg.GetJobState("repo", "job"); state != "" {
			t.Errorf("Expected empty string for nil global config, got %q", state)
		}
	})

	t.Run("nil JobOverrides map", func(t *testing.T) {
		cfg := &Config{global: &GlobalConfig{}}
		if state := cfg.GetJobState("repo", "job"); state != "" {
			t.Errorf("Expected empty string for nil JobOverrides map, got %q", state)
		}
	})
}

// TestJobOverrides_JobIDValidation tests job ID validation for security
func TestJobOverrides_JobIDValidation(t *testing.T) {
	repoSHA := "test-repo-sha"

	t.Run("SetJobOverride rejects invalid job ID format", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)

		invalidIDs := []string{
			"1invalid",      // starts with number
			"-invalid",      // starts with hyphen
			"invalid;shell", // contains semicolon (injection attempt)
			"invalid$(cmd)", // contains shell substitution
			"invalid`id`",   // contains backticks
			"invalid\nid",   // contains newline
			"invalid id",    // contains space
			"invalid>file",  // contains redirection
			"invalid|pipe",  // contains pipe
		}

		for _, id := range invalidIDs {
			err := cfg.SetJobOverride(repoSHA, "", id, JobStateRun)
			if err == nil {
				t.Errorf("Expected error for invalid job ID %q", id)
			}
		}
	})

	t.Run("SetJobOverride accepts valid job IDs", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)

		validIDs := []string{
			"build",
			"test",
			"deploy_prod",
			"release-v1",
			"_private",
			"Build_Test_Deploy",
			"job123",
			"A",
			"_",
		}

		for _, id := range validIDs {
			// Reset config for each test
			cfg = createTestConfig(nil)
			err := cfg.SetJobOverride(repoSHA, "", id, JobStateRun)
			if err != nil {
				t.Errorf("Unexpected error for valid job ID %q: %v", id, err)
			}
		}
	})

	t.Run("SetJobOverrides filters invalid job IDs", func(t *testing.T) {
		cfg := createTestConfig(nil)

		overrides := map[string]string{
			"valid_job":     JobStateRun,
			"1invalid":      JobStateRun,  // Invalid: starts with number
			"another-valid": JobStateSkip,
			"shell;inject":  JobStateRun,  // Invalid: contains semicolon
			"$(whoami)":     JobStateRun,  // Invalid: shell substitution
		}

		cfg.SetJobOverrides(repoSHA, "", overrides)

		// Only valid job IDs should be stored
		stored := cfg.GetJobOverrides(repoSHA)
		if len(stored) != 2 {
			t.Errorf("Expected 2 valid overrides, got %d: %v", len(stored), stored)
		}
		if stored["valid_job"] != JobStateRun {
			t.Error("Expected valid_job to be stored with 'run' state")
		}
		if stored["another-valid"] != JobStateSkip {
			t.Error("Expected another-valid to be stored with 'skip' state")
		}
		if _, exists := stored["1invalid"]; exists {
			t.Error("Invalid job ID '1invalid' should not be stored")
		}
		if _, exists := stored["shell;inject"]; exists {
			t.Error("Invalid job ID 'shell;inject' should not be stored")
		}
	})

	t.Run("SetJobOverrides filters invalid states", func(t *testing.T) {
		cfg := createTestConfig(nil)

		overrides := map[string]string{
			"job1": JobStateRun,
			"job2": JobStateSkip,
			"job3": "invalid_state",
			"job4": "",
			"job5": "auto", // Not a valid state (empty string means auto)
		}

		cfg.SetJobOverrides(repoSHA, "", overrides)

		stored := cfg.GetJobOverrides(repoSHA)
		if len(stored) != 2 {
			t.Errorf("Expected 2 valid overrides, got %d: %v", len(stored), stored)
		}
		if stored["job1"] != JobStateRun {
			t.Error("Expected job1 to be stored")
		}
		if stored["job2"] != JobStateSkip {
			t.Error("Expected job2 to be stored")
		}
	})

	t.Run("SetJobOverrides clears when all invalid", func(t *testing.T) {
		cfg := createTestConfig(nil)

		// First set some valid overrides
		cfg.SetJobOverrides(repoSHA, "", map[string]string{"valid": JobStateRun})
		if cfg.GetJobOverrides(repoSHA) == nil {
			t.Fatal("Setup failed: expected valid overrides to be stored")
		}

		// Then try to set all invalid
		overrides := map[string]string{
			"1invalid": JobStateRun,
			"2invalid": "bad_state",
		}

		cfg.SetJobOverrides(repoSHA, "", overrides)

		stored := cfg.GetJobOverrides(repoSHA)
		if stored != nil {
			t.Errorf("Expected nil overrides when all are invalid, got %v", stored)
		}
	})
}
