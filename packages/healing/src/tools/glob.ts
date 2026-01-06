import { stat } from "node:fs/promises";
import { join } from "node:path";
import fg from "fast-glob";
import { type ToolContext, validatePath } from "./context.js";
import {
  errorResult,
  SchemaBuilder,
  successResult,
  type Tool,
  type ToolResult,
} from "./types.js";

const MAX_GLOB_RESULTS = 200;

interface GlobInput {
  pattern: string;
  path?: string;
}

interface FileWithTime {
  path: string;
  modTime: number;
}

const isValidInput = (input: unknown): input is GlobInput =>
  typeof input === "object" &&
  input !== null &&
  typeof (input as GlobInput).pattern === "string";

/**
 * Validates the search directory and returns the search path.
 */
const resolveSearchPath = (
  ctx: ToolContext,
  searchDir: string | undefined
): { searchPath: string; displayPath: string; error?: ToolResult } => {
  if (!searchDir) {
    return { searchPath: ctx.worktreePath, displayPath: "." };
  }

  const validation = validatePath(ctx, searchDir);
  if (!(validation.valid && validation.absPath)) {
    return {
      searchPath: "",
      displayPath: searchDir,
      error: validation.error ?? errorResult("invalid path"),
    };
  }

  return { searchPath: validation.absPath, displayPath: searchDir };
};

/**
 * Collects file modification times for matched files.
 */
const collectFilesWithTimes = async (
  matches: string[],
  searchPath: string
): Promise<FileWithTime[]> => {
  const filesWithTime: FileWithTime[] = [];

  for (const match of matches) {
    const fullPath = join(searchPath, match);
    try {
      const fileStat = await stat(fullPath);
      if (fileStat.isFile()) {
        filesWithTime.push({
          path: match,
          modTime: fileStat.mtimeMs,
        });
      }
    } catch {
      // Skip files we can't stat
    }
  }

  return filesWithTime;
};

/**
 * Glob tool - finds files matching glob patterns.
 */
export const globTool: Tool = {
  name: "glob",
  description:
    "Find files matching a glob pattern. Supports ** for recursive matching. Returns file paths sorted by modification time (newest first).",
  inputSchema: new SchemaBuilder()
    .addString(
      "pattern",
      "Glob pattern to match (e.g., '**/*.go', 'src/**/*.ts')"
    )
    .addOptionalString(
      "path",
      "Directory to search in (relative to repo root, default: root)"
    )
    .build(),

  execute: async (ctx: ToolContext, input: unknown): Promise<ToolResult> => {
    if (!isValidInput(input)) {
      return errorResult("invalid input: pattern is required");
    }

    const { pattern, path: searchDir } = input;

    if (pattern === "") {
      return errorResult("pattern is required");
    }

    const { searchPath, displayPath, error } = resolveSearchPath(
      ctx,
      searchDir
    );
    if (error) {
      return error;
    }

    try {
      const dirStat = await stat(searchPath);
      if (!dirStat.isDirectory()) {
        return errorResult(`path is not a directory: ${displayPath}`);
      }
    } catch {
      return errorResult(`path not found: ${displayPath}`);
    }

    let matches: string[];
    try {
      matches = await fg(pattern, {
        cwd: searchPath,
        dot: false,
        onlyFiles: true,
        suppressErrors: true,
        followSymbolicLinks: false,
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      return errorResult(`invalid glob pattern: ${message}`);
    }

    if (matches.length === 0) {
      return successResult(`no files match pattern: ${pattern}`);
    }

    const filesWithTime = await collectFilesWithTimes(matches, searchPath);

    if (filesWithTime.length === 0) {
      return successResult(
        `no files match pattern: ${pattern} (only directories matched)`
      );
    }

    filesWithTime.sort((a, b) => b.modTime - a.modTime);

    const truncated = filesWithTime.length > MAX_GLOB_RESULTS;
    const results = truncated
      ? filesWithTime.slice(0, MAX_GLOB_RESULTS)
      : filesWithTime;

    let output = results.map((f) => f.path).join("\n");

    if (truncated) {
      output +=
        "\n\n... (showing first 200 results, refine your pattern for more specific matches)";
    }

    return successResult(output);
  },
};
