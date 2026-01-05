/**
 * Serialization helpers for error extraction output.
 * Provides JSON serialization with field redaction and ANSI stripping.
 */

import type { ErrorReport, ExtractedError } from "./types.js";
import { stripAnsi } from "./utils.js";

// ============================================================================
// Field Redaction
// ============================================================================

/**
 * Fields that may contain sensitive information and should be redacted.
 */
const sensitiveFields = new Set([
  "token",
  "apiKey",
  "api_key",
  "apikey",
  "password",
  "secret",
  "credentials",
  "authorization",
  "auth",
]);

/**
 * Check if a key represents a sensitive field.
 */
const isSensitiveKey = (key: string): boolean => {
  const lower = key.toLowerCase();
  return (
    sensitiveFields.has(lower) ||
    lower.includes("secret") ||
    lower.includes("token")
  );
};

/**
 * Redact sensitive fields from an object recursively.
 * Returns a new object with sensitive values replaced with "[REDACTED]".
 */
export const redactSensitive = <T>(obj: T): T => {
  if (obj === null || obj === undefined) {
    return obj;
  }

  if (Array.isArray(obj)) {
    return obj.map(redactSensitive) as T;
  }

  if (typeof obj === "object") {
    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj)) {
      if (isSensitiveKey(key)) {
        result[key] = "[REDACTED]";
      } else if (typeof value === "object" && value !== null) {
        result[key] = redactSensitive(value);
      } else {
        result[key] = value;
      }
    }
    return result as T;
  }

  return obj;
};

// ============================================================================
// ANSI Stripping for Serialization
// ============================================================================

/**
 * Strip ANSI codes from all string fields in an error.
 * Returns a new error object with clean strings.
 */
export const stripAnsiFromError = (err: ExtractedError): ExtractedError => ({
  ...err,
  message: stripAnsi(err.message),
  file: err.file ? stripAnsi(err.file) : undefined,
  raw: err.raw ? stripAnsi(err.raw) : undefined,
  ruleId: err.ruleId ? stripAnsi(err.ruleId) : undefined,
  stackTrace: err.stackTrace ? stripAnsi(err.stackTrace) : undefined,
  suggestions: err.suggestions?.map(stripAnsi),
  codeSnippet: err.codeSnippet
    ? {
        ...err.codeSnippet,
        lines: err.codeSnippet.lines.map(stripAnsi),
      }
    : undefined,
});

/**
 * Strip ANSI codes from all errors in a report.
 */
export const stripAnsiFromReport = (report: ErrorReport): ErrorReport => ({
  ...report,
  errors: report.errors.map(stripAnsiFromError),
});

// ============================================================================
// JSON Serialization
// ============================================================================

/**
 * Options for JSON serialization.
 */
export interface SerializeOptions {
  /** Strip ANSI codes from strings (default: true) */
  readonly stripAnsi?: boolean;
  /** Redact sensitive fields (default: false) */
  readonly redact?: boolean;
  /** Pretty print with indentation (default: false) */
  readonly pretty?: boolean;
  /** Indentation for pretty printing (default: 2) */
  readonly indent?: number;
}

/**
 * Serialize an error report to JSON.
 */
export const serializeReport = (
  report: ErrorReport,
  opts: SerializeOptions = {}
): string => {
  const {
    stripAnsi: doStripAnsi = true,
    redact = false,
    pretty = false,
    indent = 2,
  } = opts;

  let result: ErrorReport = report;

  if (doStripAnsi) {
    result = stripAnsiFromReport(result);
  }

  if (redact) {
    result = redactSensitive(result);
  }

  return pretty ? JSON.stringify(result, null, indent) : JSON.stringify(result);
};

/**
 * Serialize a single error to JSON.
 */
export const serializeError = (
  err: ExtractedError,
  opts: SerializeOptions = {}
): string => {
  const {
    stripAnsi: doStripAnsi = true,
    redact = false,
    pretty = false,
    indent = 2,
  } = opts;

  let result: ExtractedError = err;

  if (doStripAnsi) {
    result = stripAnsiFromError(result);
  }

  if (redact) {
    result = redactSensitive(result);
  }

  return pretty ? JSON.stringify(result, null, indent) : JSON.stringify(result);
};

/**
 * Serialize errors to a line-delimited JSON format (NDJSON).
 * Each error is on its own line, useful for streaming.
 */
export const serializeErrorsNDJSON = (
  errors: readonly ExtractedError[],
  opts: SerializeOptions = {}
): string => {
  const { stripAnsi: doStripAnsi = true, redact = false } = opts;

  return errors
    .map((err) => {
      let result: ExtractedError = err;
      if (doStripAnsi) {
        result = stripAnsiFromError(result);
      }
      if (redact) {
        result = redactSensitive(result);
      }
      return JSON.stringify(result);
    })
    .join("\n");
};

// ============================================================================
// Compact Output
// ============================================================================

/**
 * Format an error as a compact single-line string.
 * Format: file:line:col: severity: message [ruleId]
 */
export const formatErrorCompact = (err: ExtractedError): string => {
  const parts: string[] = [];

  // Location
  if (err.file) {
    let loc = err.file;
    if (err.line !== undefined) {
      loc += `:${err.line}`;
      if (err.column !== undefined) {
        loc += `:${err.column}`;
      }
    }
    parts.push(loc);
  }

  // Severity
  if (err.severity) {
    parts.push(err.severity);
  }

  // Message
  const message = stripAnsi(err.message);
  parts.push(message);

  // Rule ID
  if (err.ruleId) {
    parts.push(`[${err.ruleId}]`);
  }

  return parts.join(": ");
};

/**
 * Format all errors as compact single-line strings.
 */
export const formatErrorsCompact = (
  errors: readonly ExtractedError[]
): string => errors.map(formatErrorCompact).join("\n");
