package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/runner"
	"github.com/detent/cli/internal/signal"
	"github.com/spf13/cobra"
)

const (
	brandingColor = "42"  // Green - matches colorSuccess in TUI
	commandColor  = "15"  // Pure white for command name
	contextColor  = "241" // Gray - matches hintTextGray in TUI
)

var (
	// Global flags shared across commands
	workflowsDir string
	workflowFile string
)

var (
	brandingStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(brandingColor))
	commandStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(commandColor))
	contextStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(contextColor))
)

var rootCmd = &cobra.Command{
	Use:   "detent",
	Short: "Run GitHub Actions locally with enhanced error reporting",
	Long: `Detent helps you debug GitHub Actions workflows locally by running them
with act and providing structured error extraction and grouping.

It automatically injects continue-on-error to ensure all steps run, making
it easier to catch all issues in one pass. Results are grouped by file for
efficient debugging.

Requirements:
  - Docker (for running act containers)
  - act (nektos/act - automatically invoked)`,
	Version: Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		fmt.Println()
		versionText := brandingStyle.Render(fmt.Sprintf("Detent v%s", Version))
		commandText := commandStyle.Render(cmd.Name())
		fmt.Printf("%s %s\n", versionText, commandText)

		repoRoot, err := filepath.Abs(".")
		if err == nil {
			if branch, branchErr := git.GetCurrentBranch(repoRoot); branchErr == nil {
				fmt.Printf("%s\n\n", contextStyle.Render(fmt.Sprintf("└─ on branch %s", branch)))
			}
		}
	},
}

// Execute runs the root command with signal handling
func Execute() error {
	ctx := signal.SetupSignalHandler(context.Background())
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(healCmd)
	rootCmd.AddCommand(injectCmd)
	rootCmd.AddCommand(frankensteinCmd)

	// Persistent flags available to all commands
	rootCmd.PersistentFlags().StringVarP(&workflowsDir, "workflows", "w", runner.WorkflowsDir, "workflows directory path")
	rootCmd.PersistentFlags().StringVar(&workflowFile, "workflow", "", "specific workflow file (e.g., ci.yml)")

	rootCmd.SetHelpTemplate(fmt.Sprintf(`%s
{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`, brandingStyle.Render(fmt.Sprintf("Detent v%s", Version))))
}
