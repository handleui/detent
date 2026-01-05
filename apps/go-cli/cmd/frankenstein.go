package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var frankensteinCmd = &cobra.Command{
	Use:   "frankenstein",
	Short: "Test AI tools integration (deprecated - use TypeScript CLI)",
	Long:  `This command has been deprecated. Please use the TypeScript CLI instead.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("Command deprecated - use TypeScript CLI")
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}
