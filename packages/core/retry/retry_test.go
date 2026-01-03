package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0
	err := Do(context.Background(), func(_ context.Context) error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("Do() error = %v, want nil", err)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	callCount := 0
	err := Do(context.Background(), func(_ context.Context) error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary failure")
		}
		return nil
	}, WithMaxAttempts(5), WithInitialDelay(1*time.Millisecond))

	if err != nil {
		t.Errorf("Do() error = %v, want nil", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestDo_MaxRetriesExceeded(t *testing.T) {
	callCount := 0
	err := Do(context.Background(), func(_ context.Context) error {
		callCount++
		return errors.New("persistent failure")
	}, WithMaxAttempts(3), WithInitialDelay(1*time.Millisecond))

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Do() error = %v, want ErrMaxRetriesExceeded", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestDo_WrapsOriginalError(t *testing.T) {
	originalErr := errors.New("original error")
	err := Do(context.Background(), func(_ context.Context) error {
		return originalErr
	}, WithMaxAttempts(2), WithInitialDelay(1*time.Millisecond))

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Do() should wrap with ErrMaxRetriesExceeded")
	}
	if !errors.Is(err, originalErr) {
		t.Errorf("Do() should preserve original error, got %v", err)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, func(_ context.Context) error {
		callCount++
		return errors.New("failure")
	}, WithMaxAttempts(100), WithInitialDelay(100*time.Millisecond))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Do() error = %v, want context.Canceled", err)
	}
}

func TestDo_CustomRetryCondition(t *testing.T) {
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
			_ = Do(context.Background(), func(_ context.Context) error {
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

func TestDo_NonRetryableReturnsOriginalError(t *testing.T) {
	nonRetryableErr := errors.New("auth failed")

	err := Do(context.Background(), func(_ context.Context) error {
		return nonRetryableErr
	},
		WithMaxAttempts(5),
		WithInitialDelay(1*time.Millisecond),
		WithRetryCondition(func(_ error) bool {
			return false
		}),
	)

	if errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("non-retryable error should not be wrapped with ErrMaxRetriesExceeded")
	}
	if !errors.Is(err, nonRetryableErr) {
		t.Errorf("Do() error = %v, want %v", err, nonRetryableErr)
	}
}

func TestDo_ExponentialBackoff(t *testing.T) {
	var timestamps []time.Time

	_ = Do(context.Background(), func(_ context.Context) error {
		timestamps = append(timestamps, time.Now())
		return errors.New("failure")
	},
		WithMaxAttempts(4),
		WithInitialDelay(10*time.Millisecond),
		WithBackoffMultiplier(2.0),
		WithJitterFactor(0),
	)

	if len(timestamps) != 4 {
		t.Fatalf("expected 4 attempts, got %d", len(timestamps))
	}

	delays := make([]time.Duration, 0, 3)
	for i := 1; i < len(timestamps); i++ {
		delays = append(delays, timestamps[i].Sub(timestamps[i-1]))
	}

	for i := 1; i < len(delays); i++ {
		ratio := float64(delays[i]) / float64(delays[i-1])
		if ratio < 1.5 || ratio > 2.5 {
			t.Errorf("delay ratio[%d/%d] = %.2f, expected ~2.0", i, i-1, ratio)
		}
	}
}

func TestDo_ZeroAttempts(t *testing.T) {
	callCount := 0
	err := Do(context.Background(), func(_ context.Context) error {
		callCount++
		return errors.New("failure")
	}, WithMaxAttempts(0))

	if callCount != 0 {
		t.Errorf("callCount = %d, want 0 with MaxAttempts=0", callCount)
	}
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Do() error = %v, want ErrMaxRetriesExceeded", err)
	}
}

func TestAddJitter(t *testing.T) {
	base := 100 * time.Millisecond
	factor := 0.2

	results := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		result := addJitter(base, factor)
		results[result] = true

		minExpected := base - time.Duration(float64(base)*factor)
		maxExpected := base + time.Duration(float64(base)*factor)

		if result < minExpected || result > maxExpected {
			t.Errorf("addJitter(%v, %v) = %v, want in range [%v, %v]",
				base, factor, result, minExpected, maxExpected)
		}
	}

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
