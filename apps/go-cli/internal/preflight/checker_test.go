package preflight

import (
	"testing"

	"github.com/detentsh/core/git"
)

// TestPreflightResultStruct verifies that the Result struct has the expected fields
func TestPreflightResultStruct(t *testing.T) {
	called := false
	cleanup := func() { called = true }

	result := &Result{
		TmpDir:           "/tmp/test",
		WorktreeInfo:     &git.WorktreeInfo{},
		CleanupWorkflows: cleanup,
		CleanupWorktree:  cleanup,
		RepoRoot:         "/tmp/repo",
	}

	if result.TmpDir != "/tmp/test" {
		t.Errorf("expected TmpDir to be /tmp/test, got %s", result.TmpDir)
	}

	if result.WorktreeInfo == nil {
		t.Error("expected WorktreeInfo to be non-nil")
	}

	result.Cleanup()
	if !called {
		t.Error("expected Cleanup to call cleanup functions")
	}
}
