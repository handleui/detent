/**
 * Exit code classification utility for CI/CD infrastructure errors.
 * Maps UNIX/POSIX exit codes to meaningful context for error reporting.
 *
 * Exit code conventions:
 * - 0: Success
 * - 1-125: Command-specific errors
 * - 126: Command not executable
 * - 127: Command not found
 * - 128+N: Fatal signal N (e.g., 137 = 128+9 = SIGKILL)
 */

// ============================================================================
// Types
// ============================================================================

/**
 * Classification of exit code cause.
 */
export type ExitCodeClassification =
  | "success"
  | "general"
  | "not_found"
  | "permission"
  | "signal"
  | "resource";

/**
 * Information about an exit code including severity and actionable hints.
 */
export interface ExitCodeInfo {
  /** The exit code value */
  readonly code: number;

  /** Severity for display purposes */
  readonly severity: "error" | "warning" | "info";

  /** Classification of what caused the exit */
  readonly classification: ExitCodeClassification;

  /** Human-readable message describing the exit code */
  readonly message: string;

  /** Optional actionable hint for resolving the issue */
  readonly hint?: string;

  /** Whether retrying might help (transient failures) */
  readonly isTransient: boolean;

  /** Whether this indicates a CI configuration issue */
  readonly isConfiguration: boolean;

  /** Signal number if classification is "signal" (code > 128) */
  readonly signal?: number;

  /** Signal name if known (e.g., "SIGKILL", "SIGTERM") */
  readonly signalName?: string;
}

// ============================================================================
// Signal Mappings
// ============================================================================

/**
 * Common UNIX signal names mapped to their numbers.
 */
const signalNames: Readonly<Record<number, string>> = {
  1: "SIGHUP",
  2: "SIGINT",
  3: "SIGQUIT",
  4: "SIGILL",
  6: "SIGABRT",
  7: "SIGBUS",
  8: "SIGFPE",
  9: "SIGKILL",
  11: "SIGSEGV",
  13: "SIGPIPE",
  14: "SIGALRM",
  15: "SIGTERM",
};

/**
 * Signal-specific messages and hints.
 */
const signalInfo: Readonly<
  Record<number, { message: string; hint?: string; isTransient: boolean }>
> = {
  2: {
    message: "Interrupted (SIGINT)",
    hint: "Operation was cancelled by user",
    isTransient: false,
  },
  9: {
    message: "Killed (SIGKILL)",
    hint: "Process was forcefully terminated, possibly due to OOM",
    isTransient: true,
  },
  11: {
    message: "Segmentation fault (SIGSEGV)",
    hint: "Check native dependencies or memory access issues",
    isTransient: false,
  },
  13: {
    message: "Broken pipe (SIGPIPE)",
    hint: "Output destination was closed unexpectedly",
    isTransient: true,
  },
  15: {
    message: "Terminated (SIGTERM)",
    hint: "Process was asked to terminate gracefully",
    isTransient: false,
  },
};

// ============================================================================
// Exit Code Classification
// ============================================================================

/**
 * Classify an exit code and return detailed information about it.
 *
 * @param code - The exit code to classify (0-255)
 * @returns Detailed information about the exit code
 */
export const classifyExitCode = (code: number): ExitCodeInfo => {
  // Handle signal-based exit codes (128 + signal number)
  if (code > 128 && code <= 255) {
    const signal = code - 128;
    const name = signalNames[signal];
    const info = signalInfo[signal];

    return {
      code,
      severity: signal === 2 ? "info" : "warning", // SIGINT is expected cancellation
      classification: "signal",
      message: info?.message ?? `Terminated by signal ${signal}`,
      hint: info?.hint,
      isTransient: info?.isTransient ?? false,
      isConfiguration: false,
      signal,
      signalName: name,
    };
  }

  // Handle specific well-known exit codes
  switch (code) {
    case 0:
      return {
        code,
        severity: "info",
        classification: "success",
        message: "Success",
        isTransient: false,
        isConfiguration: false,
      };

    case 1:
      return {
        code,
        severity: "error",
        classification: "general",
        message: "General error",
        hint: "Check the command output for details",
        isTransient: false,
        isConfiguration: false,
      };

    case 2:
      return {
        code,
        severity: "error",
        classification: "general",
        message: "Misuse of shell command",
        hint: "Check script syntax or command arguments",
        isTransient: false,
        isConfiguration: true,
      };

    case 126:
      return {
        code,
        severity: "error",
        classification: "permission",
        message: "Command not executable",
        hint: "Run chmod +x on the script file",
        isTransient: false,
        isConfiguration: true,
      };

    case 127:
      return {
        code,
        severity: "error",
        classification: "not_found",
        message: "Command or script not found",
        hint: "Check PATH or package.json scripts",
        isTransient: false,
        isConfiguration: true,
      };

    case 128:
      return {
        code,
        severity: "error",
        classification: "general",
        message: "Invalid exit argument",
        hint: "Exit code must be 0-255",
        isTransient: false,
        isConfiguration: false,
      };

    default:
      // Generic error for other codes
      return {
        code,
        severity: "error",
        classification: "general",
        message: `Command failed with exit code ${code}`,
        isTransient: false,
        isConfiguration: false,
      };
  }
};

/**
 * Check if an exit code indicates a configuration/setup issue.
 */
export const isConfigurationError = (code: number): boolean =>
  classifyExitCode(code).isConfiguration;

/**
 * Check if an exit code indicates a transient failure that might succeed on retry.
 */
export const isTransientFailure = (code: number): boolean =>
  classifyExitCode(code).isTransient;

/**
 * Check if an exit code indicates the process was killed by a signal.
 */
export const isSignalExit = (code: number): boolean =>
  code > 128 && code <= 255;

/**
 * Get the signal number from an exit code, or undefined if not a signal exit.
 */
export const getSignalFromExitCode = (code: number): number | undefined =>
  isSignalExit(code) ? code - 128 : undefined;

/**
 * Format an exit code for display with its classification.
 *
 * @param code - The exit code
 * @returns Formatted string like "exit 127 (not found)"
 */
export const formatExitCode = (code: number): string => {
  const info = classifyExitCode(code);

  if (info.classification === "success") {
    return "exit 0";
  }

  if (info.classification === "signal" && info.signalName) {
    return `exit ${code} (${info.signalName})`;
  }

  const classificationLabels: Record<ExitCodeClassification, string> = {
    success: "success",
    general: "error",
    not_found: "not found",
    permission: "permission denied",
    signal: "signal",
    resource: "resource limit",
  };

  return `exit ${code} (${classificationLabels[info.classification]})`;
};
