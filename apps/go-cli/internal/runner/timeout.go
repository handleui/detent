package runner

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Environment variable name for act timeout configuration.
const (
	// ActTimeoutEnv overrides the default act execution timeout.
	// Value should be in minutes (e.g., "45" for 45 minutes).
	ActTimeoutEnv = "DETENT_ACT_TIMEOUT"
)

// Default timeout values.
const (
	defaultActTimeoutMinutes = 35

	// Minimum and maximum allowed timeout values (in minutes).
	minTimeoutMinutes = 1
	maxTimeoutMinutes = 120
)

// GetActTimeout returns the act execution timeout.
// Reads from DETENT_ACT_TIMEOUT, defaults to 35 minutes.
func GetActTimeout() time.Duration {
	minutes := getTimeoutFromEnv(ActTimeoutEnv, defaultActTimeoutMinutes)
	return time.Duration(minutes) * time.Minute
}

// getTimeoutFromEnv reads a timeout value from an environment variable.
// Returns the default if the env var is not set, empty, or invalid.
// Values are clamped to [minTimeoutMinutes, maxTimeoutMinutes].
func getTimeoutFromEnv(envVar string, defaultValue int) int {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultValue
	}

	minutes, err := strconv.Atoi(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid %s value %q, using default %d minutes\n",
			envVar, value, defaultValue)
		return defaultValue
	}

	return clampTimeout(minutes, defaultValue)
}

// clampTimeout ensures the timeout is within valid bounds.
// Returns the default if the value is out of range.
func clampTimeout(minutes, defaultValue int) int {
	if minutes < minTimeoutMinutes {
		fmt.Fprintf(os.Stderr, "warning: timeout %d minutes is below minimum %d, using default %d minutes\n",
			minutes, minTimeoutMinutes, defaultValue)
		return defaultValue
	}
	if minutes > maxTimeoutMinutes {
		fmt.Fprintf(os.Stderr, "warning: timeout %d minutes exceeds maximum %d, using default %d minutes\n",
			minutes, maxTimeoutMinutes, defaultValue)
		return defaultValue
	}
	return minutes
}
