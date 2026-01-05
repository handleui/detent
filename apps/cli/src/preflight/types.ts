/**
 * Represents a single preflight check that can be executed.
 */
export interface PreflightCheck {
  readonly name: string;
  readonly description: string;
  readonly run: () => Promise<PreflightResult>;
}

/**
 * Result of executing a preflight check.
 */
export interface PreflightResult {
  readonly passed: boolean;
  readonly message?: string;
  readonly error?: Error;
}

/**
 * Summary of all preflight checks that were executed.
 */
export interface PreflightSummary {
  readonly checks: ReadonlyArray<{
    readonly name: string;
    readonly passed: boolean;
    readonly message?: string;
  }>;
  readonly allPassed: boolean;
}
