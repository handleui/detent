package prompt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/detent/cli/internal/errors"
)

// MaxAttempts is the number of fix attempts before giving up.
// Research: 2 attempts with verification produces best results.
const MaxAttempts = 2

// FilePrompt contains a generated prompt for fixing errors in a single file.
type FilePrompt struct {
	FilePath        string
	Prompt          string
	ErrorCount      int
	Priority        int    // Lower = higher priority (compile=1, lint=5)
	WorkflowContext string // CI job context
}

// AttemptContext provides context about previous fix attempts.
type AttemptContext struct {
	Attempt         int      // Current attempt number (1 or 2)
	PreviousErrors  []string // Error messages from previous attempt
	PreviousFailed  bool     // Whether previous attempt failed
	FailureReason   string   // Why previous attempt failed (if known)
}

// BuildSystemPrompt returns the system context for Claude Code.
func BuildSystemPrompt() string {
	return SystemPrompt
}

// BuildFilePrompt generates a prompt for fixing all errors in a single file.
// Includes full diagnostic context: stack traces, rule IDs, workflow context.
func BuildFilePrompt(filePath string, errs []*errors.ExtractedError) *FilePrompt {
	errorCount, warningCount := CountErrors(errs)
	workflowContext := FormatWorkflowContext(errs)

	var header strings.Builder
	header.WriteString(filePath)

	// Error/warning summary
	switch {
	case errorCount > 0 && warningCount > 0:
		header.WriteString(fmt.Sprintf(" (%d errors, %d warnings)", errorCount, warningCount))
	case errorCount > 0:
		header.WriteString(fmt.Sprintf(" (%d errors)", errorCount))
	default:
		header.WriteString(fmt.Sprintf(" (%d warnings)", warningCount))
	}

	// Build prompt with full context
	var prompt strings.Builder
	prompt.WriteString(header.String())
	prompt.WriteString("\n\n")

	// Include workflow context if available
	if workflowContext != "" {
		prompt.WriteString(workflowContext)
		prompt.WriteString("\n\n")
	}

	// Formatted errors with stack traces
	prompt.WriteString(FormatErrors(errs))

	return &FilePrompt{
		FilePath:        filePath,
		Prompt:          prompt.String(),
		ErrorCount:      len(errs),
		Priority:        getHighestPriority(errs),
		WorkflowContext: workflowContext,
	}
}

// BuildFilePromptWithAttempt generates a prompt with iteration context.
// Use this for attempt 2+ when previous fix didn't work.
func BuildFilePromptWithAttempt(filePath string, errs []*errors.ExtractedError, attempt *AttemptContext) *FilePrompt {
	base := BuildFilePrompt(filePath, errs)

	if attempt == nil || attempt.Attempt <= 1 {
		return base
	}

	// Add attempt context for retry
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("ATTEMPT %d of %d\n", attempt.Attempt, MaxAttempts))

	if attempt.PreviousFailed {
		prompt.WriteString("Previous fix did not resolve all errors.\n")
		if attempt.FailureReason != "" {
			prompt.WriteString(fmt.Sprintf("Failure reason: %s\n", attempt.FailureReason))
		}
		if len(attempt.PreviousErrors) > 0 {
			prompt.WriteString("Previous errors that persisted:\n")
			for _, e := range attempt.PreviousErrors {
				prompt.WriteString(fmt.Sprintf("  - %s\n", e))
			}
		}
		prompt.WriteString("\nTry a different approach this time.\n")
	}

	prompt.WriteString("\n")
	prompt.WriteString(base.Prompt)

	base.Prompt = prompt.String()
	return base
}

// BuildAllFilePrompts generates prompts for all files in a ComprehensiveErrorGroup.
// Files with compile/type errors are prioritized first.
func BuildAllFilePrompts(group *errors.ComprehensiveErrorGroup) []*FilePrompt {
	// P0: Nil check
	if group == nil || group.ByFile == nil {
		return nil
	}

	prompts := make([]*FilePrompt, 0, len(group.ByFile))

	for filePath, errs := range group.ByFile {
		// P0: Skip empty error slices
		if len(errs) == 0 {
			continue
		}
		prompts = append(prompts, BuildFilePrompt(filePath, errs))
	}

	sortPromptsByPriority(prompts)
	return prompts
}

// sortPromptsByPriority sorts file prompts by error priority using O(n log n) sort.
func sortPromptsByPriority(prompts []*FilePrompt) {
	sort.Slice(prompts, func(i, j int) bool {
		return prompts[i].Priority < prompts[j].Priority
	})
}

// getHighestPriority returns the highest (lowest number) priority category in errors.
func getHighestPriority(errs []*errors.ExtractedError) int {
	highest := DefaultPriority
	for _, err := range errs {
		// P0: Skip nil errors
		if err == nil {
			continue
		}
		if p := getPriority(err.Category); p < highest {
			highest = p
		}
	}
	return highest
}

// BuildBatchPrompt generates a single prompt for multiple files.
// Use when total errors are few to reduce API calls.
func BuildBatchPrompt(prompts []*FilePrompt) string {
	if len(prompts) == 0 {
		return ""
	}
	if len(prompts) == 1 {
		return prompts[0].Prompt
	}

	parts := make([]string, len(prompts))
	for i, p := range prompts {
		parts[i] = p.Prompt
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// Summary generates a summary for logging/display.
func Summary(group *errors.ComprehensiveErrorGroup) string {
	catCounts := make([]string, 0)
	for cat, count := range group.Stats.ByCategory {
		if count > 0 {
			catCounts = append(catCounts, fmt.Sprintf("%s:%d", cat, count))
		}
	}

	return fmt.Sprintf("%d errors, %d warnings in %d files [%s]",
		group.Stats.ErrorCount,
		group.Stats.WarningCount,
		len(group.ByFile),
		strings.Join(catCounts, ", "),
	)
}

// VerificationResult represents the outcome of running CI after a fix.
type VerificationResult struct {
	Success      bool     // All errors fixed
	ErrorsFixed  int      // Number of errors that were fixed
	ErrorsRemain int      // Number of errors that still exist
	NewErrors    []string // New errors introduced by the fix
}

// CanRetry determines if another fix attempt should be made.
func (v *VerificationResult) CanRetry(currentAttempt int) bool {
	if v.Success {
		return false
	}
	if currentAttempt >= MaxAttempts {
		return false
	}
	// Retry if we made progress (fixed some errors) or if no new errors were introduced
	return v.ErrorsFixed > 0 || len(v.NewErrors) == 0
}
