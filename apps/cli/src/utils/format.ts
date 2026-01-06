import type { ParsedError } from "../runner/types.js";
import type { DisplayError } from "../tui/check-tui-types.js";

/**
 * Formats a duration in seconds to a human-readable string.
 *
 * Examples:
 * - 0 -> "0s"
 * - 45 -> "45s"
 * - 60 -> "1m 0s"
 * - 90 -> "1m 30s"
 * - 125 -> "2m 5s"
 * - 3661 -> "61m 1s" (no hours for simplicity)
 *
 * @param seconds - Duration in whole seconds
 * @returns Formatted duration string
 */
export const formatDuration = (seconds: number): string => {
  if (seconds < 60) {
    return `${seconds}s`;
  }

  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;

  return `${minutes}m ${remainingSeconds}s`;
};

/**
 * Formats a duration in milliseconds to a human-readable string.
 *
 * @param ms - Duration in milliseconds
 * @returns Formatted duration string with one decimal for sub-minute durations
 */
export const formatDurationMs = (ms: number): string => {
  const seconds = ms / 1000;

  if (seconds < 60) {
    return `${seconds.toFixed(1)}s`;
  }

  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = Math.round(seconds % 60);

  return `${minutes}m ${remainingSeconds}s`;
};

/**
 * Transforms ParsedError from the runner to DisplayError for TUI rendering.
 *
 * ParsedError is optimized for AI consumption (stack traces, raw output).
 * DisplayError is optimized for user display (clean, scannable format).
 */
export const toDisplayErrors = (
  errors: readonly ParsedError[]
): DisplayError[] =>
  errors.map((err) => ({
    message: err.message,
    file: err.filePath,
    line: err.line,
    column: err.column,
    severity: (err.severity === "warning" ? "warning" : "error") as
      | "error"
      | "warning",
    ruleId: err.ruleId,
    category: err.category,
  }));
