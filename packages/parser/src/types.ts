/**
 * Core types for error extraction and representation.
 * Migrated from packages/core/errors/types.go
 */

// ============================================================================
// Error Categories
// ============================================================================

/**
 * ErrorCategory represents the type of error for categorization and AI prompt generation.
 */
export type ErrorCategory =
  | "lint"
  | "type-check"
  | "test"
  | "compile"
  | "runtime"
  | "metadata"
  | "security"
  | "dependency"
  | "config"
  | "infrastructure"
  | "docs"
  | "unknown";

/**
 * All defined error categories.
 */
export const AllCategories: readonly ErrorCategory[] = [
  "lint",
  "type-check",
  "test",
  "compile",
  "runtime",
  "metadata",
  "security",
  "dependency",
  "config",
  "infrastructure",
  "docs",
  "unknown",
] as const;

/**
 * Check if a string is a valid error category.
 */
export const isValidCategory = (cat: string): cat is ErrorCategory =>
  AllCategories.includes(cat as ErrorCategory);

// ============================================================================
// Error Sources
// ============================================================================

/**
 * Error sources for attribution and filtering.
 */
export type ErrorSource =
  | "eslint"
  | "typescript"
  | "go"
  | "go-test"
  | "python"
  | "rust"
  | "docker"
  | "nodejs"
  | "metadata"
  | "infrastructure"
  | "generic";

export const ErrorSources = {
  ESLint: "eslint" as const,
  TypeScript: "typescript" as const,
  Go: "go" as const,
  GoTest: "go-test" as const,
  Python: "python" as const,
  Rust: "rust" as const,
  Docker: "docker" as const,
  NodeJS: "nodejs" as const,
  Metadata: "metadata" as const,
  Infrastructure: "infrastructure" as const,
  Generic: "generic" as const,
};

// ============================================================================
// Error Severity
// ============================================================================

export type ErrorSeverity = "error" | "warning";

// ============================================================================
// Core Interfaces
// ============================================================================

/**
 * WorkflowContext captures GitHub Actions workflow execution context.
 */
export interface WorkflowContext {
  /** From [workflow/job] prefix in act output */
  readonly job?: string;
  /** Parse from step names */
  readonly step?: string;
  /** Parse from action names */
  readonly action?: string;
}

/**
 * Clone a WorkflowContext for safe mutation.
 */
export const cloneWorkflowContext = (
  ctx: WorkflowContext | undefined
): WorkflowContext | undefined => {
  if (!ctx) {
    return undefined;
  }
  return { ...ctx };
};

/**
 * CodeSnippet contains source code context around an error location.
 */
export interface CodeSnippet {
  /** Lines of source code context */
  readonly lines: readonly string[];
  /** First line number in snippet (1-indexed in original file) */
  readonly startLine: number;
  /** Position of error line within lines array (1-indexed, e.g., 1 = lines[0]) */
  readonly errorLine: number;
  /** Language identifier: "go", "typescript", "python", etc. */
  readonly language: string;
}

/**
 * ExtractedError represents a single error extracted from CI output.
 */
export interface ExtractedError {
  readonly message: string;
  readonly file?: string;
  readonly line?: number;
  readonly column?: number;
  readonly severity?: ErrorSeverity;
  readonly raw?: string;
  /** Multi-line stack trace for detailed error context */
  readonly stackTrace?: string;
  /** e.g., "no-var", "TS2749" */
  readonly ruleId?: string;
  /** lint, type-check, test, etc. */
  readonly category?: ErrorCategory;
  /** Job/step info */
  readonly workflowContext?: WorkflowContext;
  /** Flattened from WorkflowContext.job for easier access */
  readonly workflowJob?: string;
  /** "eslint", "typescript", "go", etc. */
  readonly source?: ErrorSource;
  /** True if matched by generic fallback parser */
  readonly unknownPattern?: boolean;

  // AI-optimized fields for enhanced context
  /** Source code context around error */
  readonly codeSnippet?: CodeSnippet;
  /** Fix suggestions from tools (Rust notes, TS hints) */
  readonly suggestions?: readonly string[];
  /** True if line is a real value, false if line=0 means unknown */
  readonly lineKnown?: boolean;
  /** True if column is a real value, false if column=0 means unknown */
  readonly columnKnown?: boolean;
  /** True if stack trace was truncated due to size limits */
  readonly stackTraceTruncated?: boolean;
  /** True if message was truncated due to size limits */
  readonly messageTruncated?: boolean;

  // Infrastructure error fields
  /** Actionable hint for fixing the error */
  readonly hint?: string;
  /** Exit code if this was a process failure */
  readonly exitCode?: number;
  /** Whether this is CI configuration vs code error */
  readonly isInfrastructure?: boolean;
}

/**
 * Create a mutable error builder for constructing ExtractedError objects.
 * This helps with building errors incrementally (e.g., multi-line parsing).
 */
export interface MutableExtractedError {
  message: string;
  file?: string;
  line?: number;
  column?: number;
  severity?: ErrorSeverity;
  raw?: string;
  stackTrace?: string;
  ruleId?: string;
  category?: ErrorCategory;
  workflowContext?: WorkflowContext;
  workflowJob?: string;
  source?: ErrorSource;
  unknownPattern?: boolean;
  codeSnippet?: CodeSnippet;
  suggestions?: string[];
  lineKnown?: boolean;
  columnKnown?: boolean;
  stackTraceTruncated?: boolean;
  messageTruncated?: boolean;
  hint?: string;
  exitCode?: number;
  isInfrastructure?: boolean;
}

/**
 * Convert a mutable error to an immutable ExtractedError.
 */
export const freezeError = (err: MutableExtractedError): ExtractedError => ({
  ...err,
  suggestions: err.suggestions ? [...err.suggestions] : undefined,
  codeSnippet: err.codeSnippet
    ? { ...err.codeSnippet, lines: [...err.codeSnippet.lines] }
    : undefined,
  workflowContext: cloneWorkflowContext(err.workflowContext),
});

// ============================================================================
// Statistics & Reporting
// ============================================================================

/**
 * ErrorStats provides statistics for AI prompt generation.
 */
export interface ErrorStats {
  readonly total: number;
  readonly errorCount: number;
  readonly warningCount: number;
  readonly byCategory: Readonly<Record<ErrorCategory, number>>;
  readonly bySource: Readonly<Record<string, number>>;
  readonly byFile: Readonly<Record<string, number>>;
  readonly uniqueFiles: number;
  readonly uniqueRules: number;
}

/**
 * AIContext provides metadata for AI consumption of error data.
 */
export interface AIContext {
  // Run metadata
  readonly commitSha?: string;
  readonly treeHash?: string;
  readonly repoRoot?: string;

  // Extraction metadata
  readonly extractedAt: string;
  readonly cacheHit: boolean;
  readonly parserVersion: string;

  // Snippet availability metrics
  readonly snippetsIncluded: boolean;
  readonly snippetsFailed: number;

  // Error quality metrics
  readonly errorsWithLocation: number;
  readonly errorsWithSnippet: number;
  readonly errorsWithRuleId: number;
}

/**
 * ErrorReport provides a flat error structure with computed statistics.
 * This is the preferred structure for error data, avoiding duplication.
 */
export interface ErrorReport {
  readonly errors: readonly ExtractedError[];
  readonly stats: ErrorStats;
  readonly aiContext?: AIContext;
}

// ============================================================================
// Grouping (Deprecated - use ErrorReport)
// ============================================================================

/**
 * GroupedErrors groups errors by file path for organized output.
 * @deprecated Use ErrorReport with createErrorReport instead for a flatter, non-duplicating structure.
 */
export interface GroupedErrors {
  readonly byFile: Readonly<Record<string, readonly ExtractedError[]>>;
  readonly noFile: readonly ExtractedError[];
  readonly total: number;
}

// ============================================================================
// Factory Functions
// ============================================================================

/**
 * Create an ErrorReport from a flat list of errors.
 * Computes all statistics and optionally makes file paths relative.
 */
export const createErrorReport = (
  errors: readonly ExtractedError[],
  basePath?: string
): ErrorReport => {
  const byCategory: Record<ErrorCategory, number> = {} as Record<
    ErrorCategory,
    number
  >;
  const bySource: Record<string, number> = {};
  const byFile: Record<string, number> = {};
  const uniqueFiles = new Set<string>();
  const uniqueRules = new Set<string>();

  let errorCount = 0;
  let warningCount = 0;

  for (const err of errors) {
    if (err.severity === "error") {
      errorCount++;
    } else if (err.severity === "warning") {
      warningCount++;
    }

    const category = err.category ?? "unknown";
    byCategory[category] = (byCategory[category] ?? 0) + 1;

    if (err.source) {
      bySource[err.source] = (bySource[err.source] ?? 0) + 1;
    }

    if (err.file) {
      const file = basePath ? makeRelative(err.file, basePath) : err.file;
      byFile[file] = (byFile[file] ?? 0) + 1;
      uniqueFiles.add(file);
    }

    if (err.ruleId) {
      uniqueRules.add(err.ruleId);
    }
  }

  return {
    errors,
    stats: {
      total: errors.length,
      errorCount,
      warningCount,
      byCategory,
      bySource,
      byFile,
      uniqueFiles: uniqueFiles.size,
      uniqueRules: uniqueRules.size,
    },
  };
};

/**
 * Group errors by file path.
 * @deprecated Use createErrorReport instead.
 */
export const groupByFile = (
  errors: readonly ExtractedError[],
  basePath?: string
): GroupedErrors => {
  const byFile: Record<string, ExtractedError[]> = {};
  const noFile: ExtractedError[] = [];

  for (const err of errors) {
    if (err.file) {
      const file = basePath ? makeRelative(err.file, basePath) : err.file;
      if (!byFile[file]) {
        byFile[file] = [];
      }
      byFile[file].push(err);
    } else {
      noFile.push(err);
    }
  }

  return {
    byFile,
    noFile,
    total: errors.length,
  };
};

// ============================================================================
// Comprehensive Grouping & Filtering
// ============================================================================

/**
 * Comprehensive error grouping with multiple dimensions.
 * Provides grouped views by file, category, severity, and source.
 */
export interface ComprehensiveErrorGroup {
  /** Group by file path */
  readonly byFile: Map<string, readonly ExtractedError[]>;
  /** Group by category */
  readonly byCategory: Map<ErrorCategory, readonly ExtractedError[]>;
  /** Group by severity */
  readonly bySeverity: Map<ErrorSeverity, readonly ExtractedError[]>;
  /** Group by source tool */
  readonly bySource: Map<ErrorSource, readonly ExtractedError[]>;
  /** Total error count */
  readonly total: number;
}

/**
 * Lightweight view optimized for AI orchestrator consumption.
 * Provides summary statistics and highlights critical errors.
 */
export interface OrchestratorView {
  /** Summary counts by category */
  readonly categoryCounts: Record<string, number>;
  /** Top files with most errors (max 10) */
  readonly topFiles: readonly { file: string; count: number }[];
  /** Critical errors (first 5) */
  readonly criticalErrors: readonly ExtractedError[];
  /** Total count */
  readonly total: number;
}

/**
 * Severity ordering for comparison (higher = more severe).
 */
const severityOrder: Record<ErrorSeverity, number> = {
  error: 2,
  warning: 1,
};

/**
 * Filter errors by one or more categories.
 */
export const filterByCategory = (
  errors: readonly ExtractedError[],
  categories: ErrorCategory[]
): ExtractedError[] => {
  const categorySet = new Set(categories);
  return errors.filter((err) => err.category && categorySet.has(err.category));
};

/**
 * Filter errors by file path pattern (string prefix or RegExp).
 */
export const filterByFile = (
  errors: readonly ExtractedError[],
  pattern: string | RegExp
): ExtractedError[] => {
  if (typeof pattern === "string") {
    return errors.filter((err) => err.file?.includes(pattern));
  }
  return errors.filter((err) => err.file && pattern.test(err.file));
};

/**
 * Filter errors by minimum severity level.
 * Returns errors with severity >= minSeverity.
 */
export const filterBySeverity = (
  errors: readonly ExtractedError[],
  minSeverity: ErrorSeverity
): ExtractedError[] => {
  const minOrder = severityOrder[minSeverity];
  return errors.filter((err) => {
    const errSeverity = err.severity ?? "warning";
    return severityOrder[errSeverity] >= minOrder;
  });
};

/**
 * Filter errors by one or more source tools.
 */
export const filterBySource = (
  errors: readonly ExtractedError[],
  sources: ErrorSource[]
): ExtractedError[] => {
  const sourceSet = new Set(sources);
  return errors.filter((err) => err.source && sourceSet.has(err.source));
};

/**
 * Group errors into comprehensive multi-dimensional groups.
 */
export const groupErrors = (
  errors: readonly ExtractedError[]
): ComprehensiveErrorGroup => {
  const byFile = new Map<string, ExtractedError[]>();
  const byCategory = new Map<ErrorCategory, ExtractedError[]>();
  const bySeverity = new Map<ErrorSeverity, ExtractedError[]>();
  const bySource = new Map<ErrorSource, ExtractedError[]>();

  for (const err of errors) {
    // Group by file
    if (err.file) {
      const fileErrors = byFile.get(err.file);
      if (fileErrors) {
        fileErrors.push(err);
      } else {
        byFile.set(err.file, [err]);
      }
    }

    // Group by category
    const category = err.category ?? "unknown";
    const categoryErrors = byCategory.get(category);
    if (categoryErrors) {
      categoryErrors.push(err);
    } else {
      byCategory.set(category, [err]);
    }

    // Group by severity
    const severity = err.severity ?? "warning";
    const severityErrors = bySeverity.get(severity);
    if (severityErrors) {
      severityErrors.push(err);
    } else {
      bySeverity.set(severity, [err]);
    }

    // Group by source
    if (err.source) {
      const sourceErrors = bySource.get(err.source);
      if (sourceErrors) {
        sourceErrors.push(err);
      } else {
        bySource.set(err.source, [err]);
      }
    }
  }

  return {
    byFile,
    byCategory,
    bySeverity,
    bySource,
    total: errors.length,
  };
};

/**
 * Create a lightweight orchestrator view from errors.
 * Optimized for AI consumption with summary stats and critical highlights.
 */
export const createOrchestratorView = (
  errors: readonly ExtractedError[]
): OrchestratorView => {
  // Count by category
  const categoryCounts: Record<string, number> = {};
  const fileCounts = new Map<string, number>();
  const criticalErrors: ExtractedError[] = [];

  for (const err of errors) {
    // Category counts
    const category = err.category ?? "unknown";
    categoryCounts[category] = (categoryCounts[category] ?? 0) + 1;

    // File counts
    if (err.file) {
      fileCounts.set(err.file, (fileCounts.get(err.file) ?? 0) + 1);
    }

    // Collect critical errors (severity = error)
    if (err.severity === "error" && criticalErrors.length < 5) {
      criticalErrors.push(err);
    }
  }

  // Sort files by error count and take top 10
  const topFiles = [...fileCounts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 10)
    .map(([file, count]) => ({ file, count }));

  return {
    categoryCounts,
    topFiles,
    criticalErrors,
    total: errors.length,
  };
};

// ============================================================================
// Utilities
// ============================================================================

/**
 * Convert an absolute path to relative if it's under basePath.
 */
export const makeRelative = (filePath: string, basePath: string): string => {
  if (!(basePath && filePath.startsWith("/"))) {
    return filePath;
  }

  // Ensure basePath ends without slash for clean comparison
  const normalizedBase = basePath.endsWith("/")
    ? basePath.slice(0, -1)
    : basePath;

  if (filePath.startsWith(`${normalizedBase}/`)) {
    return filePath.slice(normalizedBase.length + 1);
  }

  return filePath;
};
