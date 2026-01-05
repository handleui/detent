import { execSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";

const HEX_STRING_REGEX = /^[0-9a-f]+$/;

import { CheckRunner } from "../runner/index.js";
import type { RunConfig } from "../runner/types.js";

// Mock the parser service to avoid requiring it to be running
vi.mock("../services/parser-client.ts", () => ({
  ParserClient: class MockParserClient {
    parse(_logs: string) {
      return { errors: [] };
    }
  },
}));

// Mock act executor to avoid requiring Docker/act for basic tests
vi.mock("../runner/executor.ts", () => ({
  ActExecutor: class MockActExecutor {
    execute() {
      return {
        exitCode: 0,
        stdout: "Mock act execution output",
        stderr: "",
        duration: 1000,
      };
    }
  },
}));

describe("check command integration", () => {
  let testRepoPath: string;
  let commitCounter = 0;

  const createGitRepo = async (path: string): Promise<void> => {
    execSync("git init", { cwd: path, stdio: "ignore" });
    execSync('git config user.email "test@example.com"', {
      cwd: path,
      stdio: "ignore",
    });
    execSync('git config user.name "Test User"', {
      cwd: path,
      stdio: "ignore",
    });

    // Create initial commit with unique content to ensure unique commit hash
    await writeFile(
      join(path, "README.md"),
      `# Test Repo ${Date.now()}-${commitCounter++}`,
      "utf-8"
    );
    execSync("git add .", { cwd: path, stdio: "ignore" });
    execSync('git commit -m "Initial commit"', { cwd: path, stdio: "ignore" });
  };

  const createWorkflowFile = async (
    path: string,
    name: string,
    content: string
  ): Promise<void> => {
    const workflowsDir = join(path, ".github", "workflows");
    await mkdir(workflowsDir, { recursive: true });
    await writeFile(join(workflowsDir, name), content, "utf-8");
  };

  const simpleWorkflow = `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "hello"
`;

  const multiJobWorkflow = `name: CI
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - run: echo "linting"
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "testing"
`;

  beforeEach(async () => {
    testRepoPath = await mkdtemp(join(tmpdir(), "detent-test-"));
  });

  afterEach(async () => {
    try {
      // Clean up any lingering worktrees before removing the test repo
      try {
        execSync("git worktree prune", { cwd: testRepoPath, stdio: "ignore" });
      } catch {
        // Ignore errors if git repo doesn't exist
      }

      await rm(testRepoPath, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  test("runs full check lifecycle successfully with valid workflow", async () => {
    await createGitRepo(testRepoPath);
    await createWorkflowFile(testRepoPath, "test.yml", simpleWorkflow);

    const config: RunConfig = {
      repoRoot: testRepoPath,
      verbose: false,
    };

    const runner = new CheckRunner(config);
    const result = await runner.run();

    expect(result).toHaveProperty("runID");
    expect(result).toHaveProperty("success");
    expect(result).toHaveProperty("errors");
    expect(result).toHaveProperty("duration");
    expect(result.runID).toMatch(HEX_STRING_REGEX);
    expect(result.success).toBe(true);
    expect(result.errors).toEqual([]);
    expect(result.duration).toBeGreaterThan(0);
  });

  test("discovers and runs specific workflow when provided", async () => {
    await createGitRepo(testRepoPath);
    await createWorkflowFile(testRepoPath, "ci.yml", multiJobWorkflow);
    await createWorkflowFile(testRepoPath, "test.yml", simpleWorkflow);

    const config: RunConfig = {
      repoRoot: testRepoPath,
      workflow: "ci.yml",
      verbose: false,
    };

    const runner = new CheckRunner(config);
    const result = await runner.run();

    expect(result.success).toBe(true);
    expect(result.errors).toEqual([]);
  });

  test("fails gracefully when workflow file not found", async () => {
    await createGitRepo(testRepoPath);
    await createWorkflowFile(testRepoPath, "existing.yml", simpleWorkflow);

    const config: RunConfig = {
      repoRoot: testRepoPath,
      workflow: "nonexistent.yml",
      verbose: false,
    };

    const runner = new CheckRunner(config);

    await expect(runner.run()).rejects.toThrow(
      'Workflow "nonexistent.yml" not found in .github/workflows/'
    );
  });

  test("fails gracefully when .github/workflows directory missing", async () => {
    await createGitRepo(testRepoPath);
    // Don't create workflows directory

    const config: RunConfig = {
      repoRoot: testRepoPath,
      verbose: false,
    };

    const runner = new CheckRunner(config);

    await expect(runner.run()).rejects.toThrow(
      "Workflows directory not found (.github/workflows)"
    );
  });

  test("fails gracefully when not in git repository", async () => {
    // Don't initialize git repo

    const config: RunConfig = {
      repoRoot: testRepoPath,
      verbose: false,
    };

    const runner = new CheckRunner(config);

    await expect(runner.run()).rejects.toThrow();
  });

  test("fails gracefully with invalid YAML in workflow", async () => {
    await createGitRepo(testRepoPath);

    const invalidYaml = `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "hello"
      invalid yaml syntax here: [
`;

    await createWorkflowFile(testRepoPath, "invalid.yml", invalidYaml);

    const config: RunConfig = {
      repoRoot: testRepoPath,
      verbose: false,
    };

    const runner = new CheckRunner(config);

    // Should fail during workflow injection when trying to parse YAML
    await expect(runner.run()).rejects.toThrow();
  });

  test("fails gracefully when specified job not found in workflow", async () => {
    await createGitRepo(testRepoPath);
    await createWorkflowFile(testRepoPath, "ci.yml", multiJobWorkflow);

    const config: RunConfig = {
      repoRoot: testRepoPath,
      workflow: "ci.yml",
      job: "nonexistent-job",
      verbose: false,
    };

    const runner = new CheckRunner(config);

    await expect(runner.run()).rejects.toThrow(
      'Job "nonexistent-job" not found in workflow ci.yml'
    );
  });

  test("validates job exists when job parameter provided", async () => {
    await createGitRepo(testRepoPath);
    await createWorkflowFile(testRepoPath, "ci.yml", multiJobWorkflow);

    const config: RunConfig = {
      repoRoot: testRepoPath,
      workflow: "ci.yml",
      job: "lint",
      verbose: false,
    };

    const runner = new CheckRunner(config);
    const result = await runner.run();

    // Should succeed because 'lint' job exists
    expect(result.success).toBe(true);
  });

  test("handles empty workflow file gracefully", async () => {
    await createGitRepo(testRepoPath);
    await createWorkflowFile(testRepoPath, "empty.yml", "");

    const config: RunConfig = {
      repoRoot: testRepoPath,
      workflow: "empty.yml",
      verbose: false,
    };

    const runner = new CheckRunner(config);

    await expect(runner.run()).rejects.toThrow(
      "Workflow file empty.yml is empty"
    );
  });

  test("runs successfully with multiple workflows", async () => {
    await createGitRepo(testRepoPath);
    await createWorkflowFile(testRepoPath, "ci.yml", multiJobWorkflow);
    await createWorkflowFile(testRepoPath, "test.yml", simpleWorkflow);
    await createWorkflowFile(testRepoPath, "deploy.yml", simpleWorkflow);

    const config: RunConfig = {
      repoRoot: testRepoPath,
      verbose: false,
    };

    const runner = new CheckRunner(config);
    const result = await runner.run();

    expect(result.success).toBe(true);
    expect(result.errors).toEqual([]);
  });

  test("generates unique run IDs for consecutive runs", async () => {
    await createGitRepo(testRepoPath);
    await createWorkflowFile(testRepoPath, "test.yml", simpleWorkflow);

    const config: RunConfig = {
      repoRoot: testRepoPath,
      verbose: false,
    };

    const runner1 = new CheckRunner(config);
    const result1 = await runner1.run();

    // Make a commit to change the git state
    await writeFile(join(testRepoPath, "test.txt"), "test", "utf-8");
    execSync("git add .", { cwd: testRepoPath, stdio: "ignore" });
    execSync('git commit -m "test commit"', {
      cwd: testRepoPath,
      stdio: "ignore",
    });

    const runner2 = new CheckRunner(config);
    const result2 = await runner2.run();

    expect(result1.runID).toBeDefined();
    expect(result2.runID).toBeDefined();
    expect(result1.runID).not.toBe(result2.runID);
  });
});
