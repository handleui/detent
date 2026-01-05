import { constants, createReadStream } from "node:fs";
import { access, stat } from "node:fs/promises";
import { createInterface } from "node:readline";
import { type ToolContext, validatePath } from "./context.js";
import {
  errorResult,
  SchemaBuilder,
  successResult,
  type Tool,
  type ToolResult,
} from "./types.js";

const DEFAULT_READ_LIMIT = 2000;
const MAX_LINE_LENGTH = 2000;

interface ReadFileInput {
  path: string;
  offset?: number;
  limit?: number;
}

const isValidInput = (input: unknown): input is ReadFileInput =>
  typeof input === "object" &&
  input !== null &&
  typeof (input as ReadFileInput).path === "string";

/**
 * Validates input parameters for reading a file.
 */
const validateReadParams = (
  offset: number,
  limit: number
): ToolResult | null => {
  if (offset < 1) {
    return errorResult("offset must be at least 1");
  }
  if (limit < 1) {
    return errorResult("limit must be at least 1");
  }
  return null;
};

/**
 * Validates that the file exists and is readable.
 */
const validateFileAccess = async (
  absPath: string,
  filePath: string
): Promise<ToolResult | null> => {
  try {
    await access(absPath, constants.R_OK);
  } catch {
    return errorResult(`file not found: ${filePath}`);
  }

  const fileStat = await stat(absPath);
  if (fileStat.isDirectory()) {
    return errorResult(`path is a directory: ${filePath}`);
  }

  return null;
};

/**
 * Reads lines from a file with offset and limit.
 */
const readLinesFromFile = async (
  absPath: string,
  offset: number,
  limit: number
): Promise<{ lines: string[]; lineNum: number; truncated: boolean }> => {
  const fileStream = createReadStream(absPath, { encoding: "utf8" });
  const rl = createInterface({
    input: fileStream,
    crlfDelay: Number.POSITIVE_INFINITY,
  });

  const lines: string[] = [];
  let lineNum = 0;
  let linesRead = 0;
  let truncated = false;

  for await (const line of rl) {
    lineNum++;

    if (lineNum < offset) {
      continue;
    }

    if (linesRead >= limit) {
      truncated = true;
      break;
    }

    let processedLine = line;
    if (processedLine.length > MAX_LINE_LENGTH) {
      processedLine = `${processedLine.slice(0, MAX_LINE_LENGTH)}...`;
    }

    lines.push(`${String(lineNum).padStart(6, " ")}\t${processedLine}`);
    linesRead++;
  }

  rl.close();
  fileStream.close();

  return { lines, lineNum, truncated };
};

/**
 * Read file tool - reads source code from files in the worktree.
 */
export const readFileTool: Tool = {
  name: "read_file",
  description:
    "Read a file from the codebase. Returns file contents with line numbers. Use offset and limit for large files.",
  inputSchema: new SchemaBuilder()
    .addString("path", "File path relative to repository root")
    .addOptionalInteger(
      "offset",
      "Line number to start reading from (1-indexed, default: 1)",
      1
    )
    .addOptionalInteger(
      "limit",
      "Maximum number of lines to read (default: 2000)",
      DEFAULT_READ_LIMIT
    )
    .build(),

  execute: async (ctx: ToolContext, input: unknown): Promise<ToolResult> => {
    if (!isValidInput(input)) {
      return errorResult("invalid input: path is required");
    }

    const { path: filePath } = input;
    const offset = input.offset ?? 1;
    const limit = input.limit ?? DEFAULT_READ_LIMIT;

    const paramError = validateReadParams(offset, limit);
    if (paramError) {
      return paramError;
    }

    const validation = validatePath(ctx, filePath);
    if (!(validation.valid && validation.absPath)) {
      return validation.error ?? errorResult("invalid path");
    }

    const absPath = validation.absPath;

    const accessError = await validateFileAccess(absPath, filePath);
    if (accessError) {
      return accessError;
    }

    const { lines, lineNum, truncated } = await readLinesFromFile(
      absPath,
      offset,
      limit
    );

    if (lines.length === 0) {
      if (offset > 1) {
        return errorResult(
          `offset ${offset} exceeds file length (${lineNum} lines)`
        );
      }
      return successResult("(empty file)");
    }

    let result = lines.join("\n");
    if (truncated) {
      result += `\n\n... (truncated at ${limit} lines, use offset to read more)`;
    }

    return successResult(result);
  },
};
