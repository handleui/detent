package act

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

var ErrTransient = errors.New("transient act failure")

var ErrMaxRetriesExceeded = errors.New("max retries exceeded")

var transientPatterns = []string{
	"cannot connect to docker daemon",
	"is the docker daemon running",
	"connection refused",
	"no such host",
	"i/o timeout",
	"network is unreachable",
	"connection reset by peer",
	"connection timed out",
	"error pulling image",
	"failed to pull",
	"image pull failed",
	"context deadline exceeded",
	"unable to find image",
	"docker daemon is not running",
	"error response from daemon",
	"container create failed",
	"cannot start container",
	"oci runtime",
}

type RetryConfig struct {
	MaxAttempts       int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
	OnRetry           func(attempt int, err error, delay time.Duration)
}

var DefaultRetryConfig = RetryConfig{
	MaxAttempts:       3,
	InitialDelay:      1 * time.Second,
	MaxDelay:          4 * time.Second,
	BackoffMultiplier: 2.0,
	OnRetry:           nil,
}

type RetryOption func(*RetryConfig)

var WithMaxAttempts = func(n int) RetryOption {
	return func(c *RetryConfig) {
		c.MaxAttempts = n
	}
}

var WithInitialDelay = func(d time.Duration) RetryOption {
	return func(c *RetryConfig) {
		c.InitialDelay = d
	}
}

var WithMaxDelay = func(d time.Duration) RetryOption {
	return func(c *RetryConfig) {
		c.MaxDelay = d
	}
}

var WithBackoffMultiplier = func(m float64) RetryOption {
	return func(c *RetryConfig) {
		c.BackoffMultiplier = m
	}
}

var WithOnRetry = func(fn func(attempt int, err error, delay time.Duration)) RetryOption {
	return func(c *RetryConfig) {
		c.OnRetry = fn
	}
}

var IsTransientError = func(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrTransient) {
		return true
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	errStr := strings.ToLower(err.Error())
	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

var classifyError = func(err error, result *RunResult) error {
	if err == nil && result != nil {
		combined := strings.ToLower(result.Stdout + result.Stderr)
		for _, pattern := range transientPatterns {
			if strings.Contains(combined, pattern) {
				return fmt.Errorf("%w: %s", ErrTransient, pattern)
			}
		}
		return nil
	}

	if err != nil && IsTransientError(err) {
		return fmt.Errorf("%w: %v", ErrTransient, err)
	}

	return err
}

var RunWithRetry = func(ctx context.Context, cfg *RunConfig, opts ...RetryOption) (*RunResult, error) {
	retryCfg := DefaultRetryConfig
	for _, opt := range opts {
		opt(&retryCfg)
	}

	if retryCfg.MaxAttempts <= 0 {
		return nil, fmt.Errorf("%w: no attempts configured", ErrMaxRetriesExceeded)
	}

	var lastErr error
	var lastResult *RunResult
	delay := retryCfg.InitialDelay

	for attempt := 1; attempt <= retryCfg.MaxAttempts; attempt++ {
		cfgCopy := *cfg
		if attempt > 1 && cfg.LogChan != nil {
			cfgCopy.LogChan = nil
		}

		result, err := Run(ctx, &cfgCopy)

		classifiedErr := classifyError(err, result)

		if classifiedErr == nil {
			return result, nil
		}

		lastErr = classifiedErr
		lastResult = result

		if !IsTransientError(classifiedErr) {
			if err != nil {
				return result, err
			}
			return result, nil
		}

		if attempt == retryCfg.MaxAttempts {
			break
		}

		jitteredDelay := addJitter(delay)

		if retryCfg.OnRetry != nil {
			retryCfg.OnRetry(attempt, classifiedErr, jitteredDelay)
		}

		timer := time.NewTimer(jitteredDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		delay = calculateNextDelay(delay, retryCfg.BackoffMultiplier, retryCfg.MaxDelay)
	}

	if lastResult != nil && lastResult.ExitCode != 0 {
		return lastResult, fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
	}

	return lastResult, fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

var calculateNextDelay = func(current time.Duration, multiplier float64, maxDelay time.Duration) time.Duration {
	if multiplier <= 1.0 {
		return min(current, maxDelay)
	}

	result := float64(current) * multiplier
	if math.IsInf(result, 0) || math.IsNaN(result) || result > float64(math.MaxInt64) {
		return maxDelay
	}

	newDelay := time.Duration(result)
	if newDelay < 0 {
		return maxDelay
	}

	return min(newDelay, maxDelay)
}

var addJitter = func(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}

	jitterRange := float64(d) * 0.2
	jitter := time.Duration(jitterRange * (2*rand.Float64() - 1)) //nolint:gosec // math/rand is fine for jitter

	result := d + jitter
	if result < 0 {
		return 0
	}
	return result
}

type retryLogWriter struct {
	underlying io.Writer
}

var newRetryLogWriter = func(w io.Writer) *retryLogWriter {
	return &retryLogWriter{underlying: w}
}

func (w *retryLogWriter) Write(p []byte) (n int, err error) {
	if w.underlying != nil {
		return w.underlying.Write(p)
	}
	return len(p), nil
}
