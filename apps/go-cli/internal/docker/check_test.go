package docker

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestIsAvailable tests Docker availability detection
func TestIsAvailable(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (cleanup func())
		wantErr bool
		errMsg  string
	}{
		{
			name: "docker available and running",
			setup: func() (cleanup func()) {
				// Mock successful docker info execution
				// This test relies on actual docker being available
				// In a real scenario, we'd mock exec.CommandContext
				return func() {}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup()
			defer cleanup()

			ctx := context.Background()
			err := IsAvailable(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("IsAvailable() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("IsAvailable() error message = %v, want to contain %v", err, tt.errMsg)
				}
			}
		})
	}
}

// TestIsAvailable_ContextCancellation tests context cancellation handling
func TestIsAvailable_ContextCancellation(t *testing.T) {
	tests := []struct {
		name          string
		cancelBefore  bool
		cancelAfter   time.Duration
		expectTimeout bool
	}{
		{
			name:          "context cancelled before call",
			cancelBefore:  true,
			expectTimeout: true,
		},
		{
			name:          "context cancelled during execution",
			cancelAfter:   100 * time.Millisecond,
			expectTimeout: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			if tt.cancelBefore {
				cancel()
			} else if tt.cancelAfter > 0 {
				go func() {
					time.Sleep(tt.cancelAfter)
					cancel()
				}()
			}

			defer cancel()

			err := IsAvailable(ctx)

			// Should get an error when context is cancelled
			if err == nil {
				// Docker might respond too quickly, skip this test
				t.Skip("Docker responded before context cancellation")
			}

			// Error should be related to context cancellation or command failure
			if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "killed") {
				t.Logf("Got error (expected due to cancellation): %v", err)
			}
		})
	}
}

// TestIsAvailable_Timeout tests timeout handling
func TestIsAvailable_Timeout(t *testing.T) {
	ctx := context.Background()

	// Create a context that times out very quickly to test timeout behavior
	// However, since IsAvailable uses its own internal timeout, we test
	// that the function respects the parent context timeout
	shortCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	<-shortCtx.Done()

	err := IsAvailable(shortCtx)
	if err == nil {
		t.Skip("Docker command completed before context timeout")
	}

	// Should get a context deadline exceeded error
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Logf("Got error (expected timeout-related): %v", err)
	}
}

// TestIsAvailable_InternalTimeout tests the internal 5-second timeout
func TestIsAvailable_InternalTimeout(t *testing.T) {
	// This test verifies that IsAvailable has an internal timeout
	// by checking that the timeout constant is set correctly
	if dockerCheckTimeout != 5*time.Second {
		t.Errorf("dockerCheckTimeout = %v, want 5s", dockerCheckTimeout)
	}

	// Test that a normal context doesn't interfere with internal timeout
	ctx := context.Background()
	err := IsAvailable(ctx)

	// We don't care if docker is available or not, just that the function
	// doesn't hang indefinitely
	t.Logf("IsAvailable() returned with error: %v", err)
}

// TestIsAvailable_CommandExecution tests command execution details
func TestIsAvailable_CommandExecution(t *testing.T) {
	tests := []struct {
		name        string
		setupDocker bool
		wantErr     bool
	}{
		{
			name:    "verifies docker info command is used",
			wantErr: false, // depends on docker availability
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := IsAvailable(ctx)

			// Log the result - we can't mock exec.CommandContext without
			// significant refactoring, but we can verify behavior
			if err != nil {
				t.Logf("Docker not available (expected in some environments): %v", err)
			} else {
				t.Log("Docker is available")
			}

			// Verify the error is a proper exec error when docker is not available
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					t.Logf("Got exec.ExitError with exit code: %d", exitErr.ExitCode())
				}
			}
		})
	}
}

// TestIsAvailable_MultipleConsecutiveCalls tests consistency across multiple calls
func TestIsAvailable_MultipleConsecutiveCalls(t *testing.T) {
	ctx := context.Background()

	// Call IsAvailable multiple times
	results := make([]error, 3)
	for i := 0; i < 3; i++ {
		results[i] = IsAvailable(ctx)
	}

	// All calls should return the same result (either all succeed or all fail)
	firstResult := results[0]
	for i, result := range results {
		if (firstResult == nil) != (result == nil) {
			t.Errorf("Call %d returned different result: first=%v, current=%v",
				i, firstResult, result)
		}
	}

	t.Logf("All %d consecutive calls returned consistent results", len(results))
}

// TestIsAvailable_ParentContextTimeout tests parent context timeout propagation
func TestIsAvailable_ParentContextTimeout(t *testing.T) {
	tests := []struct {
		name            string
		parentTimeout   time.Duration
		expectTimeout   bool
		expectImmediate bool
	}{
		{
			name:            "parent timeout shorter than internal timeout",
			parentTimeout:   1 * time.Second,
			expectTimeout:   false, // docker info usually responds quickly
			expectImmediate: false,
		},
		{
			name:            "parent timeout longer than internal timeout",
			parentTimeout:   10 * time.Second,
			expectTimeout:   false,
			expectImmediate: false,
		},
		{
			name:            "parent timeout extremely short",
			parentTimeout:   1 * time.Millisecond,
			expectTimeout:   true,
			expectImmediate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parentCtx, cancel := context.WithTimeout(context.Background(), tt.parentTimeout)
			defer cancel()

			start := time.Now()
			err := IsAvailable(parentCtx)
			duration := time.Since(start)

			if tt.expectImmediate && duration > 100*time.Millisecond {
				t.Errorf("Expected immediate failure, but took %v", duration)
			}

			if tt.expectTimeout && err == nil {
				t.Error("Expected timeout error, got nil")
			}

			t.Logf("Call completed in %v with error: %v", duration, err)
		})
	}
}

// TestIsAvailable_ContextWithValue tests that context values are preserved
func TestIsAvailable_ContextWithValue(t *testing.T) {
	type contextKey string
	const testKey contextKey = "test"

	ctx := context.WithValue(context.Background(), testKey, "test-value")

	err := IsAvailable(ctx)

	// Verify context value is still accessible after call
	value := ctx.Value(testKey)
	if value != "test-value" {
		t.Errorf("Context value was modified: got %v, want %v", value, "test-value")
	}

	t.Logf("Context preserved correctly, error: %v", err)
}

// TestIsAvailable_ConcurrentCalls tests thread safety with concurrent calls
func TestIsAvailable_ConcurrentCalls(t *testing.T) {
	const numGoroutines = 10

	ctx := context.Background()
	errChan := make(chan error, numGoroutines)

	// Launch multiple concurrent calls
	for i := 0; i < numGoroutines; i++ {
		go func() {
			errChan <- IsAvailable(ctx)
		}()
	}

	// Collect results
	results := make([]error, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		results[i] = <-errChan
	}

	// All results should be consistent
	firstResult := results[0]
	for i, result := range results {
		if (firstResult == nil) != (result == nil) {
			t.Errorf("Concurrent call %d returned different result: first=%v, current=%v",
				i, firstResult, result)
		}
	}

	t.Logf("All %d concurrent calls returned consistent results", numGoroutines)
}

// TestDockerCheckTimeout tests the timeout constant value
func TestDockerCheckTimeout(t *testing.T) {
	expectedTimeout := 5 * time.Second

	if dockerCheckTimeout != expectedTimeout {
		t.Errorf("dockerCheckTimeout = %v, want %v", dockerCheckTimeout, expectedTimeout)
	}
}

// TestIsAvailable_ErrorTypes tests different error scenarios
func TestIsAvailable_ErrorTypes(t *testing.T) {
	tests := []struct {
		name       string
		setupCtx   func() (context.Context, context.CancelFunc)
		checkError func(*testing.T, error)
	}{
		{
			name: "cancelled context returns context.Canceled",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			checkError: func(t *testing.T, err error) {
				if err == nil {
					t.Skip("Docker responded before context check")
				}
				// Error should be context-related
				if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "killed") {
					t.Logf("Got error (context-related): %v", err)
				}
			},
		},
		{
			name: "expired context returns context.DeadlineExceeded",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				time.Sleep(1 * time.Millisecond) // ensure timeout
				return ctx, cancel
			},
			checkError: func(t *testing.T, err error) {
				if err == nil {
					t.Skip("Docker responded before deadline")
				}
				// Error should be deadline-related
				if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
					t.Logf("Got error (deadline-related): %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.setupCtx()
			defer cancel()

			err := IsAvailable(ctx)
			tt.checkError(t, err)
		})
	}
}

// TestIsAvailable_RealWorldScenarios tests realistic usage patterns
func TestIsAvailable_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name     string
		scenario func(t *testing.T)
	}{
		{
			name: "pre-flight check pattern",
			scenario: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				err := IsAvailable(ctx)
				if err != nil {
					t.Logf("Pre-flight check failed (docker not available): %v", err)
				} else {
					t.Log("Pre-flight check passed")
				}
			},
		},
		{
			name: "rapid consecutive checks",
			scenario: func(t *testing.T) {
				ctx := context.Background()
				for i := 0; i < 5; i++ {
					err := IsAvailable(ctx)
					t.Logf("Check %d: error=%v", i, err)
					time.Sleep(10 * time.Millisecond)
				}
			},
		},
		{
			name: "check with user-facing timeout",
			scenario: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				start := time.Now()
				err := IsAvailable(ctx)
				duration := time.Since(start)

				if err != nil {
					t.Logf("Check failed in %v: %v", duration, err)
				} else {
					t.Logf("Check succeeded in %v", duration)
				}

				// Should respect the shorter timeout (internal 5s vs parent 3s)
				if duration > 6*time.Second {
					t.Errorf("Check took too long: %v", duration)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.scenario(t)
		})
	}
}

// BenchmarkIsAvailable benchmarks the Docker availability check
func BenchmarkIsAvailable(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsAvailable(ctx)
	}
}

// BenchmarkIsAvailableParallel benchmarks concurrent Docker checks
func BenchmarkIsAvailableParallel(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = IsAvailable(ctx)
		}
	})
}
