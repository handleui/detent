package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/go-cli/internal/runner"
	"github.com/detent/go-cli/internal/signal"
	"github.com/detent/go-cli/internal/tui"
	"github.com/detent/go-cli/internal/update"
	"github.com/detentsh/core/agent"
	"github.com/detentsh/core/git"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	// Global flags shared across commands
	workflowsDir string
	workflowFile string
)

// agentInfo holds the detected AI agent environment info.
// Initialized once in PersistentPreRunE, available to all commands.
var agentInfo agent.Info

// StartTime holds the command start time for duration calculation.
// Set in PersistentPreRunE, used by commands to calculate elapsed time.
var StartTime time.Time

// trustedRepos tracks which repos have been trusted in this session.
// Maps first commit SHA to true if trusted.
var trustedRepos = make(map[string]bool)

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
		// Track command start time
		StartTime = time.Now()

		// Skip for config and update subcommands
		for c := cmd; c != nil; c = c.Parent() {
			if c == configCmd || c == updateCmd {
				return nil
			}
		}

		// Detect AI agent environment once (cached, safe to call multiple times)
		agentInfo = agent.Detect()

		// Check for updates (non-blocking, cached 24h) - show above header
		fmt.Println()
		if latest, hasUpdate := update.Check(Version); hasUpdate {
			fmt.Println(tui.UpdateAvailable(latest))
			fmt.Println()
		}

		// Branding header
		fmt.Println(tui.Header(Version, cmd.Name()))

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

func customHelpFunc(cmd *cobra.Command, _ []string) {
	// Only show custom help for root command
	if cmd != rootCmd {
		// Use default help for subcommands
		_ = cmd.UsageFunc()(cmd)
		return
	}

	fmt.Print(`Detent runs GitHub Actions locally to catch CI errors before pushing.
It uses act under the hood and provides structured error extraction,
grouping results by file for efficient debugging.

USAGE
  $ detent <command> [flags]

REQUIREMENTS
  docker:   Container runtime for running workflow steps
  act:      nektos/act - automatically invoked by detent

CORE COMMANDS
  detent check:       Run workflows locally and extract errors

  Pass --help to any command for specific help
  (e.g., detent check --help)

TYPICAL WORKFLOW
  1. Run detent check to see all CI errors locally
  2. Fix errors manually
  3. Re-run detent check to verify fixes
  4. Push with confidence

LEARN MORE
  GitHub:   https://github.com/handleui/detent
  Issues:   https://github.com/handleui/detent/issues

`)
}

func init() {
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(healCmd)
	rootCmd.AddCommand(frankensteinCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(allowCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(workflowsCmd)

	// Persistent flags available to all commands
	rootCmd.PersistentFlags().StringVarP(&workflowsDir, "workflows", "w", runner.WorkflowsDir, "workflows directory path")
	rootCmd.PersistentFlags().StringVar(&workflowFile, "workflow", "", "specific workflow file (e.g., ci.yml)")

	rootCmd.SetHelpFunc(customHelpFunc)
}

// ensureTrustedRepo checks if the current repository is trusted, prompts if not.
// Returns error if user declines trust, not in a git repo, or not interactive.
func ensureTrustedRepo() error {
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

	// Check if already trusted in this session
	if trustedRepos[firstCommitSHA] {
		return nil
	}

	// AI agent mode? Fail with clear instructions
	if agentInfo.IsAgent {
		return fmt.Errorf("repository not trusted\n\n" +
			"This repository has not been trusted by the user.\n" +
			"The user must run 'detent check' manually in a terminal to trust this repository.\n\n" +
			"Tell the user:\n" +
			"  1. Open a terminal\n" +
			"  2. Navigate to this repository\n" +
			"  3. Run: detent check\n" +
			"  4. Select 'Yes, trust this repository' when prompted")
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

	// Mark as trusted for this session
	trustedRepos[firstCommitSHA] = true

	fmt.Fprintf(os.Stderr, "%s Repository trusted\n\n", tui.SuccessStyle.Render("✓"))
	return nil
}
