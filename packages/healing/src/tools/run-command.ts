import { spawn } from "node:child_process";
import {
  approveCommand,
  denyCommand,
  isCommandApproved,
  isCommandDenied,
  type ToolContext,
} from "./context.js";
import {
  errorResult,
  SchemaBuilder,
  type Tool,
  type ToolResult,
} from "./types.js";

/**
 * Command execution timeout in milliseconds (5 minutes).
 */
const COMMAND_TIMEOUT = 5 * 60 * 1000;

/**
 * Maximum output size in bytes (50KB).
 */
const MAX_OUTPUT = 50 * 1024;

/**
 * Blocked bytes - null, newline, carriage return.
 * Prevents command injection via control characters.
 */
const BLOCKED_BYTES = [0x00, 0x0a, 0x0d];

/**
 * Blocked patterns - always rejected regardless of approval.
 * These patterns are checked after normalizing whitespace.
 */
const BLOCKED_PATTERNS = [
  "rm -rf",
  "rm -r",
  "sudo",
  "chmod",
  "chown",
  "curl",
  "wget",
  "git push",
  "git remote",
  "git config",
  "ssh",
  "scp",
  "nc ",
  "netcat",
  "> /",
  ">>",
  "|",
  "&&",
  "||",
  ";",
  "$(",
  "`",
  "eval",
  "exec",
  "${",
];

/**
 * Blocked base commands - never allowed even with approval.
 */
const BLOCKED_COMMANDS = new Set([
  "rm",
  "sudo",
  "chmod",
  "chown",
  "curl",
  "wget",
  "ssh",
  "scp",
  "nc",
  "netcat",
  "eval",
  "exec",
  "sh",
  "bash",
  "zsh",
  "fish",
  "dash",
]);

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
 * Environment variables that are allowed to pass through.
 */
const ALLOWED_ENV_VARS = new Set([
  "PATH",
  "HOME",
  "USER",
  "TMPDIR",
  "TEMP",
  "TMP",
  "LANG",
  "LC_ALL",
  "LC_CTYPE",
  "SHELL",
  "TERM",
  "GOPATH",
  "GOROOT",
  "GOCACHE",
  "GOMODCACHE",
  "CGO_ENABLED",
  "NODE_ENV",
  "NODE_PATH",
  "NPM_CONFIG_CACHE",
  "CARGO_HOME",
  "RUSTUP_HOME",
  "JAVA_HOME",
  "MAVEN_HOME",
  "GRADLE_HOME",
]);

/**
 * Environment variable suffixes that indicate secrets.
 */
const BLOCKED_ENV_SUFFIXES = [
  "_KEY",
  "_TOKEN",
  "_SECRET",
  "_PASSWORD",
  "_CREDS",
  "_AUTH",
];

/**
 * Input schema for run_command.
 */
interface RunCommandInput {
  command: string;
}

/**
 * Result metadata for run_command.
 */
interface RunCommandMetadata extends Record<string, unknown> {
  exitCode: number;
  timedOut: boolean;
}

const WHITESPACE_REGEX = /\s+/;

/**
 * Normalizes whitespace in a command string.
 */
const normalizeCommand = (cmd: string): string =>
  cmd.split(WHITESPACE_REGEX).join(" ");

/**
 * Extracts the base command name from a path.
 * e.g., "/usr/bin/rm" -> "rm", "rm" -> "rm"
 */
const extractBaseCommand = (cmd: string): string => {
  const lastSlash = cmd.lastIndexOf("/");
  return lastSlash >= 0 ? cmd.slice(lastSlash + 1) : cmd;
};

/**
 * Checks if a command contains blocked bytes.
 */
const hasBlockedBytes = (cmd: string): boolean => {
  for (const char of cmd) {
    const code = char.charCodeAt(0);
    if (BLOCKED_BYTES.includes(code)) {
      return true;
    }
  }
  return false;
};

/**
 * Checks if a command contains blocked patterns.
 */
const hasBlockedPattern = (normalizedCmd: string): string | null => {
  for (const pattern of BLOCKED_PATTERNS) {
    if (normalizedCmd.includes(pattern)) {
      return pattern;
    }
  }
  return null;
};

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
 * Creates a filtered environment for command execution.
 */
const createSafeEnv = (): Record<string, string> => {
  const env: Record<string, string> = {};

  for (const [key, value] of Object.entries(process.env)) {
    if (value === undefined) {
      continue;
    }

    const upperKey = key.toUpperCase();
    const isBlocked = BLOCKED_ENV_SUFFIXES.some((suffix) =>
      upperKey.endsWith(suffix)
    );

    if (isBlocked) {
      continue;
    }

    if (ALLOWED_ENV_VARS.has(key)) {
      env[key] = value;
    }
  }

  return env;
};

/**
 * Executes a command and returns the result.
 */
const executeCommand = (
  ctx: ToolContext,
  fullCmd: string,
  parts: string[]
): Promise<ToolResult> => {
  const startTime = Date.now();
  let stdout = "";
  let stderr = "";
  let timedOut = false;
  let exitCode = 0;

  const [command, ...args] = parts;

  if (command === undefined) {
    return Promise.resolve(errorResult("empty command"));
  }

  return new Promise((resolve) => {
    const child = spawn(command, args, {
      cwd: ctx.worktreePath,
      env: createSafeEnv(),
      timeout: COMMAND_TIMEOUT,
    });

    const timeoutId = setTimeout(() => {
      timedOut = true;
      child.kill("SIGKILL");
    }, COMMAND_TIMEOUT);

    child.stdout.on("data", (data: Buffer) => {
      stdout += data.toString();
      if (stdout.length > MAX_OUTPUT) {
        stdout = `${stdout.slice(0, MAX_OUTPUT)}\n... (truncated)`;
      }
    });

    child.stderr.on("data", (data: Buffer) => {
      stderr += data.toString();
      if (stderr.length > MAX_OUTPUT) {
        stderr = `${stderr.slice(0, MAX_OUTPUT)}\n... (truncated)`;
      }
    });

    child.on("close", (code: number | null) => {
      clearTimeout(timeoutId);
      exitCode = code ?? 0;
      const duration = Date.now() - startTime;

      let result = `$ ${fullCmd}\n(completed in ${duration}ms)\n\n`;
      const output = stdout + stderr;

      if (timedOut) {
        result += "TIMEOUT: exceeded 5 minutes\n";
        resolve({
          content: result,
          isError: true,
          metadata: { exitCode: -1, timedOut: true } as RunCommandMetadata,
        });
        return;
      }

      if (exitCode !== 0) {
        result += `Exit code: ${exitCode}\n\n`;
        result += output;
        resolve({
          content: result,
          isError: true,
          metadata: { exitCode, timedOut: false } as RunCommandMetadata,
        });
        return;
      }

      result += output;
      resolve({
        content: result,
        isError: false,
        metadata: { exitCode: 0, timedOut: false } as RunCommandMetadata,
      });
    });

    child.on("error", (err: Error) => {
      clearTimeout(timeoutId);
      const duration = Date.now() - startTime;
      const result = `$ ${fullCmd}\n(completed in ${duration}ms)\n\nError: ${err.message}\n`;
      resolve({
        content: result,
        isError: true,
        metadata: { exitCode: -1, timedOut: false } as RunCommandMetadata,
      });
    });
  });
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

    return executeCommand(ctx, normalizedCmd, parts);
  },
};
