/**
 * Signal handling utilities for graceful shutdown.
 *
 * Provides an AbortController-style API similar to Go's context cancellation.
 * The signal handler automatically cleans itself up when the controller is aborted.
 */

type SignalHandler = () => void;

interface SignalController {
  /**
   * AbortSignal that triggers when SIGINT/SIGTERM is received.
   */
  readonly signal: AbortSignal;

  /**
   * Manually abort the controller (useful for cleanup phases).
   */
  readonly abort: () => void;

  /**
   * Clean up signal handlers. Must be called when done.
   */
  readonly cleanup: () => void;

  /**
   * Whether the controller has been aborted.
   */
  readonly aborted: boolean;
}

/**
 * Creates a signal controller that aborts on SIGINT or SIGTERM.
 *
 * Similar to Go CLI's SetupSignalHandler, this creates a cancellation mechanism
 * that propagates through the application.
 *
 * @example
 * ```ts
 * const ctrl = createSignalController();
 * try {
 *   await someOperation({ signal: ctrl.signal });
 * } finally {
 *   ctrl.cleanup();
 * }
 * ```
 */
export const createSignalController = (): SignalController => {
  const controller = new AbortController();
  let cleaned = false;

  const handleSignal = (): void => {
    if (!(cleaned || controller.signal.aborted)) {
      controller.abort();
    }
  };

  process.on("SIGINT", handleSignal);
  process.on("SIGTERM", handleSignal);

  const cleanup = (): void => {
    if (cleaned) return;
    cleaned = true;
    process.off("SIGINT", handleSignal);
    process.off("SIGTERM", handleSignal);
  };

  controller.signal.addEventListener("abort", cleanup, { once: true });

  return {
    signal: controller.signal,
    abort: () => controller.abort(),
    cleanup,
    get aborted() {
      return controller.signal.aborted;
    },
  };
};

/**
 * Wraps a cleanup function to be interruptible during execution.
 * A second signal during cleanup will force exit.
 *
 * @param cleanupFn - The cleanup function to wrap
 * @param exitCode - Exit code to use on forced exit (default: 130)
 */
export const interruptibleCleanup = async (
  cleanupFn: () => Promise<void>,
  exitCode = 130
): Promise<void> => {
  let forceExitHandler: SignalHandler | undefined;

  const setupForceExit = (): void => {
    forceExitHandler = () => {
      process.exit(exitCode);
    };
    process.on("SIGINT", forceExitHandler);
    process.on("SIGTERM", forceExitHandler);
  };

  const removeForceExit = (): void => {
    if (forceExitHandler) {
      process.off("SIGINT", forceExitHandler);
      process.off("SIGTERM", forceExitHandler);
      forceExitHandler = undefined;
    }
  };

  setupForceExit();
  try {
    await cleanupFn();
  } finally {
    removeForceExit();
  }
};

/**
 * Creates a promise that rejects after a timeout.
 *
 * @param ms - Timeout in milliseconds
 * @param message - Error message for timeout
 */
export const timeoutPromise = <T = never>(
  ms: number,
  message = "Operation timed out"
): Promise<T> =>
  new Promise((_, reject) => {
    setTimeout(() => reject(new Error(message)), ms);
  });

/**
 * Runs a promise with a timeout.
 *
 * @param promise - The promise to run
 * @param ms - Timeout in milliseconds
 * @param message - Error message for timeout
 */
export const withTimeout = async <T>(
  promise: Promise<T>,
  ms: number,
  message?: string
): Promise<T> =>
  Promise.race([
    promise,
    timeoutPromise<T>(ms, message ?? `Timed out after ${ms}ms`),
  ]);

/**
 * SIGINT exit code (128 + signal number).
 */
export const SIGINT_EXIT_CODE = 130;
