import { readdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import {
  computeCurrentRunID,
  createEphemeralWorktreePath,
  prepareWorktree,
} from "@detent/git";
import { load } from "js-yaml";
import {
  checkActInstalled,
  checkDockerRunning,
  checkGitRepository,
} from "../preflight/checks.js";
import type { DebugLogger } from "../utils/debug-logger.js";
import { formatError } from "../utils/error.js";
import type { PrepareResult, RunConfig, WorkflowFile } from "./types.js";

/**
 * Prepares the execution environment for running GitHub Actions workflows.
 *
 * Responsibilities:
 * 1. Run preflight checks (git, act, docker)
 * 2. Discover workflow files in .github/workflows
 * 3. Create ephemeral worktree for isolated execution
 * 4. Inject continue-on-error into workflow files
 */
export class WorkflowPreparer {
  private readonly config: RunConfig;
  private readonly debugLogger?: DebugLogger;

  constructor(config: RunConfig, debugLogger?: DebugLogger) {
    this.config = config;
    this.debugLogger = debugLogger;
  }

  /**
   * Prepares the execution environment.
   *
   * @returns PrepareResult with worktree path, run ID, and workflows
   * @throws Error if preflight checks fail or workflows cannot be prepared
   */
  async prepare(): Promise<PrepareResult> {
    this.debugLogger?.startPhase("Prepare");

    await this.runPreflightChecks();

    if (this.config.verbose) {
      console.log("[Prepare] Discovering workflows...");
    }

    this.debugLogger?.logPhase("Prepare", "Discovering workflows");
    const workflows = await this.discoverWorkflows();

    if (workflows.length === 0) {
      if (this.config.workflow) {
        const allWorkflows = await this.listAllWorkflows();
        if (allWorkflows.length > 0) {
          throw new Error(
            `Workflow "${this.config.workflow}" not found in .github/workflows/\n\n` +
              `Available workflows:\n${allWorkflows.map((w) => `  - ${w}`).join("\n")}`
          );
        }
        throw new Error(
          `Workflow "${this.config.workflow}" not found in .github/workflows/`
        );
      }
      throw new Error("No workflow files found in .github/workflows/");
    }

    this.debugLogger?.logPhase(
      "Prepare",
      `Found ${workflows.length} workflow(s): ${workflows.map((w) => w.name).join(", ")}`
    );

    if (this.config.verbose) {
      console.log(
        `[Prepare] ✓ Found ${workflows.length} workflow(s): ${workflows.map((w) => w.name).join(", ")}\n`
      );
      console.log("[Prepare] Creating worktree...");
    }

    this.debugLogger?.logPhase("Prepare", "Creating worktree");
    const { worktreePath, runID } = await this.createWorktree();
    this.debugLogger?.logPhase(
      "Prepare",
      `Worktree created at: ${worktreePath}`
    );
    this.debugLogger?.logPhase("Prepare", `Run ID: ${runID}`);

    this.debugLogger?.logPhase(
      "Prepare",
      `Injecting continue-on-error into ${workflows.length} workflow(s)`
    );
    await this.injectWorkflows(worktreePath, workflows);

    if (this.config.verbose) {
      console.log("[Prepare] ✓ Worktree ready\n");
    }

    this.debugLogger?.endPhase("Prepare");

    return {
      worktreePath,
      runID,
      workflows,
    };
  }

  /**
   * Runs all preflight checks and throws if any fail.
   *
   * @throws Error with details of the first failed check
   */
  private async runPreflightChecks(): Promise<void> {
    this.debugLogger?.startPhase("Preflight");

    if (this.config.verbose) {
      console.log("[Preflight] Checking git repository...");
    }

    const checks = [
      { name: "Git Repository", fn: checkGitRepository },
      { name: "Act Installation", fn: checkActInstalled },
      { name: "Docker Daemon", fn: checkDockerRunning },
    ];

    for (let i = 0; i < checks.length; i++) {
      const check = checks[i];
      if (!check) {
        throw new Error("Invalid check configuration");
      }

      if (this.config.verbose && i > 0) {
        console.log(`[Preflight] Checking ${check.name.toLowerCase()}...`);
      }

      this.debugLogger?.logPhase("Preflight", `Running check: ${check.name}`);
      const result = await check.fn();
      if (!result.passed) {
        const errorMessage = result.message || `${check.name} check failed`;
        this.debugLogger?.logPhase(
          "Preflight",
          `✗ ${check.name} failed: ${errorMessage}`
        );
        throw result.error || new Error(errorMessage);
      }
      this.debugLogger?.logPhase("Preflight", `✓ ${check.name} passed`);
    }

    if (this.config.verbose) {
      console.log("[Preflight] ✓ All checks passed\n");
    }

    this.debugLogger?.endPhase("Preflight");
  }

  /**
   * Lists all workflow files in .github/workflows directory.
   *
   * @returns Array of workflow file names
   */
  private async listAllWorkflows(): Promise<readonly string[]> {
    const workflowsDir = join(this.config.repoRoot, ".github", "workflows");

    try {
      const files = await readdir(workflowsDir);
      return files.filter(
        (file) => file.endsWith(".yml") || file.endsWith(".yaml")
      );
    } catch {
      return [];
    }
  }

  /**
   * Discovers workflow files in .github/workflows directory.
   *
   * @returns Array of workflow files matching the configuration
   * @throws Error if workflows directory doesn't exist or is inaccessible
   */
  private async discoverWorkflows(): Promise<readonly WorkflowFile[]> {
    const workflowsDir = join(this.config.repoRoot, ".github", "workflows");

    let files: string[];
    try {
      files = await readdir(workflowsDir);
    } catch (error) {
      if (
        error &&
        typeof error === "object" &&
        "code" in error &&
        error.code === "ENOENT"
      ) {
        throw new Error(
          "Workflows directory not found (.github/workflows). Is this a GitHub Actions repository?"
        );
      }
      throw new Error(
        `Failed to read workflows directory: ${formatError(error)}`
      );
    }

    const yamlFiles = files.filter(
      (file) => file.endsWith(".yml") || file.endsWith(".yaml")
    );

    const filteredFiles = this.config.workflow
      ? yamlFiles.filter((file) => file === this.config.workflow)
      : yamlFiles;

    const workflowPromises = filteredFiles.map(async (file) => {
      const filePath = join(workflowsDir, file);

      let content: string;
      try {
        content = await readFile(filePath, "utf-8");
      } catch (error) {
        throw new Error(
          `Failed to read workflow file ${file}: ${formatError(error)}`
        );
      }

      if (content.trim().length === 0) {
        throw new Error(`Workflow file ${file} is empty`);
      }

      if (this.config.job) {
        this.validateJobExists(content, this.config.job, file);
      }

      return {
        name: file,
        path: filePath,
        content,
      };
    });

    return await Promise.all(workflowPromises);
  }

  /**
   * Validates that a specific job exists in a workflow.
   *
   * @param yamlContent - Raw YAML content of the workflow
   * @param jobName - Name of the job to validate
   * @param workflowName - Name of the workflow file for error messages
   * @throws Error if job is not found in the workflow
   */
  private validateJobExists(
    yamlContent: string,
    jobName: string,
    workflowName: string
  ): void {
    let workflow: unknown;

    try {
      workflow = load(yamlContent);
    } catch (error) {
      throw new Error(
        `Failed to parse workflow ${workflowName}: ${formatError(error)}`
      );
    }

    if (!workflow || typeof workflow !== "object" || Array.isArray(workflow)) {
      throw new Error(`Workflow ${workflowName} must be an object`);
    }

    const workflowObj = workflow as Record<string, unknown>;

    if (!("jobs" in workflowObj)) {
      throw new Error(`Workflow ${workflowName} does not contain any jobs`);
    }

    const jobs = workflowObj.jobs;

    if (!jobs || typeof jobs !== "object" || Array.isArray(jobs)) {
      throw new Error(`Workflow ${workflowName} 'jobs' must be an object`);
    }

    const jobsObj = jobs as Record<string, unknown>;

    if (!(jobName in jobsObj)) {
      const availableJobs = Object.keys(jobsObj).join(", ");
      throw new Error(
        `Job "${jobName}" not found in workflow ${workflowName}. Available jobs: ${availableJobs}`
      );
    }
  }

  /**
   * Creates an ephemeral git worktree for isolated execution.
   *
   * @returns Object containing worktree path and run ID
   * @throws Error if worktree creation fails
   */
  private async createWorktree(): Promise<{
    worktreePath: string;
    runID: string;
  }> {
    const runIDInfo = await computeCurrentRunID(this.config.repoRoot);
    const worktreePath = createEphemeralWorktreePath(runIDInfo.runID);

    await prepareWorktree({
      repoRoot: this.config.repoRoot,
      worktreePath,
    });

    return {
      worktreePath,
      runID: runIDInfo.runID,
    };
  }

  /**
   * Injects workflow modifications for safe local execution.
   *
   * This method:
   * 1. Skips sensitive workflow files entirely
   * 2. Injects `if: false` on sensitive jobs to skip them
   * 3. Injects `continue-on-error: true` on non-sensitive jobs
   * 4. Injects `if: always()` on jobs with dependencies
   *
   * @param worktreePath - Path to the worktree
   * @param workflows - Array of workflow files to inject
   * @throws Error if injection or writing fails
   */
  private async injectWorkflows(
    worktreePath: string,
    workflows: readonly WorkflowFile[]
  ): Promise<void> {
    const { isSensitiveWorkflow } = await import("../workflow/sensitivity.js");
    const { injectWorkflow } = await import("./workflow-injector.js");

    await Promise.all(
      workflows.map(async (workflow) => {
        // Skip entire workflow if filename indicates sensitivity
        if (isSensitiveWorkflow(workflow.name)) {
          if (this.config.verbose) {
            console.log(
              `[Inject] ! Skipping sensitive workflow: ${workflow.name}`
            );
          }
          return;
        }

        const { content, skippedJobs } = injectWorkflow(workflow.content);

        if (this.config.verbose && skippedJobs.length > 0) {
          console.log(
            `[Inject] ! Skipping sensitive jobs in ${workflow.name}: ${skippedJobs.join(", ")}`
          );
        }

        const targetPath = join(
          worktreePath,
          ".github",
          "workflows",
          workflow.name
        );

        try {
          await writeFile(targetPath, content, "utf-8");
        } catch (error) {
          throw new Error(
            `Failed to write injected workflow ${workflow.name}: ${formatError(error)}`
          );
        }
      })
    );
  }
}
