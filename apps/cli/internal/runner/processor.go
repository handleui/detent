package runner

import (
	"strings"
	"time"

	"github.com/detentsh/core/act"
	actparser "github.com/detentsh/core/ci/act"
	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/extract"
	"github.com/detentsh/core/tools"
)

// ErrorProcessor handles extraction and processing of errors from act output.
// It uses the registry-based extractor with tool-specific parsers and provides
// both simple (by-file) and comprehensive (by-category) error groupings.
type ErrorProcessor struct {
	repoRoot string
	registry *tools.Registry
}

// ProcessedErrors contains the results of error processing.
type ProcessedErrors struct {
	Extracted            []*errors.ExtractedError
	Grouped              *errors.GroupedErrors
	GroupedComprehensive *errors.ComprehensiveErrorGroup
}

// NewErrorProcessor creates a new ErrorProcessor with the given repository root.
func NewErrorProcessor(repoRoot string) *ErrorProcessor {
	return &ErrorProcessor{
		repoRoot: repoRoot,
		registry: tools.DefaultRegistry(),
	}
}

// NewErrorProcessorWithRegistry creates a new ErrorProcessor with a custom registry.
// This is useful for testing with mock registries.
func NewErrorProcessorWithRegistry(repoRoot string, registry *tools.Registry) *ErrorProcessor {
	return &ErrorProcessor{
		repoRoot: repoRoot,
		registry: registry,
	}
}

// Process extracts errors from act output, applies severity, and groups them.
// Returns extracted errors and both simple (by-file) and comprehensive (by-category) groupings.
func (p *ErrorProcessor) Process(actResult *act.RunResult) *ProcessedErrors {
	var combinedOutput strings.Builder
	combinedOutput.Grow(len(actResult.Stdout) + len(actResult.Stderr))
	combinedOutput.WriteString(actResult.Stdout)
	combinedOutput.WriteString(actResult.Stderr)

	// Use the registry-based extractor with tool-specific parsers
	extractor := extract.NewExtractor(p.registry)
	extracted := extractor.Extract(combinedOutput.String(), actparser.NewContextParser())

	// Report any unknown patterns to Sentry for analysis
	extract.ReportUnknownPatterns(extracted)

	errors.ApplySeverity(extracted)

	// Extract source code snippets for AI consumption
	snippetsSucceeded, snippetsFailed := errors.ExtractSnippetsForErrors(extracted, p.repoRoot)

	grouped := errors.GroupByFileWithBase(extracted, p.repoRoot)
	groupedComprehensive := errors.GroupComprehensive(extracted, p.repoRoot)

	// Build AI context metadata
	errorsWithLocation := 0
	errorsWithRuleID := 0
	for _, err := range extracted {
		if err.File != "" && err.Line > 0 {
			errorsWithLocation++
		}
		if err.RuleID != "" {
			errorsWithRuleID++
		}
	}

	groupedComprehensive.AIContext = &errors.AIContext{
		ExtractedAt:        time.Now().UTC().Format(time.RFC3339),
		SnippetsIncluded:   true,
		SnippetsFailed:     snippetsFailed,
		ErrorsWithLocation: errorsWithLocation,
		ErrorsWithSnippet:  snippetsSucceeded,
		ErrorsWithRuleID:   errorsWithRuleID,
	}

	return &ProcessedErrors{
		Extracted:            extracted,
		Grouped:              grouped,
		GroupedComprehensive: groupedComprehensive,
	}
}

// ProcessOutput extracts errors from raw output strings.
// This is a convenience method when you have stdout and stderr separately.
func (p *ErrorProcessor) ProcessOutput(stdout, stderr string) *ProcessedErrors {
	return p.Process(&act.RunResult{
		Stdout: stdout,
		Stderr: stderr,
	})
}
