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
