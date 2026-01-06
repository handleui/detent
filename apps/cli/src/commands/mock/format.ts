import type { ParsedError } from "./runner/types.js";
import type { DisplayError } from "./types.js";

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
