package docker

import (
	"context"
	"os/exec"
	"time"
)

const dockerCheckTimeout = 5 * time.Second

// IsAvailable checks if Docker daemon is running and accessible.
// Returns error if docker command fails or times out after 5 seconds.
func IsAvailable(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, dockerCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(checkCtx, "docker", "info")
	return cmd.Run()
}
