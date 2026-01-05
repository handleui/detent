import { INTERNAL_FRAME_PATTERNS, MAX_STACK_TRACE_LINES } from "./system.js";

/**
 * Error categories in priority order.
 */
export type ErrorCategory =
  | "compile"
  | "type-check"
  | "test"
  | "runtime"
  | "lint"
  | "metadata"
  | "unknown";

/**
 * Error severity levels.
 */
export type ErrorSeverity = "error" | "warning";

/**
 * Extracted error with full diagnostic context.
 */
export interface ExtractedError {
  filePath: string;
  line: number;
  column?: number;
  message: string;
  category: ErrorCategory;
  severity: ErrorSeverity;
  ruleId?: string;
  source?: string;
  stackTrace?: string;
}

/**
 * Default values for error formatting.
 */
const DEFAULT_CATEGORY = "unknown";
const DEFAULT_COLUMN = 1;
const MISSING_VALUE = "-";
const DEFAULT_PRIORITY = 999;
const MAX_STACK_TRACE_BYTES = 50 * 1024;

/**
 * Category priority map. Lower number = higher priority.
 */
const CATEGORY_PRIORITY: Record<ErrorCategory, number> = {
  compile: 1,
  "type-check": 2,
  test: 3,
  runtime: 4,
  lint: 5,
  metadata: 6,
  unknown: 7,
};

/**
 * Categories that benefit from stack traces.
 * Stack traces improve accuracy from 31% to 80-90% for these categories.
 */
const CATEGORIES_NEEDING_STACK_TRACE = new Set<ErrorCategory>([
  "compile",
  "test",
  "runtime",
  "unknown",
]);

/**
 * Gets the priority for a category.
 */
export const getCategoryPriority = (category: ErrorCategory): number =>
  CATEGORY_PRIORITY[category] ?? DEFAULT_PRIORITY;

/**
 * Escapes prompt string to prevent injection.
 */
const escapePromptString = (s: string): string => {
  let result = s.replaceAll("`", "'").replaceAll("\r", "");
  while (result.includes("\n\n\n")) {
    result = result.replaceAll("\n\n\n", "\n\n");
  }
  return result;
};

/**
 * Checks if a stack frame is an internal/framework frame.
 */
const isInternalFrame = (line: string): boolean =>
  INTERNAL_FRAME_PATTERNS.some((pattern) => line.includes(pattern));

/**
 * Filters stack trace lines to remove internal frames.
 */
const filterStackTraceLines = (lines: string[]): string[] =>
  lines.filter((line) => !isInternalFrame(line) && line.trim() !== "");

/**
 * Formats a stack trace for inclusion in a prompt.
 */
export const formatStackTrace = (error: ExtractedError): string => {
  if (!error.stackTrace) {
    return "";
  }

  if (error.stackTrace.length > MAX_STACK_TRACE_BYTES) {
    return "  Stack trace: (truncated - exceeds 50KB limit)";
  }

  if (!CATEGORIES_NEEDING_STACK_TRACE.has(error.category)) {
    return "";
  }

  const lines = error.stackTrace.trim().split("\n");
  const filtered = filterStackTraceLines(lines);

  if (filtered.length === 0) {
    return "";
  }

  const originalLen = filtered.length;
  const truncated = filtered.length > MAX_STACK_TRACE_LINES;
  const displayLines = truncated
    ? filtered.slice(0, MAX_STACK_TRACE_LINES)
    : filtered;

  const parts = ["  Stack trace:"];
  for (const line of displayLines) {
    parts.push(`    ${escapePromptString(line)}`);
  }

  if (truncated) {
    const remaining = originalLen - MAX_STACK_TRACE_LINES;
    parts.push(`    ... (truncated, ${remaining} more frames)`);
  }

  return parts.join("\n");
};

/**
 * Formats a single error with full diagnostic context.
 */
export const formatError = (error: ExtractedError): string => {
  const parts: string[] = [];

  const category = error.category || DEFAULT_CATEGORY;
  const column = error.column ?? DEFAULT_COLUMN;

  const file = escapePromptString(error.filePath);
  const message = escapePromptString(error.message);

  const location =
    file !== ""
      ? `${file}:${error.line}:${column}`
      : `line ${error.line}:${column}`;
  parts.push(`[${category}] ${location}: ${message}`);

  if (error.ruleId || error.source) {
    const ruleId = error.ruleId
      ? escapePromptString(error.ruleId)
      : MISSING_VALUE;
    const source = error.source
      ? escapePromptString(error.source)
      : MISSING_VALUE;
    parts.push(`  Rule: ${ruleId} | Source: ${source}`);
  }

  const stackTrace = formatStackTrace(error);
  if (stackTrace) {
    parts.push(stackTrace);
  }

  return parts.join("\n");
};

/**
 * Prioritizes errors by category, then file, then line number.
 */
export const prioritizeErrors = (
  errors: ExtractedError[]
): ExtractedError[] => {
  if (errors.length === 0) {
    return errors;
  }

  const sorted = [...errors];
  sorted.sort((a, b) => {
    const priA = getCategoryPriority(a.category);
    const priB = getCategoryPriority(b.category);

    if (priA !== priB) {
      return priA - priB;
    }

    if (a.filePath !== b.filePath) {
      return a.filePath.localeCompare(b.filePath);
    }

    return a.line - b.line;
  });

  return sorted;
};

/**
 * Formats multiple errors, sorted by priority.
 */
export const formatErrors = (errors: ExtractedError[]): string => {
  if (errors.length === 0) {
    return "(no errors)";
  }

  const sorted = prioritizeErrors(errors);
  const parts: string[] = [];

  for (const error of sorted) {
    parts.push(formatError(error));
  }

  if (parts.length === 0) {
    return "(no valid errors)";
  }

  return parts.join("\n\n");
};

/**
 * Counts errors and warnings.
 */
export const countErrors = (
  errors: ExtractedError[]
): { errorCount: number; warningCount: number } => {
  let errorCount = 0;
  let warningCount = 0;

  for (const error of errors) {
    if (error.severity === "error") {
      errorCount++;
    } else if (error.severity === "warning") {
      warningCount++;
    }
  }

  return { errorCount, warningCount };
};

/**
 * Counts errors by category.
 */
export const countByCategory = (
  errors: ExtractedError[]
): Record<ErrorCategory, number> => {
  const counts: Record<ErrorCategory, number> = {
    compile: 0,
    "type-check": 0,
    test: 0,
    runtime: 0,
    lint: 0,
    metadata: 0,
    unknown: 0,
  };

  for (const error of errors) {
    const category = error.category || "unknown";
    counts[category]++;
  }

  return counts;
};
