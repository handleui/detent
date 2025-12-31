package workflow

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/detent/cli/internal/tools"
)

// ValidationSeverity indicates how critical a validation issue is.
type ValidationSeverity int

const (
	// SeverityError indicates a feature that will definitely not work.
	SeverityError ValidationSeverity = iota
	// SeverityWarning indicates a feature that may not work correctly or is ignored.
	SeverityWarning
)

// ValidationError represents an unsupported feature detected in a workflow.
type ValidationError struct {
	Feature     string             // The unsupported feature name
	Description string             // Human-readable description of the issue
	Suggestion  string             // Actionable suggestion to fix the issue
	JobID       string             // Job ID where the issue was found (empty for workflow-level issues)
	StepName    string             // Step name where the issue was found (empty for job-level issues)
	Severity    ValidationSeverity // How critical this issue is
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	location := ""
	if e.JobID != "" {
		location = fmt.Sprintf(" (job: %s", e.JobID)
		if e.StepName != "" {
			location += fmt.Sprintf(", step: %s", e.StepName)
		}
		location += ")"
	}

	severityPrefix := ""
	if e.Severity == SeverityWarning {
		severityPrefix = "[warning] "
	}

	msg := fmt.Sprintf("%sunsupported feature %q%s: %s", severityPrefix, e.Feature, location, e.Description)
	if e.Suggestion != "" {
		msg += ". " + e.Suggestion
	}
	return msg
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []*ValidationError

// Error implements the error interface.
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d unsupported features detected:\n", len(e)))
	for _, err := range e {
		sb.WriteString("  - ")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// HasErrors returns true if any validation errors have SeverityError.
func (e ValidationErrors) HasErrors() bool {
	for _, err := range e {
		if err.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors returns only the validation errors with SeverityError.
func (e ValidationErrors) Errors() ValidationErrors {
	var errors ValidationErrors
	for _, err := range e {
		if err.Severity == SeverityError {
			errors = append(errors, err)
		}
	}
	return errors
}

// Warnings returns only the validation errors with SeverityWarning.
func (e ValidationErrors) Warnings() ValidationErrors {
	var warnings ValidationErrors
	for _, err := range e {
		if err.Severity == SeverityWarning {
			warnings = append(warnings, err)
		}
	}
	return warnings
}

// SupportedRunsOn contains the list of supported runs-on values.
// act only supports Linux-based runners via Docker containers.
var SupportedRunsOn = map[string]bool{
	"ubuntu-latest": true,
	"ubuntu-24.04":  true,
	"ubuntu-22.04":  true,
	"ubuntu-20.04":  true,
}

// UnsupportedRunnerPatterns defines patterns for runners that are not supported.
// Each pattern maps to a descriptive message for the error.
var UnsupportedRunnerPatterns = map[string]string{
	"macos":       "macOS runners require a macOS host and are not supported in Docker-based execution",
	"windows":     "Windows runners require a Windows host and are not supported in Docker-based execution",
	"self-hosted": "self-hosted runners are not supported; use ubuntu-latest or a specific Ubuntu version",
}

// LargeRunnerPatterns identifies GitHub-hosted large runners which are not supported.
var LargeRunnerPatterns = []string{
	"-large",
	"-xlarge",
	"-2xlarge",
	"-4xlarge",
	"-8xlarge",
	"-16xlarge",
}

// reusableWorkflowPattern matches reusable workflow references.
// Examples: "./.github/workflows/reusable.yml", "org/repo/.github/workflows/reusable.yml@main"
var reusableWorkflowPattern = regexp.MustCompile(`^\.?/\.github/workflows/|^[^/]+/[^/]+/\.github/workflows/`)

// oidcTokenPattern matches OIDC token usage in expressions.
var oidcTokenPattern = regexp.MustCompile(`\$\{\{\s*secrets\.ACTIONS_ID_TOKEN_REQUEST`)

// ValidateWorkflow checks a workflow for unsupported features.
// Returns nil if the workflow is fully supported, otherwise returns ValidationErrors.
// The returned ValidationErrors may contain both errors and warnings.
// Use ValidationErrors.HasErrors() to check if there are blocking issues.
func ValidateWorkflow(wf *Workflow) error {
	if wf == nil || wf.Jobs == nil {
		return nil
	}

	var errors ValidationErrors

	// Workflow-level validations
	errors = append(errors, validateWorkflowLevel(wf)...)

	for jobID, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Check runs-on
		errors = append(errors, validateRunsOn(jobID, job.RunsOn)...)

		// Check services
		if job.Services != nil {
			errors = append(errors, &ValidationError{
				Feature:     "services",
				Description: "service containers have limited support in act",
				Suggestion:  "Services may not work correctly; consider using docker-compose for complex service dependencies",
				JobID:       jobID,
				Severity:    SeverityWarning,
			})
		}

		// Check job.environment (deployment environments)
		if job.Environment != nil {
			errors = append(errors, validateEnvironment(jobID, job.Environment)...)
		}

		// Check job-level reusable workflow (uses: at job level)
		if job.Uses != "" {
			errors = append(errors, validateJobUsesWorkflow(jobID, job.Uses)...)
		}

		// Check container with complex options
		if containerErrs := validateContainer(jobID, job.Container); len(containerErrs) > 0 {
			errors = append(errors, containerErrs...)
		}

		// Check for matrix with complex expressions
		if strategyErrs := validateStrategy(jobID, job.Strategy); len(strategyErrs) > 0 {
			errors = append(errors, strategyErrs...)
		}

		// Check steps for reusable workflows, OIDC tokens, and other issues
		errors = append(errors, validateSteps(jobID, job.Steps)...)
	}

	if len(errors) == 0 {
		return nil
	}
	return errors
}

// validateWorkflowLevel checks workflow-level features.
func validateWorkflowLevel(wf *Workflow) ValidationErrors {
	var errors ValidationErrors

	// Check for workflow_call trigger (reusable workflow definition)
	if on, ok := wf.On.(map[string]any); ok {
		if _, hasWorkflowCall := on["workflow_call"]; hasWorkflowCall {
			errors = append(errors, &ValidationError{
				Feature:     "workflow_call",
				Description: "reusable workflow definitions (workflow_call trigger) are not supported",
				Suggestion:  "Inline the reusable workflow steps directly into the calling workflow",
				Severity:    SeverityError,
			})
		}
	}

	return errors
}

// validateRunsOn checks if the runs-on value is supported.
func validateRunsOn(jobID string, runsOn any) ValidationErrors {
	var errors ValidationErrors

	switch v := runsOn.(type) {
	case string:
		if err := checkRunner(jobID, v); err != nil {
			errors = append(errors, err)
		}
	case []any:
		// Matrix-style runs-on with labels (e.g., [self-hosted, linux])
		for _, item := range v {
			if label, ok := item.(string); ok {
				if err := checkRunnerLabel(jobID, label); err != nil {
					errors = append(errors, err)
				}
			}
		}
	case map[string]any:
		// Object-style runs-on with group/labels
		if group, ok := v["group"].(string); ok {
			errors = append(errors, &ValidationError{
				Feature:     "runs-on",
				Description: fmt.Sprintf("runner groups (%q) are not supported", group),
				Suggestion:  "Use ubuntu-latest or a specific Ubuntu version instead",
				JobID:       jobID,
				Severity:    SeverityError,
			})
		}
		if labels, ok := v["labels"].([]any); ok {
			for _, item := range labels {
				if label, ok := item.(string); ok {
					if err := checkRunnerLabel(jobID, label); err != nil {
						errors = append(errors, err)
					}
				}
			}
		}
	}

	return errors
}

// checkRunner validates a single runner string value.
func checkRunner(jobID, runner string) *ValidationError {
	// Handle matrix expressions - can't validate at parse time
	if strings.Contains(runner, "${{") {
		return nil
	}

	// Check for large runners first
	lowerRunner := strings.ToLower(runner)
	for _, pattern := range LargeRunnerPatterns {
		if strings.Contains(lowerRunner, pattern) {
			return &ValidationError{
				Feature:     "runs-on",
				Description: fmt.Sprintf("large runner %q is not supported", runner),
				Suggestion:  "Use ubuntu-latest or a specific Ubuntu version (ubuntu-22.04, ubuntu-24.04)",
				JobID:       jobID,
				Severity:    SeverityError,
			}
		}
	}

	// Check for unsupported runner patterns (macOS, Windows, self-hosted)
	for pattern, description := range UnsupportedRunnerPatterns {
		if strings.Contains(lowerRunner, pattern) {
			return &ValidationError{
				Feature:     "runs-on",
				Description: fmt.Sprintf("%q: %s", runner, description),
				Suggestion:  "Use ubuntu-latest or a specific Ubuntu version (ubuntu-22.04, ubuntu-24.04)",
				JobID:       jobID,
				Severity:    SeverityError,
			}
		}
	}

	// Check if it's in our supported list
	if !SupportedRunsOn[runner] {
		return &ValidationError{
			Feature:     "runs-on",
			Description: fmt.Sprintf("%q is not a recognized runner", runner),
			Suggestion:  "Use ubuntu-latest, ubuntu-24.04, ubuntu-22.04, or ubuntu-20.04",
			JobID:       jobID,
			Severity:    SeverityError,
		}
	}

	return nil
}

// checkRunnerLabel validates a single runner label (used in array syntax).
func checkRunnerLabel(jobID, label string) *ValidationError {
	lowerLabel := strings.ToLower(label)

	// Self-hosted label is a blocking error
	if lowerLabel == "self-hosted" {
		return &ValidationError{
			Feature:     "runs-on",
			Description: "self-hosted runners are not supported",
			Suggestion:  "Use ubuntu-latest or a specific Ubuntu version instead of self-hosted runners",
			JobID:       jobID,
			Severity:    SeverityError,
		}
	}

	// Check for unsupported OS patterns
	for pattern, description := range UnsupportedRunnerPatterns {
		if pattern != "self-hosted" && strings.Contains(lowerLabel, pattern) {
			return &ValidationError{
				Feature:     "runs-on",
				Description: fmt.Sprintf("label %q: %s", label, description),
				Suggestion:  "Use ubuntu-latest or a specific Ubuntu version",
				JobID:       jobID,
				Severity:    SeverityError,
			}
		}
	}

	// Common self-hosted labels (linux, x64, arm64) are warnings, not errors
	// They're often used with self-hosted which we already catch
	if isCommonSelfHostedLabel(label) {
		return nil
	}

	// If not in supported list and not a known label pattern, it might be custom
	if !SupportedRunsOn[label] {
		return &ValidationError{
			Feature:     "runs-on",
			Description: fmt.Sprintf("custom label %q may not be supported", label),
			Suggestion:  "Ensure this label maps to a supported Ubuntu runner",
			JobID:       jobID,
			Severity:    SeverityWarning,
		}
	}

	return nil
}

// isCommonSelfHostedLabel checks if a label is a common self-hosted runner label.
// These labels are often used with self-hosted runners and are not errors on their own.
func isCommonSelfHostedLabel(label string) bool {
	commonLabels := map[string]bool{
		"linux": true,
		"x64":   true,
		"arm64": true,
		"arm":   true,
	}
	return commonLabels[strings.ToLower(label)]
}

// validateEnvironment checks if deployment environments are used.
func validateEnvironment(jobID string, environment any) ValidationErrors {
	var errors ValidationErrors

	if environment == nil {
		return errors
	}

	var envName string
	switch v := environment.(type) {
	case string:
		envName = v
	case map[string]any:
		if name, ok := v["name"].(string); ok {
			envName = name
		}
	}

	if envName != "" {
		errors = append(errors, &ValidationError{
			Feature:     "environment",
			Description: fmt.Sprintf("deployment environment %q is not supported", envName),
			Suggestion:  "Environment-scoped secrets and variables will not be available; use repository-level secrets instead",
			JobID:       jobID,
			Severity:    SeverityWarning,
		})
	}

	return errors
}

// validateJobUsesWorkflow checks if a job uses a reusable workflow (job-level uses:).
func validateJobUsesWorkflow(jobID, uses string) ValidationErrors {
	var errors ValidationErrors

	// Check if it's a reusable workflow reference
	if reusableWorkflowPattern.MatchString(uses) {
		errors = append(errors, &ValidationError{
			Feature:     "reusable-workflow",
			Description: fmt.Sprintf("reusable workflow %q is not supported", uses),
			Suggestion:  "Inline the reusable workflow steps directly into this job",
			JobID:       jobID,
			Severity:    SeverityError,
		})
	}

	return errors
}

// validateContainer checks if the container configuration is supported.
func validateContainer(jobID string, container any) ValidationErrors {
	if container == nil {
		return nil
	}

	var errors ValidationErrors

	switch v := container.(type) {
	case string:
		// Simple container image string is supported
		return nil
	case map[string]any:
		// Check for credentials (requires authentication)
		if _, ok := v["credentials"]; ok {
			errors = append(errors, &ValidationError{
				Feature:     "container",
				Description: "container credentials may not work correctly",
				Suggestion:  "Authenticate with your container registry before running act, or use public images",
				JobID:       jobID,
				Severity:    SeverityWarning,
			})
		}

		// Check for volumes
		if _, ok := v["volumes"]; ok {
			errors = append(errors, &ValidationError{
				Feature:     "container",
				Description: "container volumes have limited support",
				Suggestion:  "Volume mounts may behave differently in act; test carefully",
				JobID:       jobID,
				Severity:    SeverityWarning,
			})
		}

		// Check for specific docker options that don't work well
		if opts, ok := v["options"].(string); ok {
			if strings.Contains(opts, "--network=host") {
				errors = append(errors, &ValidationError{
					Feature:     "container",
					Description: "--network=host option is not supported",
					Suggestion:  "Remove --network=host; use Docker's default bridge networking",
					JobID:       jobID,
					Severity:    SeverityError,
				})
			}
			if strings.Contains(opts, "--privileged") {
				errors = append(errors, &ValidationError{
					Feature:     "container",
					Description: "--privileged option may cause issues",
					Suggestion:  "Avoid --privileged if possible; it may conflict with act's container management",
					JobID:       jobID,
					Severity:    SeverityWarning,
				})
			}
		}
	}

	return errors
}

// validateStrategy checks if the strategy/matrix configuration is supported.
func validateStrategy(jobID string, strategy any) ValidationErrors {
	if strategy == nil {
		return nil
	}

	var errors ValidationErrors

	strategyMap, ok := strategy.(map[string]any)
	if !ok {
		return nil
	}

	matrix, ok := strategyMap["matrix"]
	if !ok {
		return nil
	}

	matrixMap, ok := matrix.(map[string]any)
	if !ok {
		return nil
	}

	// Check for complex expressions in matrix
	for key, value := range matrixMap {
		if str, ok := value.(string); ok {
			if strings.Contains(str, "${{") && strings.Contains(str, "fromJSON") {
				errors = append(errors, &ValidationError{
					Feature:     "matrix",
					Description: fmt.Sprintf("dynamic matrix with fromJSON expression in %q has limited support", key),
					Suggestion:  "Consider defining matrix values statically, or ensure the expression evaluates correctly in act",
					JobID:       jobID,
					Severity:    SeverityWarning,
				})
			}
		}
	}

	return errors
}

// toolRegistry is the default tool parser registry used for tool detection.
// Initialized lazily to avoid import cycles.
var toolRegistry *tools.Registry

func getToolRegistry() *tools.Registry {
	if toolRegistry == nil {
		toolRegistry = tools.DefaultRegistry()
	}
	return toolRegistry
}

// validateSteps checks steps for unsupported features.
func validateSteps(jobID string, steps []*Step) ValidationErrors {
	var errors ValidationErrors
	registry := getToolRegistry()
	supportedTools := registry.SupportedToolIDs()

	// Track unsupported tools for Sentry reporting
	var unsupportedToolsForSentry []tools.UnsupportedToolInfo

	for _, step := range steps {
		if step == nil {
			continue
		}

		stepName := step.Name
		if stepName == "" && step.ID != "" {
			stepName = step.ID
		}
		if stepName == "" && step.Uses != "" {
			stepName = step.Uses
		}

		// Check for reusable workflows
		if step.Uses != "" && reusableWorkflowPattern.MatchString(step.Uses) {
			errors = append(errors, &ValidationError{
				Feature:     "reusable-workflow",
				Description: fmt.Sprintf("reusable workflow %q is not supported", step.Uses),
				Suggestion:  "Inline the reusable workflow steps directly, or call the action separately",
				JobID:       jobID,
				StepName:    stepName,
				Severity:    SeverityError,
			})
		}

		// Check for OIDC token usage in step expressions
		errors = append(errors, checkOIDCUsage(jobID, stepName, step)...)

		// Check for unsupported tools in run commands (detect all tools, not just first)
		if step.Run != "" {
			result := registry.DetectTools(step.Run, tools.DetectionOptions{CheckSupport: true})
			unsupportedTools := result.Unsupported()

			// Track for Sentry reporting
			for _, t := range unsupportedTools {
				unsupportedToolsForSentry = append(unsupportedToolsForSentry, tools.UnsupportedToolInfo{
					ToolID:      t.ID,
					DisplayName: t.DisplayName,
					StepName:    stepName,
					JobID:       jobID,
				})
			}

			// Generate warning for unsupported tools
			if len(unsupportedTools) > 0 {
				warningMsg := tools.FormatUnsupportedToolsWarning(unsupportedTools, supportedTools)
				errors = append(errors, &ValidationError{
					Feature:     "tool-parsing",
					Description: warningMsg,
					Suggestion:  "Errors will be captured but file paths and line numbers may not be extracted accurately",
					JobID:       jobID,
					StepName:    stepName,
					Severity:    SeverityWarning,
				})
			}
		}
	}

	// Report unsupported tools to Sentry for monitoring (helps prioritize parser development)
	if len(unsupportedToolsForSentry) > 0 {
		tools.ReportUnsupportedTools(unsupportedToolsForSentry)
	}

	return errors
}

// checkOIDCUsage checks if a step uses OIDC tokens.
func checkOIDCUsage(jobID, stepName string, step *Step) ValidationErrors {
	var errors ValidationErrors

	// Check in run commands
	if step.Run != "" && oidcTokenPattern.MatchString(step.Run) {
		errors = append(errors, &ValidationError{
			Feature:     "oidc-token",
			Description: "OIDC token requests are not supported",
			Suggestion:  "Use static credentials or a different authentication method; act does not provide OIDC tokens",
			JobID:       jobID,
			StepName:    stepName,
			Severity:    SeverityError,
		})
	}

	// Check in step env
	for _, v := range step.Env {
		if oidcTokenPattern.MatchString(v) {
			errors = append(errors, &ValidationError{
				Feature:     "oidc-token",
				Description: "OIDC token requests are not supported",
				Suggestion:  "Use static credentials or a different authentication method; act does not provide OIDC tokens",
				JobID:       jobID,
				StepName:    stepName,
				Severity:    SeverityError,
			})
			break
		}
	}

	// Check in with parameters
	for _, v := range step.With {
		if str, ok := v.(string); ok && oidcTokenPattern.MatchString(str) {
			errors = append(errors, &ValidationError{
				Feature:     "oidc-token",
				Description: "OIDC token requests are not supported",
				Suggestion:  "Use static credentials or a different authentication method; act does not provide OIDC tokens",
				JobID:       jobID,
				StepName:    stepName,
				Severity:    SeverityError,
			})
			break
		}
	}

	return errors
}

// ValidateWorkflows validates multiple workflows and returns combined errors.
func ValidateWorkflows(workflows []*Workflow) error {
	var allErrors ValidationErrors

	for _, wf := range workflows {
		if err := ValidateWorkflow(wf); err != nil {
			if validationErrors, ok := err.(ValidationErrors); ok {
				allErrors = append(allErrors, validationErrors...)
			}
		}
	}

	if len(allErrors) == 0 {
		return nil
	}
	return allErrors
}
