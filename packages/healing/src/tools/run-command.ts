import {
  approveCommand,
  denyCommand,
  isCommandApproved,
  isCommandDenied,
  type ToolContext,
} from "./context.js";
import {
  BLOCKED_COMMANDS,
  executeCommand,
  extractBaseCommand,
  hasBlockedBytes,
  hasBlockedPattern,
  normalizeCommand,
} from "./execute.js";
import {
  errorResult,
  SchemaBuilder,
  type Tool,
  type ToolResult,
} from "./types.js";

/**
 * Safe commands that are always allowed without prompting.
 * Map of base command to allowed subcommands (null = any subcommand OK).
 */
const SAFE_COMMANDS: Record<string, string[] | null> = {
  go: ["build", "test", "fmt", "vet", "mod", "generate", "install", "run"],
  "golangci-lint": ["run"],
  gofumpt: null,
  goimports: null,
  staticcheck: null,
  govulncheck: null,
  npm: ["install", "ci", "test", "run"],
  yarn: ["install", "test", "run"],
  pnpm: ["install", "test", "run"],
  bun: ["install", "test", "run", "x"],
  npx: null,
  bunx: null,
  cargo: ["build", "test", "check", "fmt", "clippy", "run"],
  rustfmt: null,
  python: ["-m"],
  python3: ["-m"],
  pip: ["install"],
  pip3: ["install"],
  pytest: null,
  mypy: null,
  ruff: ["check", "format"],
  black: null,
  eslint: null,
  prettier: null,
  tsc: null,
  biome: ["check", "format", "lint"],
};

/**
 * Safe npx/bunx commands that are always allowed.
 */
const SAFE_NPX_COMMANDS = new Set([
  "eslint",
  "prettier",
  "biome",
  "oxlint",
  "tsc",
  "tsc-watch",
  "vitest",
  "jest",
  "turbo",
  "nx",
]);

/**
 * Input schema for run_command.
 */
interface RunCommandInput {
  command: string;
}

/**
 * Checks if command is in the built-in safe list.
 */
const isSafeCommand = (baseCmd: string, subCmd: string): boolean => {
  const actualBase = extractBaseCommand(baseCmd);
  const allowedSubs = SAFE_COMMANDS[actualBase];

  if (allowedSubs === undefined) {
    return false;
  }

  if (allowedSubs === null) {
    if (actualBase === "npx" || actualBase === "bunx") {
      return SAFE_NPX_COMMANDS.has(subCmd);
    }
    return true;
  }

  return allowedSubs.includes(subCmd);
};

/**
 * Checks if a command is allowed to run.
 */
const isAllowed = async (
  ctx: ToolContext,
  fullCmd: string,
  parts: string[]
): Promise<boolean> => {
  const baseCmd = parts[0];
  const subCmd = parts[1] ?? "";

  if (baseCmd === undefined) {
    return false;
  }

  if (isSafeCommand(baseCmd, subCmd)) {
    return true;
  }

  if (ctx.commandChecker?.(fullCmd)) {
    return true;
  }

  if (isCommandApproved(ctx, fullCmd)) {
    return true;
  }

  if (isCommandDenied(ctx, fullCmd)) {
    return false;
  }

  if (ctx.commandApprover) {
    const decision = await ctx.commandApprover(fullCmd);

    if (decision === "deny" || decision === "never") {
      denyCommand(ctx, fullCmd);
      return false;
    }

    if (decision === "always" && ctx.commandPersister) {
      try {
        await ctx.commandPersister(fullCmd);
      } catch (err) {
        console.error("warning: failed to save command:", err);
      }
    }

    approveCommand(ctx, fullCmd);
    return true;
  }

  return false;
};

/**
 * Run command tool - executes commands with safety checks.
 */
export const runCommandTool: Tool = {
  name: "run_command",
  description:
    "Run a shell command. Common build/test/lint commands are pre-approved. Other commands require user approval.",
  inputSchema: new SchemaBuilder()
    .addString(
      "command",
      "The command to run (e.g., 'go test ./...', 'npm run lint', 'make build')"
    )
    .build(),

  execute: async (ctx: ToolContext, input: unknown): Promise<ToolResult> => {
    const { command } = input as RunCommandInput;

    if (!command) {
      return errorResult("command is required");
    }

    if (hasBlockedBytes(command)) {
      return errorResult("command contains invalid characters");
    }

    const normalizedCmd = normalizeCommand(command);

    const blockedPattern = hasBlockedPattern(normalizedCmd);
    if (blockedPattern) {
      return errorResult(`blocked pattern: "${blockedPattern}"`);
    }

    const parts = normalizedCmd.split(" ").filter(Boolean);
    if (parts.length === 0 || parts[0] === undefined) {
      return errorResult("empty command");
    }

    const baseCmd = extractBaseCommand(parts[0]);
    if (BLOCKED_COMMANDS.has(baseCmd)) {
      return errorResult(`blocked command: "${baseCmd}"`);
    }

    const allowed = await isAllowed(ctx, normalizedCmd, parts);
    if (!allowed) {
      return errorResult(`command not approved: ${normalizedCmd}`);
    }

    return executeCommand(ctx.worktreePath, normalizedCmd, parts);
  },
};
