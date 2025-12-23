package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate workflow syntax and best practices",
	Long: `Validate GitHub Actions workflow files for syntax errors and best practices.
Performs static analysis without running the workflows.

This command checks for:
  - YAML syntax errors
  - Invalid workflow schema
  - Common configuration mistakes
  - Best practice violations

Note: This command is planned for v0.0.2 and is not yet implemented.`,
	Example: `  # Validate all workflows in current directory
  detent validate

  # Validate specific workflow
  detent validate --workflow ci.yml`,
	Args: cobra.NoArgs,
	RunE: runValidate,
}

func runValidate(cmd *cobra.Command, args []string) error {
	// TODO: Implement in v0.0.2
	// When implementing, use cmd.Context() to get signal-aware context:
	// ctx := cmd.Context()
	// And check for cancellation: errors.Is(ctx.Err(), context.Canceled)
	fmt.Println("validate command not yet implemented (v0.0.2)")
	return nil
}
