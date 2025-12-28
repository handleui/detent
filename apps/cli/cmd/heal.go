package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/heal/client"
	"github.com/detent/cli/internal/heal/loop"
	"github.com/detent/cli/internal/heal/prompt"
	"github.com/detent/cli/internal/heal/tools"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/tui"
	"github.com/spf13/cobra"
)

var testAPI bool

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
	// Preflight: ensure API key is available
	apiKey, err := ensureAPIKey()
	if err != nil {
		return err
	}

	// Handle --test flag (deprecated)
	if testAPI {
		fmt.Fprintf(os.Stderr, "%s --test is deprecated, use 'dt frankenstein' instead\n",
			tui.WarningStyle.Render("!"))
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
			// Check returns ErrFoundErrors when errors are found - that's expected for heal.
			// We continue in that case but fail on other errors.
			if !errors.Is(checkErr, ErrFoundErrors) {
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

	errRecords, err := db.GetErrorsByRunID(runID)
	if err != nil {
		return fmt.Errorf("loading errors: %w", err)
	}

	if len(errRecords) == 0 {
		fmt.Println("No errors to heal")
		return nil
	}

	// Display summary
	fmt.Fprintf(os.Stderr, "%s Found %d errors\n", tui.Bullet(), len(errRecords))
	fmt.Fprintf(os.Stderr, "%s Worktree: %s\n\n", tui.Bullet(), tui.MutedStyle.Render(worktreePath))

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
	userPrompt := buildUserPrompt(errRecords)

	// Create and run healing loop with config from global settings
	loopConfig := loop.ConfigFromHealConfig(globalConfig.Heal)
	healLoop := loop.New(c.API(), registry, loopConfig)

	fmt.Fprintf(os.Stderr, "%s Starting healing...\n", tui.Bullet())

	result, err := healLoop.Run(cmd.Context(), prompt.BuildSystemPrompt(), userPrompt)
	if err != nil {
		return fmt.Errorf("healing loop failed: %w", err)
	}

	// Display result
	fmt.Fprintf(os.Stderr, "\n")
	switch {
	case result.Success:
		fmt.Fprintf(os.Stderr, "%s Healing complete\n", tui.SuccessStyle.Render("✓"))
	case result.BudgetExceeded:
		fmt.Fprintf(os.Stderr, "%s Budget limit reached\n", tui.WarningStyle.Render("!"))
	default:
		fmt.Fprintf(os.Stderr, "%s Healing incomplete\n", tui.ErrorStyle.Render("✗"))
	}

	fmt.Fprintf(os.Stderr, "%s %d iterations, %d tool calls, %s\n",
		tui.Bullet(),
		result.Iterations, result.ToolCalls, result.Duration.Round(100*1e6))

	fmt.Fprintf(os.Stderr, "%s $%.4f (%dk in, %dk out)\n",
		tui.Bullet(),
		result.CostUSD, result.InputTokens/1000, result.OutputTokens/1000)

	if result.FinalMessage != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n%s\n",
			tui.SecondaryStyle.Render("Summary"),
			result.FinalMessage)
	}

	if result.BudgetExceeded {
		return fmt.Errorf("budget limit ($%.2f) exceeded", globalConfig.Heal.BudgetUSD)
	}

	return nil
}

// buildUserPrompt formats error records into a prompt for Claude.
func buildUserPrompt(errRecords []*persistence.ErrorRecord) string {
	var sb strings.Builder

	sb.WriteString("Fix the following CI errors:\n\n")

	// Group errors by file
	byFile := make(map[string][]*persistence.ErrorRecord)
	for _, rec := range errRecords {
		byFile[rec.FilePath] = append(byFile[rec.FilePath], rec)
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

	fmt.Fprintf(os.Stderr, "%s Testing API connection...\n", tui.Bullet())

	response, err := c.Test(ctx)
	if err != nil {
		return fmt.Errorf("API test failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s Connected\n", tui.SuccessStyle.Render("✓"))
	fmt.Fprintf(os.Stderr, "\n%s\n%s\n\n",
		tui.SecondaryStyle.Render("Response"),
		response)

	return nil
}

