package runner

import (
	"context"

	"github.com/detent/go-cli/internal/actbin"
	"github.com/detent/go-cli/internal/docker"
)

// ensureActInstalled checks that act is installed and available.
// This is a package-level helper to avoid import cycles.
func ensureActInstalled(ctx context.Context) error {
	return actbin.EnsureInstalled(ctx, nil)
}

// checkDockerAvailable checks that Docker is running and available.
// This is a package-level helper to avoid import cycles.
func checkDockerAvailable(ctx context.Context) error {
	return docker.IsAvailable(ctx)
}
