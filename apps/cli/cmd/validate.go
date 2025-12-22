package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [repo-path]",
	Short: "Validate workflow files (v0.0.2)",
	Long:  `Validates GitHub Actions workflow files for syntax and best practices.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runValidate,
}

func runValidate(_ *cobra.Command, _ []string) error {
	// TODO: Implement in v0.0.2
	fmt.Println("validate command not yet implemented (v0.0.2)")
	return nil
}
