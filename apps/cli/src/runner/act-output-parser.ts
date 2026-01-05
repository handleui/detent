import type {
  JobEvent,
  ManifestEvent,
  StepEvent,
  TUIEvent,
} from "../tui/check-tui-types.js";
import type { ManifestInfo, ManifestJob } from "./workflow-injector.js";

// Regex patterns for detent markers (compiled once at module level)
const MANIFEST_PATTERN = /::detent::manifest::v2::b64::([A-Za-z0-9+/=]+)/;
const JOB_START_PATTERN = /::detent::job-start::([a-zA-Z_][a-zA-Z0-9_-]*)/;
const JOB_END_PATTERN =
  /::detent::job-end::([a-zA-Z_][a-zA-Z0-9_-]*)::(success|failure|cancelled)/;
const STEP_START_PATTERN =
  /::detent::step-start::([a-zA-Z_][a-zA-Z0-9_-]*)::(\d+)::(.+)/;

/**
 * Validates a single job object from the manifest
 */
const isValidManifestJob = (job: unknown): job is ManifestJob => {
  if (!job || typeof job !== "object") {
    return false;
  }
  const record = job as Record<string, unknown>;
  return (
    typeof record.id === "string" &&
    typeof record.name === "string" &&
    typeof record.sensitive === "boolean" &&
    Array.isArray(record.steps)
  );
};

/**
 * Validates and parses manifest structure
 */
const parseManifestPayload = (base64Data: string): ManifestInfo | undefined => {
  try {
    const json = Buffer.from(base64Data, "base64").toString("utf-8");
    const parsed: unknown = JSON.parse(json);

    if (!parsed || typeof parsed !== "object") {
      return undefined;
    }
    const manifest = parsed as Record<string, unknown>;
    if (manifest.v !== 2 || !Array.isArray(manifest.jobs)) {
      return undefined;
    }
    for (const job of manifest.jobs) {
      if (!isValidManifestJob(job)) {
        return undefined;
      }
    }
    return parsed as ManifestInfo;
  } catch {
    return undefined;
  }
};

/**
 * Creates a ManifestEvent from a valid ManifestInfo
 */
const createManifestEvent = (manifest: ManifestInfo): ManifestEvent => ({
  type: "manifest",
  jobs: manifest.jobs.map((job) => ({
    id: job.id,
    name: job.name,
    uses: job.uses,
    sensitive: job.sensitive,
    steps: job.steps,
    needs: job.needs,
  })),
});

/**
 * Creates a JobEvent for job start
 */
const createJobStartEvent = (jobId: string): JobEvent => ({
  type: "job",
  jobId,
  action: "start",
});

/**
 * Creates a JobEvent for job end
 */
const createJobEndEvent = (jobId: string, status: string): JobEvent => ({
  type: "job",
  jobId,
  action: "finish",
  success: status === "success",
});

/**
 * Creates a StepEvent if the step index is valid
 */
const createStepEvent = (
  jobId: string,
  stepIdxStr: string,
  stepName: string
): StepEvent | undefined => {
  const idx = Number.parseInt(stepIdxStr, 10);
  if (Number.isNaN(idx) || idx < 0 || idx > 10_000) {
    return undefined;
  }
  return {
    type: "step",
    jobId,
    stepIdx: idx,
    stepName,
  };
};

/**
 * Parses act output to extract TUI events from detent markers.
 *
 * The workflow injector adds echo statements like:
 * - echo '::detent::manifest::v2::b64::{base64}'
 * - echo '::detent::job-start::{jobId}'
 * - echo '::detent::step-start::{jobId}::{idx}::{name}'
 * - echo '::detent::job-end::{jobId}::{status}'
 *
 * This parser extracts these markers and converts them to TUI events.
 */
export class ActOutputParser {
  private manifestReceived = false;
  private manifest: ManifestInfo | undefined;

  /**
   * Parse a line of act output and extract detent marker events.
   * Returns empty array if line contains no detent markers.
   */
  parseLine(line: string): TUIEvent[] {
    if (!line.includes("::detent::")) {
      return [];
    }

    const cleanLine = this.stripAnsi(line);
    return this.parseCleanLine(cleanLine);
  }

  /**
   * Parses a cleaned line (no ANSI codes) for detent markers
   */
  private parseCleanLine(cleanLine: string): TUIEvent[] {
    const manifestMatch = cleanLine.match(MANIFEST_PATTERN);
    if (manifestMatch?.[1]) {
      return this.handleManifestMatch(manifestMatch[1]);
    }

    const jobStartMatch = cleanLine.match(JOB_START_PATTERN);
    if (jobStartMatch?.[1]) {
      return [createJobStartEvent(jobStartMatch[1])];
    }

    const jobEndMatch = cleanLine.match(JOB_END_PATTERN);
    if (jobEndMatch?.[1] && jobEndMatch[2]) {
      return [createJobEndEvent(jobEndMatch[1], jobEndMatch[2])];
    }

    const stepMatch = cleanLine.match(STEP_START_PATTERN);
    if (stepMatch?.[1] && stepMatch[2] && stepMatch[3]) {
      const stepEvent = createStepEvent(
        stepMatch[1],
        stepMatch[2],
        stepMatch[3]
      );
      return stepEvent ? [stepEvent] : [];
    }

    return [];
  }

  /**
   * Handles manifest marker match and updates internal state
   */
  private handleManifestMatch(base64Data: string): TUIEvent[] {
    const manifest = parseManifestPayload(base64Data);
    if (!manifest) {
      return [];
    }
    this.manifest = manifest;
    this.manifestReceived = true;
    return [createManifestEvent(manifest)];
  }

  /**
   * Check if manifest has been received
   */
  hasManifest(): boolean {
    return this.manifestReceived;
  }

  /**
   * Get the parsed manifest (if received)
   */
  getManifest(): ManifestInfo | undefined {
    return this.manifest;
  }

  /**
   * Strip ANSI escape codes from string
   */
  private stripAnsi(str: string): string {
    // biome-ignore lint/suspicious/noControlCharactersInRegex: ANSI codes require control characters
    return str.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, "");
  }
}
