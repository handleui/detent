import { spawn } from "node:child_process";
import { type ToolContext, validatePath } from "./context.js";
import {
  errorResult,
  SchemaBuilder,
  successResult,
  type Tool,
  type ToolResult,
} from "./types.js";

const GREP_TIMEOUT_MS = 30_000;
const MAX_GREP_OUTPUT = 50 * 1024; // 50KB
const MAX_GREP_MATCHES = 100;

/**
 * File type mappings for ripgrep --type flag.
 */
const FILE_TYPE_MAP: Record<string, string> = {
  go: "go",
  ts: "ts",
  typescript: "ts",
  js: "js",
  javascript: "js",
  py: "py",
  python: "py",
  rust: "rust",
  rs: "rust",
  java: "java",
  c: "c",
  cpp: "cpp",
  css: "css",
  html: "html",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  md: "md",
  markdown: "md",
};

interface GrepInput {
  pattern: string;
  path?: string;
  type?: string;
}

const isValidInput = (input: unknown): input is GrepInput =>
  typeof input === "object" &&
  input !== null &&
  typeof (input as GrepInput).pattern === "string";

/**
 * Executes ripgrep as a child process with timeout.
 */
const runRipgrep = (
  args: string[],
  cwd: string,
  timeoutMs: number
): Promise<{ stdout: string; stderr: string; exitCode: number | null }> =>
  new Promise((resolve) => {
    const proc = spawn("rg", args, { cwd });

    let stdout = "";
    let stderr = "";
    let killed = false;

    const timeout = setTimeout(() => {
      killed = true;
      proc.kill("SIGTERM");
    }, timeoutMs);

    proc.stdout.on("data", (data: Buffer) => {
      stdout += data.toString();
      if (stdout.length > MAX_GREP_OUTPUT) {
        killed = true;
        proc.kill("SIGTERM");
      }
    });

    proc.stderr.on("data", (data: Buffer) => {
      stderr += data.toString();
    });

    proc.on("error", (err) => {
      clearTimeout(timeout);
      resolve({ stdout: "", stderr: err.message, exitCode: null });
    });

    proc.on("close", (code) => {
      clearTimeout(timeout);
      if (killed && stdout.length > MAX_GREP_OUTPUT) {
        resolve({ stdout, stderr, exitCode: code });
      } else if (killed) {
        resolve({ stdout: "", stderr: "timeout", exitCode: null });
      } else {
        resolve({ stdout, stderr, exitCode: code });
      }
    });
  });

/**
 * Grep tool - searches for patterns in code using ripgrep.
 */
export const grepTool: Tool = {
  name: "grep",
  description:
    "Search for a pattern in code using ripgrep. Returns matching lines with file paths and line numbers.",
  inputSchema: new SchemaBuilder()
    .addString("pattern", "Regular expression pattern to search for")
    .addOptionalString(
      "path",
      "Directory or file to search in (relative to repo root, default: entire repo)"
    )
    .addOptionalString(
      "type",
      "File type filter (e.g., 'go', 'ts', 'py', 'rust', 'js')"
    )
    .build(),

  execute: async (ctx: ToolContext, input: unknown): Promise<ToolResult> => {
    if (!isValidInput(input)) {
      return errorResult("invalid input: pattern is required");
    }

    const { pattern, path: searchDir, type: fileType } = input;

    if (pattern === "") {
      return errorResult("pattern is required");
    }

    let searchPath = ctx.worktreePath;

    if (searchDir) {
      const validation = validatePath(ctx, searchDir);
      if (!(validation.valid && validation.absPath)) {
        return validation.error ?? errorResult("invalid path");
      }
      searchPath = validation.absPath;
    }

    const args: string[] = [
      "--line-number",
      "--no-heading",
      "--color=never",
      `--max-count=${MAX_GREP_MATCHES}`,
    ];

    if (fileType) {
      const rgType = FILE_TYPE_MAP[fileType.toLowerCase()];
      if (!rgType) {
        const supported = Object.keys(FILE_TYPE_MAP)
          .filter((k, i, arr) => arr.indexOf(k) === i)
          .slice(0, 10)
          .join(", ");
        return errorResult(
          `unknown file type: ${fileType} (supported: ${supported}, ...)`
        );
      }
      args.push("--type", rgType);
    }

    args.push("--", pattern, searchPath);

    const result = await runRipgrep(args, ctx.worktreePath, GREP_TIMEOUT_MS);

    if (result.exitCode === null) {
      if (result.stderr === "timeout") {
        return errorResult("search timed out after 30 seconds");
      }
      if (result.stderr.includes("No such file or directory")) {
        return errorResult(
          "ripgrep (rg) not found - please install it for code search"
        );
      }
      return errorResult(`grep failed: ${result.stderr}`);
    }

    if (result.exitCode === 1) {
      return successResult(`no matches found for pattern: ${pattern}`);
    }

    if (result.exitCode === 2) {
      return errorResult(`grep error: ${result.stderr}`);
    }

    let output = result.stdout;

    if (output.length > MAX_GREP_OUTPUT) {
      output = `${output.slice(0, MAX_GREP_OUTPUT)}\n... (truncated, refine your pattern for more specific matches)`;
    }

    output = output.replaceAll(`${ctx.worktreePath}/`, "");

    return successResult(output);
  },
};
