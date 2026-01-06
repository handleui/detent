/**
 * GitHub Actions log context parser.
 * Parses logs from GitHub Actions runners (fetched via GitHub API).
 *
 * GitHub Actions log format:
 * - Lines are prefixed with timestamps: 2024-01-15T10:30:45.1234567Z message
 * - Workflow commands: ::error file=x,line=1::message, ::warning::, ::notice::
 * - Group markers: ::group::title / ::endgroup::
 * - Debug output: ::debug::message
 *
 * This parser strips timestamps and filters noise, leaving clean lines for tool parsers.
 */

import type { ContextParser, LineContext, ParseLineResult } from "./types.js";

/**
 * Regex to match GitHub Actions timestamp prefix.
 * Format: 2024-01-15T10:30:45.1234567Z (ISO 8601 with nanosecond precision)
 * The timestamp is always at the start of the line followed by a space.
 */
const TIMESTAMP_REGEX = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s*/;

/**
 * Patterns that indicate noise lines in GitHub Actions output.
 * These are debug/metadata lines that should be skipped entirely.
 */
const NOISE_PATTERNS: readonly RegExp[] = [
  // Debug output (only visible when ACTIONS_STEP_DEBUG is true)
  /^::debug::/,
  // Group markers (expandable sections in UI)
  /^::group::/,
  /^::endgroup::/,
  // Empty lines
  /^\s*$/,
  // GitHub Actions internal markers (##[...])
  /^##\[/,
];

/**
 * Fast prefix checks for noise detection.
 */
const FAST_NOISE_PREFIXES: readonly string[] = [
  "::debug::",
  "::group::",
  "::endgroup::",
  "##[",
];

/**
 * Check if a cleaned line is noise that should be skipped.
 */
const isNoiseLine = (cleanedLine: string): boolean => {
  const trimmed = cleanedLine.trim();

  // Fast prefix check
  for (const prefix of FAST_NOISE_PREFIXES) {
    if (trimmed.startsWith(prefix)) {
      return true;
    }
  }

  // Regex pattern check for edge cases
  for (const pattern of NOISE_PATTERNS) {
    if (pattern.test(trimmed)) {
      return true;
    }
  }

  return false;
};

/**
 * GitHubParser extracts context from GitHub Actions log output format.
 * GitHub Actions prefixes each line with an ISO 8601 timestamp.
 *
 * Note: Job/step context is NOT available in raw log lines - GitHub Actions
 * logs are typically fetched per-job via API, so context comes from the API
 * response, not from parsing the log content itself.
 */
class GitHubParser implements ContextParser {
  /**
   * Parse a line of GitHub Actions output and extract context.
   *
   * @param line - Raw line from GitHub Actions log
   * @returns ParseLineResult with cleaned line (timestamp stripped)
   */
  parseLine = (line: string): ParseLineResult => {
    // Strip timestamp prefix if present
    const cleanLine = line.replace(TIMESTAMP_REGEX, "");

    const isNoise = isNoiseLine(cleanLine);

    const ctx: LineContext = {
      // Job/step context comes from GitHub API, not log content
      job: "",
      step: "",
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
 * Create a GitHub Actions context parser.
 */
export const createGitHubContextParser = (): ContextParser =>
  new GitHubParser();

/**
 * Singleton instance for convenience.
 */
export const githubParser: ContextParser = createGitHubContextParser();
