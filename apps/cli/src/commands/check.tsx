import { defineCommand } from "citty";
import { render } from "ink";
import { TUIEventEmitter } from "../runner/event-emitter.js";
import { CheckRunner } from "../runner/index.js";
import type { RunConfig, RunResult } from "../runner/types.js";
import { CheckTUI } from "../tui/check-tui.js";
import { printHeaderWithUpdateCheck } from "../tui/components/index.js";
import { formatError } from "../utils/error.js";
import { createSignalController, SIGINT_EXIT_CODE } from "../utils/signal.js";

const printVerboseResults = (result: RunResult): void => {
  console.log("=".repeat(60));
  console.log("RESULTS");
  console.log("=".repeat(60));
  console.log();

  if (result.success) {
    console.log("✓ No errors found");
  } else if (result.errors.length > 0) {
    console.log(`✗ Found ${result.errors.length} error(s):\n`);
    for (const error of result.errors) {
      console.log(`  ${error.errorId}`);
      console.log(`  Message: ${error.message}`);
      if (error.filePath) {
        console.log(`  Location: ${error.filePath}`);
      }
      console.log();
    }
  } else {
    // Workflow failed but no parsing errors were extracted
    console.log("✓ No parsing errors found");
  }

  console.log(`Run ID: ${result.runID}`);
  console.log(`Duration: ${(result.duration / 1000).toFixed(1)}s`);
};

const runVerboseMode = async (config: RunConfig): Promise<void> => {
  const signalCtrl = createSignalController();
  const runner = new CheckRunner(config);

  // Abort runner when signal received
  signalCtrl.signal.addEventListener("abort", () => {
    console.log("\nCancelling...");
    runner.abort();
  });

  console.log("Detent check (verbose mode)\n");

  try {
    const result = await runner.run();
    signalCtrl.cleanup();

    if (runner.isAborted()) {
      console.log("\nCancelled. Cleanup complete.");
      process.exit(SIGINT_EXIT_CODE);
    }

    const debugLogPath = runner.getDebugLogPath();
    if (debugLogPath) {
      console.log(`\nDebug log: ${debugLogPath}\n`);
    }

    printVerboseResults(result);
    process.exit(result.success ? 0 : 1);
  } catch (error) {
    signalCtrl.cleanup();
    throw error;
  }
};

const runTUIMode = async (config: RunConfig): Promise<void> => {
  await printHeaderWithUpdateCheck("check");

  const signalCtrl = createSignalController();
  const eventEmitter = new TUIEventEmitter();
  const runner = new CheckRunner(config, eventEmitter);

  // Abort runner when process signal received
  signalCtrl.signal.addEventListener("abort", () => {
    runner.abort();
  });

  // Run the workflow - this will emit events to the TUI
  const runPromise = runner.run();

  // Render TUI with exitOnCtrlC DISABLED - we handle exit ourselves
  // The TUI will call onCancel when Ctrl+C is pressed, which aborts the runner.
  // The runner will then emit a "done" event, and the TUI will exit.
  const { waitUntilExit } = render(
    <CheckTUI
      onCancel={() => {
        runner.abort();
      }}
      onEvent={(callback) => eventEmitter.on(callback)}
    />,
    {
      // CRITICAL: Disable auto-exit so we control the shutdown sequence
      exitOnCtrlC: false,
    }
  );

  // Wait for both TUI exit and runner completion
  // The TUI exits when it receives the "done" event from the runner
  const [result] = await Promise.all([runPromise, waitUntilExit()]);

  signalCtrl.cleanup();

  if (runner.isAborted()) {
    console.log("\nCancelled. Cleanup complete.");
    process.exit(SIGINT_EXIT_CODE);
  }

  process.exit(result.success ? 0 : 1);
};

export const checkCommand = defineCommand({
  meta: {
    name: "check",
    description:
      "Run GitHub Actions workflows locally and extract errors\n\n" +
      "EXAMPLES\n" +
      "  # Run all workflows\n" +
      "  detent check\n\n" +
      "  # Run specific workflow\n" +
      "  detent check ci.yml\n\n" +
      "  # Run specific job in a workflow\n" +
      "  detent check ci.yml build\n\n" +
      "  # Show detailed output\n" +
      "  detent check --verbose",
  },
  args: {
    workflow: {
      type: "positional",
      description: "Workflow name to run (optional, runs all if not specified)",
      required: false,
    },
    job: {
      type: "positional",
      description: "Job name to run (requires workflow to be specified)",
      required: false,
    },
    verbose: {
      type: "boolean",
      description: "Enable verbose output",
      alias: "v",
      default: false,
    },
  },
  run: async ({ args }) => {
    const workflow = args.workflow as string | undefined;
    const job = args.job as string | undefined;
    const verbose = args.verbose as boolean;

    if (job && !workflow) {
      console.error(
        "Error: Job argument requires a workflow to be specified\n" +
          "\n" +
          "Usage: detent check <workflow> <job>\n" +
          "Example: detent check ci.yml build"
      );
      process.exit(1);
    }

    const config: RunConfig = {
      workflow,
      job,
      repoRoot: process.cwd(),
      verbose,
    };

    // Clean up orphaned clones from previous runs (best-effort)
    try {
      const { cleanupOrphanedClones } = await import("@detent/git");
      cleanupOrphanedClones(config.repoRoot);
    } catch {
      // Ignore cleanup errors - best-effort background cleanup
    }

    try {
      if (verbose) {
        await runVerboseMode(config);
      } else {
        await runTUIMode(config);
      }
    } catch (error) {
      const message = formatError(error);
      console.error(`\n✗ Check failed: ${message}\n`);

      if (!verbose) {
        console.error("Run with --verbose for more details.");
      }

      process.exit(1);
    }
  },
});
