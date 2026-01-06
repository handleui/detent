import { spawn } from "node:child_process";
import { ensureInstalled } from "../act/index.js";
import type { DebugLogger } from "../utils/debug-logger.js";
import { ActOutputParser } from "./act-output-parser.js";
import type { TUIEventEmitter } from "./event-emitter.js";
import type { ExecuteResult, PrepareResult } from "./types.js";

/**
 * Maximum buffer size for stdout parsing (1MB).
 * Prevents memory exhaustion on verbose output.
 */
const MAX_BUFFER_SIZE = 1024 * 1024;

/**
 * Maximum total output size to retain (50MB per stream).
 * Older chunks are discarded to prevent memory exhaustion.
 */
const MAX_OUTPUT_SIZE = 50 * 1024 * 1024;

/**
 * Exit code returned when execution is aborted.
 */
const ABORT_EXIT_CODE = 130;

/**
 * Safe environment variable prefixes to pass to act containers.
 * Matches Go CLI filterEnvironment for security consistency.
 * Prevents secret leakage to containers.
 */
const SAFE_ENV_PREFIXES = [
  "PATH=",
  "HOME=",
  "USER=",
  "SHELL=",
  "LANG=",
  "LC_",
  "TERM=",
  "TMPDIR=",
  "TZ=",
] as const;

/**
 * Filters environment variables to only include safe ones.
 * This prevents secrets from leaking to act containers.
 */
const filterEnvironment = (): Record<string, string> => {
  const filtered: Record<string, string> = {};
  for (const [key, value] of Object.entries(process.env)) {
    if (value === undefined) {
      continue;
    }
    const entry = `${key}=`;
    if (SAFE_ENV_PREFIXES.some((prefix) => entry.startsWith(prefix))) {
      filtered[key] = value;
    }
  }
  return filtered;
};

/**
 * State for stdout stream processing
 */
interface StdoutProcessingState {
  buffer: string;
  parser: ActOutputParser;
}

/**
 * State for tracking accumulated output chunks with size limit
 */
interface OutputAccumulator {
  chunks: Buffer[];
  totalSize: number;
}

/**
 * Adds a chunk to the accumulator, discarding oldest chunks if size limit exceeded
 */
const addChunkToAccumulator = (
  accumulator: OutputAccumulator,
  chunk: Buffer
): void => {
  accumulator.chunks.push(chunk);
  accumulator.totalSize += chunk.length;

  // Discard oldest chunks if we exceed the limit
  while (
    accumulator.totalSize > MAX_OUTPUT_SIZE &&
    accumulator.chunks.length > 1
  ) {
    const removed = accumulator.chunks.shift();
    if (removed) {
      accumulator.totalSize -= removed.length;
    }
  }
};

/**
 * Truncates buffer if it exceeds max size, keeping content after last newline
 */
const truncateBufferIfNeeded = (
  buffer: string,
  debugLogger?: DebugLogger
): string => {
  if (buffer.length <= MAX_BUFFER_SIZE) {
    return buffer;
  }
  const lastNewline = buffer.lastIndexOf("\n");
  if (lastNewline > 0) {
    debugLogger?.log(
      `[Execute] Buffer exceeded ${MAX_BUFFER_SIZE} bytes, truncating`
    );
    return buffer.slice(lastNewline + 1);
  }
  return buffer;
};

/**
 * Parses lines from buffer and emits TUI events
 */
const processLinesForTUI = (
  state: StdoutProcessingState,
  eventEmitter: TUIEventEmitter
): void => {
  const lines = state.buffer.split("\n");
  state.buffer = lines.pop() ?? "";

  for (const line of lines) {
    const events = state.parser.parseLine(line);
    for (const event of events) {
      eventEmitter.emit(event);
    }
    eventEmitter.emit({
      type: "log",
      content: line,
    });
  }
};

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

  /**
   * Optional event emitter for TUI updates
   */
  readonly eventEmitter?: TUIEventEmitter;

  /**
   * Optional debug logger for troubleshooting
   */
  readonly debugLogger?: DebugLogger;
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
  private readonly config: {
    readonly verbose: boolean;
    readonly maxRetries: number;
    readonly retryDelay: number;
  };

  private readonly eventEmitter?: TUIEventEmitter;
  private readonly debugLogger?: DebugLogger;
  private currentChild: ReturnType<typeof spawn> | undefined;
  private aborted = false;

  constructor(config: ExecutorConfig = {}) {
    this.config = {
      verbose: config.verbose ?? false,
      // Disable retries by default - act completing with failed jobs shouldn't trigger retry
      // Retries were causing duplicate job runs (ShellCheck running 3x)
      maxRetries: config.maxRetries ?? 0,
      retryDelay: config.retryDelay ?? 1000,
    };
    this.eventEmitter = config.eventEmitter;
    this.debugLogger = config.debugLogger;
  }

  /**
   * Aborts any running execution.
   * Kills the child process group (including Docker containers spawned by act).
   */
  abort(): void {
    this.aborted = true;
    if (
      this.currentChild &&
      !this.currentChild.killed &&
      this.currentChild.pid
    ) {
      this.debugLogger?.log(
        "[Execute] Aborting execution, killing process group"
      );

      // Kill the entire process group (negative PID on Unix)
      // This ensures Docker containers spawned by act are also killed
      try {
        // On Unix, killing -pid sends signal to entire process group
        process.kill(-this.currentChild.pid, "SIGTERM");
      } catch {
        // Fallback to killing just the child if process group kill fails
        // (e.g., on Windows or if process already exited)
        try {
          this.currentChild.kill("SIGTERM");
        } catch {
          // Process may have already exited
        }
      }
    }
  }

  /**
   * Returns whether the executor has been aborted.
   */
  isAborted(): boolean {
    return this.aborted;
  }

  /**
   * Executes workflows in the prepared worktree environment.
   *
   * @param prepareResult - Environment prepared by the CheckRunner
   * @returns Execution result with exit code, output, and duration
   */
  async execute(prepareResult: PrepareResult): Promise<ExecuteResult> {
    if (this.aborted) {
      return {
        exitCode: ABORT_EXIT_CODE,
        stdout: "",
        stderr: "Execution aborted",
        duration: 0,
      };
    }

    const { path: actPath } = await ensureInstalled();

    const args = this.buildActArgs(prepareResult);

    let lastResult: ExecuteResult | undefined;
    for (let attempt = 0; attempt <= this.config.maxRetries; attempt++) {
      if (this.aborted) {
        this.debugLogger?.log("[Execute] Aborted, skipping retry");
        break;
      }

      if (attempt > 0) {
        const delay = this.config.retryDelay * attempt;
        this.debugLogger?.log(
          `[Execute] Retry attempt ${attempt} after ${delay}ms delay`
        );
        await new Promise((resolve) => setTimeout(resolve, delay));
      }

      const result = await this.executeOnce(
        actPath,
        args,
        prepareResult.clonePath
      );

      if (result.exitCode === 0 || this.aborted) {
        return result;
      }

      lastResult = result;
    }

    return (
      lastResult ?? {
        exitCode: ABORT_EXIT_CODE,
        stdout: "",
        stderr: "Execution aborted",
        duration: 0,
      }
    );
  }

  /**
   * Builds command-line arguments for act based on configuration.
   * Matches Go CLI configuration for consistency.
   */
  private buildActArgs(_prepareResult: PrepareResult): readonly string[] {
    const args: string[] = [];

    // ALWAYS use verbose mode to capture step output for error extraction
    // This matches Go CLI behavior - verbose is needed for parsing
    args.push("-v");

    // Use medium-sized images pre-configured for act (catthehacker images)
    // These have Git safe.directory configured and other act-specific setup
    args.push("-P", "ubuntu-latest=catthehacker/ubuntu:act-latest");
    args.push("-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04");
    args.push("-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04");

    // Docker-resilient flags to prevent container buildup and failures
    args.push("--rm"); // Remove containers after execution
    args.push("--no-cache-server"); // Disable cache server (can cause hangs/failures)

    // Container security hardening: drop dangerous capabilities
    args.push("--container-cap-drop", "SYS_ADMIN"); // Prevents container escapes via mount
    args.push("--container-cap-drop", "NET_ADMIN"); // Prevents network manipulation
    args.push("--container-cap-drop", "SYS_PTRACE"); // Prevents process debugging/injection
    args.push("--container-cap-drop", "MKNOD"); // Prevents device node creation

    // Pass CI environment to containers to disable git hook installation.
    // This is the official approach documented by lefthook:
    // https://lefthook.dev/usage/envs/CI.html
    args.push("--env", "CI=true");
    args.push("--env", "LEFTHOOK=0");
    args.push("--env", "HUSKY=0");
    args.push("--env", "PRE_COMMIT_ALLOW_NO_CONFIG=1");

    // Note: No special mounts needed - shallow clones have self-contained .git directories
    // that work natively with Docker (unlike worktrees which use .git files pointing externally)

    return args;
  }

  /**
   * Executes act once without retry logic.
   *
   * @param actPath - Absolute path to the act binary
   * @param args - Command-line arguments for act
   * @param cwd - Working directory (clone path)
   * @returns Execution result
   */
  private executeOnce(
    actPath: string,
    args: readonly string[],
    cwd: string
  ): Promise<ExecuteResult> {
    const startTime = Date.now();
    const stdoutState: StdoutProcessingState = {
      buffer: "",
      parser: new ActOutputParser(),
    };

    return new Promise((resolve) => {
      const child = spawn(actPath, args, {
        cwd,
        stdio: ["ignore", "pipe", "pipe"],
        // Create new process group so we can kill all children (including Docker)
        detached: process.platform !== "win32",
        // Filter environment to prevent secret leakage (matches Go CLI)
        env: filterEnvironment(),
      });

      this.currentChild = child;

      // Use accumulators with size limits to prevent memory exhaustion
      const stdoutAccum: OutputAccumulator = { chunks: [], totalSize: 0 };
      const stderrAccum: OutputAccumulator = { chunks: [], totalSize: 0 };

      child.stdout.on("data", (chunk: Buffer) => {
        addChunkToAccumulator(stdoutAccum, chunk);
        const text = chunk.toString("utf-8");

        this.debugLogger?.logActOutput(text);

        if (this.config.verbose) {
          process.stdout.write(chunk);
        }

        if (this.eventEmitter) {
          stdoutState.buffer += text;
          stdoutState.buffer = truncateBufferIfNeeded(
            stdoutState.buffer,
            this.debugLogger
          );
          processLinesForTUI(stdoutState, this.eventEmitter);
        }
      });

      child.stderr.on("data", (chunk: Buffer) => {
        addChunkToAccumulator(stderrAccum, chunk);
        const text = chunk.toString("utf-8");

        // Write to debug log
        this.debugLogger?.logActOutput(text);

        // Stream to stderr in real-time if verbose mode
        if (this.config.verbose) {
          process.stderr.write(chunk);
        }

        // Emit stderr as log events too
        if (this.eventEmitter) {
          for (const line of text.split("\n").filter((l) => l.trim())) {
            this.eventEmitter.emit({
              type: "log",
              content: line,
            });
          }
        }
      });

      child.on("close", (code) => {
        this.currentChild = undefined;
        const duration = Date.now() - startTime;
        const exitCode = this.aborted ? ABORT_EXIT_CODE : (code ?? 1);

        const stdout = Buffer.concat(stdoutAccum.chunks).toString("utf-8");
        const stderr = Buffer.concat(stderrAccum.chunks).toString("utf-8");

        resolve({
          exitCode,
          stdout,
          stderr,
          duration,
        });
      });

      child.on("error", (error) => {
        this.currentChild = undefined;
        const duration = Date.now() - startTime;

        this.debugLogger?.logError(error, "ActExecutor");

        if (this.eventEmitter) {
          this.eventEmitter.emit({
            type: "error",
            error: error instanceof Error ? error : new Error(String(error)),
            message: error.message,
          });
        }

        resolve({
          exitCode: 1,
          stdout: Buffer.concat(stdoutAccum.chunks).toString("utf-8"),
          stderr: `Process error: ${error.message}\n${Buffer.concat(stderrAccum.chunks).toString("utf-8")}`,
          duration,
        });
      });
    });
  }
}
