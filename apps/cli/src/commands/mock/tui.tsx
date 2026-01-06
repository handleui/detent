import { Spinner } from "@inkjs/ui";
import { Box, Text, useApp, useInput } from "ink";
import { useEffect, useState } from "react";
import { colors } from "../../tui/styles.js";
import { useShimmer } from "../../tui/use-shimmer.js";
import { formatErrorForTUI } from "../../utils/error.js";
import { formatDuration, formatDurationMs } from "../../utils/format.js";
import type {
  DoneEvent,
  JobEvent,
  JobStatus,
  ManifestEvent,
  StepEvent,
  TrackedJob,
  TUIEvent,
} from "./types.js";

interface ShimmerTextProps {
  readonly text: string;
  readonly isLoading: boolean;
}

const ShimmerText = ({ text, isLoading }: ShimmerTextProps): JSX.Element => {
  const shimmerOutput = useShimmer({
    text,
    baseColor: colors.muted,
    isLoading,
  });
  return <Text>{shimmerOutput}</Text>;
};

// ─────────────────────────────────────────────────────────────────────────────
// Topological Sort & Depth Calculation
// ─────────────────────────────────────────────────────────────────────────────

type ManifestJob = ManifestEvent["jobs"][number];

/**
 * Calculates depth for each job based on dependency chain.
 * Depth = max(depths of dependencies) + 1, or 0 if no dependencies.
 */
const calculateJobDepths = (
  jobs: readonly { id: string; needs?: readonly string[] }[]
): ReadonlyMap<string, number> => {
  const depths = new Map<string, number>();
  const jobById = new Map(jobs.map((j) => [j.id, j]));

  const getDepth = (id: string, visited: Set<string>): number => {
    if (depths.has(id)) {
      return depths.get(id) ?? 0;
    }

    // Cycle detection - treat as root
    if (visited.has(id)) {
      return 0;
    }

    const job = jobById.get(id);
    if (!job?.needs || job.needs.length === 0) {
      depths.set(id, 0);
      return 0;
    }

    visited.add(id);

    let maxDepDepth = -1;
    for (const depId of job.needs) {
      const depDepth = getDepth(depId, visited);
      if (depDepth > maxDepDepth) {
        maxDepDepth = depDepth;
      }
    }

    const depth = maxDepDepth + 1;
    depths.set(id, depth);
    return depth;
  };

  for (const job of jobs) {
    getDepth(job.id, new Set());
  }

  return depths;
};

interface DependencyGraph {
  readonly inDegree: Map<string, number>;
  readonly dependents: Map<string, string[]>;
  readonly jobById: Map<string, ManifestJob>;
}

/**
 * Builds dependency graph structures for topological sort.
 */
const buildDependencyGraph = (
  jobs: readonly ManifestJob[]
): DependencyGraph => {
  const jobById = new Map(jobs.map((j) => [j.id, j]));
  const inDegree = new Map<string, number>();
  const dependents = new Map<string, string[]>();

  for (const job of jobs) {
    inDegree.set(job.id, 0);
  }

  for (const job of jobs) {
    for (const dep of job.needs ?? []) {
      if (jobById.has(dep)) {
        inDegree.set(job.id, (inDegree.get(job.id) ?? 0) + 1);
        const existing = dependents.get(dep) ?? [];
        existing.push(job.id);
        dependents.set(dep, existing);
      }
    }
  }

  return { inDegree, dependents, jobById };
};

/**
 * Sorts jobs topologically (dependencies first) with depth info.
 * Uses Kahn's algorithm with alphabetical tiebreaking.
 */
const sortJobsTopologically = (
  jobs: readonly ManifestJob[]
): readonly { job: ManifestJob; depth: number }[] => {
  const depths = calculateJobDepths(jobs);
  const { inDegree, dependents, jobById } = buildDependencyGraph(jobs);

  // Find root jobs (no dependencies)
  const queue = [...inDegree.entries()]
    .filter(([, degree]) => degree === 0)
    .map(([id]) => id)
    .sort();

  const result: { job: ManifestJob; depth: number }[] = [];

  while (queue.length > 0) {
    const current = queue.shift();
    if (!current) {
      break;
    }

    const job = jobById.get(current);
    if (job) {
      result.push({ job, depth: depths.get(current) ?? 0 });
    }

    for (const depId of dependents.get(current) ?? []) {
      const newDegree = (inDegree.get(depId) ?? 1) - 1;
      inDegree.set(depId, newDegree);
      if (newDegree === 0) {
        queue.push(depId);
        queue.sort();
      }
    }
  }

  // Handle cycles: add remaining jobs at end
  const addedSet = new Set(result.map((r) => r.job.id));
  for (const job of jobs) {
    if (!addedSet.has(job.id)) {
      result.push({ job, depth: depths.get(job.id) ?? 0 });
    }
  }

  return result;
};

/**
 * Builds the " ← dep1, dep2" suffix for dependency display.
 */
const buildDependencySuffix = (
  job: TrackedJob,
  jobsById: ReadonlyMap<string, TrackedJob>
): string => {
  if (!job.needs || job.needs.length === 0) {
    return "";
  }

  const depNames = job.needs
    .map((depId) => jobsById.get(depId)?.name ?? depId)
    .slice(0, 3);

  const ellipsis = job.needs.length > 3 ? "…" : "";
  return ` ← ${depNames.join(", ")}${ellipsis}`;
};

// ─────────────────────────────────────────────────────────────────────────────

interface JobLineProps {
  readonly job: TrackedJob;
  readonly currentStepName: string;
  readonly jobsById: ReadonlyMap<string, TrackedJob>;
}

const JobLine = ({
  job,
  currentStepName,
  jobsById,
}: JobLineProps): JSX.Element => {
  const icon = getJobIcon(job.status, job.isReusable);
  const iconColor = getJobIconColor(job.status, job.isReusable);
  const textColor = getJobTextColor(job.status);

  // Indentation based on dependency depth (2 spaces per level)
  const indent = "  ".repeat(job.depth);
  const depsSuffix = buildDependencySuffix(job, jobsById);

  // For running jobs: show job name + current step
  if (job.status === "running") {
    const hasStep = job.currentStep >= 0 && job.currentStep < job.steps.length;

    return (
      <Box flexDirection="column">
        <Box>
          <Text>{indent}</Text>
          <Text color={iconColor}>{icon} </Text>
          <ShimmerText isLoading={true} text={job.name} />
          {depsSuffix && <Text color={colors.muted}>{depsSuffix}</Text>}
          {hasStep && (
            <>
              <Text color={colors.muted}> › </Text>
              <Text color={colors.muted}>{currentStepName}</Text>
            </>
          )}
        </Box>
      </Box>
    );
  }

  // For skipped_security jobs: show with (skipped: unsafe) suffix
  if (job.status === "skipped_security") {
    return (
      <Box>
        <Text>{indent}</Text>
        <Text color={iconColor}>{icon} </Text>
        <Text color={textColor}>{job.name}</Text>
        {depsSuffix && <Text color={colors.muted}>{depsSuffix}</Text>}
        <Text color={colors.muted}> (skipped: unsafe)</Text>
      </Box>
    );
  }

  // Default: show job name with dependencies
  return (
    <Box>
      <Text>{indent}</Text>
      <Text color={iconColor}>{icon} </Text>
      <Text color={textColor}>{job.name}</Text>
      {depsSuffix && <Text color={colors.muted}>{depsSuffix}</Text>}
    </Box>
  );
};

interface MockTUIProps {
  /**
   * Event stream from the runner
   */
  readonly onEvent: (callback: (event: TUIEvent) => void) => () => void;

  /**
   * Called when user cancels (Ctrl+C)
   */
  readonly onCancel?: () => void;
}

/**
 * Finalizes step statuses when a job completes
 */
const finalizeJobSteps = (job: TrackedJob, success: boolean): void => {
  for (const step of job.steps) {
    if (step.status === "running") {
      step.status = success ? "success" : "failed";
    } else if (step.status === "pending") {
      step.status = success ? "success" : "cancelled";
    }
  }
};

/**
 * Updates a running job to its final state
 */
const finalizeRunningJob = (job: TrackedJob, hasErrors: boolean): void => {
  finalizeJobSteps(job, !hasErrors);
  job.status = hasErrors ? "failed" : "success";
};

/**
 * Updates a pending job to cancelled state
 */
const finalizePendingJob = (job: TrackedJob): void => {
  for (const step of job.steps) {
    step.status = "cancelled";
  }
  job.status = job.isSensitive ? "skipped_security" : "failed";
};

/**
 * Updates all jobs to their final state when workflow completes
 */
const finalizeAllJobs = (
  jobs: TrackedJob[],
  hasErrors: boolean
): TrackedJob[] => {
  const newJobs = [...jobs];
  for (const job of newJobs) {
    if (job.status === "running") {
      finalizeRunningJob(job, hasErrors);
    } else if (job.status === "pending") {
      finalizePendingJob(job);
    }
  }
  return newJobs;
};

/**
 * Marks a job as started
 */
const startJob = (job: TrackedJob): void => {
  job.status = "running";
};

/**
 * Marks a job as finished with success/failure
 */
const finishJob = (job: TrackedJob, success: boolean): void => {
  finalizeJobSteps(job, success);
  job.status = success ? "success" : "failed";
};

/**
 * Marks a job and all its steps as skipped
 */
const skipJob = (job: TrackedJob): void => {
  for (const step of job.steps) {
    step.status = "skipped";
  }
  job.status = job.isSensitive ? "skipped_security" : "skipped";
};

/**
 * Applies a job action to update job state
 */
const applyJobAction = (
  job: TrackedJob,
  action: "start" | "finish" | "skip",
  success?: boolean
): void => {
  switch (action) {
    case "start":
      startJob(job);
      break;
    case "finish":
      finishJob(job, success ?? false);
      break;
    case "skip":
      skipJob(job);
      break;
    default:
      break;
  }
};

/**
 * Updates jobs array based on a job event
 */
const updateJobsForEvent = (
  prevJobs: TrackedJob[],
  event: JobEvent
): TrackedJob[] => {
  const newJobs = [...prevJobs];
  const job = newJobs.find((j) => j.id === event.jobId);
  if (!job) {
    return prevJobs;
  }
  applyJobAction(job, event.action, event.success);
  return newJobs;
};

/**
 * Updates jobs array based on a step event
 */
const updateJobsForStepEvent = (
  prevJobs: TrackedJob[],
  event: StepEvent
): TrackedJob[] => {
  const newJobs = [...prevJobs];
  const job = newJobs.find((j) => j.id === event.jobId);
  if (!job || event.stepIdx < 0 || event.stepIdx >= job.steps.length) {
    return prevJobs;
  }

  if (job.currentStep >= 0 && job.currentStep < job.steps.length) {
    const prevStep = job.steps[job.currentStep];
    if (prevStep?.status === "running") {
      prevStep.status = "success";
    }
  }

  job.currentStep = event.stepIdx;
  const step = job.steps[event.stepIdx];
  if (step) {
    step.status = "running";
  }

  return newJobs;
};

/**
 * Main TUI component for the mock command
 * Replicates Go CLI TUI behavior with real-time job/step tracking
 */
export const MockTUI = ({ onEvent, onCancel }: MockTUIProps): JSX.Element => {
  const { exit } = useApp();
  const [jobs, setJobs] = useState<TrackedJob[]>([]);
  const [waiting, setWaiting] = useState(true);
  const [currentStepName, setCurrentStepName] = useState<string>(
    "Waiting for workflow"
  );
  const [elapsed, setElapsed] = useState(0);
  const [done, setDone] = useState(false);
  const [doneInfo, setDoneInfo] = useState<DoneEvent | undefined>();
  const [errorMessage, setErrorMessage] = useState<string | undefined>();

  // Track if we're cancelling to prevent duplicate abort calls
  const [cancelling, setCancelling] = useState(false);

  // Handle Ctrl+C cancellation
  // IMPORTANT: We do NOT call exit() here. Instead, we call onCancel() which
  // aborts the runner. The runner will emit a "done" event after cleanup,
  // and handleDone() will call exit(). This ensures proper cleanup order.
  useInput((input, key) => {
    if (key.ctrl && input === "c" && !cancelling) {
      setCancelling(true);
      if (onCancel) {
        onCancel();
      }
      // Do NOT call exit() - wait for the "done" event from the runner
    }
  });

  // Timer for elapsed time
  useEffect(() => {
    const timer = setInterval(() => {
      setElapsed((prev) => prev + 1);
    }, 1000);

    return () => {
      clearInterval(timer);
    };
  }, []);

  // Subscribe to events
  useEffect(() => {
    const processManifest = (event: ManifestEvent): void => {
      // CRITICAL: Ignore duplicate manifests from retries to prevent TUI restart
      // When act retries (exit code != 0), it emits a new manifest which would
      // reset all job state. We only process the first manifest received.
      if (!waiting) {
        return;
      }

      // Sort jobs topologically with depth calculation
      const sortedJobs = sortJobsTopologically(event.jobs);

      const trackedJobs: TrackedJob[] = sortedJobs.map(({ job, depth }) => ({
        id: job.id,
        name: job.name,
        // Immediately mark sensitive jobs as skipped
        status: job.sensitive ? "skipped_security" : "pending",
        isReusable: Boolean(job.uses),
        isSensitive: job.sensitive,
        steps: job.steps.map((stepName, index) => ({
          index,
          name: stepName,
          status: job.sensitive ? "skipped" : "pending",
        })),
        currentStep: -1,
        needs: job.needs,
        depth,
      }));

      setJobs(trackedJobs);
      setWaiting(false);

      const firstJob = trackedJobs[0];
      if (firstJob) {
        setCurrentStepName(firstJob.name);
      }
    };

    const processJobEvent = (event: JobEvent): void => {
      setJobs((prevJobs) => {
        const newJobs = updateJobsForEvent(prevJobs, event);
        if (newJobs !== prevJobs && event.action === "start") {
          const job = newJobs.find((j) => j.id === event.jobId);
          if (job) {
            setCurrentStepName(job.name);
          }
        }
        return newJobs;
      });
    };

    const processStepEvent = (event: StepEvent): void => {
      setJobs((prevJobs) => {
        const newJobs = updateJobsForStepEvent(prevJobs, event);
        if (newJobs !== prevJobs) {
          setCurrentStepName(event.stepName);
        }
        return newJobs;
      });
    };

    const processDone = (event: DoneEvent): void => {
      setDone(true);
      setDoneInfo(event);
      setJobs((prevJobs) => finalizeAllJobs(prevJobs, event.errorCount > 0));

      setTimeout(() => {
        exit();
      }, 100);
    };

    const unsubscribe = onEvent((event) => {
      switch (event.type) {
        case "manifest":
          processManifest(event);
          break;
        case "job":
          processJobEvent(event);
          break;
        case "step":
          processStepEvent(event);
          break;
        case "done":
          processDone(event);
          break;
        case "error":
          setErrorMessage(event.message);
          setDone(true);
          setTimeout(() => {
            exit();
          }, 100);
          break;
        default:
          break;
      }
    });

    return unsubscribe;
  }, [onEvent, exit, waiting]);

  if (done) {
    return renderCompletionView(jobs, doneInfo, elapsed, errorMessage);
  }

  if (waiting) {
    return renderWaitingView(elapsed);
  }

  return renderRunningView(jobs, currentStepName, elapsed);
};

/**
 * Renders the waiting state before manifest arrives
 */
const renderWaitingView = (elapsed: number): JSX.Element => (
  <Box flexDirection="column">
    <Box>
      <Text color={colors.muted}>$ act · {formatDuration(elapsed)}</Text>
    </Box>
    <Box marginLeft={2} marginTop={1}>
      <Spinner label="Waiting for workflow" />
    </Box>
    {elapsed > 5 && (
      <Box marginLeft={2} marginTop={1}>
        <Text color={colors.muted}>This may take a moment on first run.</Text>
      </Box>
    )}
  </Box>
);

/**
 * Creates a Map for O(1) job lookups by ID
 */
const createJobsMap = (
  jobs: readonly TrackedJob[]
): ReadonlyMap<string, TrackedJob> => new Map(jobs.map((j) => [j.id, j]));

/**
 * Renders the running state with job list
 */
const renderRunningView = (
  jobs: readonly TrackedJob[],
  currentStepName: string,
  elapsed: number
): JSX.Element => {
  const jobsById = createJobsMap(jobs);
  return (
    <Box flexDirection="column">
      <Box>
        <Text color={colors.muted}>$ act · {formatDuration(elapsed)}</Text>
      </Box>
      <Box flexDirection="column" marginTop={1}>
        {jobs.map((job) => (
          <Box key={job.id} marginLeft={2}>
            <JobLine
              currentStepName={currentStepName}
              job={job}
              jobsById={jobsById}
            />
          </Box>
        ))}
      </Box>
    </Box>
  );
};

/**
 * Renders the completion view with final job statuses
 */
const renderCompletionView = (
  jobs: readonly TrackedJob[],
  doneInfo: DoneEvent | undefined,
  elapsed: number,
  errorMessage?: string
): JSX.Element => {
  const durationStr = doneInfo
    ? formatDurationMs(doneInfo.duration)
    : formatDuration(elapsed);
  const hasErrors = doneInfo ? doneInfo.errorCount > 0 : false;
  const workflowFailed = doneInfo ? doneInfo.exitCode !== 0 : false;
  const structuredErrors = doneInfo?.errors ?? [];
  const totalIssues = structuredErrors.length;
  const jobsById = createJobsMap(jobs);

  return (
    <Box flexDirection="column">
      <Box>
        <Text color={colors.muted}>$ act · {durationStr}</Text>
      </Box>
      <Box flexDirection="column" marginTop={1}>
        {jobs.map((job) => {
          const indent = "  ".repeat(job.depth);
          const depsSuffix = buildDependencySuffix(job, jobsById);

          return (
            <Box flexDirection="column" key={job.id}>
              <Box marginLeft={2}>
                <Text>{indent}</Text>
                <Text color={getJobIconColor(job.status, job.isReusable)}>
                  {getJobIcon(job.status, job.isReusable)}{" "}
                </Text>
                <Text color={getJobTextColor(job.status)}>{job.name}</Text>
                {depsSuffix && <Text color={colors.muted}>{depsSuffix}</Text>}
                {job.status === "skipped_security" && (
                  <Text color={colors.muted}> (skipped: unsafe)</Text>
                )}
              </Box>
              {/* Expand steps only for failed jobs */}
              {job.status === "failed" &&
                job.steps.length > 0 &&
                !job.isReusable && (
                  <Box flexDirection="column" marginLeft={4 + job.depth * 2}>
                    {job.steps.map((step) => (
                      <Box key={step.index}>
                        <Text color={getStepIconColor(step.status)}>
                          {getStepIcon(step.status)}{" "}
                        </Text>
                        <Text color={getStepTextColor(step.status)}>
                          {step.name}
                        </Text>
                      </Box>
                    ))}
                  </Box>
                )}
            </Box>
          );
        })}
      </Box>
      {structuredErrors.length > 0 && renderErrorsView(structuredErrors)}
      <Box marginTop={1}>
        {(() => {
          if (errorMessage) {
            return (
              <Text bold color={colors.error}>
                ✗ Mock failed: {formatErrorForTUI(errorMessage)}
              </Text>
            );
          }
          if (totalIssues > 0) {
            const issueText = totalIssues === 1 ? "issue" : "issues";
            return (
              <Text bold color={colors.brand}>
                ✓ Found {totalIssues} {issueText} in {durationStr}
              </Text>
            );
          }
          if (hasErrors || workflowFailed) {
            return (
              <Text bold color={colors.error}>
                ✗ Mock failed in {durationStr}
              </Text>
            );
          }
          return (
            <Text bold color={colors.brand}>
              ✓ Mock passed in {durationStr}
            </Text>
          );
        })()}
      </Box>
    </Box>
  );
};

/**
 * Gets the icon for a job status
 */
const getJobIcon = (status: JobStatus, isReusable: boolean): string => {
  if (isReusable) {
    switch (status) {
      case "pending":
      case "running":
        return "⟲";
      case "success":
        return "⟲";
      case "failed":
        return "⟲";
      case "skipped":
      case "skipped_security":
        return "⟲";
      default:
        return "⟲";
    }
  }

  switch (status) {
    case "pending":
      return "·";
    case "running":
      return "·";
    case "success":
      return "✓";
    case "failed":
      return "✗";
    case "skipped":
      return "—";
    case "skipped_security":
      return "⊘";
    default:
      return "·";
  }
};

/**
 * Gets the color for a job's ICON
 * - Grey dot: pending
 * - Green dot: running
 * - Green check: success
 * - Red X: failed
 * - Grey dash: skipped
 */
const getJobIconColor = (status: JobStatus, isReusable: boolean): string => {
  if (isReusable) {
    return colors.muted; // Reusable workflow icons always grey
  }

  switch (status) {
    case "pending":
      return colors.muted;
    case "running":
      return colors.brand;
    case "success":
      return colors.brand; // Green checkmark
    case "failed":
      return colors.error; // Red X
    case "skipped":
      return colors.muted;
    case "skipped_security":
      return colors.muted;
    default:
      return colors.muted;
  }
};

/**
 * Gets the color for a job's TEXT (name)
 * - Grey: pending, skipped
 * - Green: running
 * - White: finished (success, failed)
 */
const getJobTextColor = (status: JobStatus): string => {
  switch (status) {
    case "pending":
      return colors.muted;
    case "running":
      return colors.brand;
    case "success":
    case "failed":
    case "skipped":
      return colors.text;
    case "skipped_security":
      return colors.muted;
    default:
      return colors.muted;
  }
};

/**
 * Gets the icon for a step status
 */
const getStepIcon = (status: string): string => {
  switch (status) {
    case "pending":
      return "·";
    case "running":
      return "·";
    case "success":
      return "✓";
    case "failed":
      return "✗";
    case "skipped":
      return "—";
    case "cancelled":
      return "—";
    default:
      return "·";
  }
};

/**
 * Gets the color for a step's ICON
 * - Grey dot: pending, cancelled
 * - Green dot: running
 * - Green check: success
 * - Red X: failed
 * - Grey dash: skipped
 */
const getStepIconColor = (status: string): string => {
  switch (status) {
    case "pending":
    case "cancelled":
      return colors.muted;
    case "running":
      return colors.brand;
    case "success":
      return colors.brand; // Green checkmark
    case "failed":
      return colors.error; // Red X
    case "skipped":
      return colors.muted;
    default:
      return colors.muted;
  }
};

/**
 * Gets the color for a step's TEXT (name)
 * - Grey: pending, cancelled, skipped
 * - Green: running
 * - White: success, failed
 */
const getStepTextColor = (status: string): string => {
  switch (status) {
    case "pending":
    case "cancelled":
    case "skipped":
      return colors.muted;
    case "running":
      return colors.brand;
    case "success":
    case "failed":
      return colors.text;
    default:
      return colors.muted;
  }
};

interface ErrorsByCategory {
  readonly category: string;
  readonly fileGroups: readonly FileErrorGroup[];
}

interface FileErrorGroup {
  readonly file: string;
  readonly errors: readonly DisplayError[];
  readonly errorCount: number;
  readonly warningCount: number;
}

/**
 * Category display order matching Go CLI format
 */
const CATEGORY_ORDER = [
  "Lint Issues",
  "Type Errors",
  "Test Failures",
  "Build Errors",
  "Runtime Errors",
  "Other",
] as const;

/**
 * Gets the sort index for a category
 */
const getCategoryOrder = (category: string): number => {
  const index = CATEGORY_ORDER.indexOf(
    category as (typeof CATEGORY_ORDER)[number]
  );
  return index === -1 ? CATEGORY_ORDER.length : index;
};

/**
 * Sorts errors by line and column for consistent display
 */
const sortErrorsByLocation = (errors: DisplayError[]): DisplayError[] =>
  [...errors].sort((a, b) => {
    const lineA = a.line ?? 0;
    const lineB = b.line ?? 0;
    if (lineA !== lineB) {
      return lineA - lineB;
    }
    return (a.column ?? 0) - (b.column ?? 0);
  });

/**
 * Builds a FileErrorGroup from file errors
 */
const buildFileGroup = (
  file: string,
  errors: DisplayError[]
): FileErrorGroup => {
  const sortedErrors = sortErrorsByLocation(errors);
  let errorCount = 0;
  let warningCount = 0;
  for (const e of sortedErrors) {
    if (e.severity === "error") {
      errorCount++;
    } else if (e.severity === "warning") {
      warningCount++;
    }
  }
  return { file, errors: sortedErrors, errorCount, warningCount };
};

/**
 * Groups errors by category and file for structured display
 */
const groupErrors = (
  errors: readonly DisplayError[]
): readonly ErrorsByCategory[] => {
  const categoryMap = new Map<string, Map<string, DisplayError[]>>();

  for (const error of errors) {
    const category = error.category ?? "Other";
    const file = error.file ?? "unknown";

    if (!categoryMap.has(category)) {
      categoryMap.set(category, new Map());
    }
    const fileMap = categoryMap.get(category);
    if (fileMap && !fileMap.has(file)) {
      fileMap.set(file, []);
    }
    fileMap?.get(file)?.push(error);
  }

  const result: ErrorsByCategory[] = [];
  for (const [category, fileMap] of categoryMap) {
    const fileGroups: FileErrorGroup[] = [];
    for (const [file, fileErrors] of fileMap) {
      fileGroups.push(buildFileGroup(file, fileErrors));
    }
    result.push({ category, fileGroups });
  }

  result.sort(
    (a, b) => getCategoryOrder(a.category) - getCategoryOrder(b.category)
  );

  return result;
};

/**
 * Separator line matching Go CLI format (56 box-drawing characters)
 */
const SEPARATOR_LINE = "─".repeat(56);

/**
 * Renders the summary header with problem counts
 */
const ErrorSummary = ({
  totalErrors,
  totalWarnings,
  uniqueFiles,
}: {
  readonly totalErrors: number;
  readonly totalWarnings: number;
  readonly uniqueFiles: number;
}): JSX.Element => {
  const totalProblems = totalErrors + totalWarnings;
  const problemText = totalProblems === 1 ? "problem" : "problems";
  const fileText = uniqueFiles === 1 ? "file" : "files";
  const errorText = totalErrors === 1 ? "error" : "errors";
  const warningText = totalWarnings === 1 ? "warning" : "warnings";

  return (
    <Box flexDirection="column">
      <Box>
        <Text color={colors.error}>{">"} ✖ </Text>
        <Text>
          Found {totalProblems} {problemText} ({totalErrors} {errorText},{" "}
          {totalWarnings} {warningText}) across {uniqueFiles} {fileText}
        </Text>
      </Box>
      <Box marginLeft={2}>
        <Text color={colors.muted}>
          Run 'detent heal' to auto-fix or fix manually and re-run
        </Text>
      </Box>
    </Box>
  );
};

/**
 * Renders a single error line in format: line:col: symbol message [rule-id]
 */
const ErrorLine = ({
  error,
}: {
  readonly error: DisplayError;
}): JSX.Element => {
  const location = `${error.line ?? 0}:${error.column ?? 0}:`;
  const isError = error.severity === "error";

  return (
    <Box marginLeft={4}>
      <Text color={colors.muted}>{location} </Text>
      <Text color={isError ? colors.error : colors.warn}>
        {isError ? "✖" : "⚠"}{" "}
      </Text>
      <Text>{error.message}</Text>
      {error.ruleId && <Text color={colors.muted}> [{error.ruleId}]</Text>}
    </Box>
  );
};

/**
 * Renders a file group header with error/warning counts
 */
const FileHeader = ({
  file,
  errorCount,
  warningCount,
}: {
  readonly file: string;
  readonly errorCount: number;
  readonly warningCount: number;
}): JSX.Element => {
  const errorText = errorCount === 1 ? "error" : "errors";
  const warningText = warningCount === 1 ? "warning" : "warnings";

  return (
    <Box marginLeft={2} marginTop={1}>
      <Text>{file} </Text>
      <Text color={colors.muted}>
        ({errorCount} {errorText}, {warningCount} {warningText})
      </Text>
    </Box>
  );
};

/**
 * Renders a file group with its header and error lines
 */
const FileGroup = ({
  fileGroup,
}: {
  readonly fileGroup: FileErrorGroup;
}): JSX.Element => (
  <Box flexDirection="column" key={fileGroup.file}>
    <FileHeader
      errorCount={fileGroup.errorCount}
      file={fileGroup.file}
      warningCount={fileGroup.warningCount}
    />
    {fileGroup.errors.map((error) => (
      <ErrorLine
        error={error}
        key={`${error.line ?? 0}:${error.column ?? 0}:${error.message.slice(0, 50)}`}
      />
    ))}
  </Box>
);

/**
 * Renders a category section with its file groups
 */
const CategorySection = ({
  categoryGroup,
}: {
  readonly categoryGroup: ErrorsByCategory;
}): JSX.Element => (
  <Box flexDirection="column" key={categoryGroup.category} marginTop={1}>
    <Text bold>{categoryGroup.category}:</Text>
    {categoryGroup.fileGroups.map((fileGroup) => (
      <FileGroup fileGroup={fileGroup} key={fileGroup.file} />
    ))}
  </Box>
);

/**
 * Computes error statistics in a single pass
 */
const computeErrorStats = (
  errors: readonly DisplayError[]
): { totalErrors: number; totalWarnings: number; uniqueFiles: number } => {
  let totalErrors = 0;
  let totalWarnings = 0;
  const uniqueFilesSet = new Set<string>();

  for (const e of errors) {
    if (e.severity === "error") {
      totalErrors++;
    } else if (e.severity === "warning") {
      totalWarnings++;
    }
    if (e.file) {
      uniqueFilesSet.add(e.file);
    }
  }

  return { totalErrors, totalWarnings, uniqueFiles: uniqueFilesSet.size };
};

/**
 * Renders the structured error display matching Go CLI format
 */
const renderErrorsView = (
  errors: readonly DisplayError[]
): JSX.Element | null => {
  if (errors.length === 0) {
    return null;
  }

  const { totalErrors, totalWarnings, uniqueFiles } = computeErrorStats(errors);

  const grouped = groupErrors(errors);

  return (
    <Box flexDirection="column" marginTop={1}>
      <ErrorSummary
        totalErrors={totalErrors}
        totalWarnings={totalWarnings}
        uniqueFiles={uniqueFiles}
      />
      <Box marginTop={1}>
        <Text color={colors.muted}>{SEPARATOR_LINE}</Text>
      </Box>
      {grouped.map((categoryGroup) => (
        <CategorySection
          categoryGroup={categoryGroup}
          key={categoryGroup.category}
        />
      ))}
    </Box>
  );
};

type DisplayError = import("./types.js").DisplayError;
