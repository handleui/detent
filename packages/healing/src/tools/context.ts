import { isAbsolute, join, normalize, relative } from "node:path";
import { errorResult, type ToolResult } from "./types.js";

/**
 * Command approval decision from user.
 */
export type CommandApprovalDecision = "allow" | "deny" | "always" | "never";

/**
 * Identifies a failing workflow step.
 */
export interface FailingStep {
  jobId: string;
  stepIndex: number;
}

/**
 * Execution context for tools.
 */
export interface ToolContext {
  worktreePath: string;
  repoRoot: string;
  runId: string;
  firstCommitSha?: string;

  /** Commands approved for this session */
  approvedCommands: Set<string>;

  /** Commands denied for this session */
  deniedCommands: Set<string>;

  /** Checks if command is in local config */
  commandChecker?: (cmd: string) => boolean;

  /** Prompts user for unknown commands */
  commandApprover?: (cmd: string) => Promise<CommandApprovalDecision>;

  /** Saves approved command to config */
  commandPersister?: (cmd: string) => Promise<void>;

  /** Step commands by job ID (for run_check verification) */
  stepCommands?: Map<string, (string | null)[]>;

  /** Which step failed (for run_check verification) */
  failingStep?: FailingStep;
}

/**
 * Creates a new tool context.
 */
export const createToolContext = (
  worktreePath: string,
  repoRoot: string,
  runId: string
): ToolContext => ({
  worktreePath,
  repoRoot,
  runId,
  approvedCommands: new Set(),
  deniedCommands: new Set(),
});

/**
 * Checks if a command was approved this session.
 */
export const isCommandApproved = (ctx: ToolContext, cmd: string): boolean =>
  ctx.approvedCommands.has(cmd);

/**
 * Marks a command as approved for this session.
 */
export const approveCommand = (ctx: ToolContext, cmd: string): void => {
  ctx.approvedCommands.add(cmd);
};

/**
 * Checks if a command was denied this session.
 */
export const isCommandDenied = (ctx: ToolContext, cmd: string): boolean =>
  ctx.deniedCommands.has(cmd);

/**
 * Marks a command as denied for this session.
 */
export const denyCommand = (ctx: ToolContext, cmd: string): void => {
  ctx.deniedCommands.add(cmd);
};

/**
 * Result of path validation.
 */
export interface PathValidationResult {
  valid: boolean;
  absPath?: string;
  error?: ToolResult;
}

/**
 * Validates that a path is within the worktree and doesn't escape.
 * Prevents directory traversal attacks via ../ sequences.
 */
export const validatePath = (
  ctx: ToolContext,
  relPath: string
): PathValidationResult => {
  const cleanPath = normalize(relPath);

  if (isAbsolute(cleanPath)) {
    return {
      valid: false,
      error: errorResult(`absolute paths not allowed: ${relPath}`),
    };
  }

  const absPath = join(ctx.worktreePath, cleanPath);

  const rel = relative(ctx.worktreePath, absPath);
  if (rel.startsWith("..")) {
    return {
      valid: false,
      error: errorResult(`path escapes worktree: ${relPath}`),
    };
  }

  return { valid: true, absPath };
};
