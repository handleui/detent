package cache

import (
	"fmt"
	"os"
	"time"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/util"
)

// CheckResult contains the result of a cache check
type CheckResult struct {
	Hit         bool
	RunID       string
	CommitSHA   string
	TotalErrors int
}

// Check checks if there's a cached run for the current commit SHA.
// If found, it displays the cached results.
// Returns (result, err) - err is non-nil only on fatal errors.
func Check(repoRoot, outputFormat string) (*CheckResult, error) {
	result := &CheckResult{Hit: false}

	// Get current commit SHA
	commitSHA, err := git.GetCurrentCommitSHA()
	if err != nil {
		return result, nil // Not a git repo or other issue, continue with fresh run
	}

	// Open database
	db, err := persistence.NewSQLiteWriter(repoRoot)
	if err != nil {
		return result, nil // No database yet, continue with fresh run
	}
	defer func() { _ = db.Close() }()

	// Check for prior run
	runID, found, err := db.GetRunByCommit(commitSHA)
	if err != nil || !found {
		return result, nil // No cached run, continue with fresh run
	}

	// Load run details
	run, err := db.GetRunByID(runID)
	if err != nil || run == nil {
		return result, nil // Failed to load run, continue with fresh run
	}

	// Load cached errors
	cachedErrors, err := db.GetErrorsByRunID(runID)
	if err != nil {
		return result, nil // Failed to load errors, continue with fresh run
	}

	// Display cached results
	displayResults(run, cachedErrors, outputFormat, repoRoot)

	result.Hit = true
	result.RunID = runID
	result.CommitSHA = commitSHA
	result.TotalErrors = run.TotalErrors

	// Return error if there were errors in the cached run
	if run.TotalErrors > 0 {
		return result, fmt.Errorf("found errors in cached workflow run")
	}

	return result, nil
}

// displayResults shows the cached run results to the user
func displayResults(run *persistence.RunRecord, cachedErrors []*persistence.ErrorRecord, outputFormat, repoRoot string) {
	// Calculate how long ago the run was
	ago := time.Since(run.CompletedAt)
	agoStr := util.FormatDuration(ago)

	// Safe commit SHA display (bounds check)
	displaySHA := run.CommitSHA
	if len(displaySHA) > 8 {
		displaySHA = displaySHA[:8]
	}

	// Print cache hit message
	_, _ = fmt.Fprintf(os.Stderr, "Using cached results from %s ago (commit %s)\n",
		agoStr, displaySHA)
	_, _ = fmt.Fprintf(os.Stderr, "\033[38;5;240mUse --force to run fresh.\033[0m\n\n")

	// Convert cached errors to grouped format for display
	extracted := convertToExtracted(cachedErrors)
	grouped := errors.GroupByFile(extracted)

	// Display based on output format
	switch outputFormat {
	case "json":
		_ = output.FormatJSON(os.Stdout, grouped)
	case "json-detailed":
		groupedDetailed := errors.GroupComprehensive(extracted, repoRoot)
		_ = output.FormatJSONDetailed(os.Stdout, groupedDetailed)
	default:
		output.FormatText(os.Stdout, grouped)
	}
}

// convertToExtracted converts cached errors to ExtractedError slice
func convertToExtracted(cachedErrors []*persistence.ErrorRecord) []*errors.ExtractedError {
	extracted := make([]*errors.ExtractedError, 0, len(cachedErrors))

	for _, e := range cachedErrors {
		ext := &errors.ExtractedError{
			File:       e.FilePath,
			Line:       e.LineNumber,
			Column:     e.ColumnNumber,
			Message:    e.Message,
			Category:   errors.ErrorCategory(e.ErrorType),
			StackTrace: e.StackTrace,
			RuleID:     e.RuleID,
			Source:     e.Source,
			Raw:        e.Raw,
		}

		// Use stored severity if available, otherwise infer
		if e.Severity != "" {
			ext.Severity = e.Severity
		} else {
			ext.Severity = errors.InferSeverity(ext)
		}

		// Reconstruct workflow context if job info is available
		if e.WorkflowJob != "" {
			ext.WorkflowContext = &errors.WorkflowContext{
				Job: e.WorkflowJob,
			}
		}

		extracted = append(extracted, ext)
	}

	return extracted
}
