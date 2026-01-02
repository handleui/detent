package util

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0
	err := Retry(context.Background(), func(_ context.Context) error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("Retry() error = %v, want nil", err)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	callCount := 0
	err := Retry(context.Background(), func(_ context.Context) error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary failure")
		}
		return nil
	}, WithMaxAttempts(5), WithInitialDelay(1*time.Millisecond))

	if err != nil {
		t.Errorf("Retry() error = %v, want nil", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	callCount := 0
	err := Retry(context.Background(), func(_ context.Context) error {
		callCount++
		return errors.New("persistent failure")
	}, WithMaxAttempts(3), WithInitialDelay(1*time.Millisecond))

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Retry() error = %v, want ErrMaxRetriesExceeded", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestRetry_WrapsOriginalError(t *testing.T) {
	originalErr := errors.New("original error")
	err := Retry(context.Background(), func(_ context.Context) error {
		return originalErr
	}, WithMaxAttempts(2), WithInitialDelay(1*time.Millisecond))

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Retry() should wrap with ErrMaxRetriesExceeded")
	}
	if !errors.Is(err, originalErr) {
		t.Errorf("Retry() should preserve original error, got %v", err)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Retry(ctx, func(_ context.Context) error {
		callCount++
		return errors.New("failure")
	}, WithMaxAttempts(100), WithInitialDelay(100*time.Millisecond))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Retry() error = %v, want context.Canceled", err)
	}
}

func TestRetry_ContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := Retry(ctx, func(_ context.Context) error {
		return errors.New("failure")
	}, WithMaxAttempts(100), WithInitialDelay(100*time.Millisecond))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Retry() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestRetry_CustomRetryCondition(t *testing.T) {
	retryableErr := errors.New("retryable")
	nonRetryableErr := errors.New("non-retryable")

	tests := []struct {
		name      string
		err       error
		wantCalls int
	}{
		{"retryable error", retryableErr, 3},
		{"non-retryable error", nonRetryableErr, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			_ = Retry(context.Background(), func(_ context.Context) error {
				callCount++
				return tt.err
			},
				WithMaxAttempts(3),
				WithInitialDelay(1*time.Millisecond),
				WithRetryCondition(func(err error) bool {
					return errors.Is(err, retryableErr)
				}),
			)

			if callCount != tt.wantCalls {
				t.Errorf("callCount = %d, want %d", callCount, tt.wantCalls)
			}
		})
	}
}

func TestRetry_NonRetryableReturnsOriginalError(t *testing.T) {
	nonRetryableErr := errors.New("auth failed")

	err := Retry(context.Background(), func(_ context.Context) error {
		return nonRetryableErr
	},
		WithMaxAttempts(5),
		WithInitialDelay(1*time.Millisecond),
		WithRetryCondition(func(_ error) bool {
			return false // never retry
		}),
	)

	// Should return the original error, not wrapped with ErrMaxRetriesExceeded
	if errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("non-retryable error should not be wrapped with ErrMaxRetriesExceeded")
	}
	if !errors.Is(err, nonRetryableErr) {
		t.Errorf("Retry() error = %v, want %v", err, nonRetryableErr)
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	var timestamps []time.Time

	_ = Retry(context.Background(), func(_ context.Context) error {
		timestamps = append(timestamps, time.Now())
		return errors.New("failure")
	},
		WithMaxAttempts(4),
		WithInitialDelay(10*time.Millisecond),
		WithBackoffMultiplier(2.0),
		WithJitterFactor(0), // disable jitter for predictable test
	)

	if len(timestamps) != 4 {
		t.Fatalf("expected 4 attempts, got %d", len(timestamps))
	}

	// Calculate delays between attempts
	delays := make([]time.Duration, 0, 3)
	for i := 1; i < len(timestamps); i++ {
		delays = append(delays, timestamps[i].Sub(timestamps[i-1]))
	}

	// Verify increasing delays (with tolerance for timing)
	for i := 1; i < len(delays); i++ {
		// Each delay should be roughly 2x the previous (within 50% tolerance)
		ratio := float64(delays[i]) / float64(delays[i-1])
		if ratio < 1.5 || ratio > 2.5 {
			t.Errorf("delay ratio[%d/%d] = %.2f, expected ~2.0", i, i-1, ratio)
		}
	}
}

func TestRetry_MaxDelayRespected(t *testing.T) {
	var timestamps []time.Time

	_ = Retry(context.Background(), func(_ context.Context) error {
		timestamps = append(timestamps, time.Now())
		return errors.New("failure")
	},
		WithMaxAttempts(5),
		WithInitialDelay(50*time.Millisecond),
		WithMaxDelay(60*time.Millisecond), // should cap delays
		WithBackoffMultiplier(10.0),       // aggressive backoff
		WithJitterFactor(0),
	)

	// Calculate delays
	for i := 1; i < len(timestamps); i++ {
		delay := timestamps[i].Sub(timestamps[i-1])
		// Allow 20ms tolerance for timing
		if delay > 80*time.Millisecond {
			t.Errorf("delay[%d] = %v, exceeds max delay + tolerance", i, delay)
		}
	}
}

func TestAddJitter(t *testing.T) {
	base := 100 * time.Millisecond
	factor := 0.2

	// Run multiple times to verify randomness and bounds
	results := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		result := addJitter(base, factor)
		results[result] = true

		// Should be within factor range
		minExpected := base - time.Duration(float64(base)*factor)
		maxExpected := base + time.Duration(float64(base)*factor)

		if result < minExpected || result > maxExpected {
			t.Errorf("addJitter(%v, %v) = %v, want in range [%v, %v]",
				base, factor, result, minExpected, maxExpected)
		}
	}

	// Should have some variance (not all same value)
	if len(results) < 5 {
		t.Error("addJitter should produce varied results")
	}
}

func TestAddJitter_ZeroFactor(t *testing.T) {
	base := 100 * time.Millisecond
	result := addJitter(base, 0)

	if result != base {
		t.Errorf("addJitter with factor=0 should return base, got %v", result)
	}
}

func TestAddJitter_NegativeFactor(t *testing.T) {
	base := 100 * time.Millisecond
	result := addJitter(base, -0.5)

	if result != base {
		t.Errorf("addJitter with negative factor should return base, got %v", result)
	}
}

func TestRetry_SingleAttempt(t *testing.T) {
	callCount := 0
	err := Retry(context.Background(), func(_ context.Context) error {
		callCount++
		return errors.New("failure")
	}, WithMaxAttempts(1))

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("single attempt should still wrap with ErrMaxRetriesExceeded")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestRetry_ZeroAttempts(t *testing.T) {
	callCount := 0
	err := Retry(context.Background(), func(_ context.Context) error {
		callCount++
		return errors.New("failure")
	}, WithMaxAttempts(0))

	// With 0 max attempts, function should never be called - prevents resource exhaustion
	if callCount != 0 {
		t.Errorf("callCount = %d, want 0 with MaxAttempts=0", callCount)
	}
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Retry() error = %v, want ErrMaxRetriesExceeded", err)
	}
}

func TestRetry_ContextPassedToFunction(t *testing.T) {
	type ctxKey string
	key := ctxKey("test")
	ctx := context.WithValue(context.Background(), key, "value")

	var receivedCtx context.Context
	_ = Retry(ctx, func(ctx context.Context) error {
		receivedCtx = ctx
		return nil
	})

	if receivedCtx.Value(key) != "value" {
		t.Error("context should be passed to function")
	}
}

func TestRetry_NegativeMaxAttempts(t *testing.T) {
	callCount := 0
	err := Retry(context.Background(), func(_ context.Context) error {
		callCount++
		return errors.New("failure")
	}, WithMaxAttempts(-5))

	// Negative max attempts: function should never be called - prevents resource exhaustion
	if callCount != 0 {
		t.Errorf("callCount = %d, want 0 with negative MaxAttempts", callCount)
	}
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Retry() error = %v, want ErrMaxRetriesExceeded", err)
	}
}

func TestRetry_NegativeDelays(t *testing.T) {
	callCount := 0
	start := time.Now()
	_ = Retry(context.Background(), func(_ context.Context) error {
		callCount++
		if callCount < 3 {
			return errors.New("failure")
		}
		return nil
	},
		WithMaxAttempts(5),
		WithInitialDelay(-100*time.Millisecond), // negative, should be normalized to 0
		WithMaxDelay(-50*time.Millisecond),      // negative, should be normalized to 0
	)
	elapsed := time.Since(start)

	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
	// With delays normalized to 0, should complete very quickly
	if elapsed > 50*time.Millisecond {
		t.Errorf("elapsed = %v, expected quick completion with normalized delays", elapsed)
	}
}

func TestWithBackoffMultiplier_LessThanOne(t *testing.T) {
	var timestamps []time.Time

	_ = Retry(context.Background(), func(_ context.Context) error {
		timestamps = append(timestamps, time.Now())
		return errors.New("failure")
	},
		WithMaxAttempts(3),
		WithInitialDelay(10*time.Millisecond),
		WithBackoffMultiplier(0.5), // should be normalized to 1.0
		WithJitterFactor(0),
	)

	if len(timestamps) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(timestamps))
	}

	// Calculate delays
	delays := make([]time.Duration, 0, 2)
	for i := 1; i < len(timestamps); i++ {
		delays = append(delays, timestamps[i].Sub(timestamps[i-1]))
	}

	// With multiplier normalized to 1.0, delays should be constant
	for i, delay := range delays {
		if delay < 8*time.Millisecond || delay > 15*time.Millisecond {
			t.Errorf("delay[%d] = %v, expected ~10ms with constant backoff", i, delay)
		}
	}
}

func TestWithJitterFactor_OutOfRange(t *testing.T) {
	t.Run("factor > 1.0 clamped to 1.0", func(t *testing.T) {
		base := 100 * time.Millisecond
		// Factor > 1.0 should be clamped to 1.0 in addJitter
		result := addJitter(base, 5.0)

		// With factor=1.0, range is [0, 200ms]
		if result < 0 || result > 200*time.Millisecond {
			t.Errorf("addJitter with factor > 1.0 should clamp, got %v", result)
		}
	})
}

func TestSafeMultiplyDelay(t *testing.T) {
	tests := []struct {
		name       string
		delay      time.Duration
		multiplier float64
		maxDelay   time.Duration
		wantMax    bool // if true, expect maxDelay
	}{
		{
			name:       "normal multiplication",
			delay:      100 * time.Millisecond,
			multiplier: 2.0,
			maxDelay:   1 * time.Second,
			wantMax:    false,
		},
		{
			name:       "caps at maxDelay",
			delay:      100 * time.Millisecond,
			multiplier: 100.0,
			maxDelay:   1 * time.Second,
			wantMax:    true,
		},
		{
			name:       "multiplier <= 1 returns min of delay and maxDelay",
			delay:      100 * time.Millisecond,
			multiplier: 0.5,
			maxDelay:   1 * time.Second,
			wantMax:    false,
		},
		{
			name:       "very large delay with multiplier",
			delay:      time.Duration(1<<62) * time.Nanosecond,
			multiplier: 2.0,
			maxDelay:   30 * time.Second,
			wantMax:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeMultiplyDelay(tt.delay, tt.multiplier, tt.maxDelay)
			if tt.wantMax {
				if result != tt.maxDelay {
					t.Errorf("safeMultiplyDelay() = %v, want %v (maxDelay)", result, tt.maxDelay)
				}
			} else {
				if result > tt.maxDelay {
					t.Errorf("safeMultiplyDelay() = %v, exceeds maxDelay %v", result, tt.maxDelay)
				}
				if result < 0 {
					t.Errorf("safeMultiplyDelay() = %v, should not be negative", result)
				}
			}
		})
	}
}

func TestAddJitter_ZeroDuration(t *testing.T) {
	result := addJitter(0, 0.2)
	if result != 0 {
		t.Errorf("addJitter with d=0 should return 0, got %v", result)
	}
}

func TestAddJitter_NegativeDuration(t *testing.T) {
	result := addJitter(-100*time.Millisecond, 0.2)
	if result != -100*time.Millisecond {
		t.Errorf("addJitter with negative d should return d unchanged, got %v", result)
	}
}

func BenchmarkRetry_Success(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Retry(context.Background(), func(_ context.Context) error {
			return nil
		})
	}
}

func BenchmarkRetry_WithRetries(b *testing.B) {
	for i := 0; i < b.N; i++ {
		count := 0
		_ = Retry(context.Background(), func(_ context.Context) error {
			count++
			if count < 3 {
				return errors.New("fail")
			}
			return nil
		}, WithInitialDelay(1*time.Nanosecond), WithJitterFactor(0))
	}
}
