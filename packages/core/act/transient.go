package act

import (
	"context"
	"strings"
	"time"

	"github.com/detentsh/core/retry"
)

// ErrMaxRetriesExceeded is re-exported from retry for convenience.
var ErrMaxRetriesExceeded = retry.ErrMaxRetriesExceeded

// transientPatterns are error substrings that indicate transient failures
// which may succeed on retry.
var transientPatterns = []string{
	"cannot connect to the docker daemon",
	"connection refused",
	"dial unix /var/run/docker.sock",
	"error during connect",
	"docker daemon is not running",
	"failed to create container",
	"failed to pull image",
	"image pull failed",
	"error response from daemon",
	"network is unreachable",
	"temporary failure in name resolution",
	"i/o timeout",
	"tls handshake timeout",
	"no such host",
	"connection reset by peer",
	"context deadline exceeded",
}

// IsTransientError checks if the error message contains any transient pattern.
var IsTransientError = func(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, pattern := range transientPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// RetryOption configures retry behavior for RunWithRetry.
type RetryOption func(*retryConfig)

type retryConfig struct {
	maxAttempts       int
	initialDelay      time.Duration
	maxDelay          time.Duration
	backoffMultiplier float64
	jitterFactor      float64
	onRetry           func(attempt int, err error, delay time.Duration)
}

var defaultRetryConfig = func() retryConfig {
	return retryConfig{
		maxAttempts:       3,
		initialDelay:      1 * time.Second,
		maxDelay:          30 * time.Second,
		backoffMultiplier: 2.0,
		jitterFactor:      0.2,
		onRetry:           nil,
	}
}

// WithMaxAttempts sets the maximum number of attempts (includes initial attempt).
var WithMaxAttempts = func(n int) RetryOption {
	return func(c *retryConfig) {
		c.maxAttempts = n
	}
}

// WithInitialDelay sets the delay before the first retry.
var WithInitialDelay = func(d time.Duration) RetryOption {
	return func(c *retryConfig) {
		c.initialDelay = d
	}
}

// WithMaxDelay sets the maximum delay between retries.
var WithMaxDelay = func(d time.Duration) RetryOption {
	return func(c *retryConfig) {
		c.maxDelay = d
	}
}

// WithBackoffMultiplier sets the exponential backoff factor.
var WithBackoffMultiplier = func(m float64) RetryOption {
	return func(c *retryConfig) {
		c.backoffMultiplier = m
	}
}

// WithJitterFactor sets the random jitter percentage (0.0-1.0).
var WithJitterFactor = func(j float64) RetryOption {
	return func(c *retryConfig) {
		c.jitterFactor = j
	}
}

// WithOnRetry sets a callback invoked before each retry attempt.
var WithOnRetry = func(fn func(attempt int, err error, delay time.Duration)) RetryOption {
	return func(c *retryConfig) {
		c.onRetry = fn
	}
}

// RunWithRetry executes act.Run with retry logic for transient failures.
var RunWithRetry = func(ctx context.Context, cfg *RunConfig, opts ...RetryOption) (*RunResult, error) {
	retryCfg := defaultRetryConfig()
	for _, opt := range opts {
		opt(&retryCfg)
	}

	var result *RunResult
	attempt := 0

	err := retry.Do(ctx, func(ctx context.Context) error {
		attempt++
		var runErr error
		result, runErr = Run(ctx, cfg)
		return runErr
	},
		retry.WithMaxAttempts(retryCfg.maxAttempts),
		retry.WithInitialDelay(retryCfg.initialDelay),
		retry.WithMaxDelay(retryCfg.maxDelay),
		retry.WithBackoffMultiplier(retryCfg.backoffMultiplier),
		retry.WithJitterFactor(retryCfg.jitterFactor),
		retry.WithRetryCondition(func(err error) bool {
			isTransient := IsTransientError(err)
			if isTransient && retryCfg.onRetry != nil {
				delay := calculateNextDelay(attempt, retryCfg)
				retryCfg.onRetry(attempt, err, delay)
			}
			return isTransient
		}),
	)

	if err != nil {
		return nil, err
	}
	return result, nil
}

var calculateNextDelay = func(attempt int, cfg retryConfig) time.Duration {
	delay := cfg.initialDelay
	for i := 1; i < attempt; i++ {
		delay = time.Duration(float64(delay) * cfg.backoffMultiplier)
		if delay > cfg.maxDelay {
			delay = cfg.maxDelay
			break
		}
	}
	return delay
}
