import { Spinner } from "@inkjs/ui";
import { Box, Text, useApp, useInput } from "ink";
import { useEffect, useState } from "react";
import { formatErrorForTUI } from "../utils/error.js";
import { formatDuration, formatDurationMs } from "../utils/format.js";
import type {
  DoneEvent,
  JobEvent,
  JobStatus,
  ManifestEvent,
  StepEvent,
  TrackedJob,
  TUIEvent,
} from "./check-tui-types.js";
import { colors } from "./styles.js";
import { useShimmer } from "./use-shimmer.js";

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

interface JobLineProps {
  readonly job: TrackedJob;
  readonly currentStepName: string;
  readonly allJobs: readonly TrackedJob[];
}

const JobLine = ({
  job,
  currentStepName,
  allJobs,
}: JobLineProps): JSX.Element => {
  const icon = getJobIcon(job.status, job.isReusable);
  const iconColor = getJobIconColor(job.status, job.isReusable);
  const textColor = getJobTextColor(job.status);

  // Check if this job has dependencies
  const hasDeps = job.needs && job.needs.length > 0;

  // For running jobs: show job name + current step
  if (job.status === "running") {
    const hasStep = job.currentStep >= 0 && job.currentStep < job.steps.length;

    return (
      <Box flexDirection="column">
        <Box>
          <Text color={iconColor}>{icon} </Text>
          <ShimmerText isLoading={true} text={job.name} />
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

  // For pending jobs with dependencies: show nested under deps
  if (job.status === "pending" && hasDeps) {
    // Find which dependencies are blocking (not yet successful)
    const blockingDeps =
      job.needs?.filter((depId) => {
        const depJob = allJobs.find((j) => j.id === depId);
        return depJob && depJob.status !== "success";
      }) ?? [];

    // Get display names for blocking deps
    const blockingNames = blockingDeps
      .map((depId) => {
        const depJob = allJobs.find((j) => j.id === depId);
        return depJob?.name ?? depId;
      })
      .slice(0, 2); // Limit to 2 for brevity

    const waitingText =
      blockingNames.length > 0
        ? `waiting for ${blockingNames.join(", ")}${blockingDeps.length > 2 ? "…" : ""}`
        : "";

    return (
      <Box>
        <Text color={iconColor}>{icon} </Text>
        <Text color={textColor}>{job.name}</Text>
        {waitingText && <Text color={colors.muted}> · {waitingText}</Text>}
      </Box>
    );
  }

  // Default: just show job name
  return (
    <Box>
      <Text color={iconColor}>{icon} </Text>
      <Text color={textColor}>{job.name}</Text>
    </Box>
  );
};

interface CheckTUIProps {
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
 * Main TUI component for the check command
 * Replicates Go CLI TUI behavior with real-time job/step tracking
 */
export const CheckTUI = ({ onEvent, onCancel }: CheckTUIProps): JSX.Element => {
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
  const [warnings, setWarnings] = useState<string[]>([]);

  // Handle Ctrl+C cancellation
  useInput((input, key) => {
    if (key.ctrl && input === "c") {
      if (onCancel) {
        onCancel();
      }
      exit();
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
    const unsubscribe = onEvent((event) => {
      switch (event.type) {
        case "manifest":
          handleManifest(event);
          break;
        case "job":
          handleJobEvent(event);
          break;
        case "step":
          handleStepEvent(event);
          break;
        case "done":
          handleDone(event);
          break;
        case "error":
          // Error handling - store message and exit TUI
          setErrorMessage(event.message);
          setDone(true);
          setTimeout(() => {
            exit();
          }, 100);
          break;
        case "warning":
          setWarnings((prev) => [...prev, event.message]);
          break;
      }
    });

    return unsubscribe;
  }, [onEvent, exit]);

  const handleManifest = (event: ManifestEvent): void => {
    // CRITICAL: Ignore duplicate manifests from retries to prevent TUI restart
    // When act retries (exit code != 0), it emits a new manifest which would
    // reset all job state. We only process the first manifest received.
    if (!waiting) {
      return;
    }

    const trackedJobs: TrackedJob[] = event.jobs.map((job) => ({
      id: job.id,
      name: job.name,
      status: "pending" as JobStatus,
      isReusable: Boolean(job.uses),
      isSensitive: job.sensitive,
      steps: job.steps.map((stepName, index) => ({
        index,
        name: stepName,
        status: "pending" as const,
      })),
      currentStep: -1,
      needs: job.needs,
    }));

    setJobs(trackedJobs);
    setWaiting(false);

    // Update current step name to first job
    const firstJob = trackedJobs[0];
    if (firstJob) {
      setCurrentStepName(firstJob.name);
    }
  };

  const handleJobEvent = (event: JobEvent): void => {
    setJobs((prevJobs) => {
      const newJobs = [...prevJobs];
      const job = newJobs.find((j) => j.id === event.jobId);
      if (!job) return prevJobs;

      switch (event.action) {
        case "start":
          job.status = "running";
          setCurrentStepName(job.name);
          break;
        case "finish":
          finalizeJobSteps(job, event.success ?? false);
          job.status = event.success ? "success" : "failed";
          break;
        case "skip":
          for (const step of job.steps) {
            step.status = "skipped";
          }
          job.status = job.isSensitive ? "skipped_security" : "skipped";
          break;
      }

      return newJobs;
    });
  };

  const handleStepEvent = (event: StepEvent): void => {
    setJobs((prevJobs) => {
      const newJobs = [...prevJobs];
      const job = newJobs.find((j) => j.id === event.jobId);
      if (!job || event.stepIdx < 0 || event.stepIdx >= job.steps.length) {
        return prevJobs;
      }

      // Mark previous running step as success
      if (job.currentStep >= 0 && job.currentStep < job.steps.length) {
        const prevStep = job.steps[job.currentStep];
        if (prevStep?.status === "running") {
          prevStep.status = "success";
        }
      }

      // Update current step
      job.currentStep = event.stepIdx;
      const step = job.steps[event.stepIdx];
      if (step) {
        step.status = "running";
      }

      setCurrentStepName(event.stepName);

      return newJobs;
    });
  };

  const handleDone = (event: DoneEvent): void => {
    setDone(true);
    setDoneInfo(event);

    // Mark all running jobs as complete
    setJobs((prevJobs) => {
      const newJobs = [...prevJobs];
      const hasErrors = event.errorCount > 0;

      for (const job of newJobs) {
        if (job.status === "running") {
          finalizeJobSteps(job, !hasErrors);
          job.status = hasErrors ? "failed" : "success";
        } else if (job.status === "pending") {
          for (const step of job.steps) {
            step.status = "cancelled";
          }
          job.status = job.isSensitive ? "skipped_security" : "failed";
        }
      }

      return newJobs;
    });

    // Exit after a brief delay to show final state
    setTimeout(() => {
      exit();
    }, 100);
  };

  const finalizeJobSteps = (job: TrackedJob, success: boolean): void => {
    for (const step of job.steps) {
      switch (step.status) {
        case "running":
          step.status = success ? "success" : "failed";
          break;
        case "pending":
          step.status = success ? "success" : "cancelled";
          break;
      }
    }
  };

  if (done) {
    return renderCompletionView(
      jobs,
      doneInfo,
      elapsed,
      errorMessage,
      warnings
    );
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
 * Renders the running state with job list
 */
const renderRunningView = (
  jobs: readonly TrackedJob[],
  currentStepName: string,
  elapsed: number
): JSX.Element => (
  <Box flexDirection="column">
    <Box>
      <Text color={colors.muted}>$ act · {formatDuration(elapsed)}</Text>
    </Box>
    <Box flexDirection="column" marginTop={1}>
      {jobs.map((job) => (
        <Box key={job.id} marginLeft={2}>
          <JobLine allJobs={jobs} currentStepName={currentStepName} job={job} />
        </Box>
      ))}
    </Box>
  </Box>
);

/**
 * Renders the completion view with final job statuses
 */
const renderCompletionView = (
  jobs: readonly TrackedJob[],
  doneInfo: DoneEvent | undefined,
  elapsed: number,
  errorMessage?: string,
  warnings: readonly string[] = []
): JSX.Element => {
  const durationStr = doneInfo
    ? formatDurationMs(doneInfo.duration)
    : formatDuration(elapsed);
  const hasErrors = doneInfo ? doneInfo.errorCount > 0 : false;
  const workflowFailed = doneInfo ? doneInfo.exitCode !== 0 : false;
  const hasSecuritySkipped = jobs.some(
    (job) => job.status === "skipped_security"
  );
  const structuredErrors = doneInfo?.errors ?? [];

  return (
    <Box flexDirection="column">
      <Box>
        <Text color={colors.muted}>$ act · {durationStr}</Text>
      </Box>
      <Box flexDirection="column" marginTop={1}>
        {jobs.map((job) => (
          <Box flexDirection="column" key={job.id}>
            <Box marginLeft={2}>
              <Text color={getJobIconColor(job.status, job.isReusable)}>
                {getJobIcon(job.status, job.isReusable)}{" "}
              </Text>
              <Text color={getJobTextColor(job.status)}>{job.name}</Text>
            </Box>
            {/* Expand steps only for failed jobs */}
            {job.status === "failed" &&
              job.steps.length > 0 &&
              !job.isReusable && (
                <Box flexDirection="column" marginLeft={4}>
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
        ))}
      </Box>
      {structuredErrors.length > 0 && renderErrorsView(structuredErrors)}
      <Box marginTop={1}>
        {(() => {
          if (errorMessage) {
            return (
              <Text bold color={colors.error}>
                ✗ Check failed: {formatErrorForTUI(errorMessage)}
              </Text>
            );
          }
          if (hasErrors || workflowFailed) {
            return (
              <Text bold color={colors.error}>
                ✗ Check failed in {durationStr}
              </Text>
            );
          }
          return (
            <Text bold color={colors.brand}>
              ✓ Check passed in {durationStr}
            </Text>
          );
        })()}
      </Box>
      {hasSecuritySkipped && (
        <Box marginTop={1}>
          <Text color={colors.muted} italic>
            Locked jobs skipped for safety. Manage with: detent workflows
          </Text>
        </Box>
      )}
      {warnings.length > 0 && (
        <Box flexDirection="column" marginTop={1}>
          {warnings.map((warning, idx) => (
            <Text color={colors.muted} key={idx}>
              i {warning}
            </Text>
          ))}
        </Box>
      )}
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
 * Groups errors by category and file for structured display
 */
const groupErrors = (
  errors: readonly DisplayError[]
): readonly ErrorsByCategory[] => {
  const categoryMap = new Map<string, Map<string, DisplayError[]>>();

  for (const error of errors) {
    const category = error.category ?? "Issues";
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
      const errorCount = fileErrors.filter(
        (e) => e.severity === "error"
      ).length;
      const warningCount = fileErrors.filter(
        (e) => e.severity === "warning"
      ).length;
      fileGroups.push({
        file,
        errors: fileErrors,
        errorCount,
        warningCount,
      });
    }
    result.push({ category, fileGroups });
  }

  return result;
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

  const totalErrors = errors.filter((e) => e.severity === "error").length;
  const totalWarnings = errors.filter((e) => e.severity === "warning").length;
  const uniqueFiles = new Set(errors.map((e) => e.file).filter(Boolean)).size;

  const grouped = groupErrors(errors);

  const problemText =
    totalErrors + totalWarnings === 1 ? "problem" : "problems";
  const fileText = uniqueFiles === 1 ? "file" : "files";
  const errorText = totalErrors === 1 ? "error" : "errors";
  const warningText = totalWarnings === 1 ? "warning" : "warnings";

  return (
    <Box flexDirection="column" marginTop={1}>
      <Box>
        <Text color={colors.error}>{">"} </Text>
        <Text color={colors.error}>✖ </Text>
        <Text>
          Found {totalErrors + totalWarnings} {problemText} ({totalErrors}{" "}
          {errorText}, {totalWarnings} {warningText}) across {uniqueFiles}{" "}
          {fileText}
        </Text>
      </Box>

      {grouped.map((categoryGroup) => (
        <Box flexDirection="column" key={categoryGroup.category} marginTop={1}>
          <Box>
            <Text bold>{categoryGroup.category}:</Text>
          </Box>

          {categoryGroup.fileGroups.map((fileGroup) => (
            <Box flexDirection="column" key={fileGroup.file} marginLeft={2}>
              <Box marginTop={1}>
                <Text color={colors.info}>{fileGroup.file}</Text>
                <Text color={colors.muted}>
                  {" "}
                  ({fileGroup.errorCount}{" "}
                  {fileGroup.errorCount === 1 ? "error" : "errors"},{" "}
                  {fileGroup.warningCount}{" "}
                  {fileGroup.warningCount === 1 ? "warning" : "warnings"})
                </Text>
              </Box>

              {fileGroup.errors.map((error, idx) => (
                <Box key={idx} marginLeft={2}>
                  <Text color={colors.muted}>
                    {error.line ?? 0}:{error.column ?? 0}{" "}
                  </Text>
                  {error.severity === "error" ? (
                    <Text color={colors.error}>✖ </Text>
                  ) : (
                    <Text color={colors.warn}>⚠ </Text>
                  )}
                  <Text>{error.message}</Text>
                  {error.ruleId && (
                    <Text color={colors.muted}> [{error.ruleId}]</Text>
                  )}
                </Box>
              ))}
            </Box>
          ))}
        </Box>
      ))}
    </Box>
  );
};

type DisplayError = import("./check-tui-types.js").DisplayError;
