package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestIsAllowedMakeTarget(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		expected bool
	}{
		// Hardcoded allowed targets
		{"build is allowed", "build", true},
		{"test is allowed", "test", true},
		{"lint is allowed", "lint", true},
		{"fmt is allowed", "fmt", true},
		{"format is allowed", "format", true},
		{"check is allowed", "check", true},
		{"clean is allowed", "clean", true},
		{"all is allowed", "all", true},
		{"compile is allowed", "compile", true},
		{"install is allowed", "install", true},
		{"verify is allowed", "verify", true},
		{"validate is allowed", "validate", true},
		{"vet is allowed", "vet", true},
		{"staticcheck is allowed", "staticcheck", true},
		{"distclean is allowed", "distclean", true},
		{"deps is allowed", "deps", true},
		{"vendor is allowed", "vendor", true},
		{"mod is allowed", "mod", true},
		{"tidy is allowed", "tidy", true},
		{"generate is allowed", "generate", true},
		{"gen is allowed", "gen", true},
		{"proto is allowed", "proto", true},
		{"run is allowed", "run", true},
		{"dev is allowed", "dev", true},
		{"serve is allowed", "serve", true},

		// Case insensitivity
		{"BUILD uppercase allowed", "BUILD", true},
		{"Test mixed case allowed", "Test", true},
		{"LINT uppercase allowed", "LINT", true},
		{"FMT uppercase allowed", "FMT", true},
		{"ClEaN mixed case allowed", "ClEaN", true},

		// Unknown targets return false
		{"deploy not allowed", "deploy", false},
		{"custom-target not allowed", "custom-target", false},
		{"release not allowed", "release", false},
		{"push not allowed", "push", false},
		{"publish not allowed", "publish", false},
		{"empty string not allowed", "", false},
		{"random string not allowed", "xyz123", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isAllowedMakeTarget(tc.target)
			if result != tc.expected {
				t.Errorf("isAllowedMakeTarget(%q) = %v, expected %v", tc.target, result, tc.expected)
			}
		})
	}
}

func TestMakeTargetValidationInExecute(t *testing.T) {
	t.Run("hardcoded targets pass without callbacks", func(t *testing.T) {
		ctx := &Context{
			WorktreePath: t.TempDir(),
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make build test lint"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Command will fail (no Makefile) but should not be blocked by validation
		if result.Content == "" {
			t.Error("expected non-empty result content")
		}
	})

	t.Run("RepoTargetChecker is consulted for unknown targets", func(t *testing.T) {
		checkedTargets := []string{}
		ctx := &Context{
			WorktreePath: t.TempDir(),
			RepoTargetChecker: func(target string) bool {
				checkedTargets = append(checkedTargets, target)
				return target == "custom-deploy"
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make custom-deploy"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(checkedTargets) != 1 || checkedTargets[0] != "custom-deploy" {
			t.Errorf("expected RepoTargetChecker to be called with 'custom-deploy', got %v", checkedTargets)
		}

		// Should proceed to execution (will fail due to no Makefile, but not blocked)
		if result.Content == "" {
			t.Error("expected non-empty result content")
		}
	})

	t.Run("session approval is checked", func(t *testing.T) {
		ctx := &Context{
			WorktreePath:    t.TempDir(),
			ApprovedTargets: map[string]bool{"session-target": true},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make session-target"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should proceed without prompting
		if result.Content == "" {
			t.Error("expected non-empty result content")
		}
	})

	t.Run("session denial blocks without re-prompting", func(t *testing.T) {
		approverCalled := false
		ctx := &Context{
			WorktreePath:   t.TempDir(),
			DeniedTargets:  map[string]bool{"denied-target": true},
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				approverCalled = true
				return TargetApprovalResult{Allowed: true}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make denied-target"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if approverCalled {
			t.Error("TargetApprover should not be called for denied targets")
		}

		if !result.IsError {
			t.Error("expected error result for denied target")
		}

		expectedMsg := `make target "denied-target" was denied earlier in this session`
		if result.Content != expectedMsg {
			t.Errorf("expected message %q, got %q", expectedMsg, result.Content)
		}
	})

	t.Run("TargetApprover callback is called for truly unknown targets", func(t *testing.T) {
		approverCalls := []string{}
		ctx := &Context{
			WorktreePath: t.TempDir(),
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				approverCalls = append(approverCalls, target)
				return TargetApprovalResult{Allowed: true, Always: false}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make unknown-target"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(approverCalls) != 1 || approverCalls[0] != "unknown-target" {
			t.Errorf("expected TargetApprover to be called with 'unknown-target', got %v", approverCalls)
		}

		// Should proceed to execution after approval
		if result.Content == "" {
			t.Error("expected non-empty result content")
		}
	})

	t.Run("TargetApprover denial is recorded and blocks execution", func(t *testing.T) {
		ctx := &Context{
			WorktreePath: t.TempDir(),
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				return TargetApprovalResult{Allowed: false}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make unknown-target"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.IsError {
			t.Error("expected error result for denied target")
		}

		expectedMsg := `make target "unknown-target" denied by user`
		if result.Content != expectedMsg {
			t.Errorf("expected message %q, got %q", expectedMsg, result.Content)
		}

		// Verify target was recorded as denied
		if !ctx.IsTargetDenied("unknown-target") {
			t.Error("expected target to be recorded as denied")
		}
	})

	t.Run("TargetPersister is called when user selects always", func(t *testing.T) {
		persistedTargets := []string{}
		ctx := &Context{
			WorktreePath: t.TempDir(),
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				return TargetApprovalResult{Allowed: true, Always: true}, nil
			},
			TargetPersister: func(target string) error {
				persistedTargets = append(persistedTargets, target)
				return nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make persist-this"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(persistedTargets) != 1 || persistedTargets[0] != "persist-this" {
			t.Errorf("expected TargetPersister to be called with 'persist-this', got %v", persistedTargets)
		}

		// Should also be approved for session
		if !ctx.IsTargetApproved("persist-this") {
			t.Error("expected target to be approved for session")
		}

		if result.Content == "" {
			t.Error("expected non-empty result content")
		}
	})

	t.Run("TargetPersister not called when user selects session-only", func(t *testing.T) {
		persisterCalled := false
		ctx := &Context{
			WorktreePath: t.TempDir(),
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				return TargetApprovalResult{Allowed: true, Always: false}, nil
			},
			TargetPersister: func(target string) error {
				persisterCalled = true
				return nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make session-only"})
		_, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if persisterCalled {
			t.Error("TargetPersister should not be called for session-only approval")
		}

		// Should still be approved for session
		if !ctx.IsTargetApproved("session-only") {
			t.Error("expected target to be approved for session")
		}
	})

	t.Run("multiple targets in one command all validated", func(t *testing.T) {
		approverCalls := []string{}
		ctx := &Context{
			WorktreePath: t.TempDir(),
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				approverCalls = append(approverCalls, target)
				return TargetApprovalResult{Allowed: true}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		// Mix of hardcoded (build, test) and unknown targets (deploy, release)
		input, _ := json.Marshal(runCommandInput{Command: "make build test deploy release"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only ask about unknown targets
		if len(approverCalls) != 2 {
			t.Errorf("expected 2 approver calls for unknown targets, got %d: %v", len(approverCalls), approverCalls)
		}

		if approverCalls[0] != "deploy" || approverCalls[1] != "release" {
			t.Errorf("expected approver calls for 'deploy' and 'release', got %v", approverCalls)
		}

		if result.Content == "" {
			t.Error("expected non-empty result content")
		}
	})

	t.Run("flags are skipped in validation", func(t *testing.T) {
		approverCalls := []string{}
		ctx := &Context{
			WorktreePath: t.TempDir(),
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				approverCalls = append(approverCalls, target)
				return TargetApprovalResult{Allowed: true}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make -j4 --keep-going custom"})
		_, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only 'custom' should trigger approver, not -j4 or --keep-going
		if len(approverCalls) != 1 || approverCalls[0] != "custom" {
			t.Errorf("expected only 'custom' to trigger approver, got %v", approverCalls)
		}
	})

	t.Run("variable assignments are skipped in validation", func(t *testing.T) {
		approverCalls := []string{}
		ctx := &Context{
			WorktreePath: t.TempDir(),
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				approverCalls = append(approverCalls, target)
				return TargetApprovalResult{Allowed: true}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make CC=gcc CXX=g++ custom"})
		_, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only 'custom' should trigger approver, not CC=gcc or CXX=g++
		if len(approverCalls) != 1 || approverCalls[0] != "custom" {
			t.Errorf("expected only 'custom' to trigger approver, got %v", approverCalls)
		}
	})

	t.Run("no TargetApprover rejects unknown targets", func(t *testing.T) {
		ctx := &Context{
			WorktreePath:   t.TempDir(),
			TargetApprover: nil,
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make unknown-target"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.IsError {
			t.Error("expected error result when TargetApprover is nil")
		}

		expectedSubstr := `make target "unknown-target" not in allowlist`
		if result.Content == "" || result.Content[:len(expectedSubstr)] != expectedSubstr {
			t.Errorf("expected message to start with %q, got %q", expectedSubstr, result.Content)
		}
	})

	t.Run("make without targets passes validation", func(t *testing.T) {
		ctx := &Context{
			WorktreePath: t.TempDir(),
		}
		tool := NewRunCommandTool(ctx)

		// Just 'make' with no targets - should proceed to default target
		input, _ := json.Marshal(runCommandInput{Command: "make"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Will fail due to no Makefile but should not be blocked by validation
		if result.Content == "" {
			t.Error("expected non-empty result content")
		}
	})

	t.Run("validation order: hardcoded -> repo -> session -> denied -> approver", func(t *testing.T) {
		repoCheckerCalls := []string{}
		approverCalls := []string{}

		ctx := &Context{
			WorktreePath:    t.TempDir(),
			ApprovedTargets: map[string]bool{"session-approved": true},
			DeniedTargets:   map[string]bool{"session-denied": true},
			RepoTargetChecker: func(target string) bool {
				repoCheckerCalls = append(repoCheckerCalls, target)
				return target == "repo-approved"
			},
			TargetApprover: func(target string) (TargetApprovalResult, error) {
				approverCalls = append(approverCalls, target)
				return TargetApprovalResult{Allowed: true}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		// 'build' is hardcoded - should not check repo or approver
		input, _ := json.Marshal(runCommandInput{Command: "make build"})
		tool.Execute(context.Background(), input)

		if len(repoCheckerCalls) > 0 || len(approverCalls) > 0 {
			t.Error("hardcoded target should not trigger repo checker or approver")
		}

		// 'repo-approved' - should check repo but not approver
		repoCheckerCalls = nil
		approverCalls = nil
		input, _ = json.Marshal(runCommandInput{Command: "make repo-approved"})
		tool.Execute(context.Background(), input)

		if len(repoCheckerCalls) != 1 {
			t.Error("expected repo checker to be called for repo-approved target")
		}
		if len(approverCalls) > 0 {
			t.Error("approver should not be called when repo checker approves")
		}

		// 'session-approved' - should check repo (returns false) but use session approval
		repoCheckerCalls = nil
		approverCalls = nil
		input, _ = json.Marshal(runCommandInput{Command: "make session-approved"})
		tool.Execute(context.Background(), input)

		if len(repoCheckerCalls) != 1 {
			t.Error("expected repo checker to be called for session-approved target")
		}
		if len(approverCalls) > 0 {
			t.Error("approver should not be called when session is already approved")
		}
	})
}
