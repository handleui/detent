package docker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/detent/cli/internal/util"
)

const dockerCheckTimeout = 5 * time.Second

// ErrDockerNotInstalled indicates docker command is not in PATH.
var ErrDockerNotInstalled = errors.New("docker is not installed")

// ErrDockerNotRunning indicates docker daemon is not running.
var ErrDockerNotRunning = errors.New("docker daemon is not running")

// ErrDockerPermission indicates permission denied accessing docker.
var ErrDockerPermission = errors.New("permission denied accessing docker")

// IsAvailable checks if Docker daemon is running and accessible.
// Returns specific error types to help users diagnose issues:
// - ErrDockerNotInstalled: docker command not found
// - ErrDockerNotRunning: daemon not running or not responding
// - ErrDockerPermission: user lacks permission to access docker socket
func IsAvailable(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return ErrDockerNotInstalled
	}

	checkCtx, cancel := context.WithTimeout(ctx, dockerCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(checkCtx, "docker", "info")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	outputStr := strings.ToLower(string(output))

	// Check most specific patterns first
	switch {
	case strings.Contains(outputStr, "permission denied"):
		return fmt.Errorf("%w: try running with sudo or add user to docker group", ErrDockerPermission)
	case strings.Contains(outputStr, "cannot connect"),
		strings.Contains(outputStr, "is the docker daemon running"),
		strings.Contains(outputStr, "connection refused"):
		return fmt.Errorf("%w: start Docker Desktop or run 'sudo systemctl start docker'", ErrDockerNotRunning)
	}

	if checkCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%w: docker daemon not responding (timeout after %v)", ErrDockerNotRunning, dockerCheckTimeout)
	}

	return fmt.Errorf("docker check failed: %w", err)
}

// IsAvailableWithRetry checks Docker availability with retry logic.
// Only retries ErrDockerNotRunning (daemon might be starting).
// Does not retry ErrDockerNotInstalled or ErrDockerPermission.
func IsAvailableWithRetry(ctx context.Context) error {
	return util.Retry(ctx, IsAvailable,
		util.WithMaxAttempts(3),
		util.WithInitialDelay(200*time.Millisecond),
		util.WithMaxDelay(2*time.Second),
		util.WithBackoffMultiplier(2.0),
		util.WithJitterFactor(0.1),
		util.WithRetryCondition(func(err error) bool {
			// Only retry if daemon might be starting
			return errors.Is(err, ErrDockerNotRunning)
		}),
	)
}
