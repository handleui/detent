import { DebugLogger } from "../utils/debug-logger.js";
import type { TUIEventEmitter } from "./event-emitter.js";
import { ActExecutor } from "./executor.js";
import { WorkflowPreparer } from "./preparer.js";
import { ErrorProcessor } from "./processor.js";
import type {
  ExecuteResult,
  PrepareResult,
  ProcessResult,
  RunConfig,
  RunResult,
} from "./types.js";

/**
 * Orchestrates the complete workflow execution lifecycle:
 * 1. Prepare: Set up worktree and discover workflows
 * 2. Execute: Run workflows using act
 * 3. Process: Parse execution output for errors
 * 4. Cleanup: Remove worktree and temporary resources
 */
export class CheckRunner {
  private readonly config: RunConfig;
  private readonly eventEmitter?: TUIEventEmitter;
  private debugLogger?: DebugLogger;
  private executor?: ActExecutor;
  private aborted = false;

  constructor(config: RunConfig, eventEmitter?: TUIEventEmitter) {
    this.config = config;
    this.eventEmitter = eventEmitter;
  }

  /**
   * Aborts the current run and triggers cleanup.
   */
  abort(): void {
    this.aborted = true;
    this.debugLogger?.log("[Runner] Abort requested");

    if (this.executor) {
      this.executor.abort();
    }
  }

  /**
   * Returns whether the runner has been aborted.
   */
  isAborted(): boolean {
    return this.aborted;
  }

  /**
   * Runs the complete workflow execution lifecycle.
   *
   * @returns Result containing run metadata, success status, and any errors found
   */
  async run(): Promise<RunResult> {
    const startTime = Date.now();
    let prepareResult: PrepareResult | undefined;

    try {
      // Step 0.5: Create early runID for debug logger
      const { computeCurrentRunID } = await import("@detent/git");
      const runIDInfo = await computeCurrentRunID(this.config.repoRoot);
      this.debugLogger = new DebugLogger(runIDInfo.runID);

      // Log configuration and environment
      this.debugLogger.logHeader({
        verbose: this.config.verbose,
        workflow: this.config.workflow,
        job: this.config.job,
        repoRoot: this.config.repoRoot,
      });
      await this.debugLogger.logEnvironment();

      // Step 1: Prepare execution environment
      this.debugLogger.logSection("PREPARATION");
      prepareResult = await this.prepare();

      // Log preparation results
      this.debugLogger.log(`Repository: ${this.config.repoRoot}`);
      this.debugLogger.log(`Worktree: ${prepareResult.worktreePath}`);
      this.debugLogger.log(
        `Workflows: ${prepareResult.workflows.map((w) => w.name).join(", ")}`
      );

      // Emit warning for skipped sensitive items
      const skippedCount =
        (prepareResult.skippedWorkflows?.length ?? 0) +
        (prepareResult.skippedJobs?.length ?? 0);
      if (skippedCount > 0 && this.eventEmitter) {
        this.eventEmitter.emit({
          type: "warning",
          message: `Skipped ${skippedCount} sensitive workflow${skippedCount === 1 ? "" : "s"}/job${skippedCount === 1 ? "" : "s"} for safety`,
          category: "skipped",
        });
      }

      // Note: Manifest event is now emitted by the ActOutputParser when it
      // parses the ::detent::manifest:: marker from act output. This provides
      // accurate job IDs and step names from the injected workflow markers.

      // Step 2: Execute workflows
      if (this.config.verbose) {
        console.log("[Execute] Running act...\n");
      }
      this.debugLogger.logSection("ACT EXECUTION");
      this.debugLogger.startPhase("Execute");
      const executeResult = await this.execute(prepareResult);
      this.debugLogger.endPhase("Execute");
      this.debugLogger.log(
        `Exit code: ${executeResult.exitCode}, Duration: ${executeResult.duration}ms`
      );

      // Step 3: Process execution output
      if (this.config.verbose) {
        console.log("\n[Process] Parsing results...");
      }
      this.debugLogger.logSection("ERROR PARSING");
      const processResult = await this.process(executeResult);
      this.debugLogger.log(`Total errors found: ${processResult.errorCount}`);

      // Emit warning if parser failed
      if (processResult.parserFailed && this.eventEmitter) {
        this.eventEmitter.emit({
          type: "warning",
          message: "Parser failed to extract errors from output",
          category: "parser",
        });
      }

      const duration = Date.now() - startTime;
      const success =
        processResult.errorCount === 0 && executeResult.exitCode === 0;

      this.debugLogger.logSection("SUMMARY");
      this.debugLogger.log(`Status: ${success ? "SUCCESS" : "FAILED"}`);
      this.debugLogger.log(`Total duration: ${duration}ms`);
      this.debugLogger.log(`Error count: ${processResult.errorCount}`);
      this.debugLogger.log(`Exit code: ${executeResult.exitCode}`);

      // Emit done event for TUI
      if (this.eventEmitter) {
        this.eventEmitter.emit({
          type: "done",
          duration,
          exitCode: executeResult.exitCode,
          errorCount: processResult.errorCount,
          cancelled: false,
        });
      }

      return {
        runID: prepareResult.runID,
        success,
        errors: processResult.errors,
        duration,
      };
    } catch (error) {
      // Log error to debug log
      this.debugLogger?.logError(error, "RunError");

      // Emit error event for TUI
      if (this.eventEmitter) {
        const errorMessage =
          error instanceof Error ? error.message : String(error);
        this.eventEmitter.emit({
          type: "error",
          error: error instanceof Error ? error : new Error(String(error)),
          message: errorMessage,
        });
      }

      // Enhance error message with debug log path
      const debugPath = this.debugLogger?.path;
      if (debugPath && error instanceof Error) {
        const enhancedError = new Error(
          `${error.message}\n\nDebug log: ${debugPath}`
        );
        enhancedError.stack = error.stack;
        throw enhancedError;
      }

      throw error;
    } finally {
      // Step 4: Cleanup (always runs, even on error)
      if (prepareResult) {
        try {
          if (this.config.verbose) {
            console.log("[Cleanup] Removing worktree...");
          }
          this.debugLogger?.logPhase("Cleanup", "Removing worktree");
          await this.cleanup(prepareResult);
          this.debugLogger?.logPhase("Cleanup", "Cleanup complete");
          if (this.config.verbose) {
            console.log("[Cleanup] ✓ Done\n");
          }
        } catch (cleanupError) {
          this.debugLogger?.logError(cleanupError, "Cleanup");
          if (this.config.verbose) {
            console.warn(
              `[Cleanup] Warning: ${cleanupError instanceof Error ? cleanupError.message : String(cleanupError)}`
            );
          }
        }
      }

      // Close debug logger
      this.debugLogger?.close();
    }
  }

  /**
   * Prepares the execution environment.
   *
   * @returns Preparation result with worktree path, run ID, and workflows
   */
  private async prepare(): Promise<PrepareResult> {
    const preparer = new WorkflowPreparer(this.config, this.debugLogger);
    return await preparer.prepare();
  }

  /**
   * Executes workflows using the act runner.
   *
   * @param prepareResult - Result from the prepare step
   * @returns Execution result with exit code, output, and duration
   */
  private async execute(prepareResult: PrepareResult): Promise<ExecuteResult> {
    this.executor = new ActExecutor({
      verbose: this.config.verbose,
      eventEmitter: this.eventEmitter,
      debugLogger: this.debugLogger,
    });
    return await this.executor.execute(prepareResult);
  }

  /**
   * Processes execution output to extract and parse errors.
   *
   * @param executeResult - Result from the execute step
   * @returns Processing result with parsed errors
   */
  private async process(executeResult: ExecuteResult): Promise<ProcessResult> {
    const processor = new ErrorProcessor({ debugLogger: this.debugLogger });
    return await processor.process(executeResult);
  }

  /**
   * Cleans up resources created during execution.
   * Uses the cleanup function from prepareWorktree which has:
   * - Built-in timeout protection (30s)
   * - Lock release before removal
   *
   * @param prepareResult - Result from the prepare step containing cleanup function
   */
  private async cleanup(prepareResult: PrepareResult): Promise<void> {
    try {
      await prepareResult.cleanup();
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : String(error);

      // Ignore errors about non-existent worktrees (already cleaned up)
      if (
        errorMessage.includes("is not a working tree") ||
        errorMessage.includes("not found") ||
        errorMessage.includes("ENOENT")
      ) {
        return;
      }

      throw new Error(`Failed to cleanup worktree: ${errorMessage}`);
    }
  }

  /**
   * Gets the debug log path if available.
   *
   * @returns Debug log path or undefined if logger not initialized
   */
  getDebugLogPath(): string | undefined {
    return this.debugLogger?.path;
  }
}
