/**
 * ESLint error parser.
 * Handles ESLint output formats: stylish (default), compact, and unix.
 * Migrated from packages/core/tools/eslint/
 */

import {
  applyWorkflowContext,
  MultiLineParser,
  type NoisePatternProvider,
  type NoisePatterns,
  type ParseContext,
  type ParseResult,
} from "../parser-types.js";
import type { MutableExtractedError } from "../types.js";
import { stripAnsi } from "../utils.js";

// ============================================================================
// Constants
// ============================================================================

/**
 * Maximum line length to process. Lines longer than this are likely
 * malformed or minified code, not error output.
 */
const MAX_LINE_LENGTH = 4096;

/**
 * Maximum message length to extract. Prevents memory issues from
 * extremely long error messages.
 */
const MAX_MESSAGE_LENGTH = 1024;

/**
 * Valid JS/TS file extensions for pattern matching.
 */
const JS_EXTENSIONS = "js|jsx|ts|tsx|mjs|cjs|mts|cts|vue|svelte|astro";

// ============================================================================
// Patterns
// ============================================================================

/**
 * ESLint stylish format error line (indented).
 * Format: "  8:11  error  Message text  rule-name"
 *
 * Groups:
 *   1: line number
 *   2: column number
 *   3: severity ("error" or "warning")
 *   4: message and rule ID (parsed separately)
 *
 * Security: Uses possessive-like matching with bounded repetition.
 */
const stylishErrorPattern =
  /^\s{1,20}(\d{1,10}):(\d{1,10})\s{1,20}(error|warning)\s{1,20}(.{1,2048})$/;

/**
 * ESLint stylish format file path header.
 * Matches common JS/TS file extensions including ESM variants and frameworks.
 * MUST NOT contain colons to avoid matching Go/Unix error format.
 *
 * Group 1: file path
 *
 * Security: Path length bounded, character class is restrictive.
 */
const stylishFilePattern = new RegExp(
  `^([^\\s:]{1,1024}\\.(?:${JS_EXTENSIONS}))$`
);

/**
 * ESLint compact format (single-line).
 * Format: "/path/to/file.js: line 8, col 11, Error - Message (rule-id)"
 *
 * Groups:
 *   1: file path
 *   2: line number
 *   3: column number
 *   4: severity ("Error" or "Warning")
 *   5: message
 *   6: rule ID (optional, in parentheses)
 *
 * Security: Bounded lengths prevent catastrophic backtracking.
 */
const compactPattern = new RegExp(
  `^([^\\s:]{1,1024}\\.(?:${JS_EXTENSIONS})):\\s{0,5}line\\s{1,5}(\\d{1,10}),\\s{0,5}col\\s{1,5}(\\d{1,10}),\\s{0,5}(Error|Warning)\\s{0,5}-\\s{0,5}([^()]{1,1024})(?:\\s{1,5}\\(([^)]{1,256})\\))?\\s{0,5}$`
);

/**
 * ESLint unix format (single-line, colon-separated).
 * Format: "/path/to/file.js:8:11: message [error/rule-id]"
 *
 * The [severity/rule] suffix is REQUIRED to distinguish from Go errors.
 *
 * Groups:
 *   1: file path
 *   2: line number
 *   3: column number
 *   4: message
 *   5: severity ("error" or "warning")
 *   6: rule ID
 *
 * Security: Bounded lengths, explicit character classes.
 */
const unixPattern = new RegExp(
  `^([^\\s:]{1,1024}\\.(?:${JS_EXTENSIONS})):(\\d{1,10}):(\\d{1,10}):\\s{0,5}([^\\[]{1,1024})\\s{0,5}\\[(error|warning)/([^\\]]{1,256})\\]\\s{0,5}$`
);

/**
 * Extract rule ID from the end of the message in stylish format.
 * ESLint rules can be:
 *   - Simple: "no-var", "semi", "quotes"
 *   - Scoped: "react/no-unsafe", "import/no-unresolved"
 *   - Namespaced: "@typescript-eslint/no-unused-vars"
 *
 * Groups:
 *   1: message (everything before the rule)
 *   2: rule ID
 *
 * Security: Bounded rule ID length, limited nesting depth (max 3 segments).
 */
const ruleIdPattern =
  /^(.{1,1024})\s{1,10}((?:@[\w-]{1,64}\/)?[\w-]{1,64}(?:\/[\w-]{1,64})?(?:\/[\w-]{1,64})?)\s{0,5}$/;

/**
 * Summary line at the end of ESLint output.
 * Examples: "X 2 problems (1 error, 1 warning)"
 *
 * Security: Uses bounded numbers and literal characters.
 */
const summaryPattern =
  /[✖X]\s{1,5}\d{1,10}\s{1,5}problems?\s{1,5}\(\d{1,10}\s{1,5}errors?,\s{1,5}\d{1,10}\s{1,5}warnings?\)/;

/**
 * Noise patterns for lines that should be skipped.
 */
const noisePatterns: readonly RegExp[] = [
  // Summary and count lines
  /[✖X]\s+\d+\s+problems?/,
  /^\d+\s+errors?$/,
  /^\d+\s+warnings?$/,
  /^\d+\s+errors?\s+and\s+\d+/,
  /^✓\s+/,
  /^All files pass linting/,

  // Empty lines
  /^\s*$/,

  // Process/running messages
  /(?:^running|^linting)\s+eslint/i,
  /^eslint\s+--/i,
  /^Done in\s+/i,

  // Fixable hints
  /potentially\s+fixable/i,
  /--fix\s+option/i,
];

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Safely parse an integer from a regex match group.
 * Returns undefined if the value is invalid or out of range.
 */
const safeParseInt = (value: string | undefined): number | undefined => {
  if (!value) {
    return undefined;
  }
  const num = Number.parseInt(value, 10);
  if (Number.isNaN(num) || num < 0 || num > Number.MAX_SAFE_INTEGER) {
    return undefined;
  }
  return num;
};

/**
 * Truncate a string to the maximum message length.
 */
const truncateMessage = (msg: string): string => {
  if (msg.length <= MAX_MESSAGE_LENGTH) {
    return msg;
  }
  return `${msg.slice(0, MAX_MESSAGE_LENGTH - 3)}...`;
};

/**
 * Check if a line is too long to safely process.
 */
const isLineTooLong = (line: string): boolean => line.length > MAX_LINE_LENGTH;

/**
 * Extract rule ID from the end of stylish format message.
 * Returns [message, ruleId] tuple. ruleId may be empty if not found.
 */
const extractRuleId = (messageAndRule: string): [string, string] => {
  // Fast path: if string is too short to contain both message and rule
  if (messageAndRule.length < 3) {
    return [messageAndRule, ""];
  }

  const match = ruleIdPattern.exec(messageAndRule);
  if (match?.[1] && match[2]) {
    return [match[1].trim(), match[2]];
  }
  return [messageAndRule, ""];
};

// ============================================================================
// Parser Implementation
// ============================================================================

/**
 * Detected format type from canParse, used to avoid re-matching in parse.
 */
type DetectedFormat = "unix" | "stylish-file" | "stylish-error" | "compact";

/**
 * ESLint parser for stylish, compact, and unix output formats.
 *
 * Multi-line behavior: ESLint stylish format outputs file path on one line,
 * followed by indented error lines underneath.
 *
 * State management: The parser maintains state for multi-line stylish format.
 * Always call reset() between independent parsing sessions.
 */
class ESLintParser extends MultiLineParser implements NoisePatternProvider {
  readonly id = "eslint";
  readonly priority = 75;

  // Multi-line state for stylish format
  private currentFile = "";
  private inStylish = false;

  // Cache for avoiding double pattern matching between canParse and parse
  private lastStripped = "";
  private lastFormat: DetectedFormat | null = null;
  private lastMatch: RegExpExecArray | null = null;

  canParse(line: string, ctx: ParseContext): number {
    // Early rejection for overly long lines (likely minified code)
    if (isLineTooLong(line)) {
      return 0;
    }

    const stripped = stripAnsi(line);

    // Cache the stripped line for potential reuse in parse()
    this.lastStripped = stripped;
    this.lastFormat = null;
    this.lastMatch = null;

    // Check unix format first (highest specificity due to [severity/rule] suffix)
    const unixMatch = unixPattern.exec(stripped);
    if (unixMatch) {
      this.lastFormat = "unix";
      this.lastMatch = unixMatch;
      return 0.92;
    }

    // Check for file path (starts stylish multi-line sequence)
    const fileMatch = stylishFilePattern.exec(stripped);
    if (fileMatch) {
      this.lastFormat = "stylish-file";
      this.lastMatch = fileMatch;
      return 0.85;
    }

    // Check for stylish error line (indented)
    const stylishMatch = stylishErrorPattern.exec(stripped);
    if (stylishMatch) {
      this.lastFormat = "stylish-error";
      this.lastMatch = stylishMatch;
      // High confidence if we have file context
      if ((this.inStylish && this.currentFile) || ctx.lastFile) {
        return 0.9;
      }
      return 0.7;
    }

    // Check for compact format
    const compactMatch = compactPattern.exec(stripped);
    if (compactMatch) {
      this.lastFormat = "compact";
      this.lastMatch = compactMatch;
      return 0.9;
    }

    return 0;
  }

  parse(line: string, ctx: ParseContext): ParseResult {
    // Early rejection for overly long lines
    if (isLineTooLong(line)) {
      return null;
    }

    const stripped = stripAnsi(line);

    // Use cached match if available and line matches
    if (stripped === this.lastStripped && this.lastFormat && this.lastMatch) {
      const format = this.lastFormat;
      const match = this.lastMatch;

      // Clear cache after use
      this.lastFormat = null;
      this.lastMatch = null;

      switch (format) {
        case "unix":
          return this.parseUnixError(match, line, ctx);
        case "stylish-file":
          return this.handleStylishFile(match, ctx);
        case "stylish-error":
          return this.parseStylishError(match, line, ctx);
        case "compact":
          return this.parseCompactError(match, line, ctx);
        default:
          break;
      }
    }

    // Clear stale cache
    this.lastFormat = null;
    this.lastMatch = null;

    // Try unix format first (most specific)
    const unixMatch = unixPattern.exec(stripped);
    if (unixMatch) {
      return this.parseUnixError(unixMatch, line, ctx);
    }

    // Check for file path line (starts stylish sequence)
    const fileMatch = stylishFilePattern.exec(stripped);
    if (fileMatch) {
      return this.handleStylishFile(fileMatch, ctx);
    }

    // Try stylish format (indented line)
    const stylishMatch = stylishErrorPattern.exec(stripped);
    if (stylishMatch) {
      return this.parseStylishError(stylishMatch, line, ctx);
    }

    // Try compact format
    const compactMatch = compactPattern.exec(stripped);
    if (compactMatch) {
      return this.parseCompactError(compactMatch, line, ctx);
    }

    return null;
  }

  /**
   * Handle stylish format file path header line.
   * Updates parser state and context but returns null (no error to extract).
   */
  private handleStylishFile(
    match: RegExpExecArray,
    ctx: ParseContext
  ): ParseResult {
    const filePath = match[1];
    if (filePath) {
      this.currentFile = filePath;
      this.inStylish = true;
      ctx.lastFile = this.currentFile;
    }
    return null;
  }

  isNoise(line: string): boolean {
    // Early rejection for overly long lines
    if (isLineTooLong(line)) {
      return false;
    }

    const stripped = stripAnsi(line);

    // Fast path: check for common noise prefixes
    const lower = stripped.toLowerCase();
    if (
      lower.startsWith("all files pass") ||
      lower.startsWith("done in") ||
      lower.startsWith("running eslint") ||
      lower.startsWith("linting eslint")
    ) {
      return true;
    }

    // Fast path: check for common noise substrings
    if (
      lower.includes("potentially fixable") ||
      lower.includes("--fix option")
    ) {
      return true;
    }

    // Fall back to regex patterns
    return noisePatterns.some((pattern) => pattern.test(stripped));
  }

  continueMultiLine(line: string, _ctx: ParseContext): boolean {
    if (!this.inStylish) {
      return false;
    }

    // Early rejection for overly long lines
    if (isLineTooLong(line)) {
      return false;
    }

    const stripped = stripAnsi(line);

    // Empty line signals potential end
    if (stripped.trim() === "") {
      return false;
    }

    // New file path starts new file's errors
    if (stylishFilePattern.test(stripped)) {
      return false;
    }

    // Summary line signals end of all errors
    if (summaryPattern.test(stripped)) {
      this.reset();
      return false;
    }

    // Indented error line continues current file
    if (stylishErrorPattern.test(stripped)) {
      return true;
    }

    // Non-matching line ends stylish sequence
    return false;
  }

  finishMultiLine(_ctx: ParseContext): ParseResult {
    this.reset();
    return null;
  }

  reset(): void {
    this.currentFile = "";
    this.inStylish = false;
    this.lastStripped = "";
    this.lastFormat = null;
    this.lastMatch = null;
  }

  noisePatterns(): NoisePatterns {
    return {
      fastPrefixes: [
        "all files pass",
        "done in",
        "running eslint",
        "linting eslint",
      ],
      fastContains: ["potentially fixable", "--fix option"],
      regex: noisePatterns,
    };
  }

  // ============================================================================
  // Private Parse Methods
  // ============================================================================

  private parseStylishError(
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const lineNum = safeParseInt(match[1]);
    const colNum = safeParseInt(match[2]);
    const severityRaw = match[3]?.toLowerCase();
    const messageAndRule = match[4]?.trim();

    // Validate required fields
    if (!(messageAndRule && severityRaw)) {
      return null;
    }

    // Validate severity is expected value
    if (severityRaw !== "error" && severityRaw !== "warning") {
      return null;
    }

    const [rawMessage, ruleId] = extractRuleId(messageAndRule);
    const message = truncateMessage(rawMessage);

    // Determine file from parser state or context
    const file = this.currentFile || ctx.lastFile;

    const err: MutableExtractedError = {
      message,
      file: file || undefined,
      line: lineNum,
      column: colNum,
      ruleId: ruleId || undefined,
      severity: severityRaw,
      category: "lint",
      source: "eslint",
      raw: rawLine,
      lineKnown: lineNum !== undefined && lineNum > 0,
      columnKnown: colNum !== undefined && colNum > 0,
      messageTruncated: rawMessage.length > MAX_MESSAGE_LENGTH,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parseCompactError(
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const file = match[1];
    const lineNum = safeParseInt(match[2]);
    const colNum = safeParseInt(match[3]);
    const severityRaw = match[4]?.toLowerCase();
    const rawMessage = match[5]?.trim();
    const ruleId = match[6];

    // Validate required fields
    if (!(file && rawMessage && severityRaw)) {
      return null;
    }

    // Validate severity is expected value
    if (severityRaw !== "error" && severityRaw !== "warning") {
      return null;
    }

    const message = truncateMessage(rawMessage);

    const err: MutableExtractedError = {
      message,
      file,
      line: lineNum,
      column: colNum,
      ruleId: ruleId || undefined,
      severity: severityRaw,
      category: "lint",
      source: "eslint",
      raw: rawLine,
      lineKnown: lineNum !== undefined && lineNum > 0,
      columnKnown: colNum !== undefined && colNum > 0,
      messageTruncated: rawMessage.length > MAX_MESSAGE_LENGTH,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private parseUnixError(
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): ParseResult {
    const file = match[1];
    const lineNum = safeParseInt(match[2]);
    const colNum = safeParseInt(match[3]);
    const rawMessage = match[4]?.trim();
    const severityRaw = match[5]?.toLowerCase();
    const ruleId = match[6];

    // Validate required fields
    if (!(file && rawMessage && severityRaw && ruleId)) {
      return null;
    }

    // Validate severity is expected value
    if (severityRaw !== "error" && severityRaw !== "warning") {
      return null;
    }

    const message = truncateMessage(rawMessage);

    const err: MutableExtractedError = {
      message,
      file,
      line: lineNum,
      column: colNum,
      ruleId,
      severity: severityRaw,
      category: "lint",
      source: "eslint",
      raw: rawLine,
      lineKnown: lineNum !== undefined && lineNum > 0,
      columnKnown: colNum !== undefined && colNum > 0,
      messageTruncated: rawMessage.length > MAX_MESSAGE_LENGTH,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }
}

// ============================================================================
// Export
// ============================================================================

/**
 * Create a new ESLint parser instance.
 */
export const createESLintParser = (): ESLintParser => new ESLintParser();
