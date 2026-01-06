/**
 * Act CI context parser.
 * Parses output from Act (https://github.com/nektos/act) which runs GitHub Actions locally.
 *
 * Act output format:
 * - Lines are prefixed with [Job Name/Step Name] or [Job Name]
 * - Example: [Build/Setup Go] Run actions/checkout@v4
 * - Example: [Build/Run Tests] ::error file=main.go,line=42::undefined: foo
 *
 * The parser extracts job and step names from the prefix and cleans the line.
 */

import type { ContextParser, LineContext, ParseLineResult } from "./types.js";

/**
 * Regex to match Act's line prefix format: [Job Name/Step Name] or [Job Name]
 * Captures:
 * - Group 1: Job name (everything before the first / or the entire content if no /)
 * - Group 2: Step name (everything after the first /, if present)
 *
 * Handles edge cases:
 * - Nested brackets in job/step names
 * - Missing step names
 * - Special characters in names
 */
const ACT_PREFIX_REGEX = /^\[([^\]/]+)(?:\/([^\]]+))?\]\s*/;

/**
 * Regex to strip act's pipe marker from command output (e.g., "  | error: ..." -> "error: ...")
 */
const PIPE_MARKER_REGEX = /^\s*\|\s*/;

/**
 * Patterns that indicate noise lines in Act output.
 * These are debug/metadata lines that should be skipped.
 */
const NOISE_PATTERNS: readonly RegExp[] = [
  // "with:" lines showing action inputs
  /^\s*with:\s*$/,
  // Action input lines (indented key: value pairs after "with:")
  /^\s{2,}\w+:\s*.*/,
  // Empty lines after prefix removal
  /^\s*$/,
  // Debug output markers
  /^::debug::/,
  // Group markers
  /^::group::/,
  /^::endgroup::/,
  // Act-specific debug/verbose output
  /^\s*\|\s*$/,
  // Docker layer output
  /^#\d+\s+/,
  // Act internal messages
  /^\s*=>\s*/,
];

/**
 * Fast prefix checks for noise detection (lowercase).
 */
const FAST_NOISE_PREFIXES: readonly string[] = [
  "with:",
  "  with:",
  "::debug::",
  "::group::",
  "::endgroup::",
  "=>",
  "|",
];

/**
 * Check if a cleaned line is noise that should be skipped.
 */
const isNoiseLine = (cleanedLine: string): boolean => {
  const trimmed = cleanedLine.trim();
  const lower = trimmed.toLowerCase();

  // Fast prefix check
  for (const prefix of FAST_NOISE_PREFIXES) {
    if (lower.startsWith(prefix)) {
      return true;
    }
  }

  // Regex pattern check
  for (const pattern of NOISE_PATTERNS) {
    if (pattern.test(trimmed)) {
      return true;
    }
  }

  return false;
};

/**
 * ActParser extracts context from Act's log output format.
 * Act prefixes each line with [Job Name/Step Name] to indicate the current job and step.
 */
class ActParser implements ContextParser {
  /**
   * Parse a line of Act output and extract the job/step context.
   *
   * @param line - Raw line from Act output
   * @returns ParseLineResult with extracted context and cleaned line
   */
  parseLine = (line: string): ParseLineResult => {
    const match = ACT_PREFIX_REGEX.exec(line);

    if (!match) {
      // No Act prefix found - return as-is with empty context
      const isNoise = isNoiseLine(line);
      return {
        ctx: { job: "", step: "", isNoise },
        cleanLine: line,
        skip: isNoise,
      };
    }

    const job = match[1]?.trim() ?? "";
    const step = match[2]?.trim() ?? "";
    // Strip act's pipe marker for command output (e.g., "  | error: ..." -> "error: ...")
    const rawCleanLine = line.slice(match[0].length);
    const cleanLine = rawCleanLine.replace(PIPE_MARKER_REGEX, "");

    const isNoise = isNoiseLine(cleanLine);

    const ctx: LineContext = {
      job,
      step,
      isNoise,
    };

    return {
      ctx,
      cleanLine,
      skip: isNoise,
    };
  };
}

/**
 * Create a new ActParser instance.
 */
export const createActParser = (): ContextParser => new ActParser();

/**
 * Singleton instance for convenience.
 */
export const actParser: ContextParser = new ActParser();
