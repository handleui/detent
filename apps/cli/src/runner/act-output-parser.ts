import type { ManifestEvent, TUIEvent } from "../tui/check-tui-types.js";

/**
 * Parses act output to extract TUI events
 *
 * Act output format (examples):
 * - [Workflow/Job Name]
 * - [Job/Step Name]
 * - ✅ Success: Job Name
 * - ❌ Failure: Job Name
 */
export class ActOutputParser {
  private manifestEmitted = false;
  private readonly jobs = new Map<
    string,
    { name: string; steps: string[]; started: boolean }
  >();
  private currentJob: string | undefined;

  /**
   * Parse a line of act output and extract events
   */
  parseLine(line: string): TUIEvent[] {
    const events: TUIEvent[] = [];

    // Remove ANSI codes for easier parsing
    const cleanLine = this.stripAnsi(line);

    // Emit manifest once if we have jobs
    if (!this.manifestEmitted && this.jobs.size > 0) {
      events.push(this.createManifest());
      this.manifestEmitted = true;
    }

    // Parse different line patterns
    // Pattern: [Workflow/Job Name] or [Job Name]
    const workflowMatch = cleanLine.match(/\[(.+?)\]/);
    if (workflowMatch?.[1]) {
      const name = workflowMatch[1].trim();

      // Check if this is a new job
      if (!this.jobs.has(name)) {
        // Register new job
        this.jobs.set(name, { name, steps: [], started: false });

        // If manifest already emitted, we need to update it
        if (this.manifestEmitted) {
          events.push(this.createManifest());
        }
      }

      // Check if job is starting
      if (cleanLine.includes("Starting") || cleanLine.includes("Running")) {
        const job = this.jobs.get(name);
        if (job && !job.started) {
          job.started = true;
          this.currentJob = name;
          events.push({
            type: "job",
            jobId: name,
            action: "start",
          });
        }
      }
    }

    // Pattern: Success/Failure indicators
    if (cleanLine.includes("✅") || cleanLine.includes("Success")) {
      const match = cleanLine.match(/(?:✅|Success).*?:\s*(.+)/);
      if (match?.[1]) {
        const jobName = match[1].trim();
        events.push({
          type: "job",
          jobId: jobName,
          action: "finish",
          success: true,
        });
      }
    }

    if (cleanLine.includes("❌") || cleanLine.includes("Failure")) {
      const match = cleanLine.match(/(?:❌|Failure).*?:\s*(.+)/);
      if (match?.[1]) {
        const jobName = match[1].trim();
        events.push({
          type: "job",
          jobId: jobName,
          action: "finish",
          success: false,
        });
      }
    }

    // Pattern: Step execution
    // Act typically shows "| Step Name" for step output
    if (
      cleanLine.includes("|") &&
      this.currentJob &&
      !cleanLine.includes("[")
    ) {
      const stepMatch = cleanLine.match(/\|\s*(.+)/);
      if (stepMatch?.[1]) {
        const stepName = stepMatch[1].trim();
        const job = this.jobs.get(this.currentJob);
        if (job && stepName) {
          // Register step if not seen
          if (!job.steps.includes(stepName)) {
            job.steps.push(stepName);
          }

          const stepIdx = job.steps.indexOf(stepName);
          events.push({
            type: "step",
            jobId: this.currentJob,
            stepIdx,
            stepName,
          });
        }
      }
    }

    return events;
  }

  /**
   * Get the final manifest based on discovered jobs
   */
  getManifest(): ManifestEvent {
    return this.createManifest();
  }

  /**
   * Create manifest event from current job state
   */
  private createManifest(): ManifestEvent {
    const jobsArray = Array.from(this.jobs.values()).map((job) => ({
      id: job.name,
      name: job.name,
      sensitive: false,
      steps: job.steps,
    }));

    return {
      type: "manifest",
      jobs: jobsArray,
    };
  }

  /**
   * Strip ANSI escape codes from string
   */
  private stripAnsi(str: string): string {
    // biome-ignore lint/suspicious/noControlCharactersInRegex: ANSI codes require control characters
    return str.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, "");
  }
}
