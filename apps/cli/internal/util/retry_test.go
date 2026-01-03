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
			return false
		}),
	)

	if errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("non-retryable error should not be wrapped with ErrMaxRetriesExceeded")
	}
	if !errors.Is(err, nonRetryableErr) {
		t.Errorf("Retry() error = %v, want %v", err, nonRetryableErr)
	}
}

func TestRetry_ZeroAttempts(t *testing.T) {
	callCount := 0
	err := Retry(context.Background(), func(_ context.Context) error {
		callCount++
		return errors.New("failure")
	}, WithMaxAttempts(0))

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
