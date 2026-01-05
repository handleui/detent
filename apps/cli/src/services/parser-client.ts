import type { ParsedError } from "../runner/types.js";
import { formatError } from "../utils/error.js";

/**
 * Request payload for the parser service.
 */
export interface ParserRequest {
  /**
   * Raw log output to be parsed for errors.
   */
  readonly logs: string;
}

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
 * HTTP client for communicating with the parser service.
 * The parser service analyzes workflow execution logs and extracts structured errors.
 */
export class ParserClient {
  private readonly baseUrl: string;
  private readonly timeout: number;
  private static readonly MAX_LOG_SIZE = 100 * 1024 * 1024; // 100MB limit

  /**
   * Creates a new parser client instance.
   *
   * @param config - Optional configuration
   * @param config.baseUrl - Base URL of the parser service (defaults to PARSER_URL env var or http://localhost:8080)
   * @param config.timeout - Request timeout in milliseconds (defaults to 30000ms)
   */
  constructor(config?: { baseUrl?: string; timeout?: number }) {
    const rawUrl =
      config?.baseUrl ?? process.env.PARSER_URL ?? "http://localhost:8080";
    this.baseUrl = this.validateAndNormalizeUrl(rawUrl);
    this.timeout = config?.timeout ?? 30_000; // 30s default for potentially large log parsing
  }

  /**
   * Validates and normalizes the base URL to prevent injection attacks.
   *
   * @param url - The URL to validate
   * @returns Normalized URL without trailing slash
   * @throws Error if URL is invalid or uses an unsafe protocol
   */
  private validateAndNormalizeUrl(url: string): string {
    if (!url || typeof url !== "string") {
      throw new Error("Parser service URL must be a non-empty string");
    }

    if (url.includes("\0") || url.includes("\n") || url.includes("\r")) {
      throw new Error(
        "Parser service URL contains invalid characters (null bytes or newlines)"
      );
    }

    let parsedUrl: URL;
    try {
      parsedUrl = new URL(url);
    } catch {
      throw new Error(
        `Parser service URL is invalid: ${url}. Must be a valid HTTP(S) URL.`
      );
    }

    if (parsedUrl.protocol !== "http:" && parsedUrl.protocol !== "https:") {
      throw new Error(
        `Parser service URL must use HTTP or HTTPS protocol, got: ${parsedUrl.protocol}`
      );
    }

    return parsedUrl.origin + parsedUrl.pathname.replace(/\/$/, "");
  }

  /**
   * Parses workflow execution logs and extracts errors.
   *
   * @param logs - Raw log output from workflow execution
   * @returns Parsed errors from the logs
   * @throws Error if the request fails, times out, or returns a non-200 status
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

    const signal = AbortSignal.timeout(this.timeout);
    const url = `${this.baseUrl}/parse`;

    try {
      const response = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ logs } satisfies ParserRequest),
        signal,
      });

      if (!response.ok) {
        const errorText = await response.text().catch(() => "Unknown error");
        throw new Error(
          `Parser service returned ${response.status}: ${errorText}`
        );
      }

      let data: unknown;
      try {
        data = await response.json();
      } catch (error) {
        throw new Error(
          `Failed to parse parser service response: ${formatError(error)}`
        );
      }

      if (!data || typeof data !== "object" || Array.isArray(data)) {
        throw new Error("Parser service response must be an object");
      }

      const responseObj = data as Record<string, unknown>;

      if (!("errors" in responseObj && Array.isArray(responseObj.errors))) {
        throw new Error("Parser service response missing 'errors' array");
      }

      return { errors: responseObj.errors } as ParserResponse;
    } catch (error) {
      // Handle timeout specifically
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(
          `Parser service request timed out after ${this.timeout}ms`
        );
      }

      // Handle network errors
      if (error instanceof TypeError) {
        throw new Error(
          `Failed to connect to parser service at ${this.baseUrl}: ${error.message}`
        );
      }

      // Re-throw other errors as-is
      throw error;
    }
  }
}
