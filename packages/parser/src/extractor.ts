/**
 * Main extraction engine for parsing CI output.
 * Migrated from packages/core/extract/extractor.go
 */

import type { ContextParser, ParseLineResult } from "./ci-types.js";
import type { ParseContext, ToolParser } from "./parser-types.js";
import type { ParserRegistry } from "./registry.js";
import { sanitizeForTelemetry } from "./sanitize.js";
import type { ExtractedError, WorkflowContext } from "./types.js";
import { cloneWorkflowContext } from "./types.js";

// ============================================================================
// Constants
// ============================================================================

/** Maximum line length to prevent ReDoS on extremely long lines */
export const maxLineLength = 65_536; // 64KB per line

/** Maximum deduplicated errors to prevent unbounded map growth */
export const maxDeduplicationSize = 10_000;

/** Limit unknown pattern reports to prevent telemetry spam */
const maxUnknownPatternsToReport = 10;

/** Truncate long lines in telemetry reports */
const maxUnknownPatternLineLength = 500;

// ============================================================================
// Error Deduplication Key
// ============================================================================

/**
 * Create a unique key for error deduplication.
 */
const createErrKey = (
  message: string,
  file: string | undefined,
  line: number | undefined
): string => `${message}|${file ?? ""}|${line ?? 0}`;

// ============================================================================
// Extractor Class
// ============================================================================

/**
 * Extractor uses the tool registry to extract errors from CI output.
 * It delegates to tool-specific parsers for precise pattern matching.
 */
export class Extractor {
  private readonly registry: ParserRegistry;
  private currentWorkflowCtx: WorkflowContext | undefined;

  constructor(registry: ParserRegistry) {
    this.registry = registry;
  }

  /**
   * Extract parses CI output using the tool registry for error extraction.
   * It uses tool-specific parsers (Go, TypeScript, Rust, ESLint, etc.) for precise pattern matching
   * and falls back to the generic parser for unrecognized patterns.
   *
   * The extraction strategy is:
   * 1. Try tool-specific parsers first (better multi-line handling, more precise)
   * 2. Fall back to generic parser for patterns not matched by dedicated parsers
   * 3. Deduplicate results to avoid duplicates
   */
  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: main extraction loop requires handling multi-line parsers, context tracking, deduplication, and error recovery in a single pass
  extract(output: string, ctxParser: ContextParser): ExtractedError[] {
    const extracted: ExtractedError[] = [];
    // Use Set for O(1) lookup - more efficient than Map<string, boolean>
    const seen = new Set<string>();

    // Create parse context for tool parsers
    const parseCtx: ParseContext = {
      job: "",
      step: "",
      tool: "",
      lastFile: "",
      basePath: "",
      workflowContext: this.currentWorkflowCtx,
    };

    // Track active multi-line parser
    let activeParser: ToolParser | undefined;

    // Helper to add error with deduplication
    // SECURITY: Once deduplication limit is reached, stop adding errors entirely
    // to prevent memory exhaustion from malicious input with many unique errors
    const addError = (err: ExtractedError): void => {
      if (seen.size >= maxDeduplicationSize) {
        return;
      }
      const key = createErrKey(err.message, err.file, err.line);
      if (!seen.has(key)) {
        seen.add(key);
        extracted.push(err);
      }
    };

    // Process line by line
    const lines = output.split("\n");

    for (const line of lines) {
      // Skip extremely long lines to prevent ReDoS
      if (line.length > maxLineLength) {
        continue;
      }

      // Use the context parser to extract CI context and clean the line
      // SECURITY: Wrap in try-catch to handle malformed input gracefully
      let parseResult: ParseLineResult | null = null;
      try {
        parseResult = ctxParser.parseLine(line);
      } catch {
        // Skip lines that cause parsing errors
        continue;
      }
      const { ctx, cleanLine, skip } = parseResult;
      if (skip) {
        continue;
      }

      // Convert CI context to workflow context
      if (ctx.job) {
        this.currentWorkflowCtx = {
          job: ctx.job,
          step: ctx.step || undefined,
        };
        parseCtx.workflowContext = this.currentWorkflowCtx;
        parseCtx.job = ctx.job;
        parseCtx.step = ctx.step;
      }

      // Check if registry considers this line as noise
      if (this.registry.isNoise(cleanLine)) {
        continue;
      }

      let found: ExtractedError | null = null;

      // If we have an active multi-line parser, try to continue
      // Note: activeParser is only set when supportsMultiLine() is true
      if (activeParser) {
        if (activeParser.continueMultiLine(cleanLine, parseCtx)) {
          continue; // Line consumed by multi-line parser
        }
        // Multi-line sequence ended, finalize it
        found = activeParser.finishMultiLine(parseCtx);
        activeParser = undefined;
      }

      // Try to find a parser for this line
      if (!found) {
        const parser = this.registry.findParser(cleanLine, parseCtx);
        if (parser) {
          found = parser.parse(cleanLine, parseCtx);

          // Check if this parser starts a multi-line sequence
          if (!found && parser.supportsMultiLine()) {
            // Parser may have started accumulating but not returned an error yet
            activeParser = parser;
          }
        }
      }

      if (found) {
        // Apply workflow context if not already set
        const errWithCtx = applyContextToError(found, this.currentWorkflowCtx);
        addError(errWithCtx);
      }
    }

    // Finalize any pending multi-line parser
    if (activeParser) {
      const found = activeParser.finishMultiLine(parseCtx);
      if (found) {
        const errWithCtx = applyContextToError(found, this.currentWorkflowCtx);
        addError(errWithCtx);
      }
    }

    return extracted;
  }

  /**
   * Reset clears any accumulated state in the extractor and all parsers.
   */
  reset(): void {
    this.currentWorkflowCtx = undefined;
    this.registry.resetAll();
  }

  /**
   * Get the current workflow context.
   */
  getWorkflowContext(): WorkflowContext | undefined {
    return this.currentWorkflowCtx;
  }
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Apply workflow context to an error if not already set.
 */
const applyContextToError = (
  err: ExtractedError,
  ctx: WorkflowContext | undefined
): ExtractedError => {
  if (err.workflowContext || !ctx) {
    return err;
  }
  return {
    ...err,
    workflowContext: cloneWorkflowContext(ctx),
    workflowJob: ctx.job,
  };
};

// ============================================================================
// Unknown Pattern Reporting
// ============================================================================

/**
 * UnknownPatternReporter is a callback for reporting unknown error patterns.
 * This allows CLI to inject telemetry (e.g., Sentry) without parser depending on it.
 * The callback receives a slice of sanitized pattern strings.
 */
export type UnknownPatternReporter = (patterns: string[]) => void;

/**
 * Default unknown pattern reporter (no-op).
 * Can be overridden by CLI to inject Sentry or other telemetry.
 */
let defaultUnknownPatternReporter: UnknownPatternReporter | undefined;

/**
 * Set the default unknown pattern reporter.
 */
export const setUnknownPatternReporter = (
  reporter: UnknownPatternReporter | undefined
): void => {
  defaultUnknownPatternReporter = reporter;
};

/**
 * Get the current unknown pattern reporter.
 */
export const getUnknownPatternReporter = ():
  | UnknownPatternReporter
  | undefined => defaultUnknownPatternReporter;

/**
 * Report unknown error patterns for later analysis.
 * This helps identify new error formats that should be added as dedicated parsers.
 * Patterns are sanitized before being passed to the reporter callback.
 */
export const reportUnknownPatterns = (
  errors: readonly ExtractedError[]
): void => {
  if (!defaultUnknownPatternReporter) {
    return;
  }

  const unknownPatterns: string[] = [];

  for (const err of errors) {
    if (
      err.unknownPattern &&
      unknownPatterns.length < maxUnknownPatternsToReport
    ) {
      let raw = err.raw ?? err.message;
      if (raw.length > maxUnknownPatternLineLength) {
        raw = `${raw.slice(0, maxUnknownPatternLineLength)}...`;
      }
      // Sanitize the pattern to remove potential PII before adding to list
      // The pattern structure is what matters for creating new parsers, not the actual values
      const sanitized = sanitizeForTelemetry(raw);
      unknownPatterns.push(sanitized);
    }
  }

  if (unknownPatterns.length === 0) {
    return;
  }

  defaultUnknownPatternReporter(unknownPatterns);
};

// ============================================================================
// Factory
// ============================================================================

/**
 * Create a new Extractor with the given registry.
 */
export const createExtractor = (registry: ParserRegistry): Extractor =>
  new Extractor(registry);
