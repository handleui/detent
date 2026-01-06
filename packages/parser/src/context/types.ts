/**
 * CI context parser types.
 * Context parsers handle CI-specific log FORMAT (prefixes, timestamps).
 * They extract job/step context and clean lines for tool parsers.
 */

/**
 * LineContext contains CI platform-specific context extracted from a log line.
 */
export interface LineContext {
  /** Job name from CI output */
  readonly job: string;
  /** Step name (if parseable) */
  readonly step: string;
  /** True if line should be skipped (debug output) */
  readonly isNoise: boolean;
}

/**
 * Result of parsing a CI log line.
 */
export interface ParseLineResult {
  /** Extracted context */
  readonly ctx: LineContext;
  /** Cleaned line (with CI prefixes removed) */
  readonly cleanLine: string;
  /** Whether to skip this line entirely */
  readonly skip: boolean;
}

/**
 * ContextParser extracts CI platform-specific context from log lines.
 * Different CI systems (act, GitHub Actions, GitLab) implement this interface
 * to parse their specific output formats and extract job/step context.
 *
 * @example
 * ```typescript
 * // Act format: [Job Name/Step Name] actual log content
 * const result = actParser.parseLine("[Build/Test] error: failed");
 * // { ctx: { job: "Build", step: "Test" }, cleanLine: "error: failed", skip: false }
 *
 * // GitHub format: 2024-01-15T10:30:45.1234567Z actual log content
 * const result = githubParser.parseLine("2024-01-15T10:30:45.1234567Z error: failed");
 * // { ctx: { job: "", step: "" }, cleanLine: "error: failed", skip: false }
 * ```
 */
export interface ContextParser {
  /**
   * Extracts context from a CI log line.
   * Returns the context, the cleaned line (with CI prefixes removed), and whether to skip.
   * If skip is true, the line should be ignored (debug noise, metadata).
   */
  parseLine(line: string): ParseLineResult;
}
