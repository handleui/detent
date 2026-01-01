package cmd

import (
	"fmt"
	"os"

	"github.com/detent/cli/internal/repo"
	"github.com/detent/cli/internal/tui"
	"github.com/spf13/cobra"
)

var (
	listAllowed   bool
	removeAllowed bool
	allowJob      bool
)

var allowCmd = &cobra.Command{
	Use:   "allow [command|job-id]",
	Short: "Manage allowed commands and jobs for current repo",
	Long: `Manage the list of commands that the AI can run without prompting,
or sensitive jobs that should run with if: always().

Items are stored in the global config (~/.detent/detent.json) and are
scoped to the current repository using its first commit SHA.

Commands:
  Wildcards are supported for commands:
    detent allow "npm run *"     # Allow any npm run subcommand
    detent allow "bun test"      # Allow exact command

Jobs (with --job flag):
  Detent skips injecting if: always() on jobs that might publish, release,
  or deploy. Use --job to explicitly allow a sensitive job to run:
    detent allow --job release   # Allow the release job to run`,
	Example: `  # Add a command to the allowlist
  detent allow "bun test"
  detent allow "npm run *"

  # List allowed commands for current repo
  detent allow --list

  # Remove a command from the allowlist
  detent allow --remove "bun test"

  # Allow a sensitive job to run (jobs marked with 🔒)
  detent allow --job release
  detent allow --job deploy-staging

  # List allowed jobs
  detent allow --job --list

  # Remove a job from allowlist
  detent allow --job --remove release`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          runAllow,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	allowCmd.Flags().BoolVarP(&listAllowed, "list", "l", false, "list allowed items for current repo")
	allowCmd.Flags().BoolVarP(&removeAllowed, "remove", "r", false, "remove item from allowlist")
	allowCmd.Flags().BoolVarP(&allowJob, "job", "j", false, "manage sensitive jobs instead of commands")
}

func runAllow(cmd *cobra.Command, args []string) error {
	// Resolve repo context to get first commit SHA
	repoCtx, err := repo.Resolve(repo.WithFirstCommit())
	if err != nil {
		return fmt.Errorf("resolving repo: %w", err)
	}

	repoSHA := repoCtx.FirstCommitSHA

	if allowJob {
		return runAllowJob(repoSHA, args)
	}

	return runAllowCommand(repoSHA, args)
}

func runAllowJob(repoSHA string, args []string) error {
	// Handle --list flag
	if listAllowed {
		jobs := cfg.GetAllowedSensitiveJobs(repoSHA)
		if len(jobs) == 0 {
			fmt.Fprintf(os.Stderr, "%s No allowed sensitive jobs for this repo\n", tui.MutedStyle.Render("i"))
			return nil
		}

		fmt.Fprintf(os.Stderr, "%s Allowed sensitive jobs:\n", tui.Bullet())
		for _, j := range jobs {
			fmt.Fprintf(os.Stderr, "  %s\n", j)
		}
		return nil
	}

	// Require a job ID argument for add/remove
	if len(args) == 0 {
		return fmt.Errorf("job ID argument required (use --list to view)")
	}

	jobID := args[0]

	// Handle --remove flag
	if removeAllowed {
		if err := cfg.RemoveAllowedSensitiveJob(repoSHA, jobID); err != nil {
			return fmt.Errorf("removing job: %w", err)
		}
		fmt.Fprintln(os.Stderr, tui.ExitSuccess(fmt.Sprintf("Removed %q from allowed jobs", jobID)))
		return nil
	}

	// Default: add job
	// Check if already exists
	if cfg.IsSensitiveJobAllowed(repoSHA, jobID) {
		fmt.Fprintf(os.Stderr, "%s Job already allowed: %s\n", tui.MutedStyle.Render("i"), jobID)
		return nil
	}

	if err := cfg.AddAllowedSensitiveJob(repoSHA, jobID); err != nil {
		return fmt.Errorf("adding job: %w", err)
	}

	fmt.Fprintln(os.Stderr, tui.ExitSuccess(fmt.Sprintf("Added %q to allowed jobs", jobID)))
	return nil
}

func runAllowCommand(repoSHA string, args []string) error {
	// Handle --list flag
	if listAllowed {
		commands := cfg.GetAllowedCommands(repoSHA)
		if len(commands) == 0 {
			fmt.Fprintf(os.Stderr, "%s No allowed commands for this repo\n", tui.MutedStyle.Render("i"))
			return nil
		}

		fmt.Fprintf(os.Stderr, "%s Allowed commands:\n", tui.Bullet())
		for _, c := range commands {
			fmt.Fprintf(os.Stderr, "  %s\n", c)
		}
		return nil
	}

	// Require a command argument for add/remove
	if len(args) == 0 {
		return fmt.Errorf("command argument required (use --list to view)")
	}

	command := args[0]

	// Handle --remove flag
	if removeAllowed {
		if err := cfg.RemoveAllowedCommand(repoSHA, command); err != nil {
			return fmt.Errorf("removing command: %w", err)
		}
		fmt.Fprintln(os.Stderr, tui.ExitSuccess(fmt.Sprintf("Removed %q from allowlist", command)))
		return nil
	}

	// Default: add command
	// Check if already exists
	if cfg.MatchesCommand(repoSHA, command) {
		fmt.Fprintf(os.Stderr, "%s Command already allowed: %s\n", tui.MutedStyle.Render("i"), command)
		return nil
	}

	if err := cfg.AddAllowedCommand(repoSHA, command); err != nil {
		return fmt.Errorf("adding command: %w", err)
	}

	fmt.Fprintln(os.Stderr, tui.ExitSuccess(fmt.Sprintf("Added %q to allowlist", command)))
	return nil
}
