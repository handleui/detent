/**
 * Types for the Mock command TUI
 * Mirrors the Go CLI's TUI event system
 */

/**
 * Represents a structured error for display in the TUI
 */
export interface DisplayError {
  readonly message: string;
  readonly file?: string;
  readonly line?: number;
  readonly column?: number;
  readonly severity: "error" | "warning";
  readonly ruleId?: string;
  readonly category?: string;
}

/**
 * Job status states matching Go CLI ci.JobStatus
 */
export type JobStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "skipped"
  | "skipped_security";

/**
 * Step status states matching Go CLI ci.StepStatus
 */
export type StepStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "skipped"
  | "cancelled";

/**
 * Represents a step in a job
 */
export interface TrackedStep {
  readonly index: number;
  readonly name: string;
  status: StepStatus;
}

/**
 * Represents a job being tracked in the TUI
 */
export interface TrackedJob {
  readonly id: string;
  readonly name: string;
  status: JobStatus;
  readonly isReusable: boolean;
  readonly isSensitive: boolean;
  readonly steps: TrackedStep[];
  currentStep: number;
  readonly needs?: readonly string[];
  /** Depth in dependency tree (0 = no dependencies, 1+ = nested) */
  readonly depth: number;
}

/**
 * Manifest event - initializes TUI with job/step structure
 */
export interface ManifestEvent {
  readonly type: "manifest";
  readonly jobs: readonly {
    readonly id: string;
    readonly name: string;
    readonly uses?: string;
    readonly sensitive: boolean;
    readonly steps: readonly string[];
    readonly needs?: readonly string[];
  }[];
}

/**
 * Job event - updates job status
 */
export interface JobEvent {
  readonly type: "job";
  readonly jobId: string;
  readonly action: "start" | "finish" | "skip";
  readonly success?: boolean;
}

/**
 * Step event - updates step status
 */
export interface StepEvent {
  readonly type: "step";
  readonly jobId: string;
  readonly stepIdx: number;
  readonly stepName: string;
}

/**
 * Log event - raw log output (shown in verbose mode)
 */
export interface LogEvent {
  readonly type: "log";
  readonly content: string;
}

/**
 * Done event - execution complete
 */
export interface DoneEvent {
  readonly type: "done";
  readonly duration: number;
  readonly exitCode: number;
  readonly errorCount: number;
  readonly cancelled: boolean;
  readonly errors?: readonly DisplayError[];
}

/**
 * Error event - fatal error occurred
 */
export interface ErrorEvent {
  readonly type: "error";
  readonly error: Error;
  readonly message: string;
}

/**
 * Warning event - non-fatal issue occurred
 */
export interface WarningEvent {
  readonly type: "warning";
  readonly message: string;
  readonly category: "parser" | "skipped" | "cache";
}

/**
 * Union type for all TUI events
 */
export type TUIEvent =
  | ManifestEvent
  | JobEvent
  | StepEvent
  | LogEvent
  | DoneEvent
  | ErrorEvent
  | WarningEvent;
