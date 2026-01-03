package util

import (
	"context"

	"github.com/detentsh/core/retry"
)

// Re-export types and errors from core/retry for backwards compatibility.
var (
	ErrMaxRetriesExceeded = retry.ErrMaxRetriesExceeded
	ErrInvalidConfig      = retry.ErrInvalidConfig
)

// RetryableFunc is the function signature for operations that can be retried.
type RetryableFunc = retry.RetryableFunc

// RetryCondition determines if an error should trigger a retry.
type RetryCondition = retry.RetryCondition

// RetryConfig holds retry configuration.
type RetryConfig = retry.Config

// RetryOption configures retry behavior.
type RetryOption = retry.Option

// DefaultRetryConfig returns sensible defaults.
var DefaultRetryConfig = retry.DefaultConfig

// WithMaxAttempts sets the maximum number of attempts (includes initial attempt).
var WithMaxAttempts = retry.WithMaxAttempts

// WithInitialDelay sets the delay before the first retry.
var WithInitialDelay = retry.WithInitialDelay

// WithMaxDelay sets the maximum delay between retries.
var WithMaxDelay = retry.WithMaxDelay

// WithBackoffMultiplier sets the exponential backoff factor.
var WithBackoffMultiplier = retry.WithBackoffMultiplier

// WithJitterFactor sets the random jitter percentage (0.0-1.0).
var WithJitterFactor = retry.WithJitterFactor

// WithRetryCondition sets the function that determines retryable errors.
var WithRetryCondition = retry.WithRetryCondition

// Retry executes the function with retry logic.
var Retry = func(ctx context.Context, fn RetryableFunc, opts ...RetryOption) error {
	return retry.Do(ctx, fn, opts...)
}
