package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "detent",
	Short: "Run GitHub Actions locally with enhanced error reporting",
	Long: `Detent wraps nektos/act to run GitHub Actions locally while:
- Injecting continue-on-error to run all steps
- Extracting and grouping errors by file
- Providing structured error output for debugging`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(validateCmd)
}
