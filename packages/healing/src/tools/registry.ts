import type { Tool as AnthropicTool } from "@anthropic-ai/sdk/resources/messages.js";
import type { ToolContext } from "./context.js";
import type { Tool, ToolResult } from "./types.js";
import { errorResult } from "./types.js";

/**
 * Registry for managing and dispatching tools.
 */
export class ToolRegistry {
  private readonly tools: Map<string, Tool> = new Map();
  private readonly ctx: ToolContext;
  private cachedAnthropicTools: AnthropicTool[] | null = null;

  constructor(ctx: ToolContext) {
    this.ctx = ctx;
  }

  /**
   * Registers a tool with the registry.
   */
  register = (tool: Tool): void => {
    this.tools.set(tool.name, tool);
    this.cachedAnthropicTools = null;
  };

  /**
   * Registers multiple tools at once.
   */
  registerAll = (tools: Tool[]): void => {
    for (const tool of tools) {
      this.register(tool);
    }
  };

  /**
   * Gets a tool by name.
   */
  get = (name: string): Tool | undefined => this.tools.get(name);

  /**
   * Checks if a tool exists.
   */
  has = (name: string): boolean => this.tools.has(name);

  /**
   * Dispatches a tool call by name with the given input.
   */
  dispatch = async (name: string, input: unknown): Promise<ToolResult> => {
    const tool = this.tools.get(name);
    if (!tool) {
      return errorResult(`unknown tool: ${name}`);
    }

    try {
      return await tool.execute(this.ctx, input);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      return errorResult(`tool execution failed: ${message}`);
    }
  };

  /**
   * Converts registered tools to Anthropic SDK format.
   * Results are cached until a new tool is registered.
   */
  toAnthropicTools = (): AnthropicTool[] => {
    if (this.cachedAnthropicTools) {
      return this.cachedAnthropicTools;
    }

    this.cachedAnthropicTools = Array.from(this.tools.values()).map(
      (tool): AnthropicTool => ({
        name: tool.name,
        description: tool.description,
        input_schema: {
          type: "object",
          ...tool.inputSchema,
        },
      })
    );

    return this.cachedAnthropicTools;
  };

  /**
   * Returns the number of registered tools.
   */
  get size(): number {
    return this.tools.size;
  }

  /**
   * Returns all tool names.
   */
  get names(): string[] {
    return Array.from(this.tools.keys());
  }
}

/**
 * Creates a new tool registry with the given context.
 */
export const createToolRegistry = (ctx: ToolContext): ToolRegistry =>
  new ToolRegistry(ctx);
