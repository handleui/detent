/**
 * TypeScript compiler (tsc) error parser.
 * Migrated from packages/core/tools/typescript/parser.go
 *
 * Supported formats:
 * - Parenthesized: file.ts(line,col): error TSxxxx: message
 * - Colon-separated (pretty mode): file.ts:line:col - error TSxxxx: message
 * - Global errors (no location): error TS5023: Unknown compiler option 'foo'.
 *
 * Error code categories:
 * - TS1xxx: Syntax errors -> compile
 * - TS2xxx: Semantic errors -> type-check
 * - TS5xxx: Compiler options -> config
 * - TS6xxx: Message catalog -> metadata
 * - TS7xxx: Strict null checks -> type-check
 */

import {
  applyWorkflowContext,
  MultiLineParser,
  type NoisePatternProvider,
  type NoisePatterns,
  type ParseContext,
  type ParseResult,
} from "../parser-types.js";
import type {
  ErrorCategory,
  ErrorSeverity,
  MutableExtractedError,
} from "../types.js";
import { safeParseInt, stripAnsi } from "../utils.js";

// ============================================================================
// Constants
// ============================================================================

const PARSER_ID = "typescript";
const PARSER_PRIORITY = 80;

/**
 * Maximum line length to process. Lines longer than this are likely
 * malformed or minified code, not error output. Prevents ReDoS attacks.
 */
const MAX_LINE_LENGTH = 4096;

/**
 * Maximum message length to extract. Prevents memory issues from
 * extremely long error messages.
 */
const MAX_MESSAGE_LENGTH = 1024;

/** Maximum context lines for multi-line errors */
const MAX_CONTEXT_LINES = 50;

/** Maximum suggestions to accumulate */
const MAX_SUGGESTIONS = 20;

// ============================================================================
// Patterns
// ============================================================================

/**
 * File extension pattern for TypeScript files.
 * Matches: .ts, .tsx, .mts, .cts, .d.ts, .d.tsx, .d.mts, .d.cts
 *
 * Explanation:
 * - (?:d\.)? - Optional declaration file prefix
 * - [cm]? - Optional ESM (m) or CommonJS (c) indicator
 * - tsx? - ts or tsx
 */
const TS_EXT_PATTERN = "(?:d\\.)?[cm]?tsx?";

/**
 * TypeScript error pattern matching tsc parenthesized output.
 * Format: file.ts(line,col): error TSxxxx: message
 * Or: file.ts(line,col): warning TSxxxx: message
 * Or without error code: file.ts(line,col): message
 *
 * The pattern handles:
 * - Relative paths: src/app.ts, components/Button.tsx
 * - Absolute Unix paths: /home/user/project/app.ts
 * - Absolute Windows paths: C:\Users\project\app.ts
 * - Paths with dots/dashes/underscores: src/v2.0/my-component_test.tsx
 * - Declaration files: types/index.d.ts, globals.d.tsx, types.d.mts
 * - ES Module/CommonJS TypeScript files: src/app.mts, src/app.cts
 *
 * Groups:
 *   1: file path (e.g., "src/app.ts", "components/Button.tsx")
 *   2: line number
 *   3: column number
 *   4: severity (error or warning, optional)
 *   5: TS error code (optional, e.g., "TS2749")
 *   6: error message
 *
 * Security: Uses bounded lengths to prevent ReDoS.
 */
const tsParenErrorPattern = new RegExp(
  `^([^\\s(]{1,1024}\\.${TS_EXT_PATTERN})\\((\\d{1,10}),(\\d{1,10})\\):\\s*(?:(error|warning)\\s+(TS\\d{1,5}):\\s*)?(.{1,2048})\\s*$`
);

/**
 * TypeScript error pattern matching tsc pretty/colon-separated output.
 * Format: file.ts:line:col - error TSxxxx: message
 * Or: file.ts:line:col - warning TSxxxx: message
 *
 * Groups:
 *   1: file path
 *   2: line number
 *   3: column number
 *   4: severity (error or warning)
 *   5: TS error code (optional)
 *   6: error message
 *
 * Security: Uses bounded lengths to prevent ReDoS.
 */
const tsColonErrorPattern = new RegExp(
  `^([^\\s:]{1,1024}\\.${TS_EXT_PATTERN}):(\\d{1,10}):(\\d{1,10})\\s+-\\s+(error|warning)\\s+(?:(TS\\d{1,5}):\\s*)?(.{1,2048})\\s*$`
);

/**
 * TypeScript global error pattern (no file location).
 * Format: error TSxxxx: message
 * Used for compiler option errors, config errors, etc.
 *
 * Groups:
 *   1: severity (error or warning)
 *   2: TS error code
 *   3: error message
 *
 * Security: Requires error code to avoid false positives.
 */
const tsGlobalErrorPattern =
  /^(error|warning)\s+(TS\d{1,5}):\s*(.{1,2048})\s*$/;

/**
 * Suggestion pattern: "Did you mean 'X'?"
 * Extracts the suggested alternative.
 * Group 1: The suggested name
 */
const didYouMeanPattern = /Did you mean ['"]([^'"]+)['"]\?/i;

/**
 * Related information pattern in pretty output.
 * Matches lines like: "The expected type comes from property 'foo' which is declared here"
 */
const relatedInfoPattern =
  /^\s{2,}(?:The expected type comes from|'[^']+' is declared here|This type is not compatible)/;

/**
 * Code context line in pretty output (shows source code with line number).
 * Example: "10   const x = foo;"
 * Group 1: line number
 * Group 2: code content
 */
const codeContextLinePattern = /^(\d{1,10})\s{1,5}(.+)$/;

/**
 * Error pointer line (tildes or carets pointing to error location).
 * Example: "         ~~~"
 */
const errorPointerPattern = /^\s+[~^]+\s*$/;

// ============================================================================
// Error Code Category Mapping
// ============================================================================

/**
 * Map TypeScript error code to appropriate error category.
 * TS error codes follow a numbering scheme:
 * - TS1xxx: Syntax/parsing errors -> compile
 * - TS2xxx: Semantic errors -> type-check
 * - TS3xxx: Declaration emit -> compile
 * - TS4xxx: Mapper errors -> compile
 * - TS5xxx: Compiler options -> config
 * - TS6xxx: Message catalog/info -> metadata
 * - TS7xxx: Strict null checks -> type-check
 * - TS8xxx: Build optimizations -> compile
 * - TS9xxx: Reserved/special -> unknown
 */
const getErrorCategory = (ruleId: string | undefined): ErrorCategory => {
  if (!ruleId?.startsWith("TS")) {
    return "type-check";
  }

  const codeStr = ruleId.slice(2);
  const codeNum = Number.parseInt(codeStr, 10);
  if (Number.isNaN(codeNum)) {
    return "type-check";
  }

  // Extract the category (first digit indicates category)
  const category = Math.floor(codeNum / 1000);

  switch (category) {
    case 1:
      return "compile"; // Syntax errors
    case 2:
      return "type-check"; // Semantic/type errors
    case 3:
      return "compile"; // Declaration emit
    case 4:
      return "compile"; // Mapper errors
    case 5:
      return "config"; // Compiler options
    case 6:
      return "metadata"; // Message catalog
    case 7:
      return "type-check"; // Strict null checks
    case 8:
      return "compile"; // Build optimizations
    default:
      return "type-check"; // Default for unknown codes
  }
};

/**
 * Truncate message if too long, marking truncation.
 */
const truncateMessage = (msg: string): [string, boolean] => {
  if (msg.length <= MAX_MESSAGE_LENGTH) {
    return [msg, false];
  }
  return [`${msg.slice(0, MAX_MESSAGE_LENGTH - 3)}...`, true];
};

/**
 * Extract suggestion from "Did you mean 'X'?" patterns.
 */
const extractSuggestion = (message: string): string | undefined => {
  const match = didYouMeanPattern.exec(message);
  return match?.[1];
};

// ============================================================================
// Noise Detection Patterns
// ============================================================================

/**
 * Fast prefix checks for noise detection (lowercase for case-insensitive matching).
 * Checked first for performance before falling back to regex patterns.
 */
const NOISE_FAST_PREFIXES: readonly string[] = [
  "starting compilation",
  "file change detected",
  "watching for file changes",
  "found ",
  "version ",
  "message ts",
  "projects in this build",
  "building project",
  "updating output",
  "skipping build",
  "project '",
  // Build mode prefixes
  "info ts",
  "info: ",
  // JSON output markers
  '{"',
  "[{",
];

/**
 * Fast substring checks for noise detection (lowercase).
 */
const NOISE_FAST_CONTAINS: readonly string[] = [
  // Build mode related
  "up to date",
  "successfully completed",
];

/**
 * Noise patterns - lines that should be skipped as TypeScript-specific noise.
 * These are checked after fast prefix/contains checks fail.
 */
const NOISE_REGEX_PATTERNS: readonly RegExp[] = [
  // Watch mode output - timestamp prefix [HH:MM:SS AM/PM]
  /^\[\d{1,2}:\d{2}:\d{2}\s*(?:AM|PM)?\]/i,

  // Build mode project entry: * path/to/tsconfig.json
  /^\s*\*\s+.+tsconfig.*\.json/i,

  // Build mode: Project 'path' is up to date
  /^Project\s+'.+'\s+is\s+(up to date|out of date)/i,

  // Build mode: Building project 'path'
  /^Building project\s+'/i,

  // Empty and whitespace
  /^\s*$/,

  // Summary lines: "Found X errors in Y files"
  /^Found\s+\d+\s+errors?\s+in\s+\d+/i,

  // Summary lines: "Found X errors."
  /^Found\s+\d+\s+errors?\.?\s*$/i,

  // JSON output format (array of diagnostics)
  /^\s*\[\s*\{.*"code":\s*\d+/,
];

// ============================================================================
// Parser State Interface
// ============================================================================

interface TypeScriptParserState {
  inError: boolean;
  file: string | undefined;
  line: number | undefined;
  column: number | undefined;
  message: string;
  ruleId: string | undefined;
  severity: ErrorSeverity;
  contextLines: string[];
  suggestions: string[];
  messageTruncated: boolean;
}

// ============================================================================
// TypeScript Parser
// ============================================================================

/**
 * Parser for TypeScript compiler (tsc) output.
 *
 * Handles multiple formats:
 * - Parenthesized: file.ts(line,col): error TSxxxx: message
 * - Colon-separated (pretty mode): file.ts:line:col - error TSxxxx: message
 * - Global errors: error TS5023: Unknown compiler option 'foo'.
 *
 * Supports multi-line pretty output with code context and related information.
 */
export class TypeScriptParser
  extends MultiLineParser
  implements NoisePatternProvider
{
  readonly id = PARSER_ID;
  readonly priority = PARSER_PRIORITY;

  private state: TypeScriptParserState = this.createEmptyState();

  private createEmptyState(): TypeScriptParserState {
    return {
      inError: false,
      file: undefined,
      line: undefined,
      column: undefined,
      message: "",
      ruleId: undefined,
      severity: "error",
      contextLines: [],
      suggestions: [],
      messageTruncated: false,
    };
  }

  /**
   * Returns confidence score for parsing the line.
   * Returns 0.9-0.95 for TSC-style errors, 0 otherwise.
   */
  canParse(line: string, _ctx: ParseContext): number {
    // Skip overly long lines (security: prevent ReDoS)
    if (line.length > MAX_LINE_LENGTH) {
      return 0;
    }

    const stripped = stripAnsi(line);

    // If already in a multi-line error, maintain confidence for context lines
    // Check if this is a continuation line (code context, pointer, or related info)
    if (
      this.state.inError &&
      (codeContextLinePattern.test(stripped) ||
        errorPointerPattern.test(stripped) ||
        relatedInfoPattern.test(stripped) ||
        stripped.trim() === "")
    ) {
      return 0.85;
    }

    // Global error (no file location) - highest specificity due to "error TS" prefix
    if (tsGlobalErrorPattern.test(stripped)) {
      return 0.95;
    }

    // Colon-separated format (tsc --pretty or some editors)
    if (tsColonErrorPattern.test(stripped)) {
      return 0.92;
    }

    // Parenthesized format is most common (default tsc output)
    if (tsParenErrorPattern.test(stripped)) {
      return 0.9;
    }

    return 0;
  }

  /**
   * Extract an error from the line.
   * Returns null if the line doesn't contain a parseable error.
   */
  parse(line: string, ctx: ParseContext): ParseResult {
    // Skip overly long lines
    if (line.length > MAX_LINE_LENGTH) {
      return null;
    }

    const stripped = stripAnsi(line);

    // Try global error format (no file location)
    const globalMatch = tsGlobalErrorPattern.exec(stripped);
    if (globalMatch) {
      // If we have a pending error, finalize it first
      if (this.state.inError) {
        const err = this.buildError(ctx);
        this.reset();
        this.startGlobalError(globalMatch, line);
        return err;
      }
      return this.buildGlobalError(globalMatch, line, ctx);
    }

    // Try colon-separated format (pretty mode)
    const colonMatch = tsColonErrorPattern.exec(stripped);
    if (colonMatch) {
      // If we have a pending error, finalize it first
      if (this.state.inError) {
        const err = this.buildError(ctx);
        this.reset();
        this.startFileError(colonMatch, line, "colon");
        return err;
      }
      this.startFileError(colonMatch, line, "colon");
      return null; // Wait for potential multi-line context
    }

    // Try parenthesized format
    const parenMatch = tsParenErrorPattern.exec(stripped);
    if (parenMatch) {
      // If we have a pending error, finalize it first
      if (this.state.inError) {
        const err = this.buildError(ctx);
        this.reset();
        this.startFileError(parenMatch, line, "paren");
        return err;
      }
      // Parenthesized format typically doesn't have multi-line context
      return this.buildFileError(parenMatch, line, ctx, "paren");
    }

    return null;
  }

  /**
   * Returns true if the line is TypeScript-specific noise that should be skipped.
   */
  isNoise(line: string): boolean {
    // Skip overly long lines for performance
    if (line.length > MAX_LINE_LENGTH) {
      return false;
    }

    const stripped = stripAnsi(line);
    const lowerStripped = stripped.toLowerCase();

    // Fast prefix check first (most common noise patterns)
    for (const prefix of NOISE_FAST_PREFIXES) {
      if (lowerStripped.startsWith(prefix)) {
        return true;
      }
    }

    // Fast substring check
    for (const substr of NOISE_FAST_CONTAINS) {
      if (lowerStripped.includes(substr)) {
        return true;
      }
    }

    // Fall back to regex patterns (only for patterns that can't be prefix/contains)
    for (const pattern of NOISE_REGEX_PATTERNS) {
      if (pattern.test(stripped)) {
        return true;
      }
    }

    return false;
  }

  /**
   * Check if a line continues the current multi-line error.
   */
  continueMultiLine(line: string, _ctx: ParseContext): boolean {
    if (!this.state.inError) {
      return false;
    }

    // Skip overly long lines
    if (line.length > MAX_LINE_LENGTH) {
      return false;
    }

    const stripped = stripAnsi(line);

    // Check resource limits
    if (this.state.contextLines.length >= MAX_CONTEXT_LINES) {
      // Check if this starts a new error
      if (this.isNewErrorStart(stripped)) {
        return false;
      }
      return true; // Continue but don't accumulate
    }

    // Empty line might be part of pretty output spacing
    if (stripped.trim() === "") {
      // Keep going if we're in the middle of context
      if (this.state.contextLines.length > 0) {
        this.addContextLine(line);
        return true;
      }
      return false;
    }

    // New error starts a new block
    if (this.isNewErrorStart(stripped)) {
      return false;
    }

    // Code context line (line number followed by code)
    if (codeContextLinePattern.test(stripped)) {
      this.addContextLine(line);
      return true;
    }

    // Error pointer line (tildes or carets)
    if (errorPointerPattern.test(stripped)) {
      this.addContextLine(line);
      return true;
    }

    // Related information lines
    if (relatedInfoPattern.test(stripped)) {
      this.addContextLine(line);
      // Extract as additional context but not as suggestion
      return true;
    }

    // Noise patterns end the context
    if (this.isNoise(line)) {
      return false;
    }

    // Other lines might be continuation - be conservative
    return false;
  }

  /**
   * Finalize the current multi-line error.
   */
  finishMultiLine(ctx: ParseContext): ParseResult {
    if (!this.state.inError) {
      return null;
    }

    const err = this.buildError(ctx);
    this.reset();
    return err;
  }

  /**
   * Reset parser state.
   */
  reset(): void {
    this.state = this.createEmptyState();
  }

  /**
   * Returns noise patterns for registry-level optimization.
   */
  noisePatterns(): NoisePatterns {
    return {
      fastPrefixes: NOISE_FAST_PREFIXES,
      fastContains: NOISE_FAST_CONTAINS,
      regex: NOISE_REGEX_PATTERNS,
    };
  }

  // ============================================================================
  // Private Methods
  // ============================================================================

  private isNewErrorStart(stripped: string): boolean {
    return (
      tsGlobalErrorPattern.test(stripped) ||
      tsColonErrorPattern.test(stripped) ||
      tsParenErrorPattern.test(stripped)
    );
  }

  private addContextLine(line: string): void {
    if (this.state.contextLines.length < MAX_CONTEXT_LINES) {
      this.state.contextLines.push(line);
    }
  }

  private startGlobalError(match: RegExpExecArray, rawLine: string): void {
    const severity = (match[1]?.toLowerCase() ?? "error") as ErrorSeverity;
    const ruleId = match[2];
    const rawMessage = match[3] ?? "";
    const [message, truncated] = truncateMessage(rawMessage.trim());

    // Extract suggestion from message
    const suggestion = extractSuggestion(rawMessage);

    this.state = {
      inError: true,
      file: undefined,
      line: undefined,
      column: undefined,
      message,
      ruleId,
      severity,
      contextLines: [rawLine],
      suggestions: suggestion ? [suggestion] : [],
      messageTruncated: truncated,
    };
  }

  private startFileError(
    match: RegExpExecArray,
    rawLine: string,
    _format: "colon" | "paren"
  ): void {
    // Groups differ between formats:
    // Colon: 1=file, 2=line, 3=col, 4=severity, 5=code, 6=message
    // Paren: 1=file, 2=line, 3=col, 4=severity, 5=code, 6=message
    const file = match[1];
    const lineNum = safeParseInt(match[2]);
    const colNum = safeParseInt(match[3]);
    const severity = (match[4]?.toLowerCase() ?? "error") as ErrorSeverity;
    const ruleId = match[5];
    const rawMessage = match[6] ?? "";
    const [message, truncated] = truncateMessage(rawMessage.trim());

    // Extract suggestion from message
    const suggestion = extractSuggestion(rawMessage);

    this.state = {
      inError: true,
      file,
      line: lineNum,
      column: colNum,
      message,
      ruleId,
      severity,
      contextLines: [rawLine],
      suggestions: suggestion ? [suggestion] : [],
      messageTruncated: truncated,
    };
  }

  private buildGlobalError(
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext
  ): MutableExtractedError {
    const severity = (match[1]?.toLowerCase() ?? "error") as ErrorSeverity;
    const ruleId = match[2];
    const rawMessage = match[3] ?? "";
    const [message, truncated] = truncateMessage(rawMessage.trim());

    // Extract suggestion from message
    const suggestion = extractSuggestion(rawMessage);
    const suggestions =
      suggestion && suggestion.length > 0 ? [suggestion] : undefined;

    const err: MutableExtractedError = {
      message,
      severity,
      ruleId,
      category: getErrorCategory(ruleId),
      source: "typescript",
      raw: rawLine,
      lineKnown: false,
      columnKnown: false,
      messageTruncated: truncated || undefined,
      suggestions,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private buildFileError(
    match: RegExpExecArray,
    rawLine: string,
    ctx: ParseContext,
    _format: "colon" | "paren"
  ): MutableExtractedError {
    const file = match[1];
    const lineNum = safeParseInt(match[2]);
    const colNum = safeParseInt(match[3]);
    const severity = (match[4]?.toLowerCase() ?? "error") as ErrorSeverity;
    const ruleId = match[5];
    const rawMessage = match[6] ?? "";
    const [message, truncated] = truncateMessage(rawMessage.trim());

    // Extract suggestion from message
    const suggestion = extractSuggestion(rawMessage);
    const suggestions =
      suggestion && suggestion.length > 0 ? [suggestion] : undefined;

    const err: MutableExtractedError = {
      message,
      file,
      line: lineNum,
      column: colNum,
      severity,
      ruleId,
      category: getErrorCategory(ruleId),
      source: "typescript",
      raw: rawLine,
      lineKnown: lineNum !== undefined && lineNum > 0,
      columnKnown: colNum !== undefined && colNum > 0,
      messageTruncated: truncated || undefined,
      suggestions,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }

  private buildError(ctx: ParseContext): MutableExtractedError {
    const { state } = this;

    // Combine suggestions (limit to MAX_SUGGESTIONS)
    const suggestions =
      state.suggestions.length > 0
        ? state.suggestions.slice(0, MAX_SUGGESTIONS)
        : undefined;

    // Build stack trace from context lines if we have multiple
    const stackTrace =
      state.contextLines.length > 1 ? state.contextLines.join("\n") : undefined;

    const err: MutableExtractedError = {
      message: state.message,
      file: state.file,
      line: state.line,
      column: state.column,
      severity: state.severity,
      ruleId: state.ruleId,
      category: getErrorCategory(state.ruleId),
      source: "typescript",
      raw: state.contextLines[0] ?? "",
      stackTrace,
      lineKnown: state.line !== undefined && state.line > 0,
      columnKnown: state.column !== undefined && state.column > 0,
      messageTruncated: state.messageTruncated || undefined,
      suggestions,
    };

    applyWorkflowContext(err, ctx);
    return err;
  }
}

/**
 * Factory function to create a TypeScript parser instance.
 */
export const createTypeScriptParser = (): TypeScriptParser =>
  new TypeScriptParser();
