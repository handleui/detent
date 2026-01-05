package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var healCmd = &cobra.Command{
	Use:   "heal",
	Short: "Auto-fix CI errors using AI (deprecated - use TypeScript CLI)",
	Long:  `This command has been deprecated. Please use the TypeScript CLI instead.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("Command deprecated - use TypeScript CLI")
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}
