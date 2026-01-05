import { Spinner } from "@inkjs/ui";
import { Box, Text, useApp, useInput } from "ink";
import { useEffect, useState } from "react";
import { formatErrorForTUI } from "../utils/error.js";
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
      }
    });

    return unsubscribe;
  }, [onEvent, exit]);

  const handleManifest = (event: ManifestEvent): void => {
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
    <Box marginTop={1}>
      <Text color={colors.muted}>$ act · {elapsed}s</Text>
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
    <Box marginTop={1}>
      <Text color={colors.muted}>$ act · {elapsed}s</Text>
    </Box>
    <Box flexDirection="column" marginTop={1}>
      {jobs.map((job) => (
        <Box key={job.id} marginLeft={2}>
          {renderJobLine(job, currentStepName)}
        </Box>
      ))}
    </Box>
  </Box>
);

/**
 * Renders a single job line
 */
const renderJobLine = (
  job: TrackedJob,
  currentStepName: string
): JSX.Element => {
  const icon = getJobIcon(job.status, job.isReusable);
  const color = getJobColor(job.status);

  if (job.status === "running") {
    // Show spinner with current step name
    if (job.currentStep >= 0 && job.currentStep < job.steps.length) {
      return (
        <Box>
          <Text color={color}>{icon} </Text>
          <Spinner label={currentStepName} />
        </Box>
      );
    }
    return (
      <Box>
        <Text color={color}>{icon} </Text>
        <Spinner label={job.name} />
      </Box>
    );
  }

  return (
    <Text color={color}>
      {icon} {job.name}
    </Text>
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
  const durationSec = doneInfo
    ? (doneInfo.duration / 1000).toFixed(1)
    : elapsed.toFixed(1);
  const hasErrors = doneInfo ? doneInfo.errorCount > 0 : false;
  const workflowFailed = doneInfo ? doneInfo.exitCode !== 0 : false;
  const hasSecuritySkipped = jobs.some(
    (job) => job.status === "skipped_security"
  );

  return (
    <Box flexDirection="column">
      <Box marginTop={1}>
        <Text color={colors.muted}>$ act · {durationSec}s</Text>
      </Box>
      <Box flexDirection="column" marginTop={1}>
        {jobs.map((job) => (
          <Box flexDirection="column" key={job.id}>
            <Box marginLeft={2}>
              <Text color={getJobColor(job.status)}>
                {getJobIcon(job.status, job.isReusable)} {job.name}
              </Text>
            </Box>
            {/* Expand steps only for failed jobs */}
            {job.status === "failed" &&
              job.steps.length > 0 &&
              !job.isReusable && (
                <Box flexDirection="column" marginLeft={4}>
                  {job.steps.map((step) => (
                    <Box key={step.index}>
                      <Text color={getStepColor(step.status)}>
                        {getStepIcon(step.status)} {step.name}
                      </Text>
                    </Box>
                  ))}
                </Box>
              )}
          </Box>
        ))}
      </Box>
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
                ✗ Check failed in {durationSec}s
              </Text>
            );
          }
          return (
            <Text bold color={colors.brand}>
              ✓ Check passed in {durationSec}s
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
      return "⏭";
    case "skipped_security":
      return "🔒";
  }
};

/**
 * Gets the color for a job status
 */
const getJobColor = (status: JobStatus): string => {
  switch (status) {
    case "pending":
      return colors.muted;
    case "running":
      return colors.text;
    case "success":
      return colors.brand;
    case "failed":
      return colors.error;
    case "skipped":
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
      return "⏭";
    case "cancelled":
      return "·";
    default:
      return "·";
  }
};

/**
 * Gets the color for a step status
 */
const getStepColor = (status: string): string => {
  switch (status) {
    case "pending":
      return colors.muted;
    case "running":
      return colors.text;
    case "success":
      return colors.brand;
    case "failed":
      return colors.error;
    case "skipped":
    case "cancelled":
      return colors.muted;
    default:
      return colors.muted;
  }
};
