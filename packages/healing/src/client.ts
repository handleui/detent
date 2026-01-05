import Anthropic from "@anthropic-ai/sdk";

/**
 * Default per-request timeout for API calls (30 seconds).
 */
const DEFAULT_REQUEST_TIMEOUT_MS = 30_000;

/**
 * Client wraps the Anthropic SDK for healing operations.
 */
export class Client {
  private readonly anthropic: Anthropic;

  constructor(apiKey: string) {
    if (!apiKey) {
      throw new Error("No API key provided");
    }

    this.anthropic = new Anthropic({
      apiKey,
      timeout: DEFAULT_REQUEST_TIMEOUT_MS,
    });
  }

  /**
   * Returns the underlying Anthropic client for use in the healing loop.
   */
  get api(): Anthropic {
    return this.anthropic;
  }

  /**
   * Tests the API connection by sending a simple request.
   * Uses Haiku for cost efficiency (~$0.0002/call).
   */
  test = async (): Promise<string> => {
    try {
      const message = await this.anthropic.messages.create({
        model: "claude-3-5-haiku-latest",
        max_tokens: 100,
        messages: [
          {
            role: "user",
            content: "Say 'Hello from Claude!' in exactly 5 words.",
          },
        ],
      });

      for (const block of message.content) {
        if (block.type === "text") {
          return block.text;
        }
      }

      throw new Error("No text response from Claude");
    } catch (error) {
      throw this.formatAPIError(error);
    }
  };

  /**
   * Provides user-friendly error messages for Anthropic API errors.
   */
  private readonly formatAPIError = (error: unknown): Error => {
    if (error instanceof Anthropic.APIError) {
      switch (error.status) {
        case 401:
          return new Error(
            "Invalid API key: check your ANTHROPIC_API_KEY or ~/.detent/config.jsonc"
          );
        case 403:
          return new Error(`API key lacks permission: ${error.message}`);
        case 429:
          return new Error("Rate limited: too many requests, try again later");
        case 500:
        case 502:
        case 503:
          return new Error(
            `Anthropic API unavailable (status ${error.status}): try again later`
          );
        case 529:
          return new Error("Anthropic API overloaded: try again later");
        default:
          return new Error(
            `API error (status ${error.status}): ${error.message}`
          );
      }
    }

    if (error instanceof Error) {
      return new Error(`API request failed: ${error.message}`);
    }

    return new Error("API request failed: unknown error");
  };
}
