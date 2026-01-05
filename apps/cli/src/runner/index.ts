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

  constructor(config: RunConfig) {
    this.config = config;
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
      // Step 1: Prepare execution environment
      prepareResult = await this.prepare();

      // Step 2: Execute workflows
      const executeResult = await this.execute(prepareResult);

      // Step 3: Process execution output
      const processResult = await this.process(executeResult);

      // Step 4: Cleanup (always runs, even on error)
      await this.cleanup(prepareResult);

      const duration = Date.now() - startTime;
      const success =
        processResult.errorCount === 0 && executeResult.exitCode === 0;

      return {
        runID: prepareResult.runID,
        success,
        errors: processResult.errors,
        duration,
      };
    } catch (error) {
      // Ensure cleanup happens even if there's an error
      if (prepareResult) {
        try {
          await this.cleanup(prepareResult);
        } catch {
          // Ignore cleanup errors during error handling
        }
      }

      throw error;
    }
  }

  /**
   * Prepares the execution environment.
   *
   * @returns Preparation result with worktree path, run ID, and workflows
   */
  private async prepare(): Promise<PrepareResult> {
    const preparer = new WorkflowPreparer(this.config);
    return await preparer.prepare();
  }

  /**
   * Executes workflows using the act runner.
   *
   * @param prepareResult - Result from the prepare step
   * @returns Execution result with exit code, output, and duration
   */
  private async execute(prepareResult: PrepareResult): Promise<ExecuteResult> {
    const executor = new ActExecutor({ verbose: this.config.verbose });
    return await executor.execute(prepareResult);
  }

  /**
   * Processes execution output to extract and parse errors.
   *
   * @param executeResult - Result from the execute step
   * @returns Processing result with parsed errors
   */
  private async process(executeResult: ExecuteResult): Promise<ProcessResult> {
    const processor = new ErrorProcessor();
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
    } catch {
      // Gracefully handle cleanup errors - don't throw
    }
  }
}
