package errors

// InferSeverity determines the severity level for an error based on its category,
// source, and explicit severity (if already set by the extractor).
//
// Rules:
// - If severity is already set (e.g., by ESLint), keep it
// - CategoryCompile → "error" (compilation failures block builds)
// - CategoryTypeCheck → "error" (type errors block builds)
// - CategoryTest → "error" (test failures indicate broken functionality)
// - CategoryRuntime → "error" (runtime errors indicate broken execution)
// - CategoryLint → "warning" by default (linting is advisory, unless tool marks as error)
// - CategoryUnknown → "warning" (conservative default)
// - Docker errors → "error" (infrastructure failures block execution)
func InferSeverity(err *ExtractedError) string {
	// If severity is already set (e.g., ESLint, Docker), keep it
	if err.Severity != "" {
		return err.Severity
	}

	// Infer from category
	switch err.Category {
	case CategoryCompile:
		return "error"
	case CategoryTypeCheck:
		return "error"
	case CategoryTest:
		return "error"
	case CategoryRuntime:
		return "error"
	case CategoryLint:
		// Linters typically report warnings unless explicitly marked as errors
		// (ESLint already sets severity explicitly, so this is a fallback)
		return "warning"
	case CategoryUnknown:
		// Conservative default: treat unknown issues as warnings
		return "warning"
	default:
		// Fallback for any new categories
		return "warning"
	}
}

// ApplySeverity infers severity for all extracted errors based on their category.
// This is done as explicit post-processing after extraction to maintain separation
// of concerns: extraction is pure parsing, severity is business logic.
func ApplySeverity(extractedErrors []*ExtractedError) {
	for _, err := range extractedErrors {
		err.Severity = InferSeverity(err)
	}
}
