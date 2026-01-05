import { dump, load } from "js-yaml";
import { formatError } from "../utils/error.js";
import { isSensitiveJob } from "../workflow/sensitivity.js";
import type { Job } from "../workflow/types.js";

/**
 * Result of workflow injection with sensitivity information.
 */
export interface InjectionResult {
  readonly content: string;
  readonly skippedJobs: readonly string[];
}

/**
 * Manifest job info for TUI display
 */
export interface ManifestJob {
  readonly id: string;
  readonly name: string;
  readonly uses?: string;
  readonly sensitive: boolean;
  readonly steps: readonly string[];
  readonly needs?: readonly string[];
}

/**
 * Manifest info (v2 format matching Go CLI)
 * Note: Uses "v" key (not "version") to match Go CLI JSON serialization
 */
export interface ManifestInfo {
  readonly v: 2;
  readonly jobs: readonly ManifestJob[];
}

// Valid job ID pattern (GitHub Actions spec)
const VALID_JOB_ID_PATTERN = /^[a-zA-Z_][a-zA-Z0-9_-]*$/;

// Max length for run command display names
const MAX_RUN_COMMAND_DISPLAY = 40;

/**
 * Parses needs field which can be a string or array.
 */
const parseNeeds = (needs: unknown): readonly string[] => {
  if (!needs) {
    return [];
  }
  if (typeof needs === "string") {
    return needs ? [needs] : [];
  }
  if (Array.isArray(needs)) {
    return needs.filter((n): n is string => typeof n === "string");
  }
  return [];
};

/**
 * Checks if a job has dependencies.
 */
const jobHasNeeds = (job: Record<string, unknown>): boolean => {
  const needs = job.needs;
  if (!needs) {
    return false;
  }
  if (typeof needs === "string") {
    return needs.length > 0;
  }
  if (Array.isArray(needs)) {
    return needs.length > 0;
  }
  return false;
};

/**
 * Injects workflow modifications for safe local execution.
 *
 * This function:
 * 1. Adds `continue-on-error: true` to non-sensitive jobs
 * 2. Adds `if: false` to sensitive jobs to skip them
 * 3. Adds `if: always()` to non-sensitive jobs with dependencies
 *
 * @param yamlContent - Raw YAML content of the workflow file
 * @param skipSensitive - Whether to skip sensitive jobs (default: true)
 * @returns Injection result with modified YAML and list of skipped jobs
 * @throws Error if YAML is invalid or has unexpected structure
 */
export const injectWorkflow = (
  yamlContent: string,
  skipSensitive = true
): InjectionResult => {
  if (yamlContent.trim().length === 0) {
    throw new Error("Workflow YAML content is empty");
  }

  let workflow: unknown;

  try {
    workflow = load(yamlContent);
  } catch (error) {
    throw new Error(`Failed to parse workflow YAML: ${formatError(error)}`);
  }

  if (!workflow || typeof workflow !== "object" || Array.isArray(workflow)) {
    throw new Error("Workflow YAML must be an object");
  }

  const workflowObj = workflow as Record<string, unknown>;

  if (!("jobs" in workflowObj)) {
    return { content: yamlContent, skippedJobs: [] };
  }

  const jobs = workflowObj.jobs;

  if (!jobs || typeof jobs !== "object" || Array.isArray(jobs)) {
    throw new Error("Workflow 'jobs' must be an object");
  }

  const jobsObj = jobs as Record<string, unknown>;
  const skippedJobs: string[] = [];

  for (const jobId of Object.keys(jobsObj)) {
    const job = jobsObj[jobId];

    if (!job || typeof job !== "object" || Array.isArray(job)) {
      continue;
    }

    const jobObj = job as Record<string, unknown>;

    // Skip reusable workflows (they don't support if: at job level)
    if (typeof jobObj.uses === "string" && jobObj.uses.length > 0) {
      continue;
    }

    // Check if job is sensitive
    const jobForCheck: Job = {
      name: typeof jobObj.name === "string" ? jobObj.name : undefined,
      steps: Array.isArray(jobObj.steps)
        ? (jobObj.steps as readonly Record<string, unknown>[]).map((s) => ({
            uses: typeof s.uses === "string" ? s.uses : undefined,
            run: typeof s.run === "string" ? s.run : undefined,
          }))
        : undefined,
      needs: parseNeeds(jobObj.needs),
    };

    const sensitive = skipSensitive && isSensitiveJob(jobId, jobForCheck);

    if (sensitive) {
      // Force skip by setting if: false
      jobObj.if = "false";
      skippedJobs.push(jobId);
    } else {
      // Add continue-on-error for non-sensitive jobs
      jobObj["continue-on-error"] = true;

      // Add if: always() for jobs with dependencies so they run even if deps fail
      if (jobHasNeeds(jobObj)) {
        const existingIf = jobObj.if;
        if (typeof existingIf === "string" && existingIf.length > 0) {
          jobObj.if = `always() && (${existingIf})`;
        } else {
          jobObj.if = "always()";
        }
      }
    }
  }

  try {
    const content = dump(workflowObj, {
      indent: 2,
      lineWidth: 80,
      noRefs: true,
      sortKeys: false,
      quotingType: '"',
      forceQuotes: false,
    });
    return { content, skippedJobs };
  } catch (error) {
    throw new Error(`Failed to serialize workflow YAML: ${formatError(error)}`);
  }
};

/**
 * Injects `continue-on-error: true` at the job level for all jobs in a GitHub Actions workflow.
 *
 * This allows workflows to run to completion even when jobs fail, which is essential
 * for collecting all errors during detent's check phase.
 *
 * @param yamlContent - Raw YAML content of the workflow file
 * @returns Modified YAML with continue-on-error injected into all jobs
 * @throws Error if YAML is invalid or has unexpected structure
 * @deprecated Use injectWorkflow instead for sensitivity-aware injection
 */
export const injectContinueOnError = (yamlContent: string): string => {
  return injectWorkflow(yamlContent, false).content;
};

// ========== Marker Injection (ported from Go CLI) ==========

/**
 * Checks if a job ID is valid per GitHub Actions spec.
 * Prevents shell injection in marker echo commands.
 */
const isValidJobId = (jobId: string): boolean => {
  return VALID_JOB_ID_PATTERN.test(jobId);
};

/**
 * Gets a human-readable display name for a step.
 */
const getStepDisplayName = (step: Record<string, unknown>): string => {
  if (typeof step.name === "string" && step.name) {
    return step.name;
  }
  if (typeof step.id === "string" && step.id) {
    return step.id;
  }
  if (typeof step.uses === "string" && step.uses) {
    const parts = step.uses.split("@");
    if (parts[0]) {
      const segments = parts[0].split("/");
      return segments.at(-1) || parts[0];
    }
    return step.uses;
  }
  if (typeof step.run === "string" && step.run) {
    const run = step.run.trim().split("\n")[0] || "";
    if (run.length > MAX_RUN_COMMAND_DISPLAY) {
      return `${run.slice(0, MAX_RUN_COMMAND_DISPLAY - 3)}...`;
    }
    return run;
  }
  return "Step";
};

/**
 * Sanitizes a string for safe use in a single-quoted shell echo command.
 */
const sanitizeForShellEcho = (s: string): string => {
  let result = s;
  result = result.replace(/\n/g, " ");
  result = result.replace(/\r/g, " ");
  result = result.replace(/\t/g, " ");
  // biome-ignore lint/suspicious/noControlCharactersInRegex: Null bytes must be stripped for shell safety
  result = result.replace(/\x00/g, "");
  result = result.replace(/'/g, "'\\''");
  return result;
};

/**
 * Builds a manifest from workflow jobs.
 */
export const buildManifest = (
  jobsObj: Record<string, unknown>,
  skipSensitive = true
): ManifestInfo => {
  const jobs: ManifestJob[] = [];

  for (const jobId of Object.keys(jobsObj).sort()) {
    const job = jobsObj[jobId];
    if (!job || typeof job !== "object" || Array.isArray(job)) {
      continue;
    }
    if (!isValidJobId(jobId)) {
      continue;
    }

    const jobObj = job as Record<string, unknown>;

    const jobForCheck: Job = {
      name: typeof jobObj.name === "string" ? jobObj.name : undefined,
      steps: Array.isArray(jobObj.steps)
        ? (jobObj.steps as readonly Record<string, unknown>[]).map((s) => ({
            uses: typeof s.uses === "string" ? s.uses : undefined,
            run: typeof s.run === "string" ? s.run : undefined,
          }))
        : undefined,
      needs: parseNeeds(jobObj.needs),
    };

    const sensitive = skipSensitive && isSensitiveJob(jobId, jobForCheck);

    // Determine steps for manifest (reusable workflows have no steps)
    let manifestSteps: readonly string[] = [];
    if (typeof jobObj.uses !== "string" && Array.isArray(jobObj.steps)) {
      manifestSteps = (jobObj.steps as readonly Record<string, unknown>[]).map(
        getStepDisplayName
      );
    }

    const manifestJob: ManifestJob = {
      id: jobId,
      name:
        typeof jobObj.name === "string" && jobObj.name ? jobObj.name : jobId,
      uses: typeof jobObj.uses === "string" ? jobObj.uses : undefined,
      sensitive,
      steps: manifestSteps,
      needs: parseNeeds(jobObj.needs),
    };

    jobs.push(manifestJob);
  }

  return { v: 2, jobs };
};

/**
 * Injects detent lifecycle markers into workflow for reliable job/step tracking.
 * This is the key to real-time TUI updates - markers are parsed from act output.
 *
 * Each job gets:
 * - Manifest step (first job only): Contains all job/step info as base64 JSON
 * - Job-start marker: echo '::detent::job-start::{jobId}'
 * - Step-start markers: echo '::detent::step-start::{jobId}::{idx}::{name}' before each step
 * - Job-end marker: echo '::detent::job-end::{jobId}::${{ job.status }}'
 */
export const injectJobMarkers = (
  yamlContent: string,
  skipSensitive = true
): string => {
  if (yamlContent.trim().length === 0) {
    throw new Error("Workflow YAML content is empty");
  }

  let workflow: unknown;
  try {
    workflow = load(yamlContent);
  } catch (error) {
    throw new Error(`Failed to parse workflow YAML: ${formatError(error)}`);
  }

  if (!workflow || typeof workflow !== "object" || Array.isArray(workflow)) {
    throw new Error("Workflow YAML must be an object");
  }

  const workflowObj = workflow as Record<string, unknown>;

  if (!("jobs" in workflowObj)) {
    return yamlContent;
  }

  const jobs = workflowObj.jobs;
  if (!jobs || typeof jobs !== "object" || Array.isArray(jobs)) {
    throw new Error("Workflow 'jobs' must be an object");
  }

  const jobsObj = jobs as Record<string, unknown>;

  // Build manifest from all jobs
  const manifest = buildManifest(jobsObj, skipSensitive);
  const manifestJson = JSON.stringify(manifest);
  const manifestB64 = Buffer.from(manifestJson).toString("base64");

  // Find valid jobs that can receive markers (sorted alphabetically for determinism)
  const validJobIds = Object.keys(jobsObj)
    .filter((id) => {
      const job = jobsObj[id] as Record<string, unknown>;
      return (
        isValidJobId(id) && !(typeof job.uses === "string" && job.uses.length)
      );
    })
    .sort();

  // Find the best job for manifest injection:
  // Prefer jobs with NO dependencies (they run first), then fall back to first alphabetically
  const jobsWithoutDeps = validJobIds.filter((id) => {
    const job = jobsObj[id] as Record<string, unknown>;
    const needs = job.needs;
    if (!needs) {
      return true;
    }
    if (typeof needs === "string") {
      return needs.length === 0;
    }
    if (Array.isArray(needs)) {
      return needs.length === 0;
    }
    return true;
  });
  const firstJobId = jobsWithoutDeps[0] ?? validJobIds[0];

  // Inject markers into each valid job (using sorted order to ensure manifest goes in first job)
  for (const jobId of validJobIds) {
    // validJobIds already filters for valid IDs and non-reusable workflows
    const jobObj = jobsObj[jobId] as Record<string, unknown>;

    const originalSteps = Array.isArray(jobObj.steps)
      ? (jobObj.steps as Record<string, unknown>[])
      : [];
    const newSteps: Record<string, unknown>[] = [];

    // Add manifest step (first job only)
    if (jobId === firstJobId) {
      newSteps.push({
        name: "detent: manifest",
        run: `echo '::detent::manifest::v2::b64::${manifestB64}'`,
      });
    }

    // Add job-start marker
    newSteps.push({
      name: "detent: job start",
      run: `echo '::detent::job-start::${jobId}'`,
    });

    // Add step markers before each original step
    for (let i = 0; i < originalSteps.length; i++) {
      const step = originalSteps[i];
      if (!step) {
        continue;
      }

      const stepName = sanitizeForShellEcho(getStepDisplayName(step));
      newSteps.push({
        name: `detent: step ${i}`,
        run: `echo '::detent::step-start::${jobId}::${i}::${stepName}'`,
      });
      newSteps.push(step);
    }

    // Add job-end marker (with always() to capture success/failure)
    newSteps.push({
      name: "detent: job end",
      if: "always()",
      run: `echo '::detent::job-end::${jobId}::\${{ job.status }}'`,
    });

    jobObj.steps = newSteps;
  }

  return dump(workflowObj, {
    indent: 2,
    lineWidth: 80,
    noRefs: true,
    sortKeys: false,
    quotingType: '"',
    forceQuotes: false,
  });
};

/**
 * Full workflow injection: sensitivity handling + marker injection.
 * This is the main entry point for preparing workflows.
 * @deprecated Use prepareWorkflowsWithCombinedManifest for multi-workflow scenarios
 */
export const injectWorkflowWithMarkers = (
  yamlContent: string,
  skipSensitive = true
): InjectionResult => {
  // First apply sensitivity handling
  const { content: injectedContent, skippedJobs } = injectWorkflow(
    yamlContent,
    skipSensitive
  );

  // Then inject markers for tracking
  const finalContent = injectJobMarkers(injectedContent, skipSensitive);

  return { content: finalContent, skippedJobs };
};

// ========== Multi-Workflow Support (Go CLI Pattern) ==========

/**
 * Parsed workflow with metadata for multi-workflow processing.
 */
export interface ParsedWorkflowInfo {
  readonly name: string;
  readonly content: string;
  readonly jobs: Record<string, unknown>;
}

/**
 * Result of finding the best manifest job across workflows.
 */
interface ManifestJobLocation {
  readonly workflowName: string;
  readonly jobId: string;
}

/**
 * Finds the best job for manifest injection across ALL workflows.
 * Prefers jobs WITHOUT dependencies (they run first).
 * Falls back to alphabetically first valid job.
 * @deprecated Use findFirstNoDepJob for per-workflow manifest injection
 */
export const findManifestJobLocation = (
  workflows: readonly ParsedWorkflowInfo[]
): ManifestJobLocation | null => {
  let bestLocation: ManifestJobLocation | null = null;
  let fallbackLocation: ManifestJobLocation | null = null;

  // Sort workflows by name for determinism
  const sortedWorkflows = [...workflows].sort((a, b) =>
    a.name.localeCompare(b.name)
  );

  for (const wf of sortedWorkflows) {
    const jobIds = Object.keys(wf.jobs)
      .filter((id) => {
        const job = wf.jobs[id] as Record<string, unknown>;
        return (
          isValidJobId(id) &&
          job &&
          typeof job === "object" &&
          !(typeof job.uses === "string" && job.uses.length > 0)
        );
      })
      .sort();

    for (const jobId of jobIds) {
      const job = wf.jobs[jobId] as Record<string, unknown>;

      // Track fallback (any valid job)
      if (!fallbackLocation) {
        fallbackLocation = { workflowName: wf.name, jobId };
      }

      // Prefer jobs without dependencies
      if (!(jobHasNeeds(job) || bestLocation)) {
        bestLocation = { workflowName: wf.name, jobId };
      }
    }
  }

  return bestLocation ?? fallbackLocation;
};

/**
 * Finds the first job without dependencies within a single workflow.
 * This is used for per-workflow manifest injection to ensure whichever
 * workflow runs first will emit the manifest.
 */
export const findFirstNoDepJob = (
  jobsObj: Record<string, unknown>
): string | null => {
  const validJobIds = Object.keys(jobsObj)
    .filter((id) => {
      const job = jobsObj[id] as Record<string, unknown>;
      return (
        isValidJobId(id) &&
        job &&
        typeof job === "object" &&
        !(typeof job.uses === "string" && job.uses.length > 0)
      );
    })
    .sort();

  // Find first job without dependencies
  for (const jobId of validJobIds) {
    const job = jobsObj[jobId] as Record<string, unknown>;
    if (!jobHasNeeds(job)) {
      return jobId;
    }
  }

  // Fallback to first valid job
  return validJobIds[0] ?? null;
};

/**
 * Builds a combined manifest from multiple workflows.
 */
export const buildCombinedManifest = (
  workflows: readonly ParsedWorkflowInfo[],
  skipSensitive = true
): ManifestInfo => {
  const allJobs: ManifestJob[] = [];

  for (const wf of workflows) {
    const manifest = buildManifest(wf.jobs, skipSensitive);
    allJobs.push(...manifest.jobs);
  }

  // Sort jobs by ID for determinism
  allJobs.sort((a, b) => a.id.localeCompare(b.id));

  return { v: 2, jobs: allJobs };
};

/**
 * Injects markers into a workflow, optionally with manifest.
 * The manifest should only be injected into ONE job across all workflows.
 */
export const injectMarkersWithManifest = (
  yamlContent: string,
  manifestB64: string | null,
  manifestJobId: string | null,
  skipSensitive = true
): string => {
  if (yamlContent.trim().length === 0) {
    throw new Error("Workflow YAML content is empty");
  }

  let workflow: unknown;
  try {
    workflow = load(yamlContent);
  } catch (error) {
    throw new Error(`Failed to parse workflow YAML: ${formatError(error)}`);
  }

  if (!workflow || typeof workflow !== "object" || Array.isArray(workflow)) {
    throw new Error("Workflow YAML must be an object");
  }

  const workflowObj = workflow as Record<string, unknown>;

  if (!("jobs" in workflowObj)) {
    return yamlContent;
  }

  const jobs = workflowObj.jobs;
  if (!jobs || typeof jobs !== "object" || Array.isArray(jobs)) {
    throw new Error("Workflow 'jobs' must be an object");
  }

  const jobsObj = jobs as Record<string, unknown>;

  // Find valid jobs for marker injection
  const validJobIds = Object.keys(jobsObj)
    .filter((id) => {
      const job = jobsObj[id] as Record<string, unknown>;
      return (
        isValidJobId(id) && !(typeof job.uses === "string" && job.uses.length)
      );
    })
    .sort();

  // Inject markers into each valid job
  for (const jobId of validJobIds) {
    const jobObj = jobsObj[jobId] as Record<string, unknown>;

    const originalSteps = Array.isArray(jobObj.steps)
      ? (jobObj.steps as Record<string, unknown>[])
      : [];
    const newSteps: Record<string, unknown>[] = [];

    // Add manifest step ONLY to the designated job
    if (manifestB64 && manifestJobId && jobId === manifestJobId) {
      newSteps.push({
        name: "detent: manifest",
        run: `echo '::detent::manifest::v2::b64::${manifestB64}'`,
      });
    }

    // Add job-start marker
    newSteps.push({
      name: "detent: job start",
      run: `echo '::detent::job-start::${jobId}'`,
    });

    // Add step markers before each original step
    for (let i = 0; i < originalSteps.length; i++) {
      const step = originalSteps[i];
      if (!step) {
        continue;
      }

      const stepName = sanitizeForShellEcho(getStepDisplayName(step));
      newSteps.push({
        name: `detent: step ${i}`,
        run: `echo '::detent::step-start::${jobId}::${i}::${stepName}'`,
      });
      newSteps.push(step);
    }

    // Add job-end marker (with always() to capture success/failure)
    newSteps.push({
      name: "detent: job end",
      if: "always()",
      run: `echo '::detent::job-end::${jobId}::\${{ job.status }}'`,
    });

    jobObj.steps = newSteps;
  }

  return dump(workflowObj, {
    indent: 2,
    lineWidth: 80,
    noRefs: true,
    sortKeys: false,
    quotingType: '"',
    forceQuotes: false,
  });
};
