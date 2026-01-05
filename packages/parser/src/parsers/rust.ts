/**
 * Rust parser for rustc, Cargo, and Clippy error output.
 * Migrated from packages/core/tools/rust/parser.go
 */

import {
  applyWorkflowContext,
  MultiLineParser,
  type ParseContext,
  type ParseResult,
} from "../parser-types.js";
import type { MutableExtractedError } from "../types.js";
import { safeParseInt, stripAnsi } from "../utils.js";

// ============================================================================
// Constants
// ============================================================================

const PARSER_ID = "rust";
const PARSER_PRIORITY = 80;

/** Maximum context lines to prevent memory exhaustion */
const MAX_CONTEXT_LINES = 200;
/** Maximum context size in characters (~256KB) */
const MAX_CONTEXT_BYTES = 256 * 1024;
/** Maximum notes to accumulate */
const MAX_NOTES = 50;
/** Maximum help messages to accumulate */
const MAX_HELPS = 50;
/** Maximum bytes per note/help message to prevent memory exhaustion */
const MAX_NOTE_BYTES = 1024;

// ============================================================================
// Patterns
// ============================================================================

/**
 * Matches Rust compiler error/warning headers:
 * - error[E0308]: mismatched types
 * - warning[W0501]: unused variable
 * - error: cannot find type `Foo` in this scope
 * - warning: unused variable `x`
 * Groups: [1] level, [2] error code (optional), [3] message
 */
const rustErrorHeaderPattern = /^(error|warning)(?:\[([A-Z]\d{4})\])?:\s*(.+)$/;

/**
 * Matches Rust error location arrows:
 * - --> src/main.rs:4:7
 * - --> /path/to/file.rs:123:45
 * Groups: [1] file path, [2] line number, [3] column number
 */
const rustLocationPattern = /^\s*-->\s*([^:]+):(\d+):(\d+)$/;

/**
 * Matches note/help lines:
 * - = note: expected type `i32`
 * - = help: consider using `.to_string()`
 * Groups: [1] type (note/help), [2] message
 */
const rustNotePattern = /^\s*=\s*(note|help):\s*(.+)$/;

/**
 * Extracts Clippy lint codes from note lines:
 * - = note: `#[warn(clippy::redundant_clone)]` on by default
 * Groups: [1] lint name (e.g., "redundant_clone")
 * Note: Limited to 64 chars for lint name to prevent ReDoS
 */
const rustClippyLintPattern =
  /#\[(?:warn|deny|allow)\(clippy::([a-z_]{1,64})\)\]/;

/**
 * Matches source code line indicators:
 * |
 * 4 | let x: i32 = "hello";
 */
const rustCodeLinePattern = /^\s*\d*\s*\|/;

/**
 * Matches caret/underline lines that point to errors:
 * |     ^^^^^^^ expected `i32`, found `&str`
 */
const rustCaretPattern = /^\s*\|\s*[-^]+\s*(.*)$/;

/**
 * Matches test failure markers:
 * - test tests::test_foo ... FAILED
 * Groups: [1] test name
 */
const rustTestFailPattern = /^test\s+(\S+)\s+\.{3}\s+FAILED$/;

/**
 * Fast prefix checks for noise detection (checked first, case-sensitive after trim)
 * These are common Cargo/rustc progress messages
 */
const noiseFastPrefixes: readonly string[] = [
  "Compiling ",
  "Downloading ",
  "Downloaded ",
  "Finished ",
  "Running ",
  "Doc-tests ",
  "Caused by:",
  "Updating ",
  "Blocking ",
  "Fresh ",
  "Packaging ",
  "Verifying ",
  "Archiving ",
  "Uploading ",
  "Waiting ",
  "For more information",
  "Some errors have",
];

/**
 * Fast exact match checks (after trim)
 */
const noiseExactMatches: readonly string[] = [
  "aborting due to previous error",
  "aborting due to 2 previous errors",
];

/**
 * Noise patterns - lines that should be skipped (checked last, more expensive)
 */
const noisePatterns: readonly RegExp[] = [
  /^test result:/, // Test summary
  /^running\s+\d+\s+tests?$/, // Test count (bounded quantifier)
  /^test\s+\S+\s+\.{3}\s+ok$/, // Individual test pass (use \S+ instead of .+)
  /^aborting due to \d+ previous errors?$/, // rustc abort summary (bounded)
  /^error: could not compile `[^`]+`$/, // High-level compile fail (bounded)
  /^warning: build failed,/, // High-level build fail
];

/**
 * Clippy lint codes that should be treated as errors even though they're
 * reported as warnings. These indicate potential bugs or unsafe patterns.
 */
const criticalClippyLints: Readonly<Record<string, boolean>> = {
  unwrap_used: true, // Panics on None/Err
  expect_used: true, // Panics with message
  panic: true, // Explicit panic
  todo: true, // Unfinished code
  unimplemented: true, // Unfinished code
  unreachable: true, // Code that shouldn't execute
  indexing_slicing: true, // Can panic on out of bounds
  missing_panics_doc: true, // Missing panic documentation
  unwrap_in_result: true, // Unwrap inside Result-returning fn
  manual_assert: true, // Should use assert!
  arithmetic_side_effects: true, // Overflow/underflow
};

// ============================================================================
// Parser State Interface
// ============================================================================

interface RustParserState {
  inError: boolean;
  errorLevel: string;
  errorCode: string;
  errorMessage: string;
  errorFile: string;
  errorLine: number;
  errorColumn: number;
  contextLines: string[];
  contextByteCount: number;
  notes: string[];
  helps: string[];
  clippyLint: string;
}

// ============================================================================
// RustParser Class
// ============================================================================

/**
 * Parser for Rust compiler (rustc), Cargo, and Clippy output.
 * Handles multi-line error accumulation with context, notes, and suggestions.
 */
class RustParser extends MultiLineParser {
  readonly id = PARSER_ID;
  readonly priority = PARSER_PRIORITY;

  private state: RustParserState = {
    inError: false,
    errorLevel: "",
    errorCode: "",
    errorMessage: "",
    errorFile: "",
    errorLine: 0,
    errorColumn: 0,
    contextLines: [],
    contextByteCount: 0,
    notes: [],
    helps: [],
    clippyLint: "",
  };

  canParse = (line: string, _ctx: ParseContext): number => {
    const stripped = stripAnsi(line);

    // If already in a multi-line error, maintain high confidence
    if (this.state.inError) {
      return 0.9;
    }

    // Check for error/warning header with code (high confidence)
    const headerMatch = rustErrorHeaderPattern.exec(stripped);
    if (headerMatch) {
      // Higher confidence if it has an error code
      return headerMatch[2] ? 0.95 : 0.85;
    }

    // Location arrow is Rust-specific
    if (rustLocationPattern.test(stripped)) {
      return 0.9;
    }

    // Test failure
    if (rustTestFailPattern.test(stripped)) {
      return 0.95;
    }

    return 0;
  };

  parse = (line: string, ctx: ParseContext): ParseResult => {
    const stripped = stripAnsi(line);

    // Handle error/warning header
    const headerMatch = rustErrorHeaderPattern.exec(stripped);
    if (headerMatch) {
      const level = headerMatch[1] ?? "";
      const code = headerMatch[2] ?? "";
      const message = headerMatch[3] ?? "";

      // If we have a pending error, finalize it first
      if (this.state.inError) {
        const err = this.buildError(ctx);
        this.reset();
        this.startError(level, code, message, line);
        return err;
      }
      this.startError(level, code, message, line);
      return null; // Wait for location and context
    }

    // Handle location arrow (extract file/line/col)
    const locationMatch = rustLocationPattern.exec(stripped);
    if (locationMatch) {
      if (this.state.inError && !this.state.errorFile) {
        this.state.errorFile = locationMatch[1] ?? "";
        this.state.errorLine = safeParseInt(locationMatch[2]) ?? 0;
        this.state.errorColumn = safeParseInt(locationMatch[3]) ?? 0;
      }
      this.addContextLine(line);
      return null;
    }

    // Handle note/help lines
    const noteMatch = rustNotePattern.exec(stripped);
    if (noteMatch) {
      if (this.state.inError) {
        this.processNoteOrHelp(noteMatch[1] ?? "", noteMatch[2] ?? "");
      }
      this.addContextLine(line);
      return null;
    }

    // Handle test failure
    const testMatch = rustTestFailPattern.exec(stripped);
    if (testMatch) {
      return {
        message: `test failed: ${testMatch[1] ?? "unknown"}`,
        severity: "error",
        raw: line,
        category: "test",
        source: "rust",
      };
    }

    return null;
  };

  isNoise = (line: string): boolean => {
    const stripped = stripAnsi(line);
    const trimmed = stripped.trim();

    // Fast prefix checks first (most common case)
    for (const prefix of noiseFastPrefixes) {
      if (trimmed.startsWith(prefix)) {
        return true;
      }
    }

    // Fast exact matches
    for (const exact of noiseExactMatches) {
      if (trimmed === exact) {
        return true;
      }
    }

    // Regex patterns last (most expensive)
    for (const pattern of noisePatterns) {
      if (pattern.test(stripped)) {
        return true;
      }
    }
    return false;
  };

  continueMultiLine = (line: string, _ctx: ParseContext): boolean => {
    if (!this.state.inError) {
      return false;
    }

    const stripped = stripAnsi(line);

    // Check resource limits
    const atLimit =
      this.state.contextLines.length >= MAX_CONTEXT_LINES ||
      this.state.contextByteCount >= MAX_CONTEXT_BYTES;

    if (atLimit) {
      // Check if this line ends the error
      if (this.isErrorBoundary(stripped)) {
        return false;
      }
      return true; // Continue but don't accumulate
    }

    // Empty line might signal end of error
    if (stripped.trim() === "") {
      // If we've seen location, empty line likely ends the error
      if (this.state.errorFile) {
        return false;
      }
      // Before location, include empty lines
      this.addContextLine(line);
      return true;
    }

    // Code line with pipe (|) - continue accumulating
    if (rustCodeLinePattern.test(stripped)) {
      this.addContextLine(line);
      return true;
    }

    // Caret/underline line - continue accumulating
    if (rustCaretPattern.test(stripped)) {
      this.addContextLine(line);
      return true;
    }

    // Note/help lines - extract content and continue
    const noteMatch = rustNotePattern.exec(stripped);
    if (noteMatch) {
      this.processNoteOrHelp(noteMatch[1] ?? "", noteMatch[2] ?? "");
      this.addContextLine(line);
      return true;
    }

    // Location arrow for secondary spans
    if (rustLocationPattern.test(stripped)) {
      this.addContextLine(line);
      return true;
    }

    // New error/warning header ends current error
    if (rustErrorHeaderPattern.test(stripped)) {
      return false;
    }

    // Noise patterns end the error context
    if (this.isNoise(line)) {
      return false;
    }

    // Other lines - if we have a file location, probably end of error
    if (this.state.errorFile) {
      return false;
    }

    // Otherwise include and continue
    this.addContextLine(line);
    return true;
  };

  finishMultiLine = (ctx: ParseContext): ParseResult => {
    if (!this.state.inError) {
      return null;
    }

    const err = this.buildError(ctx);
    this.reset();
    return err;
  };

  reset = (): void => {
    this.state = {
      inError: false,
      errorLevel: "",
      errorCode: "",
      errorMessage: "",
      errorFile: "",
      errorLine: 0,
      errorColumn: 0,
      contextLines: [],
      contextByteCount: 0,
      notes: [],
      helps: [],
      clippyLint: "",
    };
  };

  // ============================================================================
  // Private Methods
  // ============================================================================

  private readonly startError = (
    level: string,
    code: string,
    message: string,
    rawLine: string
  ): void => {
    this.state.inError = true;
    this.state.errorLevel = level;
    this.state.errorCode = code;
    this.state.errorMessage = message;
    this.state.contextLines = [rawLine];
    this.state.contextByteCount = rawLine.length + 1; // +1 for newline
    this.state.notes = [];
    this.state.helps = [];
    this.state.clippyLint = "";
    this.state.errorFile = "";
    this.state.errorLine = 0;
    this.state.errorColumn = 0;
  };

  private readonly addContextLine = (line: string): void => {
    if (!this.state.inError) {
      return;
    }

    // Check resource limits
    if (
      this.state.contextLines.length >= MAX_CONTEXT_LINES ||
      this.state.contextByteCount >= MAX_CONTEXT_BYTES
    ) {
      return;
    }

    this.state.contextLines.push(line);
    this.state.contextByteCount += line.length + 1; // +1 for newline
  };

  private readonly processNoteOrHelp = (
    noteType: string,
    noteMsg: string
  ): void => {
    // Truncate excessively long messages to prevent memory exhaustion
    const truncatedMsg =
      noteMsg.length > MAX_NOTE_BYTES
        ? `${noteMsg.slice(0, MAX_NOTE_BYTES)}...`
        : noteMsg;

    if (noteType === "note") {
      if (this.state.notes.length < MAX_NOTES) {
        this.state.notes.push(truncatedMsg);
      }
      // Check for Clippy lint code in note (only if not already found)
      if (!this.state.clippyLint) {
        const lintMatch = rustClippyLintPattern.exec(noteMsg);
        if (lintMatch?.[1]) {
          this.state.clippyLint = lintMatch[1];
        }
      }
    } else if (noteType === "help" && this.state.helps.length < MAX_HELPS) {
      this.state.helps.push(truncatedMsg);
    }
  };

  private readonly buildError = (ctx: ParseContext): ParseResult => {
    if (!this.state.inError) {
      return null;
    }

    // Determine severity - only "error" or "warning" are valid
    let severity: "error" | "warning" =
      this.state.errorLevel === "warning" ? "warning" : "error";
    // Check if this Clippy lint should be treated as error
    if (
      severity === "warning" &&
      this.state.clippyLint &&
      criticalClippyLints[this.state.clippyLint]
    ) {
      severity = "error";
    }

    // Determine rule ID
    let ruleId = this.state.errorCode;
    if (this.state.clippyLint) {
      if (ruleId) {
        ruleId = `${ruleId}/clippy::${this.state.clippyLint}`;
      } else {
        ruleId = `clippy::${this.state.clippyLint}`;
      }
    }

    // Determine category
    const category = this.state.clippyLint ? "lint" : "compile";

    const stackTrace = this.state.contextLines.join("\n");

    // Build suggestions from notes and helps
    const suggestions: string[] = [];
    for (const note of this.state.notes) {
      suggestions.push(`note: ${note}`);
    }
    for (const help of this.state.helps) {
      suggestions.push(`help: ${help}`);
    }

    // Check if truncation occurred
    const truncated =
      this.state.contextLines.length >= MAX_CONTEXT_LINES ||
      this.state.contextByteCount >= MAX_CONTEXT_BYTES;

    const err: MutableExtractedError = {
      message: this.state.errorMessage,
      file: this.state.errorFile || undefined,
      line: this.state.errorLine > 0 ? this.state.errorLine : undefined,
      column: this.state.errorColumn > 0 ? this.state.errorColumn : undefined,
      severity,
      raw: stackTrace,
      stackTrace,
      ruleId: ruleId || undefined,
      category,
      source: "rust",
      suggestions: suggestions.length > 0 ? suggestions : undefined,
      lineKnown: this.state.errorLine > 0,
      columnKnown: this.state.errorColumn > 0,
      stackTraceTruncated: truncated,
    };

    applyWorkflowContext(err, ctx);

    return err;
  };

  private readonly isErrorBoundary = (stripped: string): boolean => {
    if (stripped.trim() === "") {
      return true;
    }
    if (rustErrorHeaderPattern.test(stripped)) {
      return true;
    }
    return false;
  };
}

// ============================================================================
// Factory Export
// ============================================================================

export const createRustParser = (): RustParser => new RustParser();
