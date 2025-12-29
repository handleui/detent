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
)

var allowCmd = &cobra.Command{
	Use:   "allow [command]",
	Short: "Manage allowed commands for current repo",
	Long: `Manage the list of commands that the AI can run without prompting.

Commands are stored in the global config (~/.detent/detent.json) and are
scoped to the current repository using its first commit SHA.

Wildcards are supported:
  detent allow "npm run *"     # Allow any npm run subcommand
  detent allow "bun test"      # Allow exact command`,
	Example: `  # Add a command to the allowlist
  detent allow "bun test"
  detent allow "npm run *"

  # List allowed commands for current repo
  detent allow --list

  # Remove a command from the allowlist
  detent allow --remove "bun test"`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          runAllow,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	allowCmd.Flags().BoolVarP(&listAllowed, "list", "l", false, "list allowed commands for current repo")
	allowCmd.Flags().BoolVarP(&removeAllowed, "remove", "r", false, "remove command from allowlist")
}

func runAllow(cmd *cobra.Command, args []string) error {
	// Resolve repo context to get first commit SHA
	repoCtx, err := repo.Resolve(repo.WithFirstCommit())
	if err != nil {
		return fmt.Errorf("resolving repo: %w", err)
	}

	repoSHA := repoCtx.FirstCommitSHA

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
		fmt.Fprintf(os.Stderr, "%s Removed: %s\n", tui.SuccessStyle.Render("-"), command)
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

	fmt.Fprintf(os.Stderr, "%s Added: %s\n", tui.SuccessStyle.Render("+"), command)
	return nil
}
