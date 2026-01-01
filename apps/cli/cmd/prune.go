package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/detent/cli/internal/tui"
	"github.com/detentsh/core/git"
	"github.com/spf13/cobra"
)

var (
	pruneForce bool
	pruneAll   bool
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Clean up orphaned worktrees",
	Long: `Remove orphaned worktrees left behind by interrupted runs.

By default, only cleans worktrees belonging to the current repository.
Use --all to clean orphaned worktrees from all repositories.

Active worktrees (with running processes) are never removed.`,
	RunE: runPrune,
}

func init() {
	pruneCmd.Flags().BoolVarP(&pruneForce, "force", "f", false, "remove unlocked worktrees regardless of age")
	pruneCmd.Flags().BoolVarP(&pruneAll, "all", "a", false, "clean worktrees from all repositories")
}

func runPrune(cmd *cobra.Command, _ []string) error {
	start := time.Now()

	ctx, cancel := context.WithTimeout(cmd.Context(), git.CleanupTimeout)
	defer cancel()

	repoRoot, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	// Prune git worktree metadata
	if pruneErr := git.PruneWorktreeMetadata(ctx, repoRoot); pruneErr != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to prune metadata: %s\n",
			tui.WarningStyle.Render("!"),
			tui.MutedStyle.Render(pruneErr.Error()))
	}

	// Clean orphaned temp directories
	var removed int
	if pruneAll {
		removed, err = git.CleanOrphanedTempDirs("", pruneForce)
	} else {
		removed, err = git.CleanOrphanedTempDirs(repoRoot, pruneForce)
	}
	if err != nil {
		return fmt.Errorf("cleaning orphaned worktrees: %w", err)
	}

	duration := time.Since(start).Round(time.Millisecond)
	if removed > 0 {
		fmt.Fprintf(os.Stderr, "%s Cleaned %d worktree(s) in %s\n\n",
			tui.SuccessStyle.Render("✓"), removed, duration)
	} else {
		fmt.Fprintf(os.Stderr, "%s No orphaned worktrees in %s\n\n",
			tui.SuccessStyle.Render("✓"), duration)
	}

	return nil
}
