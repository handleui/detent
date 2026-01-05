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

  constructor(config: RunConfig, eventEmitter?: TUIEventEmitter) {
    this.config = config;
    this.eventEmitter = eventEmitter;
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

      // Emit manifest event with discovered workflows
      if (this.eventEmitter && prepareResult.workflows.length > 0) {
        const manifest = {
          type: "manifest" as const,
          jobs: prepareResult.workflows.map((wf) => ({
            id: wf.name,
            name: wf.name.replace(".yml", "").replace(".yaml", ""),
            sensitive: false,
            steps: [], // Steps will be discovered during execution
          })),
        };
        this.eventEmitter.emit(manifest);
      }

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
    const executor = new ActExecutor({
      verbose: this.config.verbose,
      eventEmitter: this.eventEmitter,
      debugLogger: this.debugLogger,
    });
    return await executor.execute(prepareResult);
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
   *
   * @param prepareResult - Result from the prepare step containing worktree info
   */
  private async cleanup(prepareResult: PrepareResult): Promise<void> {
    try {
      const { execGit } = await import("@detent/git");
      await execGit(
        ["worktree", "remove", "--force", prepareResult.worktreePath],
        {
          cwd: this.config.repoRoot,
        }
      );
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : String(error);

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
