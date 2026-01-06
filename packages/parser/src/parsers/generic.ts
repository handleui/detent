/**
 * Generic fallback parser for unrecognized error formats.
 * Migrated from packages/core/tools/generic/parser.go
 *
 * IMPORTANT: This parser is VERY STRICT to avoid false positives in Sentry reports.
 * Only genuinely unrecognized error patterns should be flagged. When in doubt, skip it.
 *
 * Security considerations:
 * - All patterns use anchors to prevent partial matching abuse
 * - Regex patterns avoid nested quantifiers to prevent ReDoS
 * - Line length limits prevent memory exhaustion
 *
 * Performance considerations:
 * - Fast string checks run before expensive regex
 * - Early rejection of non-matching lines
 * - Noise patterns are checked before error patterns
 */

import {
  applyWorkflowContext,
  BaseParser,
  type NoisePatternProvider,
  type NoisePatterns,
  type ParseContext,
  type ParseResult,
} from "../parser-types.js";
import type { MutableExtractedError } from "../types.js";
import { stripAnsi } from "../utils.js";

const PARSER_ID = "generic";
const PARSER_PRIORITY = 10;

/** Minimum line length for a meaningful error message */
const MIN_LINE_LENGTH = 10;

/** Maximum line length to prevent processing minified code or data dumps */
const MAX_LINE_LENGTH = 500;

/**
 * Patterns that match lines looking like REAL errors (high confidence).
 * These patterns require strong structural signals, not just keyword presence.
 *
 * All patterns are anchored to prevent partial matching abuse and use simple
 * quantifiers to avoid ReDoS vulnerabilities.
 */
const actualErrorPatterns: readonly RegExp[] = [
  // Generic file:line:col: message format (very common across tools)
  // Must have a file-like path (contains / or \), line number, and message
  // Example: /path/to/file.txt:42:5: some error message
  /^[^\s:]+(?:\/|\\)[^\s:]+:\d+(?::\d+)?:\s+\S/,

  // Must start with "Error:" or "error:" (case-insensitive)
  /^error:\s+\S/i,
  // Must have format: "FATAL:" or "Fatal:" at start
  /^fatal:\s+\S/i,
  // Structured log format: [ERROR] message or [FATAL] message
  /^\s{0,4}\[(ERROR|FATAL|FAIL)\]\s*:?\s+\S/i,
  // Build/compile/test failure with clear structure
  /^(?:build|compilation|compile)\s+failed\s*$/i,
  // File/directory not found (unambiguous format)
  /^no\s+such\s+file\s+or\s+directory/i,
  // Segfault/crash (unambiguous)
  /^segmentation\s+fault/i,
  /^killed\s*$/i,
  /^out\s+of\s+memory/i,
  // Assertion failures (common in tests/builds)
  /^assertion\s+failed/i,
  // Panic (common in Go, Rust)
  /^panic:/i,
];

/**
 * Patterns that match lines that should NEVER be flagged as errors.
 * These are common CI/CD output patterns that contain error keywords but aren't errors.
 */
const noisePatterns: readonly RegExp[] = [
  // === CODE AND COMMENTS ===
  // NOTE: Removed "--" (SQL comment) as it conflicts with Go test "--- FAIL:" output
  /(?:^\s*(#|\/\/|\/\*|\*))/i, // Comments (excluding SQL --)
  /(?:error.*handler)/i, // Error handler code
  /(?:on_?error)/i, // Error callbacks
  /(?:error_?(code|type|msg|message|class|kind))/i, // Error variables
  /(?:if.*error)/i, // Conditional error handling
  /(?:catch.*error)/i, // Try-catch
  /(?:return.*error)/i, // Return error
  /(?:handle.*error)/i, // Handle error
  /(?:throw.*error)/i, // Throw error
  /(?:raise.*error)/i, // Python raise
  /(?:new\s+error)/i, // new Error()
  /(?:^[A-Z_]+_ERROR\s*=)/i, // Constant definitions
  /(?:\.error\s*[=(])/i, // .error property/method

  // === SUCCESS INDICATORS ===
  /[✓✔✅]/, // Success checkmarks
  /(?:(passed|success|succeeded|ok)\s*$)/i, // Success endings
  /(?:^(pass|ok|success))/i, // Success starts
  /(?:0\s+(error|failure)s?\s*(found)?)/i, // "0 errors found"
  /(?:no\s+(error|failure)s?\s*(found)?)/i, // "no errors found"
  /(?:completed\s+(successfully|with\s+exit\s+code\s+0))/i, // Success completion
  /(?:build\s+succeeded)/i, // Build success
  /(?:all\s+tests?\s+passed)/i, // Test success

  // === PROGRESS/DOWNLOAD/INSTALL MESSAGES ===
  /(?:^(downloading|fetching|loading|installing|extracting))/i,
  /(?:^(pulling|pushing|uploading|cloning|checking\s+out))/i,
  /(?:(already|successfully)\s+(installed|downloaded|cached))/i,
  /(?:using\s+cached)/i,
  /(?:cache\s+(hit|restored|saved))/i,
  /(?:from\s+cache)/i,
  /(?:^resolving\s+)/i,

  // === PACKAGE MANAGER / BUILD TOOL NOISE ===
  /error:\s*prepare\s+script/i, // bun/npm prepare script failures (not code errors)
  /error:\s*postinstall\s+script/i, // postinstall failures
  /error:\s*preinstall\s+script/i, // preinstall failures
  /lefthook.*install/i, // lefthook git hook installer noise
  /(?:^(npm|yarn|pnpm)\s+(warn|notice|info))/i,
  /(?:^\d+\s*packages?\s+)/i, // npm package counts

  // === RETRY/RECOVERY MESSAGES ===
  /(?:retry(ing)?\s+)/i,
  /(?:attempt\s+\d+)/i,
  /(?:will\s+retry)/i,
  /(?:retrying\s+in)/i,
  /(?:connection\s+reset.*retry)/i,

  // === CI PLATFORM WORKFLOW COMMANDS ===
  // GitHub Actions: ::command::, Azure DevOps: ##[command]
  // These are CI platform annotations, not actual errors
  /(?:^::(debug|notice|warning|error|group|endgroup|set-output|save-state|add-mask)::)/i,
  /(?:^##\[)/, // Azure DevOps/GitHub Actions annotation format

  // === TEST FRAMEWORK OUTPUT ===
  /(?:^(=== RUN|=== PAUSE|=== CONT|--- PASS|--- SKIP))/i, // Go test
  /(?:^(PASS|FAIL)\s+\S+\s+[\d.]+s)/i, // Go test summary
  /(?:^ok\s+\S+\s+[\d.]+s)/i, // Go test package pass
  /(?:^\?\s+\S+\s+\[no test files\])/i, // Go no tests
  /(?:^(it|describe|test)\s*\()/i, // Jest/Mocha/Vitest
  /(?:^(PASSED|FAILED)\s*\()/i, // pytest
  /(?:^\d+\s+(passing|pending|failing)\s*$)/i, // Mocha summary

  // === COVERAGE/ANALYSIS TOOLS ===
  /(?:^coverage:)/i,
  /(?:codecov)/i,
  /(?:^(uploading|uploaded)\s+.*coverage)/i,
  /(?:\d+%\s+coverage)/i,

  // === DOCKER/CONTAINER OUTPUT ===
  /(?:^(step|layer)\s+\d+\/\d+)/i, // Docker build steps
  /(?:^#\d+\s+)/i, // Docker buildx output
  /(?:^(pulling|pushed|built))/i, // Docker operations
  /(?:^using\s+docker)/i, // Docker info
  /(?:image\s+(pulled|built|tagged))/i, // Docker image ops
  /(?:^sha256:[a-f0-9]+)/i, // Docker digests

  // === STACK TRACES (belong to parent error, not separate) ===
  /(?:^\s+at\s+)/i, // JS/TS stack traces
  /(?:^\s+File\s+".+",\s+line\s+\d+)/i, // Python traceback
  /(?:^goroutine\s+\d+\s+\[)/i, // Go goroutine header
  /^\s+\S+\.go:\d+/, // Go stack frame file
  /^\S+\([^)]*\)\s*$/, // Go stack frame function
  /(?:^traceback\s+\(most recent)/i, // Python traceback header
  /^\s{4,}/, // Heavy indentation (usually stack trace continuation)

  // === VERSION/INFO OUTPUT ===
  /(?:^(version|v)\s*[\d.]+)/i,
  /(?:^(node|npm|yarn|go|python|ruby|java)\s+v?[\d.]+)/i,
  /(?:^(using|running)\s+(node|npm|go|python|ruby|java))/i,

  // === CI PLATFORM NOISE ===
  /(?:^(run|running)\s+)/i, // GitHub Actions "Run" lines
  /(?:^\[command\])/i, // Azure DevOps
  /(?:^(starting|finished)\s+)/i, // Generic CI
  /(?:^(job|step|stage)\s+'\S+')/i, // CI job/step names
  /(?:^(added|removed|changed)\s+\d+\s+)/i, // Git diff summary
  /(?:^(time|duration|elapsed))/i, // Timing info

  // === LINTER/TOOL STATUS (not errors themselves) ===
  /(?:^(running|checking|analyzing|linting)\s+)/i,
  /(?:^(issues|problems|warnings):\s*\d+)/i,
  /(?:^found\s+\d+\s+(issue|problem|warning|error)s?)/i,
  /(?:^(level|severity)=)/i, // golangci-lint debug

  // === EMPTY/WHITESPACE/DECORATIVE ===
  /^\s*$/, // Empty lines
  /^[-=_*]{3,}\s*$/, // Horizontal rules
  /^[│├└┌┐┘┤┴┬┼]+$/, // Box drawing characters

  // === URLs AND PATHS (often contain "error" in path names) ===
  /(?:https?:\/\/\S+error)/i,
  /(?:\/errors?\/)/i, // Path containing /error/ or /errors/
  /(?:error\.(js|ts|go|py))/i, // Error module files
];

/**
 * Fast prefix strings for noise detection (lowercase for case-insensitive matching).
 * These are checked first for O(n) rejection before expensive regex operations.
 */
const fastNoisePrefixes: readonly string[] = [
  // GitHub Actions workflow commands
  "::debug::",
  "::notice::",
  "::warning::",
  "::error::",
  "::group::",
  "::endgroup::",
  "##[",
  // Common informational prefixes
  "downloading ",
  "fetching ",
  "installing ",
  "pulling ",
  "pushing ",
  "uploading ",
  "resolving ",
  "running ",
  "starting ",
  "finished ",
  "checking ",
  "analyzing ",
  "linting ",
  // Comments
  "#",
  "//",
  "/*",
  "*",
];

/**
 * Fast prefix strings for potential error detection (lowercase).
 * Lines starting with these are candidates for error matching.
 */
const fastErrorPrefixes: readonly string[] = [
  "error:",
  "fatal:",
  "panic:",
  "no such file",
  "segmentation fault",
  "killed",
  "out of memory",
  "assertion failed",
  "build failed",
  "compilation failed",
  "compile failed",
];

/**
 * Fast substring checks for noise detection (lowercase for case-insensitive matching).
 */
const fastContains: readonly string[] = [
  // Act/IO artifacts (not real errors)
  "error: eof",
  // Success indicators
  "all files pass",
  "build succeeded",
  "all tests passed",
  "completed successfully",
  // Cache indicators
  "using cached",
  "cache hit",
  "cache restored",
  "from cache",
  // Already installed/downloaded
  "already installed",
  "already downloaded",
  "successfully installed",
  "successfully downloaded",
];

/**
 * Check if a line matches any noise pattern.
 * Uses tiered approach: fast string checks before expensive regex.
 */
const matchesNoisePattern = (line: string, lowerTrimmed: string): boolean => {
  // Fast prefix check (O(n) where n = number of prefixes)
  for (const prefix of fastNoisePrefixes) {
    if (lowerTrimmed.startsWith(prefix)) {
      return true;
    }
  }

  // Fast contains check
  for (const substr of fastContains) {
    if (lowerTrimmed.includes(substr)) {
      return true;
    }
  }

  // Regex check (most expensive, checked last)
  for (const pattern of noisePatterns) {
    if (pattern.test(line)) {
      return true;
    }
  }

  return false;
};

/**
 * Quick check if a line might be an error based on fast prefix matching.
 * This is used to skip expensive regex matching for lines that clearly aren't errors.
 */
const mightBeError = (lowerTrimmed: string): boolean => {
  // Check for fast error prefixes
  for (const prefix of fastErrorPrefixes) {
    if (lowerTrimmed.startsWith(prefix)) {
      return true;
    }
  }

  // Check for file:line pattern (contains : followed by digit)
  // This is a very fast heuristic for the generic file:line:col format
  const colonIdx = lowerTrimmed.indexOf(":");
  if (colonIdx > 0 && colonIdx < lowerTrimmed.length - 1) {
    const afterColon = lowerTrimmed.charCodeAt(colonIdx + 1);
    // Check if character after : is a digit (48-57 are ASCII codes for 0-9)
    if (afterColon >= 48 && afterColon <= 57) {
      return true;
    }
  }

  // Check for bracketed error indicators
  if (
    lowerTrimmed.includes("[error]") ||
    lowerTrimmed.includes("[fatal]") ||
    lowerTrimmed.includes("[fail]")
  ) {
    return true;
  }

  return false;
};

/**
 * GenericParser implements a fallback parser for unrecognized error formats.
 * It matches lines containing common error indicators and flags them for Sentry reporting.
 *
 * Design principles:
 * 1. Very conservative - low false positive rate is critical
 * 2. Fast rejection - most lines should be rejected quickly
 * 3. Sets unknownPattern: true for telemetry/Sentry tracking
 */
class GenericParser extends BaseParser implements NoisePatternProvider {
  readonly id = PARSER_ID;
  readonly priority = PARSER_PRIORITY;

  /**
   * Returns a confidence score for parsing the given line.
   * This is intentionally very strict - we'd rather miss some errors than spam Sentry.
   *
   * Performance: Uses tiered rejection:
   * 1. Length checks (fastest)
   * 2. Fast prefix/contains checks
   * 3. Regex patterns (slowest)
   */
  canParse = (line: string, _ctx: ParseContext): number => {
    // Strip ANSI codes for consistent matching
    const stripped = stripAnsi(line);
    const trimmed = stripped.trim();

    // FAST PATH 1: Skip empty or very short lines
    if (trimmed.length < MIN_LINE_LENGTH) {
      return 0;
    }

    // FAST PATH 2: Skip very long lines (likely minified code or data dumps)
    if (trimmed.length > MAX_LINE_LENGTH) {
      return 0;
    }

    const lowerTrimmed = trimmed.toLowerCase();

    // FAST PATH 3: Quick check if this line could possibly be an error
    // This avoids expensive regex matching for most lines
    if (!mightBeError(lowerTrimmed)) {
      return 0;
    }

    // FAST PATH 4: Check if this is noise that should NEVER be flagged
    if (matchesNoisePattern(stripped, lowerTrimmed)) {
      return 0;
    }

    // SLOW PATH: Only match lines that look like REAL errors with strong structural signals
    for (const pattern of actualErrorPatterns) {
      if (pattern.test(trimmed)) {
        return 0.15; // Low score - only wins if no specific parser claims it
      }
    }

    // DO NOT match based on generic error keywords alone (too many false positives)
    // Lines like "some error occurred" or "the operation failed" without structure
    // are too ambiguous to flag as unknown patterns for Sentry.
    return 0;
  };

  /**
   * Parse the line and extract an error.
   * Only called after canParse returns > 0, so we can trust the line is valid.
   */
  parse = (line: string, ctx: ParseContext): ParseResult => {
    const stripped = stripAnsi(line);
    const trimmed = stripped.trim();

    // Defensive check - should not happen if canParse was called first
    if (trimmed.length < MIN_LINE_LENGTH) {
      return null;
    }

    // Create an error with the unknown pattern flag
    const err: MutableExtractedError = {
      message: trimmed,
      severity: "error",
      raw: line,
      category: "unknown",
      source: "generic",
      unknownPattern: true, // Flag for Sentry reporting
    };

    applyWorkflowContext(err, ctx);

    return err;
  };

  /**
   * Check if the line is noise that should be skipped.
   */
  isNoise = (line: string): boolean => {
    const stripped = stripAnsi(line);
    const lowerTrimmed = stripped.trim().toLowerCase();
    return matchesNoisePattern(stripped, lowerTrimmed);
  };

  /**
   * Returns noise patterns for registry-level optimization.
   */
  noisePatterns = (): NoisePatterns => ({
    fastPrefixes: fastNoisePrefixes,
    fastContains,
    regex: noisePatterns,
  });
}

/**
 * Create a new GenericParser instance.
 */
export const createGenericParser = (): GenericParser => new GenericParser();
