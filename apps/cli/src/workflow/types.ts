/**
 * Represents a GitHub Actions workflow file.
 */
export interface Workflow {
  readonly name?: string;
  readonly on?: unknown;
  readonly env?: Record<string, string>;
  readonly jobs: Record<string, Job>;
  readonly defaults?: unknown;
  readonly concurrency?: unknown;
  readonly permissions?: unknown;
}

/**
 * Represents a job in a workflow.
 */
export interface Job {
  readonly name?: string;
  readonly "runs-on"?: unknown;
  readonly steps?: readonly Step[];
  readonly env?: Record<string, string>;
  readonly if?: string;
  readonly needs?: string | readonly string[];
  readonly strategy?: unknown;
  readonly container?: unknown;
  readonly services?: unknown;
  readonly outputs?: unknown;
  readonly permissions?: unknown;
  readonly "continue-on-error"?: unknown;
  readonly "timeout-minutes"?: unknown;
  readonly defaults?: unknown;
  readonly concurrency?: unknown;
  readonly environment?: unknown;
  readonly uses?: string;
  readonly with?: Record<string, unknown>;
  readonly secrets?: unknown;
}

/**
 * Represents a step in a job.
 */
export interface Step {
  readonly id?: string;
  readonly name?: string;
  readonly uses?: string;
  readonly run?: string;
  readonly with?: Record<string, unknown>;
  readonly env?: Record<string, string>;
  readonly if?: string;
  readonly "continue-on-error"?: boolean;
  readonly "timeout-minutes"?: unknown;
  readonly "working-directory"?: string;
  readonly shell?: string;
}

/**
 * Contains extracted job information for display.
 */
export interface JobInfo {
  readonly id: string;
  readonly name: string;
  readonly needs: readonly string[];
}
