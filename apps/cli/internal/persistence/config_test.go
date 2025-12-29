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
		configPath := filepath.Join(tmpDir, "config.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("Config file was not created")
		}

		// Reload config and verify
		loadedCfg, err := Load("")
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
	cfg := &Config{
		ExtraCommands: []string{"bun run typecheck", "pnpm exec playwright *", "make deploy"},
	}

	if !cfg.MatchesCommand("bun run typecheck") {
		t.Error("Expected exact match for 'bun run typecheck'")
	}
	if !cfg.MatchesCommand("pnpm exec playwright test") {
		t.Error("Expected wildcard match for 'pnpm exec playwright test'")
	}
	if !cfg.MatchesCommand("make deploy") {
		t.Error("Expected match for 'make deploy'")
	}
	if cfg.MatchesCommand("npm run test") {
		t.Error("Expected no match for 'npm run test'")
	}
}

// TestConfigMerge tests the config merge precedence
func TestConfigMerge(t *testing.T) {
	t.Run("local overrides global", func(t *testing.T) {
		budget := 5.0
		timeout := 30
		global := &GlobalConfig{
			Model:           "claude-sonnet-4-5",
			BudgetPerRunUSD: float64Ptr(1.0),
			TimeoutMins:     intPtr(10),
		}
		local := &LocalConfig{
			Model:           "claude-opus-4-5",
			BudgetPerRunUSD: &budget,
			TimeoutMins:     &timeout,
		}

		cfg := merge(global, local, "")

		if cfg.Model != "claude-opus-4-5" {
			t.Errorf("Model = %v, want claude-opus-4-5", cfg.Model)
		}
		if cfg.BudgetPerRunUSD != 5.0 {
			t.Errorf("BudgetPerRunUSD = %v, want 5.0", cfg.BudgetPerRunUSD)
		}
		if cfg.TimeoutMins != 30 {
			t.Errorf("TimeoutMins = %v, want 30", cfg.TimeoutMins)
		}
	})

	t.Run("global used when local nil", func(t *testing.T) {
		global := &GlobalConfig{
			Model:           "claude-sonnet-4-5",
			BudgetPerRunUSD: float64Ptr(2.0),
			TimeoutMins:     intPtr(15),
		}

		cfg := merge(global, nil, "")

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

	t.Run("defaults used when both nil", func(t *testing.T) {
		cfg := merge(nil, nil, "")

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
			configPath := filepath.Join(tmpDir, "config.json")
			if err := os.WriteFile(configPath, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			cfg, err := Load("")
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

			cfg := merge(global, nil, "")

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

			cfg := merge(global, nil, "")

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
		configPath := filepath.Join(tmpDir, "config.json")
		content := `{"api_key": "global-key-67890"}`
		if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.APIKey != "env-key-12345" {
			t.Errorf("APIKey = %v, want env-key-12345", cfg.APIKey)
		}
	})

	t.Run("env overrides when both global and local exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoDir := t.TempDir()
		// Use DETENT_HOME to bypass the cached directory path
		t.Setenv(DetentHomeEnv, tmpDir)
		t.Setenv("ANTHROPIC_API_KEY", "env-key-override")

		// Create global config with api_key (DETENT_HOME points to .detent dir equivalent)
		configPath := filepath.Join(tmpDir, "config.json")
		globalContent := `{"api_key": "global-key"}`
		if err := os.WriteFile(configPath, []byte(globalContent), 0o600); err != nil {
			t.Fatalf("Failed to write global config file: %v", err)
		}

		// Create local config
		localPath := filepath.Join(repoDir, "detent.json")
		localContent := `{"model": "claude-opus-4-5"}`
		if err := os.WriteFile(localPath, []byte(localContent), 0o600); err != nil {
			t.Fatalf("Failed to write local config file: %v", err)
		}

		cfg, err := Load(repoDir)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.APIKey != "env-key-override" {
			t.Errorf("APIKey = %v, want env-key-override", cfg.APIKey)
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

// TestRecordSpend tests the spend tracking functionality
func TestRecordSpend(t *testing.T) {
	t.Run("records spend for current month", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		if err := cfg.RecordSpend(1.50); err != nil {
			t.Fatalf("RecordSpend() error = %v", err)
		}

		if cfg.GetMonthlySpend() != 1.50 {
			t.Errorf("GetMonthlySpend() = %v, want 1.50", cfg.GetMonthlySpend())
		}
	})

	t.Run("ignores negative amounts", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		if err := cfg.RecordSpend(5.00); err != nil {
			t.Fatalf("RecordSpend() error = %v", err)
		}
		// Attempt to record negative amount (should be ignored)
		if err := cfg.RecordSpend(-10.00); err != nil {
			t.Fatalf("RecordSpend() error = %v", err)
		}

		// Spend should still be 5.00, not -5.00
		if cfg.GetMonthlySpend() != 5.00 {
			t.Errorf("GetMonthlySpend() = %v, want 5.00 (negative amount should be ignored)", cfg.GetMonthlySpend())
		}
	})

	t.Run("accumulates spend", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		if err := cfg.RecordSpend(1.00); err != nil {
			t.Fatalf("RecordSpend() error = %v", err)
		}
		if err := cfg.RecordSpend(0.50); err != nil {
			t.Fatalf("RecordSpend() error = %v", err)
		}

		if cfg.GetMonthlySpend() != 1.50 {
			t.Errorf("GetMonthlySpend() = %v, want 1.50", cfg.GetMonthlySpend())
		}
	})

	t.Run("persists to disk", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cfg := createTestConfig(nil)
		if err := cfg.RecordSpend(2.00); err != nil {
			t.Fatalf("RecordSpend() error = %v", err)
		}

		// Reload config
		loadedCfg, err := Load("")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if loadedCfg.GetMonthlySpend() != 2.00 {
			t.Errorf("GetMonthlySpend() after reload = %v, want 2.00", loadedCfg.GetMonthlySpend())
		}
	})
}

// TestSpendHistoryPruning tests that old months are pruned
func TestSpendHistoryPruning(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(DetentHomeEnv, tmpDir)

	now := time.Now()

	cfg := &Config{
		Model:           DefaultModel,
		BudgetPerRunUSD: DefaultBudgetPerRunUSD,
		TimeoutMins:     DefaultTimeoutMins,
		global: &GlobalConfig{
			SpendHistory: map[string]float64{
				now.Format("2006-01"):                                             5.00,  // Current month - keep
				now.AddDate(0, -1, 0).Format("2006-01"):                           3.00,  // 1 month ago - keep
				now.AddDate(0, -2, 0).Format("2006-01"):                           2.00,  // 2 months ago - keep
				now.AddDate(0, -3, 0).Format("2006-01"):                           10.00, // 3 months ago - prune
				now.AddDate(0, -6, 0).Format("2006-01"):                           1.00,  // 6 months ago - prune
				time.Date(now.Year()-1, now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01"): 0.50, // Last year - prune
			},
		},
	}

	// Record a small amount to trigger pruning
	if err := cfg.RecordSpend(0.01); err != nil {
		t.Fatalf("RecordSpend() error = %v", err)
	}

	// Check that old months were pruned
	history := cfg.global.SpendHistory
	if len(history) > 3 {
		t.Errorf("SpendHistory should have at most 3 entries, got %d", len(history))
	}

	// Current month should exist with accumulated value
	currentMonth := now.Format("2006-01")
	if history[currentMonth] != 5.01 {
		t.Errorf("Current month spend = %v, want 5.01", history[currentMonth])
	}

	// Old months should be pruned
	oldMonth := now.AddDate(0, -6, 0).Format("2006-01")
	if _, exists := history[oldMonth]; exists {
		t.Error("Old month (6 months ago) should have been pruned")
	}
}

// TestRemainingMonthlyBudget tests the remaining budget calculation
func TestRemainingMonthlyBudget(t *testing.T) {
	t.Run("returns -1 when unlimited", func(t *testing.T) {
		cfg := &Config{
			BudgetMonthlyUSD: 0, // 0 means unlimited
			global:           &GlobalConfig{},
		}

		if cfg.RemainingMonthlyBudget() != -1 {
			t.Errorf("RemainingMonthlyBudget() = %v, want -1 (unlimited)", cfg.RemainingMonthlyBudget())
		}
	})

	t.Run("calculates remaining correctly", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		currentMonth := time.Now().Format("2006-01")
		cfg := &Config{
			BudgetMonthlyUSD: 50.0,
			global: &GlobalConfig{
				SpendHistory: map[string]float64{
					currentMonth: 20.0,
				},
			},
		}

		remaining := cfg.RemainingMonthlyBudget()
		if remaining != 30.0 {
			t.Errorf("RemainingMonthlyBudget() = %v, want 30.0", remaining)
		}
	})

	t.Run("returns 0 when over budget", func(t *testing.T) {
		currentMonth := time.Now().Format("2006-01")
		cfg := &Config{
			BudgetMonthlyUSD: 10.0,
			global: &GlobalConfig{
				SpendHistory: map[string]float64{
					currentMonth: 15.0, // Over budget
				},
			},
		}

		if cfg.RemainingMonthlyBudget() != 0 {
			t.Errorf("RemainingMonthlyBudget() = %v, want 0 (over budget)", cfg.RemainingMonthlyBudget())
		}
	})

	t.Run("returns full budget when no spend", func(t *testing.T) {
		cfg := &Config{
			BudgetMonthlyUSD: 100.0,
			global:           &GlobalConfig{},
		}

		if cfg.RemainingMonthlyBudget() != 100.0 {
			t.Errorf("RemainingMonthlyBudget() = %v, want 100.0", cfg.RemainingMonthlyBudget())
		}
	})
}

// TestMonthlyBudgetClamping tests that monthly budget values are clamped correctly
func TestMonthlyBudgetClamping(t *testing.T) {
	tests := []struct {
		name       string
		budget     float64
		wantBudget float64
	}{
		{
			name:       "negative clamps to 0",
			budget:     -10.0,
			wantBudget: 0,
		},
		{
			name:       "within range stays same",
			budget:     50.0,
			wantBudget: 50.0,
		},
		{
			name:       "at max stays at max",
			budget:     1000.0,
			wantBudget: 1000.0,
		},
		{
			name:       "over max clamps to max",
			budget:     5000.0,
			wantBudget: 1000.0,
		},
		{
			name:       "zero stays zero (unlimited)",
			budget:     0,
			wantBudget: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global := &GlobalConfig{
				BudgetMonthlyUSD: &tt.budget,
			}

			cfg := merge(global, nil, "")

			if cfg.BudgetMonthlyUSD != tt.wantBudget {
				t.Errorf("BudgetMonthlyUSD = %v, want %v", cfg.BudgetMonthlyUSD, tt.wantBudget)
			}
		})
	}
}
