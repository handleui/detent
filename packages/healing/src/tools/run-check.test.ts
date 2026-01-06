import { tmpdir } from "node:os";
import { describe, expect, test } from "vitest";
import type { ToolContext } from "./context.js";
import { runCheckTool } from "./run-check.js";

const BLOCKED_COMMAND_OR_PATTERN_REGEX = /blocked (command|pattern)/;
const BLOCKED_PATTERN_REGEX = /blocked pattern/;

const createContext = (overrides: Partial<ToolContext> = {}): ToolContext => ({
  worktreePath: tmpdir(),
  repoRoot: tmpdir(),
  runId: "test-run-123",
  approvedCommands: new Set(),
  deniedCommands: new Set(),
  ...overrides,
});

describe("run_check", () => {
  test("returns error when no failingStep in context", async () => {
    const ctx = createContext({ stepCommands: new Map([["build", ["echo"]]]) });
    const result = await runCheckTool.execute(ctx, {});
    expect(result.isError).toBe(true);
    expect(result.content).toBe("no failing step context available");
  });

  test("returns error when no stepCommands in context", async () => {
    const ctx = createContext({
      failingStep: { jobId: "build", stepIndex: 0 },
    });
    const result = await runCheckTool.execute(ctx, {});
    expect(result.isError).toBe(true);
    expect(result.content).toBe("no step commands available");
  });

  test("returns error when step has no run command (uses action)", async () => {
    const ctx = createContext({
      failingStep: { jobId: "build", stepIndex: 0 },
      stepCommands: new Map([["build", [null]]]),
    });
    const result = await runCheckTool.execute(ctx, {});
    expect(result.isError).toBe(true);
    expect(result.content).toBe(
      "step does not have a run command (uses action instead)"
    );
  });

  test("blocks dangerous commands (rm, bash, etc.)", async () => {
    const dangerousCommands = ["rm file.txt", "bash script.sh", "sudo ls"];
    for (const cmd of dangerousCommands) {
      const ctx = createContext({
        failingStep: { jobId: "build", stepIndex: 0 },
        stepCommands: new Map([["build", [cmd]]]),
      });
      const result = await runCheckTool.execute(ctx, {});
      expect(result.isError).toBe(true);
      expect(result.content).toMatch(BLOCKED_COMMAND_OR_PATTERN_REGEX);
    }
  });

  test("blocks dangerous patterns (|, &&, etc.)", async () => {
    const dangerousPatterns = [
      "echo foo | cat",
      "ls && rm -rf /",
      "echo $(whoami)",
      "echo `id`",
    ];
    for (const cmd of dangerousPatterns) {
      const ctx = createContext({
        failingStep: { jobId: "build", stepIndex: 0 },
        stepCommands: new Map([["build", [cmd]]]),
      });
      const result = await runCheckTool.execute(ctx, {});
      expect(result.isError).toBe(true);
      expect(result.content).toMatch(BLOCKED_PATTERN_REGEX);
    }
  });

  test("successfully executes a safe command", async () => {
    const ctx = createContext({
      failingStep: { jobId: "build", stepIndex: 0 },
      stepCommands: new Map([["build", ["echo hello"]]]),
    });
    const result = await runCheckTool.execute(ctx, {});
    expect(result.isError).toBe(false);
    expect(result.content).toContain("$ echo hello");
    expect(result.content).toContain("hello");
  });
});
