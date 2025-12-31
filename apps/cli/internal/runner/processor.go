package runner

import (
	"strings"

	"github.com/detent/cli/internal/act"
	actparser "github.com/detent/cli/internal/ci/act"
	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/extract"
	"github.com/detent/cli/internal/tools"
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
	grouped := errors.GroupByFileWithBase(extracted, p.repoRoot)
	groupedComprehensive := errors.GroupComprehensive(extracted, p.repoRoot)

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
