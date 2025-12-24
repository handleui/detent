package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetCurrentCommitSHA retrieves the current git commit SHA.
// Returns an error if not in a git repository or git is not available.
func GetCurrentCommitSHA() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git commit SHA: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
