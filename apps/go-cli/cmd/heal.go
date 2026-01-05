package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/go-cli/internal/persistence"
	"github.com/detent/go-cli/internal/repo"
	"github.com/detent/go-cli/internal/sentry"
	"github.com/detent/go-cli/internal/tui"
	"github.com/detentsh/core/git"
	"github.com/detentsh/core/heal/client"
	"github.com/detentsh/core/heal/loop"
	"github.com/detentsh/core/heal/prompt"
	"github.com/detentsh/core/heal/tools"
	"github.com/spf13/cobra"
)

var (
	testAPI      bool
	healForceRun bool
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
	healCmd.Flags().BoolVarP(&healForceRun, "force", "f", false, "force fresh check run")
	healCmd.Flags().BoolVar(&testAPI, "test", false, "test Claude API connection")
}

func runHeal(cmd *cobra.Command, args []string) error {
	sentry.AddBreadcrumb("heal", "starting heal command")

	// Preflight: ensure API key is available
	apiKey, err := ensureAPIKey()
	if err != nil {
		sentry.CaptureError(err)
		return err
	}

	// Handle --test flag (deprecated)
	if testAPI {
		fmt.Fprintf(os.Stderr, "%s --test is deprecated, use 'dt frankenstein' instead\n",
			tui.WarningStyle.Render("!"))
		return runHealTest(cmd.Context(), apiKey)
	}

	// Resolve repository context (path, first commit SHA, run ID)
	repoCtx, err := repo.Resolve(repo.WithAll())
	if err != nil {
		return err
	}

	// Open database for error data (per-repo)
	sentry.AddBreadcrumb("heal", "opening database")
	db, err := persistence.NewSQLiteWriter(repoCtx.Path)
	if err != nil {
		sentry.CaptureError(err)
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Acquire exclusive heal lock to prevent concurrent heals on same repo
	sentry.AddBreadcrumb("heal", "acquiring lock")
	lockTimeout := time.Duration(cfg.TimeoutMins) * time.Minute
	holderID, err := db.AcquireHealLock(repoCtx.Path, lockTimeout)
	if err != nil {
		if errors.Is(err, persistence.ErrHealLockHeld) {
			return fmt.Errorf("cannot start heal: %w", err)
		}
		sentry.CaptureError(err)
		return fmt.Errorf("acquiring heal lock: %w", err)
	}
	defer func() { _ = db.ReleaseHealLock(repoCtx.Path, holderID) }()

	// Get global spend database for cross-repo budget tracking
	spendDB, err := persistence.GetSpendDB()
	if err != nil {
		return fmt.Errorf("opening spend database: %w", err)
	}

	// Check if this run exists in the database (cache hit)
	// If check fails, treat as cache miss (fail-safe) but log for debugging
	runExists, cacheErr := db.RunExists(repoCtx.RunID)
	if cacheErr != nil {
		sentry.AddBreadcrumb("heal", fmt.Sprintf("cache check failed: %v", cacheErr))
		runExists = false // fail-safe: treat as cache miss
	}
	sentry.AddBreadcrumb("heal", fmt.Sprintf("cache_hit=%v force=%v", runExists, healForceRun))

	// If no cached run or force flag, run check first
	if !runExists || healForceRun {
		fmt.Fprintf(os.Stderr, "Running check to identify errors...\n")
		if checkErr := runCheck(cmd, args); checkErr != nil {
			// Check returns ErrFoundErrors when errors are found - that's expected for heal.
			// We continue in that case but fail on other errors.
			if !errors.Is(checkErr, ErrFoundErrors) {
				return checkErr
			}
		}
	} else {
		// Feedback on cache hit (shown before loading to give immediate feedback)
		fmt.Fprintf(os.Stderr, "%s Using cached errors (--force to re-check)\n", tui.Bullet())
	}

	// Load errors (from cache or fresh after check)
	errRecords, err := db.GetErrorsByRunID(repoCtx.RunID)
	if err != nil {
		return fmt.Errorf("loading errors: %w", err)
	}

	if len(errRecords) == 0 {
		fmt.Println("No errors to heal")
		return nil
	}

	// Create ephemeral worktree for healing
	sentry.AddBreadcrumb("heal", "creating worktree")
	worktreePath, err := git.CreateEphemeralWorktreePath(repoCtx.RunID)
	if err != nil {
		sentry.CaptureError(err)
		return fmt.Errorf("creating worktree path: %w", err)
	}

	worktreeInfo, cleanupWorktree, err := git.PrepareWorktree(cmd.Context(), repoCtx.Path, worktreePath)
	if err != nil {
		sentry.CaptureError(err)
		return fmt.Errorf("creating worktree: %w", err)
	}
	defer cleanupWorktree()

	worktreePath = worktreeInfo.Path

	// Check monthly budget before proceeding (uses global spend DB)
	monthlySpend, err := spendDB.GetMonthlySpend("")
	if err != nil {
		return fmt.Errorf("getting monthly spend: %w", err)
	}
	var remainingMonthly float64 = -1 // -1 means unlimited
	if cfg.BudgetMonthlyUSD > 0 {
		remainingMonthly = cfg.BudgetMonthlyUSD - monthlySpend
		if remainingMonthly <= 0 {
			return fmt.Errorf("monthly budget exhausted ($%.2f/$%.2f)", monthlySpend, cfg.BudgetMonthlyUSD)
		}
	}

	// Display summary
	fmt.Fprintf(os.Stderr, "%s Found %d errors\n", tui.Bullet(), len(errRecords))
	fmt.Fprintf(os.Stderr, "%s Worktree: %s\n\n", tui.Bullet(), tui.MutedStyle.Render(worktreePath))

	// Create Anthropic client
	c, err := client.New(apiKey)
	if err != nil {
		return err
	}

	// Set up tool context with command approval
	repoSHA := repoCtx.FirstCommitSHA
	remoteURL, _ := git.GetRemoteURL(repoCtx.Path)
	toolCtx := &tools.Context{
		WorktreePath:   worktreePath,
		RepoRoot:       repoCtx.Path,
		RunID:          repoCtx.RunID,
		FirstCommitSHA: repoSHA,
		CommandChecker: func(cmd string) bool {
			return cfg.MatchesCommand(repoSHA, cmd)
		},
		CommandApprover:  promptForCommand,
		CommandPersister: func(cmd string) error {
			return cfg.AddAllowedCommand(repoSHA, remoteURL, cmd)
		},
	}

	// Create tool registry and register tools
	registry := tools.NewRegistry(toolCtx)
	registry.Register(tools.NewReadFileTool(toolCtx))
	registry.Register(tools.NewEditFileTool(toolCtx))
	registry.Register(tools.NewGlobTool(toolCtx))
	registry.Register(tools.NewGrepTool(toolCtx))
	registry.Register(tools.NewRunCommandTool(toolCtx))

	// Build user prompt from errors
	userPrompt := buildUserPrompt(errRecords)

	// Create and run healing loop with merged config
	loopConfig := loop.ConfigFromSettings(cfg.Model, cfg.TimeoutMins, cfg.BudgetPerRunUSD, remainingMonthly)
	healLoop := loop.New(c.API(), registry, loopConfig)

	sentry.AddBreadcrumb("heal", "starting healing loop")
	sentry.SetTag("model", cfg.Model)
	sentry.SetTag("error_count", fmt.Sprintf("%d", len(errRecords)))

	fmt.Fprintf(os.Stderr, "%s Starting healing...\n", tui.Bullet())

	// Compute repo ID for spend tracking (non-critical, continue if fails)
	repoID, repoIDErr := persistence.ComputeRepoID(repoCtx.Path)
	if repoIDErr != nil {
		sentry.AddBreadcrumb("heal", fmt.Sprintf("repo ID computation failed: %v", repoIDErr))
		repoID = "" // spend tracking will use empty ID
	}

	result, err := healLoop.Run(cmd.Context(), prompt.BuildSystemPrompt(), userPrompt)
	if err != nil {
		// Record spend even on error (to global spend DB)
		if result != nil && result.CostUSD > 0 {
			_ = spendDB.RecordSpend(result.CostUSD, repoID)
		}
		sentry.CaptureError(err)
		return fmt.Errorf("healing loop failed: %w", err)
	}

	// Record spend after healing (to global spend DB)
	if result.CostUSD > 0 {
		if recordErr := spendDB.RecordSpend(result.CostUSD, repoID); recordErr != nil {
			fmt.Fprintf(os.Stderr, "%s Failed to record spend: %v\n", tui.WarningStyle.Render("!"), recordErr)
		}
	}

	// Display result
	fmt.Fprintf(os.Stderr, "\n")
	switch {
	case result.Success:
		fmt.Fprintf(os.Stderr, "%s Healing complete\n", tui.SuccessStyle.Render("✓"))
	case result.BudgetExceeded && result.BudgetExceededReason == "monthly":
		fmt.Fprintf(os.Stderr, "%s Monthly budget limit reached\n", tui.WarningStyle.Render("!"))
	case result.BudgetExceeded:
		fmt.Fprintf(os.Stderr, "%s Per-run budget limit reached\n", tui.WarningStyle.Render("!"))
	default:
		fmt.Fprintf(os.Stderr, "%s Healing incomplete\n", tui.ErrorStyle.Render("✗"))
	}

	fmt.Fprintf(os.Stderr, "%s %d iterations, %d tool calls, %s\n",
		tui.Bullet(),
		result.Iterations, result.ToolCalls, result.Duration.Round(100*1e6))

	fmt.Fprintf(os.Stderr, "%s $%.4f (%dk in, %dk out)\n",
		tui.Bullet(),
		result.CostUSD, result.InputTokens/1000, result.OutputTokens/1000)

	// Display monthly budget info if set
	if cfg.BudgetMonthlyUSD > 0 {
		fmt.Fprintf(os.Stderr, "%s Monthly: $%.2f / $%.2f\n",
			tui.Bullet(),
			monthlySpend+result.CostUSD, cfg.BudgetMonthlyUSD)
	}

	if result.FinalMessage != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n%s\n",
			tui.SecondaryStyle.Render("Summary"),
			result.FinalMessage)
	}

	if result.BudgetExceeded {
		if result.BudgetExceededReason == "monthly" {
			return fmt.Errorf("monthly budget limit ($%.2f) exceeded", cfg.BudgetMonthlyUSD)
		}
		return fmt.Errorf("per-run budget limit ($%.2f) exceeded", cfg.BudgetPerRunUSD)
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
		fmt.Fprintf(&sb, "## %s (%d errors)\n\n", filePath, len(fileErrors))

		for _, err := range fileErrors {
			// Format: [category] file:line:column: message
			fmt.Fprintf(&sb, "[%s] %s:%d:%d: %s\n",
				err.ErrorType, err.FilePath, err.LineNumber, err.ColumnNumber, err.Message)

			if err.RuleID != "" {
				fmt.Fprintf(&sb, "  Rule: %s | Source: %s\n", err.RuleID, err.Source)
			}

			if err.StackTrace != "" {
				sb.WriteString("  Stack trace:\n")
				lines := strings.Split(err.StackTrace, "\n")
				for _, line := range lines[:min(10, len(lines))] {
					fmt.Fprintf(&sb, "    %s\n", line)
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

// promptForCommand shows a TUI prompt for unknown commands.
func promptForCommand(cmd string) (tools.CommandApproval, error) {
	model := tui.NewCommandPromptModel(cmd)
	program := tea.NewProgram(model)

	if _, err := program.Run(); err != nil {
		return tools.CommandApproval{}, err
	}

	result := model.GetResult()
	if result == nil || result.Cancelled {
		return tools.CommandApproval{Allowed: false}, nil
	}

	if result.Allowed {
		if result.Always {
			fmt.Fprintf(os.Stderr, "%s Command saved to config\n\n", tui.Arrow())
		} else {
			fmt.Fprintf(os.Stderr, "%s Command allowed for this session\n\n", tui.Arrow())
		}
	}

	return tools.CommandApproval{
		Allowed: result.Allowed,
		Always:  result.Always,
	}, nil
}

