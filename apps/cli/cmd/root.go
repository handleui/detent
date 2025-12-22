package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var brandingStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#808080"))

var rootCmd = &cobra.Command{
	Use:   "detent",
	Short: "Run GitHub Actions locally with enhanced error reporting",
	Long: `Detent wraps nektos/act to run GitHub Actions locally while:
- Injecting continue-on-error to run all steps
- Extracting and grouping errors by file
- Providing structured error output for debugging`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println(brandingStyle.Render(fmt.Sprintf("Detent v%s", Version)))
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(validateCmd)
}
