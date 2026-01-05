// biome-ignore-all lint/performance/noBarrelFile: This is the tools submodule's public API

export type {
  CommandApprovalDecision,
  PathValidationResult,
  ToolContext,
} from "./context.js";
export {
  approveCommand,
  createToolContext,
  denyCommand,
  isCommandApproved,
  isCommandDenied,
  validatePath,
} from "./context.js";
export { editFileTool } from "./edit-file.js";
export { globTool } from "./glob.js";
export { grepTool } from "./grep.js";
export { readFileTool } from "./read-file.js";
export { createToolRegistry, ToolRegistry } from "./registry.js";
export { runCommandTool } from "./run-command.js";
export type { Tool, ToolResult } from "./types.js";
export { errorResult, SchemaBuilder, successResult } from "./types.js";

import { editFileTool } from "./edit-file.js";
import { globTool } from "./glob.js";
import { grepTool } from "./grep.js";
import { readFileTool } from "./read-file.js";
import { runCommandTool } from "./run-command.js";
import type { Tool } from "./types.js";

/**
 * Returns all built-in tools.
 */
export const getAllTools = (): Tool[] => [
  readFileTool,
  editFileTool,
  globTool,
  grepTool,
  runCommandTool,
];
