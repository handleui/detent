package cmd

import (
	"context"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/signal"
	"github.com/spf13/cobra"
)

var brandingStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#808080"))

var rootCmd = &cobra.Command{
	Use:     "detent",
	Short:   "Run GitHub Actions locally with enhanced error reporting",
	Long:    "Debug GitHub Actions locally with structured error extraction and grouping",
	Version: Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println(brandingStyle.Render(fmt.Sprintf("Detent v%s", Version)))
	},
}

func Execute() error {
	ctx := signal.SetupSignalHandler(context.Background())
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(injectCmd)

	rootCmd.SetHelpTemplate(fmt.Sprintf(`%s
{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`, brandingStyle.Render(fmt.Sprintf("Detent v%s", Version))))
}
