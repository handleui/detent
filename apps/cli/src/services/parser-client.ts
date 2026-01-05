import {
  actParser,
  createExtractor,
  createGenericParser,
  createGolangParser,
  createPythonParser,
  createRegistry,
  createRustParser,
  createTypeScriptParser,
  type ExtractedError,
} from "@detent/parser";
import type { ParsedError } from "../runner/types.js";

/**
 * Response from the parser service.
 */
export interface ParserResponse {
  /**
   * Array of parsed errors extracted from the logs.
   */
  readonly errors: readonly ParsedError[];
}

/**
 * Parser client that uses the local @detent/parser package to extract errors from logs.
 * Replaces the previous HTTP-based parser service.
 */
export class ParserClient {
  private static readonly MAX_LOG_SIZE = 100 * 1024 * 1024; // 100MB limit

  constructor(config?: { baseUrl?: string; timeout?: number }) {
    // Config is no longer used - kept for backwards compatibility
    void config;
  }

  /**
   * Parses workflow execution logs and extracts errors.
   *
   * @param logs - Raw log output from workflow execution
   * @returns Parsed errors from the logs
   * @throws Error if logs are invalid or too large
   */
  async parse(logs: string): Promise<ParserResponse> {
    if (!logs || typeof logs !== "string") {
      throw new Error("Logs must be a non-empty string");
    }

    const logsSizeBytes = Buffer.byteLength(logs, "utf-8");
    if (logsSizeBytes > ParserClient.MAX_LOG_SIZE) {
      throw new Error(
        `Logs size (${(logsSizeBytes / 1024 / 1024).toFixed(2)}MB) exceeds maximum allowed size (${ParserClient.MAX_LOG_SIZE / 1024 / 1024}MB)`
      );
    }

    // Create registry and register all tool parsers
    const registry = createRegistry();
    registry.register(createGolangParser());
    registry.register(createTypeScriptParser());
    registry.register(createPythonParser());
    registry.register(createRustParser());
    registry.register(createGenericParser());
    registry.initNoiseChecker();

    // Create extractor with the registry
    const extractor = createExtractor(registry);

    // Extract errors using the act context parser
    const extractedErrors = extractor.extract(logs, actParser);

    // Convert ExtractedError[] to ParsedError[]
    const parsedErrors = this.convertToParsedErrors(extractedErrors);

    return { errors: parsedErrors };
  }

  /**
   * Converts ExtractedError from @detent/parser to ParsedError format.
   *
   * @param extractedErrors - Errors from the parser
   * @returns Errors in ParsedError format
   */
  private convertToParsedErrors(
    extractedErrors: readonly ExtractedError[]
  ): ParsedError[] {
    return extractedErrors.map((err) => ({
      errorId: this.generateErrorId(err),
      contentHash: this.generateContentHash(err),
      filePath: err.file,
      message: err.message,
      severity: err.severity ?? "error",
    }));
  }

  /**
   * Generates a unique error ID.
   *
   * @param err - Extracted error
   * @returns Unique error ID
   */
  private generateErrorId(err: ExtractedError): string {
    const timestamp = Date.now();
    const random = Math.random().toString(36).slice(2, 11);
    const fileHash = err.file ? this.simpleHash(err.file) : "nofile";
    return `${timestamp}-${fileHash}-${random}`;
  }

  /**
   * Generates a content hash for deduplication.
   *
   * @param err - Extracted error
   * @returns Content hash
   */
  private generateContentHash(err: ExtractedError): string {
    const content = `${err.message}|${err.file ?? ""}|${err.line ?? 0}`;
    return this.simpleHash(content);
  }

  /**
   * Simple hash function for generating IDs and hashes.
   *
   * @param str - String to hash
   * @returns Hash string
   */
  private simpleHash(str: string): string {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
      const char = str.charCodeAt(i);
      // biome-ignore lint/suspicious/noBitwiseOperators: intentional hash computation
      hash = (hash << 5) - hash + char;
      // biome-ignore lint/suspicious/noBitwiseOperators: convert to 32-bit integer
      hash &= hash;
    }
    return Math.abs(hash).toString(36);
  }
}
