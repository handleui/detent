package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/heal/client"
	"github.com/detent/cli/internal/heal/loop"
	"github.com/detent/cli/internal/heal/prompt"
	"github.com/detent/cli/internal/heal/tools"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/tui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var testAPI bool

var (
	healSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	healDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	healTextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

var healCmd = &cobra.Command{
	Use:   "heal",
	Short: "Auto-fix CI errors using AI",
	Long: `Heal uses AI to automatically fix errors found by the check command.

The command performs these steps:
  1. Checks if a prior run exists for the current codebase state
  2. If not, runs 'check' to identify errors and create a worktree
  3. Loads errors from the database
  4. Uses Claude to read, analyze, and fix errors in the isolated worktree
  5. Verifies fixes by re-running the relevant CI checks

The same codebase state (tree hash + commit) always maps to the same run,
so heal can reuse existing worktrees created by check.`,
	Example: `  # Heal errors from the last check run
  detent heal

  # Force a fresh check before healing
  detent heal --force`,
	Args:          cobra.NoArgs,
	RunE:          runHeal,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	healCmd.Flags().BoolVarP(&forceRun, "force", "f", false, "force fresh check run")
	healCmd.Flags().BoolVar(&testAPI, "test", false, "test Claude API connection")
}

func runHeal(cmd *cobra.Command, args []string) error {
	// Load global config
	config, err := persistence.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Preflight: ensure API key is available
	apiKey, err := ensureAPIKey(config)
	if err != nil {
		return err
	}

	// Handle --test flag (deprecated)
	if testAPI {
		fmt.Fprintf(os.Stderr, "%s %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("⚠"),
			healTextStyle.Render("--test is deprecated, use 'dt frankenstein' instead"))
		return runHealTest(cmd.Context(), apiKey)
	}

	// Resolve repository path
	repoPath, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	// Compute deterministic run ID from current codebase state
	runID, _, _, err := git.ComputeCurrentRunID(repoPath)
	if err != nil {
		return err
	}

	// Check if worktree exists (means check already ran for this state)
	worktreeExists := git.WorktreeExists(runID)

	if !worktreeExists || forceRun {
		fmt.Fprintf(os.Stderr, "Running check to create worktree...\n")
		if checkErr := runCheck(cmd, args); checkErr != nil {
			// Check returns error if there are errors found - that's expected for heal
			// We continue if the error is about found errors, but fail on other errors
			if checkErr.Error() != "found errors in workflow execution" {
				return checkErr
			}
		}
	}

	// Verify worktree now exists
	worktreePath, err := git.GetWorktreePath(runID)
	if err != nil {
		return fmt.Errorf("getting worktree path: %w", err)
	}

	if _, statErr := os.Stat(worktreePath); os.IsNotExist(statErr) {
		return fmt.Errorf("worktree not found at %s - check may have failed", worktreePath)
	}

	// Load errors from database
	db, err := persistence.NewSQLiteWriter(repoPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	errors, err := db.GetErrorsByRunID(runID)
	if err != nil {
		return fmt.Errorf("loading errors: %w", err)
	}

	if len(errors) == 0 {
		fmt.Println("No errors to heal")
		return nil
	}

	// Display summary
	fmt.Fprintf(os.Stderr, "%s %s\n",
		healDimStyle.Render(">"),
		healTextStyle.Render(fmt.Sprintf("Found %d errors in run %s", len(errors), runID)))
	fmt.Fprintf(os.Stderr, "%s %s\n\n",
		healDimStyle.Render(">"),
		healTextStyle.Render(fmt.Sprintf("Worktree: %s", worktreePath)))

	// Create Anthropic client
	c, err := client.New(apiKey)
	if err != nil {
		return err
	}

	// Set up tool context
	toolCtx := &tools.Context{
		WorktreePath: worktreePath,
		RepoRoot:     repoPath,
		RunID:        runID,
	}

	// Create tool registry and register tools
	registry := tools.NewRegistry(toolCtx)
	registry.Register(tools.NewReadFileTool(toolCtx))
	registry.Register(tools.NewEditFileTool(toolCtx))
	registry.Register(tools.NewGlobTool(toolCtx))
	registry.Register(tools.NewGrepTool(toolCtx))
	registry.Register(tools.NewVerifyTool(toolCtx))
	registry.Register(tools.NewRunCommandTool(toolCtx))

	// Build user prompt from errors
	userPrompt := buildUserPrompt(errors)

	// Create and run healing loop with config from global settings
	loopConfig := loop.ConfigFromHealConfig(config.Heal)
	healLoop := loop.New(c.API(), registry, loopConfig)

	fmt.Fprintf(os.Stderr, "%s %s\n",
		healDimStyle.Render(">"),
		healTextStyle.Render("Starting AI healing loop..."))

	result, err := healLoop.Run(cmd.Context(), prompt.BuildSystemPrompt(), userPrompt)
	if err != nil {
		return fmt.Errorf("healing loop failed: %w", err)
	}

	// Display result
	fmt.Fprintf(os.Stderr, "\n")
	if result.Success {
		fmt.Fprintf(os.Stderr, "%s %s\n",
			healSuccessStyle.Render("+"),
			healTextStyle.Render("Healing completed successfully"))
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("!"),
			healTextStyle.Render("Healing incomplete"))
	}

	fmt.Fprintf(os.Stderr, "%s %s\n",
		healDimStyle.Render(">"),
		healTextStyle.Render(fmt.Sprintf("Iterations: %d, Tool calls: %d, Duration: %s",
			result.Iterations, result.ToolCalls, result.Duration.Round(100*1e6))))

	if result.FinalMessage != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n%s\n",
			healDimStyle.Render("Summary:"),
			result.FinalMessage)
	}

	return nil
}

// buildUserPrompt formats error records into a prompt for Claude.
func buildUserPrompt(errors []*persistence.ErrorRecord) string {
	var sb strings.Builder

	sb.WriteString("Fix the following CI errors:\n\n")

	// Group errors by file
	byFile := make(map[string][]*persistence.ErrorRecord)
	for _, err := range errors {
		byFile[err.FilePath] = append(byFile[err.FilePath], err)
	}

	for filePath, fileErrors := range byFile {
		sb.WriteString(fmt.Sprintf("## %s (%d errors)\n\n", filePath, len(fileErrors)))

		for _, err := range fileErrors {
			// Format: [category] file:line:column: message
			sb.WriteString(fmt.Sprintf("[%s] %s:%d:%d: %s\n",
				err.ErrorType, err.FilePath, err.LineNumber, err.ColumnNumber, err.Message))

			if err.RuleID != "" {
				sb.WriteString(fmt.Sprintf("  Rule: %s | Source: %s\n", err.RuleID, err.Source))
			}

			if err.StackTrace != "" {
				sb.WriteString("  Stack trace:\n")
				for _, line := range strings.Split(err.StackTrace, "\n")[:min(10, len(strings.Split(err.StackTrace, "\n")))] {
					sb.WriteString(fmt.Sprintf("    %s\n", line))
				}
			}

			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nUse the available tools to read files, make edits, and verify your fixes.")

	return sb.String()
}

// runHealTest tests the Claude API connection.
func runHealTest(ctx context.Context, apiKey string) error {
	c, err := client.New(apiKey)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "%s %s\n",
		healDimStyle.Render(">"),
		healTextStyle.Render("Testing Claude API connection..."))

	response, err := c.Test(ctx)
	if err != nil {
		return fmt.Errorf("API test failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s %s\n",
		healSuccessStyle.Render("+"),
		healTextStyle.Render("Connected"))

	fmt.Fprintf(os.Stderr, "\n%s\n%s\n\n",
		healDimStyle.Render("Claude says:"),
		response)

	return nil
}

// ensureAPIKey checks for API key and prompts interactively if missing.
// Returns the API key or an error if unavailable.
func ensureAPIKey(config *persistence.GlobalConfig) (string, error) {
	// Check existing key (config takes precedence over env)
	existingKey := persistence.ResolveAPIKey(config.AnthropicAPIKey)
	if existingKey != "" {
		return existingKey, nil
	}

	// No key found - prompt if interactive terminal
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", fmt.Errorf("no API key: set ANTHROPIC_API_KEY env var or add anthropic_api_key to ~/.detent/config.yaml")
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

	// Save key to global config
	config.AnthropicAPIKey = result.Key
	if saveErr := persistence.SaveGlobalConfig(config); saveErr != nil {
		return "", fmt.Errorf("saving config: %w", saveErr)
	}

	fmt.Fprintf(os.Stderr, "%s %s %s\n\n",
		healSuccessStyle.Render("+"),
		healTextStyle.Render("API key saved to"),
		healDimStyle.Render("~/.detent/config.yaml"))

	return result.Key, nil
}
