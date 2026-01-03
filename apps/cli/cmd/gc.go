package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/tui"
	"github.com/spf13/cobra"
)

const defaultRetentionDays = 30

var (
	gcRetentionDays int
	gcDryRun        bool
	gcAll           bool
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Clean up old run data based on retention policy",
	Long: `Remove old workflow runs and associated data based on retention policy.

By default, keeps data from the last 30 days. Only cleans the current repository
unless --all is specified.

Applied heals are always preserved for audit purposes.
Open errors are never deleted regardless of age.
The global spend log (spend.db) is never modified.`,
	RunE: runGC,
}

func init() {
	gcCmd.Flags().IntVarP(&gcRetentionDays, "retention", "r", defaultRetentionDays, "days to retain data")
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "show what would be deleted without deleting")
	gcCmd.Flags().BoolVarP(&gcAll, "all", "a", false, "clean all repositories in ~/.detent/repos/")
}

var runGC = func(cmd *cobra.Command, _ []string) error {
	start := time.Now()

	retentionDays := gcRetentionDays
	if retentionDays < 0 {
		return fmt.Errorf("retention days cannot be negative: %d", retentionDays)
	}
	if retentionDays == 0 {
		retentionDays = defaultRetentionDays
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	modeStr := ""
	if gcDryRun {
		modeStr = " (dry-run)"
	}
	fmt.Fprintf(os.Stderr, "%s Retention: %d days%s\n", tui.Bullet(), retentionDays, modeStr)
	fmt.Fprintf(os.Stderr, "%s Cutoff: %s\n\n", tui.Bullet(), cutoff.Format("2006-01-02 15:04:05"))

	var totalStats persistence.GCStats
	var dbsProcessed int
	var dbsFailed int

	if gcAll {
		dbs, err := persistence.ListRepoDatabases()
		if err != nil {
			return fmt.Errorf("listing databases: %w", err)
		}

		if len(dbs) == 0 {
			fmt.Fprintf(os.Stderr, "%s No databases found in ~/.detent/repos/\n\n",
				tui.SuccessStyle.Render("✓"))
			return nil
		}

		for _, dbPath := range dbs {
			stats, err := processDB(dbPath, retentionDays, gcDryRun)
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
	} else {
		repoRoot, err := filepath.Abs(".")
		if err != nil {
			return fmt.Errorf("resolving current directory: %w", err)
		}

		dbPath, err := persistence.GetDatabasePath(repoRoot)
		if err != nil {
			return fmt.Errorf("getting database path: %w", err)
		}

		if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "%s No database found for current repository\n\n",
				tui.SuccessStyle.Render("✓"))
			return nil
		}

		stats, err := processDB(dbPath, retentionDays, gcDryRun)
		if err != nil {
			return err
		}
		totalStats = *stats
		dbsProcessed = 1
	}

	duration := time.Since(start).Round(time.Millisecond)
	printGCResults(&totalStats, dbsProcessed, dbsFailed, duration, gcDryRun)

	return nil
}

var processDB = func(dbPath string, retentionDays int, dryRun bool) (*persistence.GCStats, error) {
	db, err := persistence.OpenDatabaseDirect(dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	return db.GarbageCollect(retentionDays, dryRun)
}

var printGCResults = func(stats *persistence.GCStats, dbCount, dbsFailed int, duration time.Duration, dryRun bool) {
	verb := "Deleted"
	if dryRun {
		verb = "Would delete"
	}

	hasChanges := stats.RunsDeleted > 0 || stats.ErrorsDeleted > 0 || stats.HealsDeleted > 0

	if hasChanges {
		fmt.Fprintf(os.Stderr, "%s %s:\n", tui.SuccessStyle.Render("✓"), verb)
		if stats.RunsDeleted > 0 {
			fmt.Fprintf(os.Stderr, "  %s %d run(s)\n", tui.Bullet(), stats.RunsDeleted)
		}
		if stats.RunErrorsDeleted > 0 {
			fmt.Fprintf(os.Stderr, "  %s %d run-error link(s)\n", tui.Bullet(), stats.RunErrorsDeleted)
		}
		if stats.ErrorLocationsDeleted > 0 {
			fmt.Fprintf(os.Stderr, "  %s %d error location(s)\n", tui.Bullet(), stats.ErrorLocationsDeleted)
		}
		if stats.HealsDeleted > 0 {
			fmt.Fprintf(os.Stderr, "  %s %d heal(s)\n", tui.Bullet(), stats.HealsDeleted)
		}
		if stats.ErrorsDeleted > 0 {
			fmt.Fprintf(os.Stderr, "  %s %d orphaned error(s)\n", tui.Bullet(), stats.ErrorsDeleted)
		}
	} else {
		fmt.Fprintf(os.Stderr, "%s No data older than retention period\n",
			tui.SuccessStyle.Render("✓"))
	}

	if dbCount > 1 {
		fmt.Fprintf(os.Stderr, "  %s across %d database(s)\n", tui.Bullet(), dbCount)
	}
	if dbsFailed > 0 {
		fmt.Fprintf(os.Stderr, "  %s %d database(s) failed\n", tui.WarningStyle.Render("!"), dbsFailed)
	}
	fmt.Fprintf(os.Stderr, "  %s completed in %s\n\n", tui.Bullet(), duration)
}
