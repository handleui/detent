import { ParserClient } from "../services/parser-client.js";
import { formatError } from "../utils/error.js";
import type { ExecuteResult, ParsedError, ProcessResult } from "./types.js";

/**
 * Configuration for the error processor.
 */
interface ProcessorConfig {
  /**
   * Base URL for the parser service.
   * Defaults to PARSER_URL env var or http://localhost:8080
   */
  readonly parserUrl?: string;

  /**
   * Request timeout in milliseconds.
   * Defaults to 30000ms (30 seconds)
   */
  readonly timeout?: number;
}

/**
 * Processes workflow execution output to extract and parse errors.
 *
 * Responsibilities:
 * - Combines stdout and stderr into complete log output
 * - Sends logs to the parser service via HTTP
 * - Handles parser service failures gracefully
 * - Returns structured error information
 */
export class ErrorProcessor {
  private readonly parser: ParserClient;

  constructor(config: ProcessorConfig = {}) {
    this.parser = new ParserClient({
      baseUrl: config.parserUrl,
      timeout: config.timeout,
    });
  }

  /**
   * Processes execution output to extract parsed errors.
   *
   * @param executeResult - Result from workflow execution
   * @returns Processing result with parsed errors and count
   */
  async process(executeResult: ExecuteResult): Promise<ProcessResult> {
    const combinedLogs = this.combineLogs(executeResult);

    if (combinedLogs.trim().length === 0) {
      return {
        errors: [],
        errorCount: 0,
      };
    }

    try {
      const response = await this.parser.parse(combinedLogs);

      return {
        errors: response.errors,
        errorCount: response.errors.length,
      };
    } catch (error) {
      console.warn(`Warning: Parser service failed: ${formatError(error)}`);
      console.warn("Continuing without error parsing...");

      return {
        errors: [],
        errorCount: 0,
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
