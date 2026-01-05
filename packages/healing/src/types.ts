/**
 * Configuration for the healing loop.
 */
export interface HealConfig {
  /** Total timeout in milliseconds */
  timeout: number;
  /** Model to use for healing */
  model: string;
  /** Budget limit per run in USD (0 = unlimited) */
  budgetPerRunUSD: number;
  /** Remaining monthly budget in USD (-1 = unlimited) */
  remainingMonthlyUSD: number;
  /** Enable verbose logging */
  verbose: boolean;
}

/**
 * Result of a healing attempt.
 */
export interface HealResult {
  /** Whether the healing was successful */
  success: boolean;
  /** Number of message rounds completed */
  iterations: number;
  /** Claude's final response */
  finalMessage: string;
  /** Total number of tool calls made */
  toolCalls: number;
  /** How long the loop took in milliseconds */
  duration: number;
  /** Total input tokens used across all API calls */
  inputTokens: number;
  /** Total output tokens used across all API calls */
  outputTokens: number;
  /** Total tokens used to create cache entries */
  cacheCreationInputTokens: number;
  /** Total tokens read from cache */
  cacheReadInputTokens: number;
  /** Calculated cost in USD based on token usage */
  costUSD: number;
  /** Whether the loop stopped due to budget limit */
  budgetExceeded: boolean;
  /** Which budget was exceeded */
  budgetExceededReason?: "per-run" | "monthly";
}

/**
 * Token usage for cost calculation.
 */
export interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  cacheCreationInputTokens: number;
  cacheReadInputTokens: number;
}

/**
 * Default configuration values.
 */
export const DEFAULT_CONFIG: HealConfig = {
  timeout: 600_000, // 10 minutes
  model: "claude-sonnet-4-20250514",
  budgetPerRunUSD: 1.0,
  remainingMonthlyUSD: -1, // unlimited by default
  verbose: false,
};
