import type { Stats } from "node:fs";
import { readFile, stat, writeFile } from "node:fs/promises";
import { type ToolContext, validatePath } from "./context.js";
import {
  errorResult,
  SchemaBuilder,
  successResult,
  type Tool,
  type ToolResult,
} from "./types.js";

interface EditFileInput {
  path: string;
  old_string: string;
  new_string: string;
}

const isValidInput = (input: unknown): input is EditFileInput =>
  typeof input === "object" &&
  input !== null &&
  typeof (input as EditFileInput).path === "string" &&
  typeof (input as EditFileInput).old_string === "string" &&
  typeof (input as EditFileInput).new_string === "string";

/**
 * Counts occurrences of a substring in a string.
 */
const countOccurrences = (text: string, search: string): number => {
  let count = 0;
  let pos = 0;
  let foundPos = text.indexOf(search, pos);
  while (foundPos !== -1) {
    count++;
    pos = foundPos + search.length;
    foundPos = text.indexOf(search, pos);
  }
  return count;
};

/**
 * Counts lines in a string.
 */
const countLines = (text: string): number => {
  if (text === "") {
    return 0;
  }
  return text.split("\n").length;
};

/**
 * Edit file tool - applies targeted edits to files.
 */
export const editFileTool: Tool = {
  name: "edit_file",
  description:
    "Replace a string in a file. The old_string must match exactly once in the file (for safety). Use read_file first to see the exact content.",
  inputSchema: new SchemaBuilder()
    .addString("path", "File path relative to repository root")
    .addString(
      "old_string",
      "Exact string to find and replace (must be unique in file)"
    )
    .addString("new_string", "String to replace it with")
    .build(),

  execute: async (ctx: ToolContext, input: unknown): Promise<ToolResult> => {
    if (!isValidInput(input)) {
      return errorResult(
        "invalid input: path, old_string, and new_string are required"
      );
    }

    const {
      path: filePath,
      old_string: oldString,
      new_string: newString,
    } = input;

    if (filePath === "") {
      return errorResult("path is required");
    }

    if (oldString === "") {
      return errorResult("old_string is required");
    }

    if (oldString === newString) {
      return errorResult("old_string and new_string are identical");
    }

    const validation = validatePath(ctx, filePath);
    if (!(validation.valid && validation.absPath)) {
      return validation.error ?? errorResult("invalid path");
    }

    const absPath = validation.absPath;

    let fileStat: Stats;
    try {
      fileStat = await stat(absPath);
    } catch {
      return errorResult(`file not found: ${filePath}`);
    }

    if (fileStat.isDirectory()) {
      return errorResult(`path is a directory: ${filePath}`);
    }

    let content: string;
    try {
      content = await readFile(absPath, "utf8");
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      return errorResult(`failed to read file: ${message}`);
    }

    const occurrences = countOccurrences(content, oldString);

    if (occurrences === 0) {
      return errorResult(
        "old_string not found in file. Use read_file to see exact content."
      );
    }

    if (occurrences > 1) {
      return errorResult(
        `old_string found ${occurrences} times in file (must be unique). Include more context to make it unique.`
      );
    }

    const newContent = content.replace(oldString, newString);

    try {
      await writeFile(absPath, newContent, {
        encoding: "utf8",
        mode: fileStat.mode,
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      return errorResult(`failed to write file: ${message}`);
    }

    const oldLines = countLines(oldString);
    const newLines = countLines(newString);

    let summary: string;
    if (oldLines === newLines) {
      summary = `replaced ${oldLines} line(s)`;
    } else if (newLines > oldLines) {
      summary = `replaced ${oldLines} line(s) with ${newLines} line(s) (+${newLines - oldLines})`;
    } else {
      summary = `replaced ${oldLines} line(s) with ${newLines} line(s) (-${oldLines - newLines})`;
    }

    return successResult(`file updated: ${filePath} (${summary})`);
  },
};
