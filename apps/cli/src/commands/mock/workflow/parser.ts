import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import { load } from "js-yaml";
import type { Job, JobInfo, Workflow } from "./types.js";

/**
 * Maximum allowed size for a workflow file (1MB).
 * Prevents resource exhaustion from maliciously large files.
 */
const MAX_WORKFLOW_SIZE_BYTES = 1 * 1024 * 1024;

/**
 * Validates workflow content for potentially malicious or malformed content.
 */
const validateWorkflowContent = (data: string): void => {
  if (data.length > MAX_WORKFLOW_SIZE_BYTES) {
    throw new Error(
      `Workflow file exceeds maximum size of ${MAX_WORKFLOW_SIZE_BYTES} bytes`
    );
  }

  if (data.includes("\0")) {
    throw new Error(
      "Workflow file contains null bytes (binary content not allowed)"
    );
  }

  let controlCount = 0;
  for (const char of data) {
    const code = char.charCodeAt(0);
    if (code < 32 && code !== 10 && code !== 13 && code !== 9) {
      controlCount++;
    }
  }
  if (controlCount > 10) {
    throw new Error(
      `Workflow file contains excessive control characters (${controlCount} found)`
    );
  }
};

/**
 * Parses a workflow YAML string into a Workflow object.
 *
 * @param content - Raw YAML content
 * @returns Parsed workflow
 */
export const parseWorkflow = (content: string): Workflow => {
  validateWorkflowContent(content);

  const parsed = load(content);

  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("Workflow YAML must be an object");
  }

  const workflow = parsed as Record<string, unknown>;

  if (!workflow.jobs || typeof workflow.jobs !== "object") {
    throw new Error("Workflow must have a 'jobs' object");
  }

  return workflow as unknown as Workflow;
};

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

interface DependencyGraph {
  inDegree: Map<string, number>;
  dependents: Map<string, string[]>;
}

/**
 * Builds the dependency graph for topological sorting.
 * Returns in-degree map and adjacency list of dependents.
 */
const buildDependencyGraph = (
  jobInfoMap: Map<string, JobInfo>
): DependencyGraph => {
  const inDegree = new Map<string, number>();
  const dependents = new Map<string, string[]>();

  for (const id of jobInfoMap.keys()) {
    inDegree.set(id, 0);
  }

  for (const [id, job] of jobInfoMap) {
    for (const dep of job.needs) {
      if (!jobInfoMap.has(dep)) {
        continue;
      }
      inDegree.set(id, (inDegree.get(id) ?? 0) + 1);
      const existing = dependents.get(dep) ?? [];
      existing.push(id);
      dependents.set(dep, existing);
    }
  }

  return { inDegree, dependents };
};

/**
 * Finds all jobs with zero in-degree (no dependencies).
 */
const findRootJobs = (inDegree: Map<string, number>): string[] => {
  const roots: string[] = [];
  for (const [id, degree] of inDegree) {
    if (degree === 0) {
      roots.push(id);
    }
  }
  return roots.sort();
};

/**
 * Processes dependents of a job, reducing their in-degree.
 * Returns jobs that are now ready (in-degree became 0).
 */
const processJobDependents = (
  jobId: string,
  dependents: Map<string, string[]>,
  inDegree: Map<string, number>
): string[] => {
  const readyJobs: string[] = [];
  for (const dependent of dependents.get(jobId) ?? []) {
    const newDegree = (inDegree.get(dependent) ?? 1) - 1;
    inDegree.set(dependent, newDegree);
    if (newDegree === 0) {
      readyJobs.push(dependent);
    }
  }
  return readyJobs.sort();
};

/**
 * Adds remaining jobs (from cycles or missing deps) to the result.
 */
const appendRemainingJobs = (
  result: JobInfo[],
  jobInfoMap: Map<string, JobInfo>
): void => {
  if (result.length >= jobInfoMap.size) {
    return;
  }

  const addedSet = new Set(result.map((j) => j.id));
  const remaining = [...jobInfoMap.keys()]
    .filter((id) => !addedSet.has(id))
    .sort();

  for (const id of remaining) {
    const job = jobInfoMap.get(id);
    if (job) {
      result.push(job);
    }
  }
};

/**
 * Performs topological sort of jobs based on their needs dependencies.
 * Jobs with no dependencies come first, followed by jobs that depend on them.
 * Within each level, jobs are sorted alphabetically for deterministic ordering.
 */
const topologicalSort = (
  jobInfoMap: Map<string, JobInfo>
): readonly JobInfo[] => {
  if (jobInfoMap.size === 0) {
    return [];
  }

  const { inDegree, dependents } = buildDependencyGraph(jobInfoMap);
  const queue = findRootJobs(inDegree);
  const result: JobInfo[] = [];

  while (queue.length > 0) {
    const current = queue.shift();
    if (!current) {
      continue;
    }

    const job = jobInfoMap.get(current);
    if (job) {
      result.push(job);
    }

    const readyJobs = processJobDependents(current, dependents, inDegree);
    queue.push(...readyJobs);
  }

  appendRemainingJobs(result, jobInfoMap);

  return result;
};

/**
 * Extracts job information from a workflow for display.
 * Jobs are sorted topologically by needs dependencies.
 *
 * @param workflow - Parsed workflow
 * @returns Array of JobInfo sorted by dependencies
 */
export const extractJobInfo = (workflow: Workflow): readonly JobInfo[] => {
  if (!workflow.jobs) {
    return [];
  }

  const jobInfoMap = new Map<string, JobInfo>();

  for (const [id, job] of Object.entries(workflow.jobs)) {
    if (!job) {
      continue;
    }

    jobInfoMap.set(id, {
      id,
      name: job.name || id,
      needs: parseNeeds(job.needs),
    });
  }

  return topologicalSort(jobInfoMap);
};

/**
 * Discovers workflow files in a directory.
 *
 * @param dir - Path to .github/workflows directory
 * @returns Array of workflow filenames
 */
export const discoverWorkflows = async (
  dir: string
): Promise<readonly string[]> => {
  try {
    const files = await readdir(dir);
    return files
      .filter((file) => file.endsWith(".yml") || file.endsWith(".yaml"))
      .sort();
  } catch {
    return [];
  }
};

/**
 * Result of parsing a workflow file.
 */
export interface ParsedWorkflow {
  readonly filename: string;
  readonly workflow: Workflow;
  readonly jobs: readonly JobInfo[];
}

/**
 * Parses all workflows in a directory.
 *
 * @param workflowsDir - Path to .github/workflows directory
 * @returns Array of parsed workflows
 */
export const parseWorkflowsFromDir = async (
  workflowsDir: string
): Promise<readonly ParsedWorkflow[]> => {
  const filenames = await discoverWorkflows(workflowsDir);
  const results: ParsedWorkflow[] = [];

  for (const filename of filenames) {
    try {
      const content = await readFile(join(workflowsDir, filename), "utf-8");
      const workflow = parseWorkflow(content);
      const jobs = extractJobInfo(workflow);
      results.push({ filename, workflow, jobs });
    } catch {
      // Skip workflows that fail to parse
    }
  }

  return results;
};

/**
 * Gets a specific job from a workflow.
 *
 * @param workflow - Parsed workflow
 * @param jobId - Job ID to find
 * @returns The job if found
 */
export const getJob = (workflow: Workflow, jobId: string): Job | undefined => {
  return workflow.jobs[jobId];
};
