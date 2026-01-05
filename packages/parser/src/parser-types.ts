/**
 * Parser interface types for tool-specific error parsers.
 * Migrated from packages/core/tools/parser/types.go
 */

import type {
  ExtractedError,
  MutableExtractedError,
  WorkflowContext,
} from "./types.js";

// ============================================================================
// Noise Detection
// ============================================================================

/**
 * NoisePatterns contains categorized noise detection patterns for optimization.
 * Fast string checks are performed before expensive regex matching.
 */
export interface NoisePatterns {
  /**
   * Fast prefixes that indicate noise (checked first, case-insensitive).
   * These should be lowercase for case-insensitive matching.
   */
  readonly fastPrefixes: readonly string[];

  /**
   * Fast substrings that indicate noise (case-insensitive).
   * These should be lowercase for case-insensitive matching.
   */
  readonly fastContains: readonly string[];

  /**
   * Regex patterns for noise detection (checked last, most expensive).
   */
  readonly regex: readonly RegExp[];
}

/**
 * NoisePatternProvider is an optional interface that parsers can implement
 * to expose their noise patterns for registry-level optimization.
 */
export interface NoisePatternProvider {
  /**
   * Returns the parser's noise detection patterns.
   * The returned struct contains fast prefix/contains checks and regex patterns.
   */
  noisePatterns(): NoisePatterns;
}

// ============================================================================
// Parse Context
// ============================================================================

/**
 * ParseContext provides shared context for tool parsers during extraction.
 * It maintains state across lines and provides contextual information.
 *
 * Note: In TypeScript (single-threaded), we don't need the same thread-safety
 * concerns as Go, but we still provide Clone() for isolation when needed.
 */
export interface ParseContext {
  /** Current workflow job name (e.g., "[CLI] Test") */
  job: string;

  /** Current step name if detected (e.g., "Run golangci-lint") */
  step: string;

  /** Detected tool ID for this step, if known from step parsing */
  tool: string;

  /**
   * Tracks the most recently seen file path for multi-line formats
   * where file paths appear on separate lines (e.g., ESLint output).
   */
  lastFile: string;

  /** Workspace root for converting absolute paths to relative */
  basePath: string;

  /** Full workflow context for error attribution */
  workflowContext?: WorkflowContext;
}

/**
 * Create a new ParseContext with the given workflow context.
 */
export const createParseContext = (
  workflowContext?: WorkflowContext
): ParseContext => ({
  job: "",
  step: "",
  tool: "",
  lastFile: "",
  basePath: "",
  workflowContext,
});

/**
 * Clone a ParseContext for isolated modifications.
 */
export const cloneParseContext = (ctx: ParseContext): ParseContext => ({
  ...ctx,
  workflowContext: ctx.workflowContext ? { ...ctx.workflowContext } : undefined,
});

/**
 * Apply workflow context from ParseContext to an ExtractedError.
 * Returns the error with workflow context applied (immutable).
 */
export const applyWorkflowContext = (
  err: MutableExtractedError,
  ctx: ParseContext | undefined
): void => {
  if (!ctx?.workflowContext) {
    return;
  }
  err.workflowContext = { ...ctx.workflowContext };
};

// ============================================================================
// Tool Parser Interface
// ============================================================================

/**
 * ParseResult represents the result of parsing a line.
 * null means the line doesn't contain a parseable error.
 */
export type ParseResult = ExtractedError | null;

/**
 * ToolParser defines the interface for tool-specific error parsers.
 * Each tool (Go, ESLint, TypeScript, etc.) implements this interface
 * to handle its specific output format and error patterns.
 */
export interface ToolParser {
  /**
   * Unique identifier for this parser (e.g., "go", "eslint", "typescript").
   */
  readonly id: string;

  /**
   * Parse order priority. Higher values are tried first.
   * Recommended ranges:
   *   90-100: Very specific parsers (exact format match)
   *   70-89:  Specific parsers (language-specific format)
   *   50-69:  General parsers (common patterns)
   *   0-49:   Fallback parsers (last resort)
   */
  readonly priority: number;

  /**
   * Returns a confidence score (0.0-1.0) indicating how likely
   * this parser can handle the given line. Higher scores indicate more
   * confidence. Returns 0 if the line doesn't match this parser's format.
   */
  canParse(line: string, ctx: ParseContext): number;

  /**
   * Extracts an error from the line.
   * Returns null if the line doesn't contain a parseable error.
   */
  parse(line: string, ctx: ParseContext): ParseResult;

  /**
   * Returns true if the line is tool-specific noise that should be skipped.
   * Examples: progress indicators, timing info, decorative output.
   */
  isNoise(line: string): boolean;

  /**
   * Returns true if this parser handles multi-line errors
   * (e.g., Python tracebacks, Go panics, Rust error messages).
   */
  supportsMultiLine(): boolean;

  /**
   * Processes a continuation line for multi-line errors.
   * Returns true if the line was consumed as part of the multi-line error,
   * false if it signals the end of the multi-line sequence.
   * Only called when supportsMultiLine() returns true and an error is in progress.
   */
  continueMultiLine(line: string, ctx: ParseContext): boolean;

  /**
   * Finalizes the current multi-line error and returns it.
   * Called when continueMultiLine returns false or when input ends.
   */
  finishMultiLine(ctx: ParseContext): ParseResult;

  /**
   * Clears any accumulated multi-line state.
   * Called between parsing runs or when switching context.
   */
  reset(): void;
}

// ============================================================================
// Base Parser Implementation
// ============================================================================

/**
 * Abstract base class for parsers that provides default implementations
 * for multi-line methods. Most simple parsers can extend this.
 */
export abstract class BaseParser implements ToolParser {
  abstract readonly id: string;
  abstract readonly priority: number;

  abstract canParse(line: string, ctx: ParseContext): number;
  abstract parse(line: string, ctx: ParseContext): ParseResult;
  abstract isNoise(line: string): boolean;

  supportsMultiLine(): boolean {
    return false;
  }

  continueMultiLine(_line: string, _ctx: ParseContext): boolean {
    return false;
  }

  finishMultiLine(_ctx: ParseContext): ParseResult {
    return null;
  }

  reset(): void {
    // No-op for simple parsers
  }
}

/**
 * Abstract base class for parsers with multi-line support.
 */
export abstract class MultiLineParser implements ToolParser {
  abstract readonly id: string;
  abstract readonly priority: number;

  abstract canParse(line: string, ctx: ParseContext): number;
  abstract parse(line: string, ctx: ParseContext): ParseResult;
  abstract isNoise(line: string): boolean;
  abstract continueMultiLine(line: string, ctx: ParseContext): boolean;
  abstract finishMultiLine(ctx: ParseContext): ParseResult;
  abstract reset(): void;

  supportsMultiLine(): boolean {
    return true;
  }
}
