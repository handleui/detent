package git

import "os"

// DetectExecutionMode determines if the workflow is running via GitHub Actions or act (local).
// Returns "github" if GITHUB_ACTIONS environment variable is set to "true", otherwise "act".
func DetectExecutionMode() string {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return "github"
	}
	return "act"
}
