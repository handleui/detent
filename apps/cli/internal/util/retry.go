package util

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// ErrMaxRetriesExceeded indicates all retry attempts failed.
var ErrMaxRetriesExceeded = errors.New("max retries exceeded")

// ErrInvalidConfig indicates the retry configuration is invalid.
var ErrInvalidConfig = errors.New("invalid retry config")

// RetryableFunc is the function signature for operations that can be retried.
// The context passed to the function is the same context passed to Retry,
// allowing the function to respect cancellation signals.
type RetryableFunc func(ctx context.Context) error

// RetryCondition determines if an error should trigger a retry.
// Return true to retry the operation, false to stop immediately.
type RetryCondition func(err error) bool

// RetryConfig holds retry configuration.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts including the initial call.
	// Must be >= 1. Use 1 for no retries.
	MaxAttempts int
	// InitialDelay is the delay before the first retry attempt.
	// Must be >= 0. A value of 0 means no delay.
	InitialDelay time.Duration
	// MaxDelay caps the delay between retry attempts.
	// Must be >= InitialDelay when both are > 0.
	MaxDelay time.Duration
	// BackoffMultiplier is the factor by which delay increases after each retry.
	// Must be >= 1.0. Use 1.0 for constant delay.
	BackoffMultiplier float64
	// JitterFactor adds randomness to delays to prevent thundering herd.
	// Must be in range [0.0, 1.0]. Value of 0.2 means +/-20% jitter.
	JitterFactor float64
	// ShouldRetry determines which errors are retryable.
	// If nil, all errors are retried.
	ShouldRetry RetryCondition
}

// RetryOption configures retry behavior.
type RetryOption func(*RetryConfig)

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      500 * time.Millisecond,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		JitterFactor:      0.2, // 20% jitter
		ShouldRetry:       nil, // retry all errors by default
	}
}

// WithMaxAttempts sets the maximum number of attempts (includes initial attempt).
// Values <= 0 will cause Retry to return ErrMaxRetriesExceeded without calling the function.
func WithMaxAttempts(n int) RetryOption {
	return func(c *RetryConfig) {
		c.MaxAttempts = n
	}
}

// WithInitialDelay sets the delay before the first retry.
// Negative values are treated as 0 (no delay).
func WithInitialDelay(d time.Duration) RetryOption {
	return func(c *RetryConfig) {
		if d < 0 {
			d = 0
		}
		c.InitialDelay = d
	}
}

// WithMaxDelay sets the maximum delay between retries.
// Negative values are treated as 0.
func WithMaxDelay(d time.Duration) RetryOption {
	return func(c *RetryConfig) {
		if d < 0 {
			d = 0
		}
		c.MaxDelay = d
	}
}

// WithBackoffMultiplier sets the exponential backoff factor.
// Values < 1.0 are treated as 1.0 (constant delay).
func WithBackoffMultiplier(m float64) RetryOption {
	return func(c *RetryConfig) {
		if m < 1.0 {
			m = 1.0
		}
		c.BackoffMultiplier = m
	}
}

// WithJitterFactor sets the random jitter percentage (0.0-1.0).
// Values outside this range are clamped.
func WithJitterFactor(j float64) RetryOption {
	return func(c *RetryConfig) {
		if j < 0 {
			j = 0
		} else if j > 1.0 {
			j = 1.0
		}
		c.JitterFactor = j
	}
}

// WithRetryCondition sets the function that determines retryable errors.
func WithRetryCondition(cond RetryCondition) RetryOption {
	return func(c *RetryConfig) {
		c.ShouldRetry = cond
	}
}

// Retry executes the function with retry logic.
// Returns the first successful result or the last error after all retries.
func Retry(ctx context.Context, fn RetryableFunc, opts ...RetryOption) error {
	cfg := DefaultRetryConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	// Handle edge case: 0 or negative MaxAttempts - return error without calling fn
	if cfg.MaxAttempts <= 0 {
		return fmt.Errorf("%w: no attempts configured", ErrMaxRetriesExceeded)
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check custom retry condition
		if cfg.ShouldRetry != nil && !cfg.ShouldRetry(err) {
			return err
		}

		// Don't wait after last attempt
		if attempt == cfg.MaxAttempts {
			break
		}

		// Calculate delay with jitter
		actualDelay := addJitter(delay, cfg.JitterFactor)

		// Wait or cancel using timer to avoid leaks
		timer := time.NewTimer(actualDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		// Increase delay for next attempt with overflow protection
		delay = safeMultiplyDelay(delay, cfg.BackoffMultiplier, cfg.MaxDelay)
	}

	return fmt.Errorf("%w: %w", ErrMaxRetriesExceeded, lastErr)
}

// safeMultiplyDelay multiplies delay by multiplier with overflow protection.
// Returns maxDelay if the result would overflow or exceed maxDelay.
func safeMultiplyDelay(delay time.Duration, multiplier float64, maxDelay time.Duration) time.Duration {
	if multiplier <= 1.0 {
		return min(delay, maxDelay)
	}

	// Check for potential overflow before multiplication
	result := float64(delay) * multiplier
	if math.IsInf(result, 0) || math.IsNaN(result) || result > float64(math.MaxInt64) {
		return maxDelay
	}

	newDelay := time.Duration(result)
	if newDelay < 0 { // overflow occurred
		return maxDelay
	}

	return min(newDelay, maxDelay)
}

// addJitter adds random jitter to a duration.
// Jitter is applied as +/- (factor * duration), so factor=0.2 means +/-20%.
func addJitter(d time.Duration, factor float64) time.Duration {
	if factor <= 0 || d <= 0 {
		return d
	}

	// Clamp factor to valid range
	if factor > 1.0 {
		factor = 1.0
	}

	// Generate random value in range [-factor, +factor]
	jitterRange := float64(d) * factor
	jitter := time.Duration(jitterRange * (2*rand.Float64() - 1)) //nolint:gosec // math/rand is fine for jitter

	result := d + jitter
	if result < 0 {
		return 0
	}
	return result
}
