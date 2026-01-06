import { readdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import {
  computeCurrentRunID,
  createEphemeralClonePath,
  prepareClone,
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
 * 3. Create ephemeral clone for isolated execution
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
   * @returns PrepareResult with clone path, run ID, and workflows
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
      console.log("[Prepare] Creating clone...");
    }

    this.debugLogger?.logPhase("Prepare", "Creating clone");
    const { clonePath, runID, cleanup } = await this.createClone();
    this.debugLogger?.logPhase("Prepare", `Clone created at: ${clonePath}`);
    this.debugLogger?.logPhase("Prepare", `Run ID: ${runID}`);

    this.debugLogger?.logPhase(
      "Prepare",
      `Injecting continue-on-error into ${workflows.length} workflow(s)`
    );
    const { skippedWorkflows, skippedJobs, manifest } =
      await this.injectWorkflows(clonePath, workflows);

    if (this.config.verbose) {
      console.log("[Prepare] ✓ Clone ready\n");
    }

    this.debugLogger?.endPhase("Prepare");

    return {
      clonePath,
      runID,
      workflows,
      manifest,
      skippedWorkflows:
        skippedWorkflows.length > 0 ? skippedWorkflows : undefined,
      skippedJobs: skippedJobs.length > 0 ? skippedJobs : undefined,
      cleanup,
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
   * Creates an ephemeral git clone for isolated execution.
   *
   * @returns Object containing clone path, run ID, and cleanup function
   * @throws Error if clone creation fails
   */
  private async createClone(): Promise<{
    clonePath: string;
    runID: string;
    cleanup: () => Promise<void>;
  }> {
    const runIDInfo = await computeCurrentRunID(this.config.repoRoot);
    const clonePath = createEphemeralClonePath(runIDInfo.runID);

    const { cleanup } = await prepareClone({
      repoRoot: this.config.repoRoot,
      clonePath,
    });

    return {
      clonePath,
      runID: runIDInfo.runID,
      cleanup,
    };
  }

  /**
   * Injects workflow modifications for safe local execution.
   *
   * Strategy for manifest injection:
   * 1. Build a COMBINED manifest from ALL workflows (contains all jobs)
   * 2. Inject manifest into EACH workflow's first no-deps job
   *
   * This ensures that whichever workflow runs first (based on event triggers),
   * its first job will emit the complete manifest. The parser handles duplicate
   * manifests by using the first one received.
   *
   * @param clonePath - Path to the clone
   * @param workflows - Array of workflow files to inject
   * @returns Object containing lists of skipped workflows and jobs
   * @throws Error if injection or writing fails
   */
  private async injectWorkflows(
    clonePath: string,
    workflows: readonly WorkflowFile[]
  ): Promise<{
    skippedWorkflows: readonly string[];
    skippedJobs: readonly string[];
    manifest: import("./types.js").Manifest;
  }> {
    const { isSensitiveWorkflow } = await import("../workflow/sensitivity.js");
    const {
      injectWorkflow,
      injectMarkersWithManifest,
      buildCombinedManifest,
      findFirstNoDepJob,
    } = await import("./workflow-injector.js");

    const allSkippedWorkflows: string[] = [];
    const allSkippedJobs: string[] = [];

    const parsedWorkflows = this.filterAndParseWorkflows(
      workflows,
      isSensitiveWorkflow,
      injectWorkflow,
      allSkippedWorkflows,
      allSkippedJobs
    );

    // Build manifest for TUI (emitted directly, not via act)
    const manifest = this.buildManifest(parsedWorkflows, buildCombinedManifest);

    // Build base64 for act echo (backup/validation)
    const manifestB64 = Buffer.from(JSON.stringify(manifest)).toString(
      "base64"
    );

    await this.writeInjectedWorkflows(
      clonePath,
      parsedWorkflows,
      manifestB64,
      injectMarkersWithManifest,
      findFirstNoDepJob
    );

    return {
      skippedWorkflows: allSkippedWorkflows,
      skippedJobs: allSkippedJobs,
      manifest,
    };
  }

  /**
   * Builds the manifest object for TUI display.
   */
  private buildManifest(
    parsedWorkflows: readonly ParsedWorkflow[],
    buildCombinedManifest: (
      infos: readonly {
        name: string;
        content: string;
        jobs: Record<string, unknown>;
      }[]
    ) => import("./types.js").Manifest
  ): import("./types.js").Manifest {
    const workflowInfos = parsedWorkflows.map((pw) => ({
      name: pw.name,
      content: pw.injectedContent,
      jobs: pw.jobs,
    }));

    return buildCombinedManifest(workflowInfos);
  }

  /**
   * Filters sensitive workflows and parses remaining ones for injection.
   */
  private filterAndParseWorkflows(
    workflows: readonly WorkflowFile[],
    isSensitiveWorkflow: (name: string) => boolean,
    injectWorkflow: (content: string) => {
      content: string;
      skippedJobs: readonly string[];
    },
    allSkippedWorkflows: string[],
    allSkippedJobs: string[]
  ): readonly ParsedWorkflow[] {
    const parsedWorkflows: ParsedWorkflow[] = [];

    for (const workflow of workflows) {
      if (isSensitiveWorkflow(workflow.name)) {
        allSkippedWorkflows.push(workflow.name);
        this.logSkippedWorkflow(workflow.name);
        continue;
      }

      const result = this.processWorkflow(
        workflow,
        injectWorkflow,
        allSkippedJobs
      );

      if (result) {
        parsedWorkflows.push(result);
      }
    }

    return parsedWorkflows;
  }

  /**
   * Logs a skipped sensitive workflow if verbose mode is enabled.
   */
  private logSkippedWorkflow(workflowName: string): void {
    if (this.config.verbose) {
      console.log(`[Inject] ! Skipping sensitive workflow: ${workflowName}`);
    }
  }

  /**
   * Processes a single workflow: injects modifications and parses for jobs.
   */
  private processWorkflow(
    workflow: WorkflowFile,
    injectWorkflow: (content: string) => {
      content: string;
      skippedJobs: readonly string[];
    },
    allSkippedJobs: string[]
  ): ParsedWorkflow | undefined {
    const { content: injectedContent, skippedJobs } = injectWorkflow(
      workflow.content
    );

    this.collectSkippedJobs(workflow.name, skippedJobs, allSkippedJobs);

    const jobs = this.extractJobs(injectedContent);
    if (!jobs) {
      return undefined;
    }

    return {
      name: workflow.name,
      injectedContent,
      jobs,
    };
  }

  /**
   * Collects skipped jobs and logs them if verbose mode is enabled.
   */
  private collectSkippedJobs(
    workflowName: string,
    skippedJobs: readonly string[],
    allSkippedJobs: string[]
  ): void {
    if (skippedJobs.length === 0) {
      return;
    }

    for (const jobName of skippedJobs) {
      allSkippedJobs.push(`${workflowName}:${jobName}`);
    }

    if (this.config.verbose) {
      console.log(
        `[Inject] ! Skipping sensitive jobs in ${workflowName}: ${skippedJobs.join(", ")}`
      );
    }
  }

  /**
   * Extracts jobs from injected workflow content.
   */
  private extractJobs(content: string): Record<string, unknown> | undefined {
    let parsed: unknown;
    try {
      parsed = load(content);
    } catch {
      return undefined;
    }

    if (
      !parsed ||
      typeof parsed !== "object" ||
      Array.isArray(parsed) ||
      !("jobs" in parsed)
    ) {
      return undefined;
    }

    const jobs = (parsed as Record<string, unknown>).jobs;
    if (!jobs || typeof jobs !== "object" || Array.isArray(jobs)) {
      return undefined;
    }

    return jobs as Record<string, unknown>;
  }

  /**
   * Writes injected workflows to the clone.
   */
  private async writeInjectedWorkflows(
    clonePath: string,
    parsedWorkflows: readonly ParsedWorkflow[],
    manifestB64: string,
    injectMarkersWithManifest: (
      content: string,
      manifest: string | null,
      jobId: string | null
    ) => string,
    findFirstNoDepJob: (jobs: Record<string, unknown>) => string | null
  ): Promise<void> {
    await Promise.all(
      parsedWorkflows.map(async (pw) => {
        const manifestJobId = findFirstNoDepJob(pw.jobs);
        const finalContent = injectMarkersWithManifest(
          pw.injectedContent,
          manifestB64,
          manifestJobId
        );

        const targetPath = join(clonePath, ".github", "workflows", pw.name);

        try {
          await writeFile(targetPath, finalContent, "utf-8");
        } catch (error) {
          throw new Error(
            `Failed to write injected workflow ${pw.name}: ${formatError(error)}`
          );
        }
      })
    );
  }
}

interface ParsedWorkflow {
  name: string;
  injectedContent: string;
  jobs: Record<string, unknown>;
}
