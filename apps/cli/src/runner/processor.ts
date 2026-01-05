import { ParserClient } from "../services/parser-client.js";
import type { DebugLogger } from "../utils/debug-logger.js";
import type { ExecuteResult, ParsedError, ProcessResult } from "./types.js";

/**
 * Configuration for the error processor.
 */
interface ProcessorConfig {
  /**
   * Optional debug logger for troubleshooting
   */
  readonly debugLogger?: DebugLogger;
}

/**
 * Processes workflow execution output to extract and parse errors.
 *
 * Responsibilities:
 * - Combines stdout and stderr into complete log output
 * - Uses local @detent/parser package to extract errors
 * - Handles parser failures gracefully
 * - Returns structured error information
 */
export class ErrorProcessor {
  private readonly parser: ParserClient;
  private readonly debugLogger?: DebugLogger;

  constructor(config: ProcessorConfig = {}) {
    this.parser = new ParserClient();
    this.debugLogger = config.debugLogger;
  }

  /**
   * Processes execution output to extract parsed errors.
   *
   * @param executeResult - Result from workflow execution
   * @returns Processing result with parsed errors and count
   */
  async process(executeResult: ExecuteResult): Promise<ProcessResult> {
    this.debugLogger?.startPhase("Process");

    const combinedLogs = this.combineLogs(executeResult);
    this.debugLogger?.logPhase(
      "Process",
      `Parsing ${combinedLogs.length} bytes of output`
    );

    if (combinedLogs.trim().length === 0) {
      this.debugLogger?.logPhase("Process", "No output to parse");
      this.debugLogger?.endPhase("Process");
      return {
        errors: [],
        errorCount: 0,
      };
    }

    try {
      const response = await this.parser.parse(combinedLogs);

      this.debugLogger?.logPhase(
        "Process",
        `Extracted ${response.errors.length} error(s)`
      );

      if (response.errors.length > 0) {
        for (const error of response.errors) {
          this.debugLogger?.logPhase(
            "Process",
            `  - ${error.errorId}: ${error.message.substring(0, 80)}${error.message.length > 80 ? "..." : ""}`
          );
        }
      }

      this.debugLogger?.endPhase("Process");

      return {
        errors: response.errors,
        errorCount: response.errors.length,
      };
    } catch (error) {
      this.debugLogger?.logError(error, "Parser");

      this.debugLogger?.endPhase("Process");

      return {
        errors: [],
        errorCount: 0,
        parserFailed: true,
      };
    }
  }

  /**
   * Persists parsed errors to storage.
   *
   * TODO: Future integration with @detent/persistence
   * This will be implemented when the persistence layer is ready.
   * For now, this is a no-op to maintain the interface contract.
   *
   * @param _errors - Parsed errors to persist (unused)
   * @param _runID - Run identifier for the errors (unused)
   */
  async persist(
    _errors: readonly ParsedError[],
    _runID: string
  ): Promise<void> {
    // No-op: Persistence will be implemented in a future phase
    // when @detent/persistence is ready to handle error storage
  }

  /**
   * Combines stdout and stderr into a single log string for parsing.
   *
   * @param executeResult - Execution result with output streams
   * @returns Combined log output
   */
  private combineLogs(executeResult: ExecuteResult): string {
    const parts: string[] = [];

    if (executeResult.stdout.trim().length > 0) {
      parts.push(executeResult.stdout);
    }

    if (executeResult.stderr.trim().length > 0) {
      parts.push(executeResult.stderr);
    }

    return parts.join("\n");
  }
}
