package retry

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
type RetryableFunc func(ctx context.Context) error

// RetryCondition determines if an error should trigger a retry.
type RetryCondition func(err error) bool

// Config holds retry configuration.
type Config struct {
	MaxAttempts       int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
	JitterFactor      float64
	ShouldRetry       RetryCondition
}

// Option configures retry behavior.
type Option func(*Config)

// DefaultConfig returns sensible defaults.
var DefaultConfig = func() Config {
	return Config{
		MaxAttempts:       3,
		InitialDelay:      500 * time.Millisecond,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		JitterFactor:      0.2,
		ShouldRetry:       nil,
	}
}

// WithMaxAttempts sets the maximum number of attempts (includes initial attempt).
var WithMaxAttempts = func(n int) Option {
	return func(c *Config) {
		c.MaxAttempts = n
	}
}

// WithInitialDelay sets the delay before the first retry.
var WithInitialDelay = func(d time.Duration) Option {
	return func(c *Config) {
		if d < 0 {
			d = 0
		}
		c.InitialDelay = d
	}
}

// WithMaxDelay sets the maximum delay between retries.
var WithMaxDelay = func(d time.Duration) Option {
	return func(c *Config) {
		if d < 0 {
			d = 0
		}
		c.MaxDelay = d
	}
}

// WithBackoffMultiplier sets the exponential backoff factor.
var WithBackoffMultiplier = func(m float64) Option {
	return func(c *Config) {
		if m < 1.0 {
			m = 1.0
		}
		c.BackoffMultiplier = m
	}
}

// WithJitterFactor sets the random jitter percentage (0.0-1.0).
var WithJitterFactor = func(j float64) Option {
	return func(c *Config) {
		if j < 0 {
			j = 0
		} else if j > 1.0 {
			j = 1.0
		}
		c.JitterFactor = j
	}
}

// WithRetryCondition sets the function that determines retryable errors.
var WithRetryCondition = func(cond RetryCondition) Option {
	return func(c *Config) {
		c.ShouldRetry = cond
	}
}

// Do executes the function with retry logic.
var Do = func(ctx context.Context, fn RetryableFunc, opts ...Option) error {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

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

		if cfg.ShouldRetry != nil && !cfg.ShouldRetry(err) {
			return err
		}

		if attempt == cfg.MaxAttempts {
			break
		}

		actualDelay := addJitter(delay, cfg.JitterFactor)

		timer := time.NewTimer(actualDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		delay = safeMultiplyDelay(delay, cfg.BackoffMultiplier, cfg.MaxDelay)
	}

	return fmt.Errorf("%w: %w", ErrMaxRetriesExceeded, lastErr)
}

var safeMultiplyDelay = func(delay time.Duration, multiplier float64, maxDelay time.Duration) time.Duration {
	if multiplier <= 1.0 {
		return min(delay, maxDelay)
	}

	result := float64(delay) * multiplier
	if math.IsInf(result, 0) || math.IsNaN(result) || result > float64(math.MaxInt64) {
		return maxDelay
	}

	newDelay := time.Duration(result)
	if newDelay < 0 {
		return maxDelay
	}

	return min(newDelay, maxDelay)
}

var addJitter = func(d time.Duration, factor float64) time.Duration {
	if factor <= 0 || d <= 0 {
		return d
	}

	if factor > 1.0 {
		factor = 1.0
	}

	jitterRange := float64(d) * factor
	jitter := time.Duration(jitterRange * (2*rand.Float64() - 1)) //nolint:gosec // math/rand is fine for jitter

	result := d + jitter
	if result < 0 {
		return 0
	}
	return result
}
