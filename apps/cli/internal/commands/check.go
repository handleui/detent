package commands

import (
	"fmt"
	"os/exec"
)

// IsAvailable checks if a command is available in the system PATH.
// Uses exec.LookPath which works cross-platform (Unix, macOS, Windows).
// Returns nil if command exists, error with helpful message if not.
func IsAvailable(command string) error {
	_, err := exec.LookPath(command)
	if err != nil {
		return fmt.Errorf("%s not found in PATH", command)
	}
	return nil
}

// CheckAct verifies act is installed and provides installation instructions if missing.
func CheckAct() error {
	if err := IsAvailable("act"); err != nil {
		return fmt.Errorf("act is not installed\n\nInstall act:\n  macOS:   brew install act\n  Linux:   curl -s https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash\n  Windows: choco install act-cli\n\nMore info: https://github.com/nektos/act")
	}
	return nil
}

// CheckDocker verifies docker is available (command exists, not necessarily running).
func CheckDocker() error {
	if err := IsAvailable("docker"); err != nil {
		return fmt.Errorf("docker is not installed\n\nInstall Docker Desktop:\n  https://www.docker.com/products/docker-desktop")
	}
	return nil
}
