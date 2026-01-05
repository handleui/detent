import type {
  JobEvent,
  ManifestEvent,
  StepEvent,
  TUIEvent,
} from "../tui/check-tui-types.js";
import type { ManifestInfo } from "./workflow-injector.js";

// Regex patterns for detent markers (compiled once at module level)
const MANIFEST_PATTERN = /::detent::manifest::v2::b64::([A-Za-z0-9+/=]+)/;
const JOB_START_PATTERN = /::detent::job-start::([a-zA-Z_][a-zA-Z0-9_-]*)/;
const JOB_END_PATTERN =
  /::detent::job-end::([a-zA-Z_][a-zA-Z0-9_-]*)::(success|failure|cancelled)/;
const STEP_START_PATTERN =
  /::detent::step-start::([a-zA-Z_][a-zA-Z0-9_-]*)::(\d+)::(.+)/;

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
    const events: TUIEvent[] = [];

    // Fast path: skip lines that don't contain detent markers (like Go CLI)
    if (!line.includes("::detent::")) {
      return events;
    }

    // Remove ANSI codes for cleaner parsing
    const cleanLine = this.stripAnsi(line);

    // Check for manifest marker
    const manifestMatch = cleanLine.match(MANIFEST_PATTERN);
    if (manifestMatch?.[1]) {
      try {
        const json = Buffer.from(manifestMatch[1], "base64").toString("utf-8");
        const parsed: unknown = JSON.parse(json);

        // Validate manifest structure (Go CLI uses "v" key, not "version")
        if (!parsed || typeof parsed !== "object") {
          throw new Error("Invalid manifest");
        }
        const manifest = parsed as Record<string, unknown>;
        if (manifest.v !== 2) {
          throw new Error("Unsupported manifest version");
        }
        if (!Array.isArray(manifest.jobs)) {
          throw new Error("Invalid manifest jobs");
        }
        // Validate each job has required fields
        for (const job of manifest.jobs) {
          if (
            !job ||
            typeof job !== "object" ||
            typeof (job as Record<string, unknown>).id !== "string" ||
            typeof (job as Record<string, unknown>).name !== "string" ||
            typeof (job as Record<string, unknown>).sensitive !== "boolean" ||
            !Array.isArray((job as Record<string, unknown>).steps)
          ) {
            throw new Error("Invalid job in manifest");
          }
        }

        this.manifest = parsed as ManifestInfo;
        this.manifestReceived = true;

        const manifestEvent: ManifestEvent = {
          type: "manifest",
          jobs: this.manifest.jobs.map((job) => ({
            id: job.id,
            name: job.name,
            uses: job.uses,
            sensitive: job.sensitive,
            steps: job.steps,
            needs: job.needs,
          })),
        };
        events.push(manifestEvent);
      } catch {
        // Invalid manifest - ignore
      }
      return events;
    }

    // Check for job-start marker
    const jobStartMatch = cleanLine.match(JOB_START_PATTERN);
    if (jobStartMatch?.[1]) {
      const jobEvent: JobEvent = {
        type: "job",
        jobId: jobStartMatch[1],
        action: "start",
      };
      events.push(jobEvent);
      return events;
    }

    // Check for job-end marker
    const jobEndMatch = cleanLine.match(JOB_END_PATTERN);
    if (jobEndMatch?.[1] && jobEndMatch[2]) {
      const status = jobEndMatch[2];
      const jobEvent: JobEvent = {
        type: "job",
        jobId: jobEndMatch[1],
        action: "finish",
        success: status === "success",
      };
      events.push(jobEvent);
      return events;
    }

    // Check for step-start marker
    const stepMatch = cleanLine.match(STEP_START_PATTERN);
    if (stepMatch?.[1] && stepMatch[2] && stepMatch[3]) {
      const idx = Number.parseInt(stepMatch[2], 10);
      // Validate stepIdx bounds to skip malformed events
      if (Number.isNaN(idx) || idx < 0 || idx > 10_000) {
        return [];
      }
      const stepEvent: StepEvent = {
        type: "step",
        jobId: stepMatch[1],
        stepIdx: idx,
        stepName: stepMatch[3],
      };
      events.push(stepEvent);
      return events;
    }

    return events;
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
