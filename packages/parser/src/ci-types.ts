/**
 * CI platform types for context parsing.
 * Migrated from packages/core/ci/types.go
 */

// ============================================================================
// Job Status
// ============================================================================

/**
 * JobStatus represents the status of a workflow job.
 */
export type JobStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "skipped"
  | "skipped_security";

export const JobStatuses = {
  Pending: "pending" as const,
  Running: "running" as const,
  Success: "success" as const,
  Failed: "failed" as const,
  Skipped: "skipped" as const,
  /** Skipped by Detent to prevent accidental production releases */
  SkippedSecurity: "skipped_security" as const,
};

// ============================================================================
// Step Status
// ============================================================================

/**
 * StepStatus represents the status of a workflow step.
 */
export type StepStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "skipped"
  | "cancelled";

export const StepStatuses = {
  Pending: "pending" as const,
  Running: "running" as const,
  Success: "success" as const,
  Failed: "failed" as const,
  Skipped: "skipped" as const,
  Cancelled: "cancelled" as const,
};

// ============================================================================
// Events
// ============================================================================

/**
 * JobEvent represents a job lifecycle event parsed from CI output.
 */
export interface JobEvent {
  /** Job ID (key in workflow jobs map) */
  readonly jobId: string;
  /** "start", "finish", or "skip" */
  readonly action: "start" | "finish" | "skip";
  /** Only relevant when action="finish" */
  readonly success: boolean;
}

/**
 * StepEvent represents a step lifecycle event parsed from CI output.
 */
export interface StepEvent {
  /** Job ID this step belongs to */
  readonly jobId: string;
  /** Step index (0-based) */
  readonly stepIdx: number;
  /** Step display name */
  readonly stepName: string;
}

// ============================================================================
// Manifest
// ============================================================================

/**
 * ManifestJob contains information about a single job in the manifest.
 */
export interface ManifestJob {
  /** Job ID (key in jobs map) */
  readonly id: string;
  /** Display name */
  readonly name: string;
  /** Step names in order (empty for uses: jobs) */
  readonly steps?: readonly string[];
  /** Job IDs this job depends on */
  readonly needs?: readonly string[];
  /** Reusable workflow reference (if present, no steps) */
  readonly uses?: string;
  /** True if job may publish, release, or deploy */
  readonly sensitive?: boolean;
}

/**
 * ManifestInfo contains the full manifest for a workflow run.
 * This is the v2 manifest format that includes step information.
 */
export interface ManifestInfo {
  /** Manifest version (2 for this format) */
  readonly version: number;
  /** All jobs in topological order */
  readonly jobs: readonly ManifestJob[];
}

/**
 * ManifestEvent is emitted when a manifest is parsed from CI output.
 */
export interface ManifestEvent {
  readonly manifest: ManifestInfo;
}

// ============================================================================
// Context Parsing
// ============================================================================

/**
 * LineContext contains CI platform-specific context extracted from a log line.
 */
export interface LineContext {
  /** Job name from CI output */
  readonly job: string;
  /** Step name (if parseable) */
  readonly step: string;
  /** True if line should be skipped (debug output) */
  readonly isNoise: boolean;
}

/**
 * Result of parsing a CI log line.
 */
export interface ParseLineResult {
  /** Extracted context */
  readonly ctx: LineContext;
  /** Cleaned line (with CI prefixes removed) */
  readonly cleanLine: string;
  /** Whether to skip this line entirely */
  readonly skip: boolean;
}

/**
 * ContextParser extracts CI platform-specific context from log lines.
 * Different CI systems (act, GitHub Actions, GitLab) implement this interface
 * to parse their specific output formats and extract job/step context.
 */
export interface ContextParser {
  /**
   * Extracts context from a CI log line.
   * Returns the context, the cleaned line (with CI prefixes removed), and whether to skip.
   * If skip is true, the line should be ignored (debug noise, metadata).
   */
  parseLine(line: string): ParseLineResult;
}

// ============================================================================
// Default Passthrough Parser
// ============================================================================

/**
 * PassthroughParser is a ContextParser that passes lines through unchanged.
 * Use this when parsing raw log output without CI prefixes.
 */
export const passthroughParser: ContextParser = {
  parseLine(line: string): ParseLineResult {
    return {
      ctx: { job: "", step: "", isNoise: false },
      cleanLine: line,
      skip: false,
    };
  },
};
