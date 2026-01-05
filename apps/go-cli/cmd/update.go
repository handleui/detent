package cmd

import (
	"fmt"

	"github.com/detent/go-cli/internal/tui"
	"github.com/detent/go-cli/internal/update"
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
	// Header with consistent padding
	fmt.Println()
	fmt.Println(tui.Header(Version, "update"))

	latest, hasUpdate := update.Check(Version)

	if !hasUpdate {
		fmt.Println(tui.ExitSuccess("Already on latest"))
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println(tui.MutedStyle.Render("v"+Version) + " " + tui.MutedStyle.Render("→") + " " + tui.AccentStyle.Render(latest))
	fmt.Println()

	if err := update.Run(); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Println()
	fmt.Println(tui.ExitSuccess("Updated"))
	fmt.Println()

	return nil
}
