package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/runner"
	"github.com/detent/cli/internal/signal"
	"github.com/detent/cli/internal/tui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	// Global flags shared across commands
	workflowsDir string
	workflowFile string
)

// globalConfig holds the loaded configuration, available to all commands.
// Initialized in PersistentPreRunE.
var globalConfig *persistence.GlobalConfig

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
		// Skip for config subcommands
		for c := cmd; c != nil; c = c.Parent() {
			if c == configCmd {
				return nil
			}
		}

		// Branding: "Detent v0.1.0 · branch main"
		fmt.Println()
		repoRoot, _ := filepath.Abs(".")
		branch, _ := git.GetCurrentBranch(repoRoot)

		header := tui.BrandStyle.Render("Detent") + " " + tui.MutedStyle.Render("v"+Version)
		if branch != "" {
			header += " " + tui.MutedStyle.Render("· "+branch)
		}
		fmt.Println(header)
		fmt.Println()

		// Load config
		cfg, configErr := persistence.LoadGlobalConfig()
		if configErr != nil {
			fmt.Fprintf(os.Stderr, "%s Config error: %s\n",
				tui.WarningStyle.Render("!"),
				tui.MutedStyle.Render(configErr.Error()))
			fmt.Fprintf(os.Stderr, "%s Run: detent config reset\n\n", tui.Bullet())
			cfg = &persistence.GlobalConfig{
				Heal: persistence.DefaultHealConfig(),
			}
		}
		globalConfig = cfg

		// Trust check - FIRST thing before any command runs
		// This ensures we never execute repo code without explicit trust
		if err := ensureTrustedRepo(); err != nil {
			return err
		}

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

	rootCmd.SetHelpTemplate(`Detent v` + Version + `
{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`)
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

	fmt.Fprintf(os.Stderr, "%s API key saved\n\n", tui.SuccessStyle.Render("✓"))

	return result.Key, nil
}

// ensureTrustedRepo checks if the current repository is trusted, prompts if not.
// Returns error if user declines trust, not in a git repo, or not interactive.
func ensureTrustedRepo() error {
	if globalConfig == nil {
		return fmt.Errorf("internal error: configuration not initialized")
	}

	repoRoot, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	firstCommitSHA, err := git.GetFirstCommitSHA(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to identify repository: %w", err)
	}
	if firstCommitSHA == "" {
		return fmt.Errorf("repository has no commits yet")
	}

	// Check if already trusted
	if globalConfig.IsTrustedRepo(firstCommitSHA) {
		return nil
	}

	// Not interactive? Fail with instructions
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("repository not trusted: run 'detent check' interactively first")
	}

	// Show trust prompt
	remoteURL, _ := git.GetRemoteURL(repoRoot)
	shortSHA := firstCommitSHA
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}

	model := tui.NewTrustPromptModel(tui.TrustPromptInfo{
		RemoteURL:      remoteURL,
		FirstCommitSHA: shortSHA,
	})
	program := tea.NewProgram(model)

	if _, runErr := program.Run(); runErr != nil {
		return fmt.Errorf("trust prompt failed: %w", runErr)
	}

	result := model.GetResult()
	if result == nil || result.Cancelled {
		return fmt.Errorf("trust prompt cancelled")
	}
	if !result.Trusted {
		return fmt.Errorf("repository trust declined")
	}

	// Save trust to config
	if trustErr := globalConfig.TrustRepo(firstCommitSHA, remoteURL); trustErr != nil {
		return fmt.Errorf("failed to save trust: %w", trustErr)
	}

	fmt.Fprintf(os.Stderr, "%s Repository trusted\n\n", tui.SuccessStyle.Render("✓"))
	return nil
}
