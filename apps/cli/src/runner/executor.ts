import { spawn } from "node:child_process";
import { ensureInstalled } from "../act/index.js";
import type { ExecuteResult, PrepareResult } from "./types.js";

/**
 * Configuration for the act executor.
 */
interface ExecutorConfig {
  /**
   * Enable verbose output from act.
   */
  readonly verbose?: boolean;

  /**
   * Maximum number of retry attempts for failed executions.
   * Defaults to 2.
   */
  readonly maxRetries?: number;

  /**
   * Delay in milliseconds between retry attempts.
   * Defaults to exponential backoff: 1000ms, 2000ms
   */
  readonly retryDelay?: number;
}

/**
 * Executes GitHub Actions workflows locally using the act CLI.
 *
 * Responsibilities:
 * - Ensures act is installed before execution
 * - Spawns act process with appropriate flags
 * - Captures stdout and stderr streams
 * - Implements retry logic for flaky executions
 * - Tracks execution duration
 */
export class ActExecutor {
  private readonly config: Required<ExecutorConfig>;

  constructor(config: ExecutorConfig = {}) {
    this.config = {
      verbose: config.verbose ?? false,
      maxRetries: config.maxRetries ?? 2,
      retryDelay: config.retryDelay ?? 1000,
    };
  }

  /**
   * Executes workflows in the prepared worktree environment.
   *
   * @param prepareResult - Environment prepared by the CheckRunner
   * @returns Execution result with exit code, output, and duration
   */
  async execute(prepareResult: PrepareResult): Promise<ExecuteResult> {
    const { path: actPath } = await ensureInstalled();

    const args = this.buildActArgs(prepareResult);

    let lastResult: ExecuteResult | undefined;
    for (let attempt = 0; attempt <= this.config.maxRetries; attempt++) {
      if (attempt > 0) {
        const delay = this.config.retryDelay * attempt;
        await new Promise((resolve) => setTimeout(resolve, delay));
      }

      const result = await this.executeOnce(
        actPath,
        args,
        prepareResult.worktreePath
      );

      if (result.exitCode === 0) {
        return result;
      }

      lastResult = result;
    }

    return lastResult as ExecuteResult;
  }

  /**
   * Builds command-line arguments for act based on configuration.
   */
  private buildActArgs(_prepareResult: PrepareResult): readonly string[] {
    const args: string[] = [];

    if (this.config.verbose) {
      args.push("--verbose");
    }

    // Note: workflow and job filtering will be implemented when
    // CheckRunner passes these through PrepareResult or config
    // For now, we run all workflows in the worktree

    return args;
  }

  /**
   * Executes act once without retry logic.
   *
   * @param actPath - Absolute path to the act binary
   * @param args - Command-line arguments for act
   * @param cwd - Working directory (worktree path)
   * @returns Execution result
   */
  private executeOnce(
    actPath: string,
    args: readonly string[],
    cwd: string
  ): Promise<ExecuteResult> {
    const startTime = Date.now();

    return new Promise((resolve) => {
      const child = spawn(actPath, args, {
        cwd,
        stdio: ["ignore", "pipe", "pipe"],
      });

      const stdoutChunks: Buffer[] = [];
      const stderrChunks: Buffer[] = [];

      child.stdout.on("data", (chunk: Buffer) => {
        stdoutChunks.push(chunk);
      });

      child.stderr.on("data", (chunk: Buffer) => {
        stderrChunks.push(chunk);
      });

      child.on("close", (code) => {
        const duration = Date.now() - startTime;
        const exitCode = code ?? 1;

        const stdout = Buffer.concat(stdoutChunks).toString("utf-8");
        const stderr = Buffer.concat(stderrChunks).toString("utf-8");

        resolve({
          exitCode,
          stdout,
          stderr,
          duration,
        });
      });

      child.on("error", (error) => {
        const duration = Date.now() - startTime;

        resolve({
          exitCode: 1,
          stdout: Buffer.concat(stdoutChunks).toString("utf-8"),
          stderr: `Process error: ${error.message}\n${Buffer.concat(stderrChunks).toString("utf-8")}`,
          duration,
        });
      });
    });
  }
}
