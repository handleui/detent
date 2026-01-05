import { dump, load } from "js-yaml";
import { formatError } from "../utils/error.js";

/**
 * Injects `continue-on-error: true` at the job level for all jobs in a GitHub Actions workflow.
 *
 * This allows workflows to run to completion even when jobs fail, which is essential
 * for collecting all errors during detent's check phase.
 *
 * @param yamlContent - Raw YAML content of the workflow file
 * @returns Modified YAML with continue-on-error injected into all jobs
 * @throws Error if YAML is invalid or has unexpected structure
 */
export const injectContinueOnError = (yamlContent: string): string => {
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

  for (const jobName of Object.keys(jobsObj)) {
    const job = jobsObj[jobName];

    if (!job || typeof job !== "object" || Array.isArray(job)) {
      continue;
    }

    const jobObj = job as Record<string, unknown>;
    jobObj["continue-on-error"] = true;
  }

  try {
    return dump(workflowObj, {
      indent: 2,
      lineWidth: 80,
      noRefs: true,
      sortKeys: false,
      quotingType: '"',
      forceQuotes: false,
    });
  } catch (error) {
    throw new Error(`Failed to serialize workflow YAML: ${formatError(error)}`);
  }
};
