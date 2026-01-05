/**
 * Configuration for running GitHub Actions workflows locally.
 */
export interface RunConfig {
  /**
   * Optional workflow name to run (e.g., "ci.yml").
   * If not specified, all workflows will be run.
   */
  readonly workflow?: string;

  /**
   * Optional job name to run within the workflow.
   * If not specified, all jobs will be run.
   */
  readonly job?: string;

  /**
   * Absolute path to the Git repository root.
   */
  readonly repoRoot: string;

  /**
   * Enable verbose output for debugging.
   */
  readonly verbose?: boolean;
}

/**
 * Result of preparing the execution environment.
 * Contains paths and metadata needed for workflow execution.
 */
export interface PrepareResult {
  /**
   * Absolute path to the created worktree for isolated execution.
   */
  readonly worktreePath: string;

  /**
   * Unique identifier for this run (used for tracking and cleanup).
   */
  readonly runID: string;

  /**
   * List of workflow files to be executed.
   */
  readonly workflows: readonly WorkflowFile[];
}

/**
 * Represents a GitHub Actions workflow file.
 */
export interface WorkflowFile {
  /**
   * Workflow filename (e.g., "ci.yml").
   */
  readonly name: string;

  /**
   * Absolute path to the workflow file.
   */
  readonly path: string;

  /**
   * Raw YAML content of the workflow file.
   */
  readonly content: string;
}

/**
 * Result of executing workflows with the act runner.
 */
export interface ExecuteResult {
  /**
   * Exit code from the act process.
   * 0 indicates success, non-zero indicates failure.
   */
  readonly exitCode: number;

  /**
   * Standard output captured from act execution.
   */
  readonly stdout: string;

  /**
   * Standard error output captured from act execution.
   */
  readonly stderr: string;

  /**
   * Total execution time in milliseconds.
   */
  readonly duration: number;
}

/**
 * Result of processing execution output through the parser service.
 */
export interface ProcessResult {
  /**
   * Array of parsed errors from the execution output.
   */
  readonly errors: readonly ParsedError[];

  /**
   * Total count of errors found.
   */
  readonly errorCount: number;
}

/**
 * Represents a parsed error from workflow execution.
 */
export interface ParsedError {
  /**
   * Unique identifier for this error instance.
   */
  readonly errorId: string;

  /**
   * Hash of the error content for deduplication.
   */
  readonly contentHash: string;

  /**
   * Optional file path where the error occurred.
   */
  readonly filePath?: string;

  /**
   * Human-readable error message.
   */
  readonly message: string;

  /**
   * Error severity level (e.g., "error", "warning").
   */
  readonly severity: string;
}

/**
 * Final result of a complete workflow run.
 */
export interface RunResult {
  /**
   * Unique identifier for this run.
   */
  readonly runID: string;

  /**
   * Whether the run completed successfully (no errors).
   */
  readonly success: boolean;

  /**
   * Array of errors encountered during execution.
   */
  readonly errors: readonly ParsedError[];

  /**
   * Total execution time in milliseconds.
   */
  readonly duration: number;
}
