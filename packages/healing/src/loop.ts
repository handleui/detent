import type {
  ContentBlock,
  MessageParam,
} from "@anthropic-ai/sdk/resources/messages.js";
import type { Client } from "./client.js";
import { calculateCost } from "./pricing.js";
import type { ToolRegistry } from "./tools/registry.js";
import type { HealConfig, HealResult, TokenUsage } from "./types.js";
import { DEFAULT_CONFIG } from "./types.js";

/**
 * Maximum number of tool call rounds (not user-configurable).
 */
const MAX_ITERATIONS = 50;

/**
 * Maximum tokens per response.
 */
const MAX_TOKENS_PER_RESPONSE = 8192;

/**
 * Maps tool names to their key parameter for verbose output.
 */
const KEY_PARAM_NAMES: Record<string, string> = {
  read_file: "path",
  edit_file: "path",
  glob: "pattern",
  grep: "pattern",
  run_check: "category",
  run_command: "command",
};

/**
 * Extracts the most relevant parameter for verbose output.
 */
const extractKeyParam = (
  toolName: string,
  input: Record<string, unknown>
): string => {
  const paramName = KEY_PARAM_NAMES[toolName];
  if (!paramName) {
    return "";
  }

  const value = input[paramName];
  if (typeof value !== "string") {
    return "";
  }

  if (value.length > 50) {
    return `${value.slice(0, 47)}...`;
  }
  return value;
};

/**
 * Creates the initial result object.
 */
const createInitialResult = (): HealResult => ({
  success: false,
  iterations: 0,
  finalMessage: "",
  toolCalls: 0,
  duration: 0,
  inputTokens: 0,
  outputTokens: 0,
  cacheCreationInputTokens: 0,
  cacheReadInputTokens: 0,
  costUSD: 0,
  budgetExceeded: false,
});

/**
 * Calculates token usage from result.
 */
const getUsageFromResult = (result: HealResult): TokenUsage => ({
  inputTokens: result.inputTokens,
  outputTokens: result.outputTokens,
  cacheCreationInputTokens: result.cacheCreationInputTokens,
  cacheReadInputTokens: result.cacheReadInputTokens,
});

/**
 * Checks if budget limits have been exceeded.
 */
const checkBudgetLimits = (
  config: HealConfig,
  result: HealResult,
  startTime: number
): HealResult | null => {
  if (config.budgetPerRunUSD > 0 && result.costUSD > config.budgetPerRunUSD) {
    return {
      ...result,
      budgetExceeded: true,
      budgetExceededReason: "per-run",
      duration: Date.now() - startTime,
    };
  }

  if (
    config.remainingMonthlyUSD >= 0 &&
    result.costUSD > config.remainingMonthlyUSD
  ) {
    return {
      ...result,
      budgetExceeded: true,
      budgetExceededReason: "monthly",
      duration: Date.now() - startTime,
    };
  }

  return null;
};

/**
 * HealLoop orchestrates the agentic healing process.
 */
export class HealLoop {
  private readonly client: Client;
  private readonly registry: ToolRegistry;
  private readonly config: HealConfig;
  private readonly verboseWriter: ((msg: string) => void) | null;

  constructor(
    client: Client,
    registry: ToolRegistry,
    config: Partial<HealConfig> = {}
  ) {
    this.client = client;
    this.registry = registry;
    this.config = { ...DEFAULT_CONFIG, ...config };
    this.verboseWriter = this.config.verbose
      ? (msg: string) => process.stderr.write(msg)
      : null;
  }

  /**
   * Executes the healing loop with the given system prompt and initial user message.
   */
  run = async (
    systemPrompt: string,
    userPrompt: string
  ): Promise<HealResult> => {
    const startTime = Date.now();
    const messages: MessageParam[] = [{ role: "user", content: userPrompt }];
    const result = createInitialResult();
    const api = this.client.api;
    const modelName = this.config.model;

    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(
        () => reject(new Error("Healing loop timeout exceeded")),
        this.config.timeout
      );
    });

    try {
      for (let iteration = 0; iteration < MAX_ITERATIONS; iteration++) {
        result.iterations = iteration + 1;

        const response = await Promise.race([
          api.messages.create({
            model: modelName,
            max_tokens: MAX_TOKENS_PER_RESPONSE,
            system: systemPrompt,
            messages,
            tools: this.registry.toAnthropicTools(),
          }),
          timeoutPromise,
        ]);

        this.updateTokenUsage(result, response.usage, modelName);

        const budgetExceeded = checkBudgetLimits(
          this.config,
          result,
          startTime
        );
        if (budgetExceeded) {
          return budgetExceeded;
        }

        if (response.stop_reason === "end_turn") {
          return this.createSuccessResult(result, response.content, startTime);
        }

        const { toolResults, hasToolUse } = await this.processToolCalls(
          response.content,
          result
        );

        if (!hasToolUse) {
          return this.createSuccessResult(result, response.content, startTime);
        }

        messages.push(
          { role: "assistant", content: response.content },
          { role: "user", content: toolResults }
        );
      }

      result.duration = Date.now() - startTime;
      result.finalMessage = `Max iterations (${MAX_ITERATIONS}) exceeded`;
      return result;
    } catch (error) {
      result.duration = Date.now() - startTime;
      result.costUSD = calculateCost(modelName, getUsageFromResult(result));
      result.finalMessage =
        error instanceof Error ? error.message : "Unknown error occurred";
      return result;
    }
  };

  /**
   * Updates the result with token usage from response.
   */
  private readonly updateTokenUsage = (
    result: HealResult,
    usage: {
      input_tokens: number;
      output_tokens: number;
      cache_creation_input_tokens?: number | null;
      cache_read_input_tokens?: number | null;
    },
    modelName: string
  ): void => {
    result.inputTokens += usage.input_tokens;
    result.outputTokens += usage.output_tokens;
    result.cacheCreationInputTokens += usage.cache_creation_input_tokens ?? 0;
    result.cacheReadInputTokens += usage.cache_read_input_tokens ?? 0;
    result.costUSD = calculateCost(modelName, getUsageFromResult(result));
  };

  /**
   * Creates a success result with final message.
   */
  private readonly createSuccessResult = (
    result: HealResult,
    content: ContentBlock[],
    startTime: number
  ): HealResult => ({
    ...result,
    finalMessage: this.extractTextContent(content),
    success: true,
    duration: Date.now() - startTime,
  });

  /**
   * Processes tool calls from response content.
   */
  private readonly processToolCalls = async (
    content: ContentBlock[],
    result: HealResult
  ): Promise<{
    toolResults: Array<{
      type: "tool_result";
      tool_use_id: string;
      content: string;
      is_error?: boolean;
    }>;
    hasToolUse: boolean;
  }> => {
    const toolResults: Array<{
      type: "tool_result";
      tool_use_id: string;
      content: string;
      is_error?: boolean;
    }> = [];
    let hasToolUse = false;

    for (const block of content) {
      if (block.type === "tool_use") {
        hasToolUse = true;
        result.toolCalls++;

        const input = block.input as Record<string, unknown>;
        this.logToolCall(block.name, input);

        const toolResult = await this.registry.dispatch(block.name, input);
        toolResults.push({
          type: "tool_result",
          tool_use_id: block.id,
          content: toolResult.content,
          is_error: toolResult.isError || undefined,
        });
      }
    }

    return { toolResults, hasToolUse };
  };

  /**
   * Extracts text content from a response.
   */
  private readonly extractTextContent = (content: ContentBlock[]): string => {
    for (const block of content) {
      if (block.type === "text") {
        return block.text;
      }
    }
    return "";
  };

  /**
   * Logs a tool call in verbose mode with the key parameter.
   */
  private readonly logToolCall = (
    toolName: string,
    input: Record<string, unknown>
  ): void => {
    if (!this.verboseWriter) {
      return;
    }

    const keyParam = extractKeyParam(toolName, input);
    if (keyParam) {
      this.verboseWriter(`  -> ${toolName}: ${keyParam}\n`);
    } else {
      this.verboseWriter(`  -> ${toolName}\n`);
    }
  };
}

/**
 * Creates a config from settings.
 * This is the canonical way to configure the healing loop from application settings.
 */
export const createConfig = (
  model: string,
  timeoutMins: number,
  budgetPerRunUSD: number,
  remainingMonthlyUSD: number
): HealConfig => ({
  timeout: timeoutMins > 0 ? timeoutMins * 60_000 : DEFAULT_CONFIG.timeout,
  model: model || DEFAULT_CONFIG.model,
  budgetPerRunUSD:
    budgetPerRunUSD >= 0 ? budgetPerRunUSD : DEFAULT_CONFIG.budgetPerRunUSD,
  remainingMonthlyUSD,
  verbose: false,
});
