// biome-ignore-all lint/performance/noBarrelFile: This is the package's public API

/**
 * @detent/parser - TypeScript error extraction library
 * Migrated from packages/core (Go)
 */

// ============================================================================
// Types
// ============================================================================

export type {
  ContextParser,
  JobEvent,
  JobStatus,
  LineContext,
  ManifestEvent,
  ManifestInfo,
  ManifestJob,
  ParseLineResult,
  StepEvent,
  StepStatus,
} from "./ci-types.js";
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
// Constants & Utilities
// ============================================================================

export {
  JobStatuses,
  passthroughParser,
  StepStatuses,
} from "./ci-types.js";
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
// Parsers
// ============================================================================

export {
  actParser,
  createActParser,
  createESLintParser,
  createGenericParser,
  createGolangParser,
  createPythonParser,
  createRustParser,
  createTypeScriptParser,
  GolangParser,
  PythonParser,
  TypeScriptParser,
} from "./parsers/index.js";
