package docker

import (
	"context"
	"os/exec"
	"time"
)

// IsAvailable checks if Docker is running and accessible
func IsAvailable(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(checkCtx, "docker", "info")
	return cmd.Run()
}
