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
		Model:       DefaultModel,
		BudgetUSD:   DefaultBudgetUSD,
		TimeoutMins: DefaultTimeoutMins,
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
		t.Setenv("HOME", tmpDir)

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

	t.Run("re-trust preserves approved targets", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := createTestConfig(map[string]TrustedRepo{
			"abc123": {
				RemoteURL:       "github.com/user/repo",
				TrustedAt:       time.Now().Add(-time.Hour),
				ApprovedTargets: []string{"build", "test"},
			},
		})

		err := cfg.TrustRepo("abc123", "github.com/user/repo-updated")
		if err != nil {
			t.Fatalf("TrustRepo() error = %v", err)
		}

		repo := cfg.global.TrustedRepos["abc123"]
		if len(repo.ApprovedTargets) != 2 {
			t.Errorf("ApprovedTargets length = %d, want 2", len(repo.ApprovedTargets))
		}
		if repo.ApprovedTargets[0] != "build" || repo.ApprovedTargets[1] != "test" {
			t.Errorf("ApprovedTargets = %v, want [build, test]", repo.ApprovedTargets)
		}
		if repo.RemoteURL != "github.com/user/repo-updated" {
			t.Errorf("RemoteURL = %v, want github.com/user/repo-updated", repo.RemoteURL)
		}
	})

	t.Run("saves to disk", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := createTestConfig(nil)
		err := cfg.TrustRepo("abc123", "github.com/user/repo")
		if err != nil {
			t.Fatalf("TrustRepo() error = %v", err)
		}

		// Verify config file was created
		configPath := filepath.Join(tmpDir, ".detent", "config.jsonc")
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

// TestIsTargetApprovedForRepo tests the IsTargetApprovedForRepo method
func TestIsTargetApprovedForRepo(t *testing.T) {
	tests := []struct {
		name           string
		trustedRepos   map[string]TrustedRepo
		firstCommitSHA string
		target         string
		want           bool
	}{
		{
			name:           "nil map returns false",
			trustedRepos:   nil,
			firstCommitSHA: "abc123",
			target:         "build",
			want:           false,
		},
		{
			name:           "empty map returns false",
			trustedRepos:   map[string]TrustedRepo{},
			firstCommitSHA: "abc123",
			target:         "build",
			want:           false,
		},
		{
			name: "repo not found returns false",
			trustedRepos: map[string]TrustedRepo{
				"def456": {
					ApprovedTargets: []string{"build"},
				},
			},
			firstCommitSHA: "abc123",
			target:         "build",
			want:           false,
		},
		{
			name: "target found returns true",
			trustedRepos: map[string]TrustedRepo{
				"abc123": {
					ApprovedTargets: []string{"build", "test", "lint"},
				},
			},
			firstCommitSHA: "abc123",
			target:         "test",
			want:           true,
		},
		{
			name: "target not found returns false",
			trustedRepos: map[string]TrustedRepo{
				"abc123": {
					ApprovedTargets: []string{"build", "test"},
				},
			},
			firstCommitSHA: "abc123",
			target:         "deploy",
			want:           false,
		},
		{
			name: "case-insensitive match uppercase target",
			trustedRepos: map[string]TrustedRepo{
				"abc123": {
					ApprovedTargets: []string{"build"},
				},
			},
			firstCommitSHA: "abc123",
			target:         "BUILD",
			want:           true,
		},
		{
			name: "case-insensitive match mixed case target",
			trustedRepos: map[string]TrustedRepo{
				"abc123": {
					ApprovedTargets: []string{"build"},
				},
			},
			firstCommitSHA: "abc123",
			target:         "Build",
			want:           true,
		},
		{
			name: "empty approved targets returns false",
			trustedRepos: map[string]TrustedRepo{
				"abc123": {
					ApprovedTargets: []string{},
				},
			},
			firstCommitSHA: "abc123",
			target:         "build",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createTestConfig(tt.trustedRepos)
			got := cfg.IsTargetApprovedForRepo(tt.firstCommitSHA, tt.target)
			if got != tt.want {
				t.Errorf("IsTargetApprovedForRepo(%q, %q) = %v, want %v", tt.firstCommitSHA, tt.target, got, tt.want)
			}
		})
	}
}

// TestApproveTargetForRepo tests the ApproveTargetForRepo method
func TestApproveTargetForRepo(t *testing.T) {
	t.Run("approve new target", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := createTestConfig(map[string]TrustedRepo{
			"abc123": {
				RemoteURL: "github.com/user/repo",
				TrustedAt: time.Now(),
			},
		})

		err := cfg.ApproveTargetForRepo("abc123", "build")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		if !cfg.IsTargetApprovedForRepo("abc123", "build") {
			t.Error("Expected target to be approved after ApproveTargetForRepo()")
		}
	})

	t.Run("already approved is no-op", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := createTestConfig(map[string]TrustedRepo{
			"abc123": {
				RemoteURL:       "github.com/user/repo",
				TrustedAt:       time.Now(),
				ApprovedTargets: []string{"build"},
			},
		})

		err := cfg.ApproveTargetForRepo("abc123", "build")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		repo := cfg.global.TrustedRepos["abc123"]
		if len(repo.ApprovedTargets) != 1 {
			t.Errorf("ApprovedTargets length = %d, want 1 (should not duplicate)", len(repo.ApprovedTargets))
		}
	})

	t.Run("case-insensitive already approved", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := createTestConfig(map[string]TrustedRepo{
			"abc123": {
				RemoteURL:       "github.com/user/repo",
				TrustedAt:       time.Now(),
				ApprovedTargets: []string{"build"},
			},
		})

		err := cfg.ApproveTargetForRepo("abc123", "BUILD")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		repo := cfg.global.TrustedRepos["abc123"]
		if len(repo.ApprovedTargets) != 1 {
			t.Errorf("ApprovedTargets length = %d, want 1 (case-insensitive match)", len(repo.ApprovedTargets))
		}
	})

	t.Run("stored lowercase", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := createTestConfig(map[string]TrustedRepo{
			"abc123": {
				RemoteURL: "github.com/user/repo",
				TrustedAt: time.Now(),
			},
		})

		err := cfg.ApproveTargetForRepo("abc123", "BUILD")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		repo := cfg.global.TrustedRepos["abc123"]
		if len(repo.ApprovedTargets) != 1 {
			t.Fatalf("ApprovedTargets length = %d, want 1", len(repo.ApprovedTargets))
		}
		if repo.ApprovedTargets[0] != "build" {
			t.Errorf("ApprovedTargets[0] = %q, want %q (stored lowercase)", repo.ApprovedTargets[0], "build")
		}
	})

	t.Run("repo not trusted nil map error", func(t *testing.T) {
		cfg := createTestConfig(nil)

		err := cfg.ApproveTargetForRepo("abc123", "build")
		if err == nil {
			t.Fatal("Expected error when repo not trusted")
		}
		if err.Error() != "repository not trusted" {
			t.Errorf("Error = %v, want 'repository not trusted'", err)
		}
	})

	t.Run("repo not trusted empty map error", func(t *testing.T) {
		cfg := createTestConfig(map[string]TrustedRepo{})

		err := cfg.ApproveTargetForRepo("abc123", "build")
		if err == nil {
			t.Fatal("Expected error when repo not trusted")
		}
		if err.Error() != "repository not trusted" {
			t.Errorf("Error = %v, want 'repository not trusted'", err)
		}
	})

	t.Run("repo not trusted different sha error", func(t *testing.T) {
		cfg := createTestConfig(map[string]TrustedRepo{
			"def456": {
				RemoteURL: "github.com/other/repo",
				TrustedAt: time.Now(),
			},
		})

		err := cfg.ApproveTargetForRepo("abc123", "build")
		if err == nil {
			t.Fatal("Expected error when specific repo not trusted")
		}
		if err.Error() != "repository not trusted" {
			t.Errorf("Error = %v, want 'repository not trusted'", err)
		}
	})

	t.Run("saves to disk", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := createTestConfig(map[string]TrustedRepo{
			"abc123": {
				RemoteURL: "github.com/user/repo",
				TrustedAt: time.Now(),
			},
		})

		// First save the initial config
		err := cfg.SaveGlobal()
		if err != nil {
			t.Fatalf("SaveGlobal() error = %v", err)
		}

		err = cfg.ApproveTargetForRepo("abc123", "build")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		// Reload config and verify
		loadedCfg, err := Load("")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if !loadedCfg.IsTargetApprovedForRepo("abc123", "build") {
			t.Error("Loaded config should have approved target")
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
			Model:       "claude-sonnet-4-5",
			BudgetUSD:   float64Ptr(1.0),
			TimeoutMins: intPtr(10),
		}
		local := &LocalConfig{
			Model:       "claude-opus-4-5",
			BudgetUSD:   &budget,
			TimeoutMins: &timeout,
		}

		cfg := merge(global, local, "")

		if cfg.Model != "claude-opus-4-5" {
			t.Errorf("Model = %v, want claude-opus-4-5", cfg.Model)
		}
		if cfg.BudgetUSD != 5.0 {
			t.Errorf("BudgetUSD = %v, want 5.0", cfg.BudgetUSD)
		}
		if cfg.TimeoutMins != 30 {
			t.Errorf("TimeoutMins = %v, want 30", cfg.TimeoutMins)
		}
	})

	t.Run("global used when local nil", func(t *testing.T) {
		global := &GlobalConfig{
			Model:       "claude-sonnet-4-5",
			BudgetUSD:   float64Ptr(2.0),
			TimeoutMins: intPtr(15),
		}

		cfg := merge(global, nil, "")

		if cfg.Model != "claude-sonnet-4-5" {
			t.Errorf("Model = %v, want claude-sonnet-4-5", cfg.Model)
		}
		if cfg.BudgetUSD != 2.0 {
			t.Errorf("BudgetUSD = %v, want 2.0", cfg.BudgetUSD)
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
		if cfg.BudgetUSD != DefaultBudgetUSD {
			t.Errorf("BudgetUSD = %v, want %v", cfg.BudgetUSD, DefaultBudgetUSD)
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
			content: `{"budget_usd": "five"}`,
			wantErr: true,
		},
		{
			name:    "wrong type for timeout - string instead of number",
			content: `{"timeout_mins": "ten"}`,
			wantErr: true,
		},
		{
			name:    "wrong type for verbose - string instead of bool",
			content: `{"verbose": "yes"}`,
			wantErr: true,
		},
		{
			name:    "null values are valid",
			content: `{"model": null, "budget_usd": null, "timeout_mins": null}`,
			wantErr: false,
			checkResult: func(t *testing.T, cfg *Config) {
				if cfg.Model != DefaultModel {
					t.Errorf("Model = %v, want %v", cfg.Model, DefaultModel)
				}
				if cfg.BudgetUSD != DefaultBudgetUSD {
					t.Errorf("BudgetUSD = %v, want %v", cfg.BudgetUSD, DefaultBudgetUSD)
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
		{
			name:    "valid JSONC with comments",
			content: `{
				// This is a comment
				"model": "claude-opus-4-5",
				/* Multi-line
				   comment */
				"budget_usd": 5.0
			}`,
			wantErr: false,
			checkResult: func(t *testing.T, cfg *Config) {
				if cfg.Model != "claude-opus-4-5" {
					t.Errorf("Model = %v, want claude-opus-4-5", cfg.Model)
				}
				if cfg.BudgetUSD != 5.0 {
					t.Errorf("BudgetUSD = %v, want 5.0", cfg.BudgetUSD)
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
			configPath := filepath.Join(tmpDir, "config.jsonc")
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
				BudgetUSD:   tt.budget,
				TimeoutMins: tt.timeout,
			}

			cfg := merge(global, nil, "")

			if cfg.BudgetUSD != tt.wantBudget {
				t.Errorf("BudgetUSD = %v, want %v", cfg.BudgetUSD, tt.wantBudget)
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
		configPath := filepath.Join(tmpDir, "config.jsonc")
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
		configPath := filepath.Join(tmpDir, "config.jsonc")
		globalContent := `{"api_key": "global-key"}`
		if err := os.WriteFile(configPath, []byte(globalContent), 0o600); err != nil {
			t.Fatalf("Failed to write global config file: %v", err)
		}

		// Create local config
		localPath := filepath.Join(repoDir, "detent.jsonc")
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
