package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "Manage which workflow jobs run (deprecated - use TypeScript CLI)",
	Long:  `This command has been deprecated. Please use the TypeScript CLI instead.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("Command deprecated - use TypeScript CLI")
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}
