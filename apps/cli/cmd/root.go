package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/runner"
	"github.com/detent/cli/internal/signal"
	"github.com/detent/cli/internal/tui"
	"github.com/mattn/go-isatty"
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

// globalConfig holds the loaded configuration, available to all commands.
// Initialized in PersistentPreRunE.
var globalConfig *persistence.GlobalConfig

// warnStyle is used for warning messages.
var warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for config subcommands (they handle it themselves)
		// Walk up the command tree to check if any parent is the config command
		for c := cmd; c != nil; c = c.Parent() {
			if c == configCmd {
				return nil
			}
		}

		// Print branding
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

		// Load and validate global config
		cfg, configErr := persistence.LoadGlobalConfig()
		if configErr != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n",
				warnStyle.Render("⚠"),
				contextStyle.Render(fmt.Sprintf("Config error: %v", configErr)))
			fmt.Fprintf(os.Stderr, "  %s %s %s\n\n",
				contextStyle.Render("Run"),
				commandStyle.Render("detent config reset"),
				contextStyle.Render("to fix"))
			// Continue with defaults
			cfg = &persistence.GlobalConfig{
				Heal: persistence.DefaultHealConfig(),
			}
		}
		globalConfig = cfg

		return nil
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
	rootCmd.AddCommand(configCmd)

	// Persistent flags available to all commands
	rootCmd.PersistentFlags().StringVarP(&workflowsDir, "workflows", "w", runner.WorkflowsDir, "workflows directory path")
	rootCmd.PersistentFlags().StringVar(&workflowFile, "workflow", "", "specific workflow file (e.g., ci.yml)")

	rootCmd.SetHelpTemplate(fmt.Sprintf(`%s
{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`, brandingStyle.Render(fmt.Sprintf("Detent v%s", Version))))
}

// ensureAPIKey checks for API key and prompts interactively if missing.
// Uses globalConfig and saves the key if prompted.
// Returns the API key or an error if unavailable.
func ensureAPIKey() (string, error) {
	if globalConfig == nil {
		// This indicates a programming error - config should always be loaded
		// before commands that need API keys are run
		return "", fmt.Errorf("internal error: configuration not initialized")
	}

	// Check existing key (config takes precedence over env)
	existingKey := persistence.ResolveAPIKey(globalConfig.AnthropicAPIKey)
	if existingKey != "" {
		return existingKey, nil
	}

	// No key found - prompt if interactive terminal
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", fmt.Errorf("no API key found: set ANTHROPIC_API_KEY environment variable or run 'detent config show' for configuration options")
	}

	// Show interactive prompt
	model := tui.NewAPIKeyPromptModel()
	program := tea.NewProgram(model)

	if _, runErr := program.Run(); runErr != nil {
		return "", fmt.Errorf("prompt failed: %w", runErr)
	}

	result := model.GetResult()
	if result == nil || result.Cancelled {
		return "", fmt.Errorf("API key input cancelled")
	}

	// Save key to global config (create a copy to avoid partial state on error)
	updatedConfig := *globalConfig
	updatedConfig.AnthropicAPIKey = result.Key
	if saveErr := persistence.SaveGlobalConfig(&updatedConfig); saveErr != nil {
		return "", fmt.Errorf("failed to save API key: %w", saveErr)
	}

	// Only update the global after successful save
	globalConfig.AnthropicAPIKey = result.Key

	fmt.Fprintf(os.Stderr, "%s %s\n\n",
		brandingStyle.Render("+"),
		contextStyle.Render("API key saved to configuration"))

	return result.Key, nil
}
