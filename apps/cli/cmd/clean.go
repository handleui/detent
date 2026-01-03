package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/tui"
	"github.com/detentsh/core/git"
	"github.com/spf13/cobra"
)

const defaultRetentionDays = 30

var (
	cleanForce         bool
	cleanAll           bool
	cleanRetentionDays int
	cleanDryRun        bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up orphaned worktrees and old run data",
	Long: `Remove orphaned worktrees and old workflow runs based on retention policy.

Worktree cleanup:
  Removes orphaned worktrees left behind by interrupted runs.
  Active worktrees (with running processes) are never removed.
  Use --force to remove unlocked worktrees regardless of age.

Data cleanup:
  Removes old workflow runs and associated data based on retention policy.
  By default, keeps data from the last 30 days.
  Applied heals are always preserved for audit purposes.
  Open errors are never deleted regardless of age.

By default, only cleans the current repository.
Use --all to clean orphaned worktrees and data from all repositories.`,
	RunE: runClean,
}

func init() {
	cleanCmd.Flags().BoolVarP(&cleanForce, "force", "f", false, "remove unlocked worktrees regardless of age")
	cleanCmd.Flags().BoolVarP(&cleanAll, "all", "a", false, "clean worktrees and data from all repositories")
	cleanCmd.Flags().IntVarP(&cleanRetentionDays, "retention", "r", defaultRetentionDays, "days to retain data")
	cleanCmd.Flags().BoolVar(&cleanDryRun, "dry-run", false, "show what would be deleted without deleting")
}

func runClean(cmd *cobra.Command, _ []string) error {
	start := time.Now()

	repoRoot, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	retentionDays := cleanRetentionDays
	if retentionDays < 0 {
		return fmt.Errorf("retention days cannot be negative: %d", retentionDays)
	}
	if retentionDays == 0 {
		retentionDays = defaultRetentionDays
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	modeStr := ""
	if cleanDryRun {
		modeStr = " (dry-run)"
	}
	fmt.Fprintf(os.Stderr, "%s Retention: %d days%s\n", tui.Bullet(), retentionDays, modeStr)
	fmt.Fprintf(os.Stderr, "%s Cutoff: %s\n\n", tui.Bullet(), cutoff.Format("2006-01-02 15:04:05"))

	// Phase 1: Worktree cleanup
	worktreesRemoved, worktreeErr := cleanWorktrees(cmd.Context(), repoRoot)

	// Phase 2: Data cleanup
	dataStats, dataErr := cleanData(repoRoot, retentionDays)

	// Print results
	duration := time.Since(start).Round(time.Millisecond)
	printCleanResults(worktreesRemoved, worktreeErr, dataStats, dataErr, duration)

	// Return first error if any
	if worktreeErr != nil {
		return worktreeErr
	}
	return dataErr
}

func cleanWorktrees(ctx context.Context, repoRoot string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, git.CleanupTimeout)
	defer cancel()

	// Prune git worktree metadata
	if pruneErr := git.PruneWorktreeMetadata(ctx, repoRoot); pruneErr != nil {
		fmt.Fprintf(os.Stderr, "%s Failed to prune metadata: %s\n",
			tui.WarningStyle.Render("!"),
			tui.MutedStyle.Render(pruneErr.Error()))
	}

	// Clean orphaned temp directories
	var removed int
	var err error
	if cleanAll {
		removed, err = git.CleanOrphanedTempDirs("", cleanForce)
	} else {
		removed, err = git.CleanOrphanedTempDirs(repoRoot, cleanForce)
	}
	if err != nil {
		return removed, fmt.Errorf("cleaning orphaned worktrees: %w", err)
	}

	return removed, nil
}

func cleanData(repoRoot string, retentionDays int) (*persistence.GCStats, error) {
	var totalStats persistence.GCStats
	var dbsProcessed int
	var dbsFailed int

	if cleanAll {
		dbs, err := persistence.ListRepoDatabases()
		if err != nil {
			return nil, fmt.Errorf("listing databases: %w", err)
		}

		for _, dbPath := range dbs {
			stats, err := processCleanDB(dbPath, retentionDays, cleanDryRun)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s: %s\n",
					tui.WarningStyle.Render("!"),
					filepath.Base(dbPath),
					tui.MutedStyle.Render(err.Error()))
				dbsFailed++
				continue
			}

			totalStats.RunsDeleted += stats.RunsDeleted
			totalStats.RunErrorsDeleted += stats.RunErrorsDeleted
			totalStats.ErrorLocationsDeleted += stats.ErrorLocationsDeleted
			totalStats.HealsDeleted += stats.HealsDeleted
			totalStats.ErrorsDeleted += stats.ErrorsDeleted
			dbsProcessed++
		}

		if dbsFailed > 0 && dbsProcessed == 0 {
			return nil, fmt.Errorf("all %d database(s) failed to process", dbsFailed)
		}
	} else {
		dbPath, err := persistence.GetDatabasePath(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("getting database path: %w", err)
		}

		if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
			return &totalStats, nil
		}

		stats, err := processCleanDB(dbPath, retentionDays, cleanDryRun)
		if err != nil {
			return nil, err
		}
		totalStats = *stats
	}

	return &totalStats, nil
}

var processCleanDB = func(dbPath string, retentionDays int, dryRun bool) (*persistence.GCStats, error) {
	db, err := persistence.OpenDatabaseDirect(dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	return db.GarbageCollect(retentionDays, dryRun)
}

func printCleanResults(worktreesRemoved int, worktreeErr error, dataStats *persistence.GCStats, dataErr error, duration time.Duration) {
	verb := "Deleted"
	if cleanDryRun {
		verb = "Would delete"
	}

	// Worktree results
	if worktreeErr == nil {
		if worktreesRemoved > 0 {
			fmt.Fprintf(os.Stderr, "%s Worktrees: %s %d orphaned worktree(s)\n",
				tui.SuccessStyle.Render("✓"),
				verb,
				worktreesRemoved)
		} else {
			fmt.Fprintf(os.Stderr, "%s Worktrees: no orphaned worktrees\n",
				tui.SuccessStyle.Render("✓"))
		}
	}

	// Data results
	if dataErr == nil && dataStats != nil {
		hasChanges := dataStats.RunsDeleted > 0 || dataStats.ErrorsDeleted > 0 || dataStats.HealsDeleted > 0

		if hasChanges {
			fmt.Fprintf(os.Stderr, "%s Data: %s:\n", tui.SuccessStyle.Render("✓"), verb)
			if dataStats.RunsDeleted > 0 {
				fmt.Fprintf(os.Stderr, "  %s %d run(s)\n", tui.Bullet(), dataStats.RunsDeleted)
			}
			if dataStats.RunErrorsDeleted > 0 {
				fmt.Fprintf(os.Stderr, "  %s %d run-error link(s)\n", tui.Bullet(), dataStats.RunErrorsDeleted)
			}
			if dataStats.ErrorLocationsDeleted > 0 {
				fmt.Fprintf(os.Stderr, "  %s %d error location(s)\n", tui.Bullet(), dataStats.ErrorLocationsDeleted)
			}
			if dataStats.HealsDeleted > 0 {
				fmt.Fprintf(os.Stderr, "  %s %d heal(s)\n", tui.Bullet(), dataStats.HealsDeleted)
			}
			if dataStats.ErrorsDeleted > 0 {
				fmt.Fprintf(os.Stderr, "  %s %d orphaned error(s)\n", tui.Bullet(), dataStats.ErrorsDeleted)
			}
		} else {
			fmt.Fprintf(os.Stderr, "%s Data: no data older than retention period\n",
				tui.SuccessStyle.Render("✓"))
		}
	}

	fmt.Fprintf(os.Stderr, "  %s completed in %s\n\n", tui.Bullet(), duration)
}
