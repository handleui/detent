package act

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "wrapped ErrTransient",
			err:      ErrTransient,
			expected: true,
		},
		{
			name:     "docker daemon not running",
			err:      errors.New("Cannot connect to Docker daemon at unix:///var/run/docker.sock"),
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp: connection refused"),
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("dial tcp: network is unreachable"),
			expected: true,
		},
		{
			name:     "image pull failed",
			err:      errors.New("error pulling image catthehacker/ubuntu:act-latest"),
			expected: true,
		},
		{
			name:     "io timeout",
			err:      errors.New("read tcp: i/o timeout"),
			expected: true,
		},
		{
			name:     "container create failed",
			err:      errors.New("container create failed: resource temporarily unavailable"),
			expected: true,
		},
		{
			name:     "oci runtime error",
			err:      errors.New("OCI runtime exec failed"),
			expected: true,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "regular workflow error",
			err:      errors.New("exit status 1"),
			expected: false,
		},
		{
			name:     "file not found",
			err:      errors.New("file not found: main.go"),
			expected: false,
		},
		{
			name:     "syntax error",
			err:      errors.New("syntax error in workflow file"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTransientError(tt.err)
			if result != tt.expected {
				t.Errorf("IsTransientError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		result         *RunResult
		expectWrapped  bool
		expectOriginal bool
	}{
		{
			name:           "nil error, nil result",
			err:            nil,
			result:         nil,
			expectWrapped:  false,
			expectOriginal: false,
		},
		{
			name: "nil error with transient pattern in stderr",
			err:  nil,
			result: &RunResult{
				Stderr: "Cannot connect to Docker daemon",
			},
			expectWrapped:  true,
			expectOriginal: false,
		},
		{
			name: "nil error with clean output",
			err:  nil,
			result: &RunResult{
				Stdout: "Build successful",
				Stderr: "",
			},
			expectWrapped:  false,
			expectOriginal: false,
		},
		{
			name:           "transient error",
			err:            errors.New("connection refused"),
			result:         nil,
			expectWrapped:  true,
			expectOriginal: false,
		},
		{
			name:           "non-transient error",
			err:            errors.New("exit status 1"),
			result:         nil,
			expectWrapped:  false,
			expectOriginal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classified := classifyError(tt.err, tt.result)

			if tt.expectWrapped {
				if !errors.Is(classified, ErrTransient) {
					t.Errorf("classifyError() = %v, expected to wrap ErrTransient", classified)
				}
			} else if tt.expectOriginal {
				if classified != tt.err {
					t.Errorf("classifyError() = %v, expected original error %v", classified, tt.err)
				}
			} else {
				if classified != nil && tt.err == nil && tt.result != nil && !errors.Is(classified, ErrTransient) {
					t.Errorf("classifyError() = %v, expected nil or ErrTransient", classified)
				}
			}
		})
	}
}

func TestCalculateNextDelay(t *testing.T) {
	tests := []struct {
		name       string
		current    time.Duration
		multiplier float64
		maxDelay   time.Duration
		expected   time.Duration
	}{
		{
			name:       "normal exponential backoff",
			current:    1 * time.Second,
			multiplier: 2.0,
			maxDelay:   30 * time.Second,
			expected:   2 * time.Second,
		},
		{
			name:       "capped at max delay",
			current:    20 * time.Second,
			multiplier: 2.0,
			maxDelay:   30 * time.Second,
			expected:   30 * time.Second,
		},
		{
			name:       "multiplier less than 1",
			current:    1 * time.Second,
			multiplier: 0.5,
			maxDelay:   30 * time.Second,
			expected:   1 * time.Second,
		},
		{
			name:       "multiplier exactly 1",
			current:    1 * time.Second,
			multiplier: 1.0,
			maxDelay:   30 * time.Second,
			expected:   1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateNextDelay(tt.current, tt.multiplier, tt.maxDelay)
			if result != tt.expected {
				t.Errorf("calculateNextDelay(%v, %v, %v) = %v, want %v",
					tt.current, tt.multiplier, tt.maxDelay, result, tt.expected)
			}
		})
	}
}

func TestAddJitter(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{"zero duration", 0},
		{"negative duration", -1 * time.Second},
		{"positive duration", 1 * time.Second},
		{"large duration", 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.duration <= 0 {
				result := addJitter(tt.duration)
				if result != tt.duration {
					t.Errorf("addJitter(%v) = %v, expected unchanged", tt.duration, result)
				}
				return
			}

			results := make(map[time.Duration]bool)
			for i := 0; i < 50; i++ {
				result := addJitter(tt.duration)
				results[result] = true

				minExpected := tt.duration - time.Duration(float64(tt.duration)*0.2)
				maxExpected := tt.duration + time.Duration(float64(tt.duration)*0.2)

				if result < minExpected || result > maxExpected {
					t.Errorf("addJitter(%v) = %v, outside expected range [%v, %v]",
						tt.duration, result, minExpected, maxExpected)
				}
			}

			if len(results) < 3 {
				t.Logf("Warning: addJitter produced only %d unique values in 50 runs", len(results))
			}
		})
	}
}

func createMockActForRetry(t *testing.T, behavior string) string {
	t.Helper()

	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/mock-act"

	var script string
	switch behavior {
	case "success":
		script = `#!/bin/sh
echo "Workflow completed successfully"
exit 0
`
	case "transient_then_success":
		script = `#!/bin/sh
COUNTER_FILE="/tmp/act_retry_counter_$$"
if [ -f "$COUNTER_FILE" ]; then
    COUNT=$(cat "$COUNTER_FILE")
else
    COUNT=0
fi
COUNT=$((COUNT + 1))
echo "$COUNT" > "$COUNTER_FILE"
if [ "$COUNT" -lt 2 ]; then
    echo "Cannot connect to Docker daemon" >&2
    exit 1
fi
rm -f "$COUNTER_FILE"
echo "Workflow completed successfully"
exit 0
`
	case "always_transient":
		script = `#!/bin/sh
echo "Cannot connect to Docker daemon" >&2
exit 1
`
	case "workflow_failure":
		script = `#!/bin/sh
echo "Build failed: exit status 1" >&2
exit 1
`
	default:
		t.Fatalf("Unknown behavior: %s", behavior)
	}

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	return scriptPath
}

func TestRunWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	mockAct := createMockActForRetry(t, "success")

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
		WorkDir:   t.TempDir(),
	}

	attemptCount := 0
	result, err := RunWithRetry(context.Background(), cfg,
		WithMaxAttempts(3),
		WithInitialDelay(10*time.Millisecond),
		WithOnRetry(func(attempt int, err error, delay time.Duration) {
			attemptCount = attempt
		}),
	)

	if err != nil {
		t.Fatalf("RunWithRetry() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("RunWithRetry() returned nil result")
	}

	if result.ExitCode != 0 {
		t.Errorf("RunWithRetry() ExitCode = %d, want 0", result.ExitCode)
	}

	if attemptCount != 0 {
		t.Errorf("OnRetry was called %d times, expected 0", attemptCount)
	}
}

func TestRunWithRetry_WorkflowFailureNoRetry(t *testing.T) {
	mockAct := createMockActForRetry(t, "workflow_failure")

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
		WorkDir:   t.TempDir(),
	}

	retryCount := 0
	result, err := RunWithRetry(context.Background(), cfg,
		WithMaxAttempts(3),
		WithInitialDelay(10*time.Millisecond),
		WithOnRetry(func(attempt int, err error, delay time.Duration) {
			retryCount++
		}),
	)

	if result == nil {
		t.Fatal("RunWithRetry() should return result for workflow failures")
	}

	if result.ExitCode == 0 {
		t.Errorf("RunWithRetry() ExitCode = 0, expected non-zero for failure")
	}

	if retryCount > 0 {
		t.Errorf("RunWithRetry() retried %d times for non-transient error", retryCount)
	}

	if err != nil && errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Non-transient failures should not return ErrMaxRetriesExceeded")
	}
}

func TestRunWithRetry_TransientFailureThenSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	counterFile := tmpDir + "/counter"
	scriptPath := tmpDir + "/mock-act"

	script := `#!/bin/sh
COUNTER_FILE="` + counterFile + `"
if [ -f "$COUNTER_FILE" ]; then
    COUNT=$(cat "$COUNTER_FILE")
else
    COUNT=0
fi
COUNT=$((COUNT + 1))
echo "$COUNT" > "$COUNTER_FILE"
if [ "$COUNT" -lt 2 ]; then
    echo "Cannot connect to Docker daemon" >&2
    exit 1
fi
echo "Workflow completed successfully"
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: scriptPath,
		WorkDir:   t.TempDir(),
	}

	retryCount := 0
	result, err := RunWithRetry(context.Background(), cfg,
		WithMaxAttempts(3),
		WithInitialDelay(10*time.Millisecond),
		WithOnRetry(func(attempt int, err error, delay time.Duration) {
			retryCount++
			t.Logf("Retry attempt %d after error: %v", attempt, err)
		}),
	)

	if err != nil {
		t.Fatalf("RunWithRetry() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("RunWithRetry() returned nil result")
	}

	if result.ExitCode != 0 {
		t.Errorf("RunWithRetry() ExitCode = %d, want 0", result.ExitCode)
	}

	if retryCount != 1 {
		t.Errorf("OnRetry was called %d times, expected 1", retryCount)
	}
}

func TestRunWithRetry_MaxRetriesExceeded(t *testing.T) {
	mockAct := createMockActForRetry(t, "always_transient")

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: mockAct,
		WorkDir:   t.TempDir(),
	}

	retryCount := 0
	_, err := RunWithRetry(context.Background(), cfg,
		WithMaxAttempts(3),
		WithInitialDelay(10*time.Millisecond),
		WithOnRetry(func(attempt int, err error, delay time.Duration) {
			retryCount++
		}),
	)

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("RunWithRetry() error = %v, want ErrMaxRetriesExceeded", err)
	}

	if retryCount != 2 {
		t.Errorf("OnRetry was called %d times, expected 2", retryCount)
	}
}

func TestRunWithRetry_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/mock-act"

	script := `#!/bin/sh
echo "Cannot connect to Docker daemon" >&2
sleep 5
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: scriptPath,
		WorkDir:   t.TempDir(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := RunWithRetry(ctx, cfg,
		WithMaxAttempts(5),
		WithInitialDelay(1*time.Second),
	)

	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("RunWithRetry() error = %v, expected context error", err)
	}
}

func TestRunWithRetry_ZeroAttempts(t *testing.T) {
	cfg := &RunConfig{
		Event:   "push",
		WorkDir: t.TempDir(),
	}

	_, err := RunWithRetry(context.Background(), cfg,
		WithMaxAttempts(0),
	)

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("RunWithRetry() with 0 attempts should return ErrMaxRetriesExceeded, got %v", err)
	}
}

func TestRunWithRetry_ExponentialBackoff(t *testing.T) {
	tmpDir := t.TempDir()
	counterFile := tmpDir + "/counter"
	scriptPath := tmpDir + "/mock-act"

	script := `#!/bin/sh
COUNTER_FILE="` + counterFile + `"
if [ -f "$COUNTER_FILE" ]; then
    COUNT=$(cat "$COUNTER_FILE")
else
    COUNT=0
fi
COUNT=$((COUNT + 1))
echo "$COUNT" > "$COUNTER_FILE"
echo "Cannot connect to Docker daemon" >&2
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: scriptPath,
		WorkDir:   t.TempDir(),
	}

	var delays []time.Duration
	_, _ = RunWithRetry(context.Background(), cfg,
		WithMaxAttempts(4),
		WithInitialDelay(50*time.Millisecond),
		WithBackoffMultiplier(2.0),
		WithMaxDelay(500*time.Millisecond),
		WithOnRetry(func(attempt int, err error, delay time.Duration) {
			delays = append(delays, delay)
		}),
	)

	if len(delays) != 3 {
		t.Errorf("Expected 3 retry delays, got %d", len(delays))
		return
	}

	for i := 1; i < len(delays); i++ {
		if delays[i] <= delays[i-1] {
			t.Logf("Delay[%d]=%v <= Delay[%d]=%v (may be due to jitter or max delay cap)",
				i, delays[i], i-1, delays[i-1])
		}
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	if DefaultRetryConfig.MaxAttempts != 3 {
		t.Errorf("DefaultRetryConfig.MaxAttempts = %d, want 3", DefaultRetryConfig.MaxAttempts)
	}
	if DefaultRetryConfig.InitialDelay != 1*time.Second {
		t.Errorf("DefaultRetryConfig.InitialDelay = %v, want 1s", DefaultRetryConfig.InitialDelay)
	}
	if DefaultRetryConfig.MaxDelay != 4*time.Second {
		t.Errorf("DefaultRetryConfig.MaxDelay = %v, want 4s", DefaultRetryConfig.MaxDelay)
	}
	if DefaultRetryConfig.BackoffMultiplier != 2.0 {
		t.Errorf("DefaultRetryConfig.BackoffMultiplier = %v, want 2.0", DefaultRetryConfig.BackoffMultiplier)
	}
}

func TestTransientPatterns(t *testing.T) {
	testCases := []struct {
		errMsg   string
		expected bool
	}{
		{"Cannot connect to Docker daemon", true},
		{"is the docker daemon running", true},
		{"dial tcp: connection refused", true},
		{"network is unreachable", true},
		{"error pulling image", true},
		{"OCI runtime exec failed", true},
		{"Build failed: exit status 1", false},
		{"Test failed", false},
		{"npm ERR! code ENOENT", false},
	}

	for _, tc := range testCases {
		t.Run(tc.errMsg, func(t *testing.T) {
			err := errors.New(tc.errMsg)
			result := IsTransientError(err)
			if result != tc.expected {
				t.Errorf("IsTransientError(%q) = %v, want %v", tc.errMsg, result, tc.expected)
			}
		})
	}
}

func TestRunWithRetry_PreservesResultOnTransientFailure(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/mock-act"

	script := `#!/bin/sh
echo "Step 1: Setup" >&2
echo "Step 2: Build" >&2
echo "Cannot connect to Docker daemon - connection refused" >&2
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("Failed to create mock script: %v", err)
	}

	cfg := &RunConfig{
		Event:     "push",
		ActBinary: scriptPath,
		WorkDir:   t.TempDir(),
	}

	result, err := RunWithRetry(context.Background(), cfg,
		WithMaxAttempts(2),
		WithInitialDelay(10*time.Millisecond),
	)

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Expected ErrMaxRetriesExceeded, got %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be preserved even on failure")
	}

	if !strings.Contains(result.Stderr, "Step 1: Setup") {
		t.Errorf("Result stderr should contain partial output, got: %s", result.Stderr)
	}
}
