package cmd

import (
	"fmt"

	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/update"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:           "update",
	Short:         "Update detent to the latest version",
	Long:          `Downloads and installs the latest version of detent using the official install script.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(_ *cobra.Command, _ []string) error {
	fmt.Println()

	latest, hasUpdate := update.Check(Version)

	if !hasUpdate {
		fmt.Println(tui.ExitSuccess("Already on the latest version"))
		return nil
	}

	fmt.Printf("  Updating to %s...\n\n", tui.AccentStyle.Render(latest))

	if err := update.Run(); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Println()
	fmt.Println(tui.ExitSuccess("Updated to " + latest))

	return nil
}
