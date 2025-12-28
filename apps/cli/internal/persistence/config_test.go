package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
			cfg := &GlobalConfig{
				TrustedRepos: tt.trustedRepos,
			}
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
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		cfg := &GlobalConfig{}
		err := cfg.TrustRepo("abc123", "github.com/user/repo")
		if err != nil {
			t.Fatalf("TrustRepo() error = %v", err)
		}

		if !cfg.IsTrustedRepo("abc123") {
			t.Error("Expected repo to be trusted after TrustRepo()")
		}

		repo := cfg.TrustedRepos["abc123"]
		if repo.RemoteURL != "github.com/user/repo" {
			t.Errorf("RemoteURL = %v, want %v", repo.RemoteURL, "github.com/user/repo")
		}
		if repo.TrustedAt.IsZero() {
			t.Error("TrustedAt should not be zero")
		}
	})

	t.Run("re-trust preserves approved targets", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		cfg := &GlobalConfig{
			TrustedRepos: map[string]TrustedRepo{
				"abc123": {
					RemoteURL:       "github.com/user/repo",
					TrustedAt:       time.Now().Add(-time.Hour),
					ApprovedTargets: []string{"build", "test"},
				},
			},
		}

		err := cfg.TrustRepo("abc123", "github.com/user/repo-updated")
		if err != nil {
			t.Fatalf("TrustRepo() error = %v", err)
		}

		repo := cfg.TrustedRepos["abc123"]
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
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		cfg := &GlobalConfig{}
		err := cfg.TrustRepo("abc123", "github.com/user/repo")
		if err != nil {
			t.Fatalf("TrustRepo() error = %v", err)
		}

		// Verify config file was created
		configPath := filepath.Join(tmpDir, ".detent", "config.yaml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Error("Config file was not created")
		}

		// Reload config and verify
		loadedCfg, err := LoadGlobalConfig()
		if err != nil {
			t.Fatalf("LoadGlobalConfig() error = %v", err)
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
			cfg := &GlobalConfig{
				TrustedRepos: tt.trustedRepos,
			}
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
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		cfg := &GlobalConfig{
			TrustedRepos: map[string]TrustedRepo{
				"abc123": {
					RemoteURL: "github.com/user/repo",
					TrustedAt: time.Now(),
				},
			},
		}

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
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		cfg := &GlobalConfig{
			TrustedRepos: map[string]TrustedRepo{
				"abc123": {
					RemoteURL:       "github.com/user/repo",
					TrustedAt:       time.Now(),
					ApprovedTargets: []string{"build"},
				},
			},
		}

		err := cfg.ApproveTargetForRepo("abc123", "build")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		repo := cfg.TrustedRepos["abc123"]
		if len(repo.ApprovedTargets) != 1 {
			t.Errorf("ApprovedTargets length = %d, want 1 (should not duplicate)", len(repo.ApprovedTargets))
		}
	})

	t.Run("case-insensitive already approved", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		cfg := &GlobalConfig{
			TrustedRepos: map[string]TrustedRepo{
				"abc123": {
					RemoteURL:       "github.com/user/repo",
					TrustedAt:       time.Now(),
					ApprovedTargets: []string{"build"},
				},
			},
		}

		err := cfg.ApproveTargetForRepo("abc123", "BUILD")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		repo := cfg.TrustedRepos["abc123"]
		if len(repo.ApprovedTargets) != 1 {
			t.Errorf("ApprovedTargets length = %d, want 1 (case-insensitive match)", len(repo.ApprovedTargets))
		}
	})

	t.Run("stored lowercase", func(t *testing.T) {
		tmpDir := t.TempDir()
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		cfg := &GlobalConfig{
			TrustedRepos: map[string]TrustedRepo{
				"abc123": {
					RemoteURL: "github.com/user/repo",
					TrustedAt: time.Now(),
				},
			},
		}

		err := cfg.ApproveTargetForRepo("abc123", "BUILD")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		repo := cfg.TrustedRepos["abc123"]
		if len(repo.ApprovedTargets) != 1 {
			t.Fatalf("ApprovedTargets length = %d, want 1", len(repo.ApprovedTargets))
		}
		if repo.ApprovedTargets[0] != "build" {
			t.Errorf("ApprovedTargets[0] = %q, want %q (stored lowercase)", repo.ApprovedTargets[0], "build")
		}
	})

	t.Run("repo not trusted nil map error", func(t *testing.T) {
		cfg := &GlobalConfig{
			TrustedRepos: nil,
		}

		err := cfg.ApproveTargetForRepo("abc123", "build")
		if err == nil {
			t.Fatal("Expected error when repo not trusted")
		}
		if err.Error() != "repository not trusted" {
			t.Errorf("Error = %v, want 'repository not trusted'", err)
		}
	})

	t.Run("repo not trusted empty map error", func(t *testing.T) {
		cfg := &GlobalConfig{
			TrustedRepos: map[string]TrustedRepo{},
		}

		err := cfg.ApproveTargetForRepo("abc123", "build")
		if err == nil {
			t.Fatal("Expected error when repo not trusted")
		}
		if err.Error() != "repository not trusted" {
			t.Errorf("Error = %v, want 'repository not trusted'", err)
		}
	})

	t.Run("repo not trusted different sha error", func(t *testing.T) {
		cfg := &GlobalConfig{
			TrustedRepos: map[string]TrustedRepo{
				"def456": {
					RemoteURL: "github.com/other/repo",
					TrustedAt: time.Now(),
				},
			},
		}

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
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tmpDir)
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		cfg := &GlobalConfig{
			TrustedRepos: map[string]TrustedRepo{
				"abc123": {
					RemoteURL: "github.com/user/repo",
					TrustedAt: time.Now(),
				},
			},
		}

		// First save the initial config
		err := SaveGlobalConfig(cfg)
		if err != nil {
			t.Fatalf("SaveGlobalConfig() error = %v", err)
		}

		err = cfg.ApproveTargetForRepo("abc123", "build")
		if err != nil {
			t.Fatalf("ApproveTargetForRepo() error = %v", err)
		}

		// Reload config and verify
		loadedCfg, err := LoadGlobalConfig()
		if err != nil {
			t.Fatalf("LoadGlobalConfig() error = %v", err)
		}
		if !loadedCfg.IsTargetApprovedForRepo("abc123", "build") {
			t.Error("Loaded config should have approved target")
		}
	})
}
