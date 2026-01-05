package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up orphaned worktrees and old run data (deprecated - use TypeScript CLI)",
	Long:  `This command has been deprecated. Please use the TypeScript CLI instead.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("Command deprecated - use TypeScript CLI")
		return nil
	},
}
