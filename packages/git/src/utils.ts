import {
  ErrGitTimeout,
  ErrInvalidInput,
  type GitExecOptions,
  type GitExecResult,
  type RunID,
} from "./types.js";

const DEFAULT_TIMEOUT = 30_000;
const DEFAULT_MAX_BUFFER = 50 * 1024 * 1024;
const MAX_RUNID_LENGTH = 64;
const MAX_ARGS_LENGTH = 1000;

const ESSENTIAL_ENV_VARS = [
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
] as const;

const GIT_SECURITY_ENV = {
  GIT_CONFIG_NOSYSTEM: "1",
  GIT_CONFIG_NOGLOBAL: "1",
  GIT_TERMINAL_PROMPT: "0",
  GIT_SSH_COMMAND: "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new",
  GIT_ASKPASS: "/bin/true",
  GIT_EDITOR: "/bin/true",
  GIT_PAGER: "cat",
  GIT_ATTR_NOSYSTEM: "1",
} as const;

let cachedGitEnv: Readonly<Record<string, string>> | null = null;

export const safeGitEnv = (): Readonly<Record<string, string>> => {
  if (cachedGitEnv) {
    return cachedGitEnv;
  }

  const env: Record<string, string> = {};

  for (const key of ESSENTIAL_ENV_VARS) {
    const value = process.env[key];
    if (value !== undefined) {
      env[key] = value;
    }
  }

  cachedGitEnv = Object.freeze({ ...env, ...GIT_SECURITY_ENV });
  return cachedGitEnv;
};

export const execGit = async (
  args: string[],
  options: GitExecOptions = {}
  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: Input validation requires thorough checks
): Promise<GitExecResult> => {
  const { execFile } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execFileAsync = promisify(execFile);

  if (args.length > MAX_ARGS_LENGTH) {
    throw new ErrInvalidInput(
      `too many arguments: ${args.length} exceeds maximum of ${MAX_ARGS_LENGTH}`
    );
  }

  for (const arg of args) {
    if (typeof arg !== "string") {
      throw new ErrInvalidInput("all git arguments must be strings");
    }
    if (arg.includes("\0")) {
      throw new ErrInvalidInput("git arguments must not contain null bytes");
    }
    if (arg.length > 32_768) {
      throw new ErrInvalidInput(
        "argument exceeds maximum length of 32768 bytes"
      );
    }
  }

  if (options.cwd) {
    if (typeof options.cwd !== "string") {
      throw new ErrInvalidInput("cwd must be a string");
    }
    if (options.cwd.includes("\0")) {
      throw new ErrInvalidInput("cwd must not contain null bytes");
    }
    if (options.cwd.length > 4096) {
      throw new ErrInvalidInput("cwd exceeds maximum length of 4096 bytes");
    }
  }

  const fullArgs = ["-c", "core.hooksPath=/dev/null", ...args];

  const execOptions = {
    cwd: options.cwd,
    timeout: options.timeout ?? DEFAULT_TIMEOUT,
    env: safeGitEnv(),
    maxBuffer: options.maxBuffer ?? DEFAULT_MAX_BUFFER,
    windowsHide: true,
    shell: false,
  };

  try {
    const { stdout, stderr } = await execFileAsync(
      "git",
      fullArgs,
      execOptions
    );
    return {
      stdout: stdout.trim(),
      stderr: stderr.trim(),
    };
  } catch (error) {
    if (error instanceof Error && "stdout" in error && "stderr" in error) {
      const execError = error as {
        stdout: string;
        stderr: string;
        message: string;
        code?: string | number;
        killed?: boolean;
        signal?: string;
      };

      if (execError.killed || execError.signal) {
        throw new ErrGitTimeout(
          `git command timeout or killed (signal: ${execError.signal || "SIGTERM"})`
        );
      }

      const _stdout =
        typeof execError.stdout === "string" ? execError.stdout.trim() : "";
      const stderr =
        typeof execError.stderr === "string" ? execError.stderr.trim() : "";

      throw new Error(
        `git command failed: ${stderr || execError.message} (exit code: ${execError.code ?? "unknown"})`
      );
    }
    throw error;
  }
};

export const isValidRunID = (runID: string): runID is RunID => {
  if (runID === "" || runID.length > MAX_RUNID_LENGTH) {
    return false;
  }

  for (let i = 0; i < runID.length; i++) {
    const c = runID.charCodeAt(i);
    const isDigit = c >= 48 && c <= 57;
    const isLowerHex = c >= 97 && c <= 102;
    const isUpperHex = c >= 65 && c <= 70;
    if (!(isDigit || isLowerHex || isUpperHex)) {
      return false;
    }
  }

  return true;
};
