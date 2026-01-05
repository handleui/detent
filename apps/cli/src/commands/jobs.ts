import { join } from "node:path";
import { defineCommand } from "citty";
import { printHeaderWithUpdateCheck } from "../tui/components/index.js";
import { ANSI_RESET, colors, hexToAnsi } from "../tui/styles.js";
import { detectAgent } from "../utils/agent.js";
import type { ParsedWorkflow } from "../workflow/parser.js";
import { parseWorkflowsFromDir } from "../workflow/parser.js";
import type { SensitivityReason } from "../workflow/sensitivity.js";
import {
  formatSensitivityReason,
  getSensitivityReason,
  isSensitiveWorkflow,
} from "../workflow/sensitivity.js";

// ─────────────────────────────────────────────────────────────────────────────
// Styling
// ─────────────────────────────────────────────────────────────────────────────

const dim = (s: string): string =>
  `${hexToAnsi(colors.muted)}${s}${ANSI_RESET}`;
const warn = (s: string): string =>
  `${hexToAnsi(colors.warn)}${s}${ANSI_RESET}`;

// ─────────────────────────────────────────────────────────────────────────────
// Data Types
// ─────────────────────────────────────────────────────────────────────────────

interface JobDisplay {
  readonly name: string;
  readonly deps: readonly string[];
  readonly sensitive: boolean;
  readonly reason: SensitivityReason | null;
}

interface WorkflowDisplay {
  readonly filename: string;
  readonly sensitive: boolean;
  readonly jobs: readonly JobDisplay[];
}

interface Stats {
  sensitive: number;
  safe: number;
  withDeps: number;
}

// ─────────────────────────────────────────────────────────────────────────────
// Data Extraction
// ─────────────────────────────────────────────────────────────────────────────

const extractWorkflowData = (parsed: ParsedWorkflow): WorkflowDisplay => {
  const { workflow, jobs, filename } = parsed;
  const fileIsSensitive = isSensitiveWorkflow(filename);

  const jobDisplays: JobDisplay[] = jobs.map((job) => {
    const workflowJob = workflow.jobs[job.id];
    const reason = workflowJob
      ? getSensitivityReason(job.id, workflowJob)
      : null;

    return {
      name: job.name !== job.id ? job.name : job.id,
      deps: job.needs,
      sensitive: reason !== null || fileIsSensitive,
      reason,
    };
  });

  const allSensitive = jobs.length > 0 && jobDisplays.every((j) => j.sensitive);

  return {
    filename,
    sensitive: fileIsSensitive || allSensitive,
    jobs: jobDisplays,
  };
};

const computeStats = (workflows: readonly WorkflowDisplay[]): Stats => {
  let sensitive = 0;
  let safe = 0;
  let withDeps = 0;

  for (const wf of workflows) {
    for (const job of wf.jobs) {
      if (job.sensitive) {
        sensitive++;
      } else {
        safe++;
      }
      if (job.deps.length > 0) {
        withDeps++;
      }
    }
  }

  return { sensitive, safe, withDeps };
};

// ─────────────────────────────────────────────────────────────────────────────
// Rendering
// ─────────────────────────────────────────────────────────────────────────────

const renderJob = (job: JobDisplay, verbose: boolean): string => {
  const marker = job.sensitive ? `${warn("!")} ` : "   ";
  const reason =
    verbose && job.reason ? dim(formatSensitivityReason(job.reason)) : "";
  const padding = reason ? " ".repeat(Math.max(1, 40 - job.name.length)) : "";

  return `  ${marker}${job.name}${padding}${reason}`;
};

const renderWorkflow = (wf: WorkflowDisplay, verbose: boolean): string[] => {
  const lines: string[] = [];

  lines.push(wf.filename);
  lines.push(""); // Gap after filename for visual breathing room

  if (wf.jobs.length === 0) {
    lines.push(dim("  (empty)"));
  } else {
    for (const job of wf.jobs) {
      lines.push(renderJob(job, verbose));
    }
  }

  return lines;
};

const renderStats = (stats: Stats, verbose: boolean): string[] => {
  const lines: string[] = [];

  const parts = [
    `${stats.sensitive} ${dim("sensitive")}`,
    `${stats.safe} ${dim("safe")}`,
    `${stats.withDeps} ${dim("deps")}`,
  ];
  lines.push(parts.join(dim("  ")));

  if (stats.sensitive > 0 && !verbose) {
    lines.push(dim("-v for details"));
  }

  return lines;
};

// ─────────────────────────────────────────────────────────────────────────────
// Command
// ─────────────────────────────────────────────────────────────────────────────

export const jobsCommand = defineCommand({
  meta: {
    name: "jobs",
    description:
      "List all jobs in GitHub Actions workflows with sensitivity markers\n\n" +
      "Jobs that perform deployment, publishing, or release operations are marked\n" +
      "as sensitive and will be skipped during `detent check` to prevent accidental\n" +
      "production releases.\n\n" +
      "EXAMPLES\n" +
      "  # List all jobs\n" +
      "  detent jobs\n\n" +
      "  # Show why jobs are marked sensitive\n" +
      "  detent jobs --verbose",
  },
  args: {
    verbose: {
      type: "boolean",
      description: "Show detailed sensitivity reasons",
      alias: "v",
      default: false,
    },
  },
  run: async ({ args }) => {
    const agent = detectAgent();
    const verbose = (args.verbose as boolean) || agent.isAgent;
    const workflowsDir = join(process.cwd(), ".github", "workflows");

    printHeaderWithUpdateCheck("jobs");

    const parsed = await parseWorkflowsFromDir(workflowsDir);

    if (parsed.length === 0) {
      console.log(dim("No workflows found in .github/workflows/"));
      console.log();
      return;
    }

    const workflows = parsed.map(extractWorkflowData);
    const stats = computeStats(workflows);

    for (const wf of workflows) {
      for (const line of renderWorkflow(wf, verbose)) {
        console.log(line);
      }
      console.log();
    }

    for (const line of renderStats(stats, verbose)) {
      console.log(line);
    }
    console.log();
  },
});
