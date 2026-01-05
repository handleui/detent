/**
 * Severity inference for extracted errors.
 * Migrated from packages/core/errors/severity.go
 */

import type {
  ErrorCategory,
  ExtractedError,
  MutableExtractedError,
} from "./types.js";

// ============================================================================
// Severity Inference
// ============================================================================

/**
 * Infer severity level for an error based on its category.
 *
 * Rules:
 * - If severity is already set (e.g., by ESLint), keep it
 * - compile → "error" (compilation failures block builds)
 * - type-check → "error" (type errors block builds)
 * - test → "error" (test failures indicate broken functionality)
 * - runtime → "error" (runtime errors indicate broken execution)
 * - lint → "warning" by default (linting is advisory)
 * - docs → "warning" (documentation issues are advisory)
 * - metadata → undefined (not counted as problems)
 * - unknown → "warning" (conservative default)
 */
export const inferSeverity = (
  category: ErrorCategory | undefined,
  existingSeverity?: string
): "error" | "warning" | undefined => {
  // If severity is already set (e.g., ESLint, Docker), keep it
  if (existingSeverity === "error" || existingSeverity === "warning") {
    return existingSeverity;
  }

  // Infer from category
  switch (category) {
    case "compile":
      return "error";
    case "type-check":
      return "error";
    case "test":
      return "error";
    case "runtime":
      return "error";
    case "security":
      return "error";
    case "infrastructure":
      return "error";
    case "lint":
      // Linters typically report warnings unless explicitly marked as errors
      return "warning";
    case "docs":
      // Documentation issues are advisory
      return "warning";
    case "dependency":
      // Dependency issues could be errors or warnings, default to warning
      return "warning";
    case "config":
      // Config issues could block execution, but often are warnings
      return "warning";
    case "metadata":
      // Metadata errors (exit codes, job failures) are not code problems
      // Return undefined so they're not counted in error/warning totals
      return undefined;
    case "unknown":
      // Conservative default: treat unknown issues as warnings
      return "warning";
    default:
      // Fallback for any new categories
      return "warning";
  }
};

/**
 * Apply severity inference to a single error.
 * Mutates the error in place.
 */
export const applySeverityToError = (err: MutableExtractedError): void => {
  const inferred = inferSeverity(err.category, err.severity);
  if (inferred) {
    err.severity = inferred;
  }
};

/**
 * Apply severity inference to all extracted errors.
 * This is done as explicit post-processing after extraction to maintain separation
 * of concerns: extraction is pure parsing, severity is business logic.
 */
export const applySeverity = (errors: MutableExtractedError[]): void => {
  for (const err of errors) {
    applySeverityToError(err);
  }
};

/**
 * Create a new error with inferred severity (immutable version).
 */
export const withInferredSeverity = (err: ExtractedError): ExtractedError => {
  const inferred = inferSeverity(err.category, err.severity);
  if (inferred && inferred !== err.severity) {
    return { ...err, severity: inferred };
  }
  return err;
};
