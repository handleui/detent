import { spawn } from "node:child_process";
import { errorResult, type ToolResult } from "./types.js";

/**
 * Command execution timeout in milliseconds (5 minutes).
 */
export const COMMAND_TIMEOUT = 5 * 60 * 1000;

/**
 * Maximum output size in bytes (50KB).
 */
export const MAX_OUTPUT = 50 * 1024;

/**
 * Blocked bytes - null, newline, carriage return.
 * Prevents command injection via control characters.
 */
export const BLOCKED_BYTES = [0x00, 0x0a, 0x0d];

/**
 * Blocked patterns - always rejected regardless of source.
 * These patterns are checked after normalizing whitespace.
 */
export const BLOCKED_PATTERNS = [
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
 * Blocked base commands - never allowed.
 */
export const BLOCKED_COMMANDS = new Set([
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
 * Environment variables that are allowed to pass through.
 */
export const ALLOWED_ENV_VARS = new Set([
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
export const BLOCKED_ENV_SUFFIXES = [
  "_KEY",
  "_TOKEN",
  "_SECRET",
  "_PASSWORD",
  "_CREDS",
  "_AUTH",
];

/**
 * Result metadata for command execution.
 */
export interface ExecuteMetadata extends Record<string, unknown> {
  exitCode: number;
  timedOut: boolean;
}

const WHITESPACE_REGEX = /\s+/;

/**
 * Normalizes whitespace in a command string.
 */
export const normalizeCommand = (cmd: string): string =>
  cmd.split(WHITESPACE_REGEX).join(" ");

/**
 * Extracts the base command name from a path.
 * e.g., "/usr/bin/rm" -> "rm", "rm" -> "rm"
 */
export const extractBaseCommand = (cmd: string): string => {
  const lastSlash = cmd.lastIndexOf("/");
  return lastSlash >= 0 ? cmd.slice(lastSlash + 1) : cmd;
};

/**
 * Checks if a command contains blocked bytes.
 */
export const hasBlockedBytes = (cmd: string): boolean => {
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
 * Returns the matched pattern if blocked, null otherwise.
 */
export const hasBlockedPattern = (normalizedCmd: string): string | null => {
  for (const pattern of BLOCKED_PATTERNS) {
    if (normalizedCmd.includes(pattern)) {
      return pattern;
    }
  }
  return null;
};

/**
 * Creates a filtered environment for command execution.
 * Only allows known-safe environment variables and blocks
 * any variables with secret-indicating suffixes.
 */
export const createSafeEnv = (): Record<string, string> => {
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
 * Validates a command for security issues.
 * Returns an error message if blocked, null if safe.
 */
export const validateCommand = (command: string): string | null => {
  if (hasBlockedBytes(command)) {
    return "command contains invalid characters";
  }

  const normalizedCmd = normalizeCommand(command);
  const blockedPattern = hasBlockedPattern(normalizedCmd);
  if (blockedPattern) {
    return `blocked pattern: "${blockedPattern}"`;
  }

  const parts = normalizedCmd.split(" ").filter(Boolean);
  if (parts.length === 0 || parts[0] === undefined) {
    return "empty command";
  }

  const baseCmd = extractBaseCommand(parts[0]);
  if (BLOCKED_COMMANDS.has(baseCmd)) {
    return `blocked command: "${baseCmd}"`;
  }

  return null;
};

/**
 * Parses a command string into normalized form and parts.
 */
export const parseCommand = (
  command: string
): { normalized: string; parts: string[] } => {
  const normalized = normalizeCommand(command);
  const parts = normalized.split(" ").filter(Boolean);
  return { normalized, parts };
};

/**
 * Executes a command and returns the result.
 */
export const executeCommand = (
  cwd: string,
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
      cwd,
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
          metadata: { exitCode: -1, timedOut: true } as ExecuteMetadata,
        });
        return;
      }

      if (exitCode !== 0) {
        result += `Exit code: ${exitCode}\n\n`;
        result += output;
        resolve({
          content: result,
          isError: true,
          metadata: { exitCode, timedOut: false } as ExecuteMetadata,
        });
        return;
      }

      result += output;
      resolve({
        content: result,
        isError: false,
        metadata: { exitCode: 0, timedOut: false } as ExecuteMetadata,
      });
    });

    child.on("error", (err: Error) => {
      clearTimeout(timeoutId);
      const duration = Date.now() - startTime;
      const result = `$ ${fullCmd}\n(completed in ${duration}ms)\n\nError: ${err.message}\n`;
      resolve({
        content: result,
        isError: true,
        metadata: { exitCode: -1, timedOut: false } as ExecuteMetadata,
      });
    });
  });
};
