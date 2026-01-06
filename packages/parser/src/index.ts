/**
 * @detent/parser - TypeScript error extraction library
 *
 * Architecture:
 * - context/  : CI log FORMAT parsers (act, github, passthrough)
 * - parsers/  : Tool error CONTENT parsers (go, ts, python, etc.)
 * - events/   : CI event types (job, step, manifest)
 */

// ============================================================================
// Context Parsers (CI log FORMAT)
// ============================================================================

export type {
  ContextParser,
  LineContext,
  ParseLineResult,
} from "./context/index.js";

export {
  actParser,
  createActParser,
  createGitHubContextParser,
  createPassthroughParser,
  githubParser,
  passthroughParser,
} from "./context/index.js";

// ============================================================================
// CI Events (job/step lifecycle)
// ============================================================================

export type {
  JobEvent,
  JobStatus,
  ManifestEvent,
  ManifestInfo,
  ManifestJob,
  StepEvent,
  StepStatus,
} from "./events/index.js";

export { JobStatuses, StepStatuses } from "./events/index.js";

// ============================================================================
// Tool Parsers (error CONTENT)
// ============================================================================

export {
  createESLintParser,
  createGenericParser,
  createGolangParser,
  createInfrastructureParser,
  createPythonParser,
  createRustParser,
  createTypeScriptParser,
  GolangParser,
  PythonParser,
  TypeScriptParser,
} from "./parsers/index.js";

// ============================================================================
// Core Types
// ============================================================================

export type { UnknownPatternReporter } from "./extractor.js";
export type {
  NoisePatternProvider,
  NoisePatterns,
  ParseContext,
  ParseResult,
  ToolParser,
} from "./parser-types.js";
export type {
  DetectedTool,
  DetectionOptions,
  DetectionResult,
} from "./registry.js";
export type { SerializeOptions } from "./serialize.js";
export type {
  AIContext,
  CodeSnippet,
  ComprehensiveErrorGroup,
  ErrorCategory,
  ErrorReport,
  ErrorSeverity,
  ErrorSource,
  ErrorStats,
  ExtractedError,
  GroupedErrors,
  MutableExtractedError,
  OrchestratorView,
  WorkflowContext,
} from "./types.js";

// ============================================================================
// Core Utilities
// ============================================================================

export {
  createExtractor,
  Extractor,
  getUnknownPatternReporter,
  maxDeduplicationSize,
  maxLineLength,
  reportUnknownPatterns,
  setUnknownPatternReporter,
} from "./extractor.js";
export {
  applyWorkflowContext,
  BaseParser,
  cloneParseContext,
  createParseContext,
  MultiLineParser,
} from "./parser-types.js";
export {
  allSupported,
  createRegistry,
  detectAllToolsFromRun,
  detectToolFromRun,
  firstTool,
  firstToolID,
  formatUnsupportedToolsWarning,
  hasTools,
  ParserRegistry,
  unsupportedTools,
} from "./registry.js";
export type { RedactionPattern } from "./sanitize.js";
export {
  redactErrorMessage,
  redactionPatterns,
  redactPII,
  redactReport,
  redactSensitiveData,
  sanitizeForTelemetry,
} from "./sanitize.js";
export {
  formatErrorCompact,
  formatErrorsCompact,
  redactSensitive,
  serializeError,
  serializeErrorsNDJSON,
  serializeReport,
  stripAnsiFromError,
  stripAnsiFromReport,
} from "./serialize.js";
export {
  applySeverity,
  applySeverityToError,
  inferSeverity,
  withInferredSeverity,
} from "./severity.js";
export {
  DefaultContextLines,
  extractSnippet,
  extractSnippetsForErrors,
  MaxFileSize,
  MaxLineLength,
  MaxSnippetSize,
} from "./snippet.js";
export {
  AllCategories,
  cloneWorkflowContext,
  createErrorReport,
  createOrchestratorView,
  ErrorSources,
  filterByCategory,
  filterByFile,
  filterBySeverity,
  filterBySource,
  freezeError,
  groupByFile,
  groupErrors,
  isValidCategory,
  makeRelative,
} from "./types.js";
export {
  extensionToParserID,
  extractFileExtension,
  parseLocation,
  patterns,
  safeParseInt,
  splitCommands,
  stripAnsi,
} from "./utils.js";

// ============================================================================
// Default Registry Factory
// ============================================================================

import {
  createESLintParser,
  createGenericParser,
  createGolangParser,
  createInfrastructureParser,
  createPythonParser,
  createRustParser,
  createTypeScriptParser,
} from "./parsers/index.js";
import { createRegistry, type ParserRegistry } from "./registry.js";

/**
 * Create a parser registry with all default parsers registered.
 * Parsers are registered in priority order (automatic sorting by registry).
 *
 * Priority order (highest to lowest):
 * - Language-specific (80): Go, Python, Rust, TypeScript
 * - ESLint (75): JavaScript/TypeScript linting
 * - Infrastructure (70): CI/CD infrastructure failures
 * - Generic (10): Fallback for unknown formats
 */
export const createDefaultRegistry = (): ParserRegistry => {
  const registry = createRegistry();

  // Language-specific parsers (priority 80)
  registry.register(createGolangParser());
  registry.register(createPythonParser());
  registry.register(createRustParser());
  registry.register(createTypeScriptParser());

  // ESLint parser (priority 75)
  registry.register(createESLintParser());

  // Infrastructure parser (priority 70) - CI/CD failures
  registry.register(createInfrastructureParser());

  // Generic fallback parser (priority 10)
  registry.register(createGenericParser());

  // Initialize noise checker for optimized noise filtering
  registry.initNoiseChecker();

  return registry;
};

// ============================================================================
// Convenience API for Simple Usage
// ============================================================================

import { actParser, githubParser, passthroughParser } from "./context/index.js";
import { createExtractor, type Extractor } from "./extractor.js";
import type { ExtractedError } from "./types.js";

/**
 * Singleton extractor with default registry.
 * Created lazily on first use.
 */
let defaultExtractor: Extractor | undefined;

/**
 * Get the default extractor (creates it on first call).
 * Uses the default registry with all parsers and noise checker initialized.
 */
export const getDefaultExtractor = (): Extractor => {
  if (!defaultExtractor) {
    defaultExtractor = createExtractor(createDefaultRegistry());
  }
  return defaultExtractor;
};

/**
 * Parse logs using the default extractor and passthrough context parser.
 * This is the simplest way to extract errors from raw log output.
 *
 * @example
 * ```typescript
 * import { parse } from "@detent/parser";
 *
 * const errors = parse(logOutput);
 * console.log(errors); // ExtractedError[]
 * ```
 */
export const parse = (logs: string): ExtractedError[] =>
  getDefaultExtractor().extract(logs, passthroughParser);

/**
 * Parse logs from Act (local GitHub Actions runner) format.
 * Strips [Job/Step] prefixes and extracts job/step context.
 *
 * @example
 * ```typescript
 * import { parseActLogs } from "@detent/parser";
 *
 * const errors = parseActLogs(actOutput);
 * console.log(errors); // ExtractedError[] with workflowContext
 * ```
 */
export const parseActLogs = (logs: string): ExtractedError[] =>
  getDefaultExtractor().extract(logs, actParser);

/**
 * Parse logs from GitHub Actions format.
 * Strips ISO timestamps and extracts errors.
 *
 * @example
 * ```typescript
 * import { parseGitHubLogs } from "@detent/parser";
 *
 * const errors = parseGitHubLogs(githubLogs);
 * console.log(errors); // ExtractedError[]
 * ```
 */
export const parseGitHubLogs = (logs: string): ExtractedError[] =>
  getDefaultExtractor().extract(logs, githubParser);

/**
 * Reset the default extractor state.
 * Call this between parsing unrelated log outputs to clear any accumulated state.
 */
export const resetDefaultExtractor = (): void => {
  defaultExtractor?.reset();
};
