import { defineCommand } from "citty";
import { render } from "ink";
import { TUIEventEmitter } from "../runner/event-emitter.js";
import { CheckRunner } from "../runner/index.js";
import type { RunConfig } from "../runner/types.js";
import { CheckTUI } from "../tui/check-tui.js";
import { printHeaderWithUpdateCheck } from "../tui/components/index.js";
import { formatError } from "../utils/error.js";
import { createSignalController, SIGINT_EXIT_CODE } from "../utils/signal.js";

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

    // Validate: job requires workflow
    if (job && !workflow) {
      console.error(
        "Error: Job argument requires a workflow to be specified\n" +
          "\n" +
          "Usage: detent check <workflow> <job>\n" +
          "Example: detent check ci.yml build"
      );
      process.exit(1);
    }

    // Build RunConfig
    const config: RunConfig = {
      workflow,
      job,
      repoRoot: process.cwd(),
      verbose,
    };

    // Create signal controller for graceful shutdown
    const signalCtrl = createSignalController();

    // Track if we're in cleanup phase (second signal = force exit)
    let inCleanup = false;

    // Setup force exit handler for second signal during cleanup
    const forceExitHandler = (): void => {
      if (inCleanup) {
        process.exit(SIGINT_EXIT_CODE);
      }
    };
    process.on("SIGINT", forceExitHandler);
    process.on("SIGTERM", forceExitHandler);

    try {
      if (verbose) {
        // Verbose mode: run without TUI, show detailed output
        const runner = new CheckRunner(config);

        // Abort runner when signal received
        signalCtrl.signal.addEventListener("abort", () => {
          console.log("\nCancelling...");
          runner.abort();
        });

        console.log("Detent check (verbose mode)\n");

        inCleanup = true; // Runner cleanup happens inside run()
        const result = await runner.run();
        inCleanup = false;

        // Clean up signal handlers
        signalCtrl.cleanup();
        process.off("SIGINT", forceExitHandler);
        process.off("SIGTERM", forceExitHandler);

        // If aborted, exit cleanly
        if (runner.isAborted()) {
          console.log("\nCancelled. Cleanup complete.");
          process.exit(SIGINT_EXIT_CODE);
        }

        // Display debug log path
        const debugLogPath = runner.getDebugLogPath();
        if (debugLogPath) {
          console.log(`\nDebug log: ${debugLogPath}\n`);
        }

        // Display detailed results
        console.log("=".repeat(60));
        console.log("RESULTS");
        console.log("=".repeat(60));
        console.log();

        if (result.success) {
          console.log("✓ No errors found");
        } else {
          console.log(`✗ Found ${result.errors.length} error(s):\n`);
          for (const error of result.errors) {
            console.log(`  ${error.errorId}`);
            console.log(`  Message: ${error.message}`);
            if (error.filePath) {
              console.log(`  Location: ${error.filePath}`);
            }
            console.log();
          }
        }

        console.log(`Run ID: ${result.runID}`);
        console.log(`Duration: ${(result.duration / 1000).toFixed(1)}s`);

        process.exit(result.success ? 0 : 1);
      } else {
        // Non-verbose mode: use TUI
        await printHeaderWithUpdateCheck("check");

        const eventEmitter = new TUIEventEmitter();

        // Create runner with event emitter
        const runner = new CheckRunner(config, eventEmitter);

        // Abort runner when signal received (process-level handler for TUI mode)
        signalCtrl.signal.addEventListener("abort", () => {
          runner.abort();
        });

        // Render TUI
        const { waitUntilExit, unmount } = render(
          <CheckTUI
            onCancel={() => {
              runner.abort();
            }}
            onEvent={(callback) => eventEmitter.on(callback)}
          />
        );

        // Run workflow in background
        const runPromise = runner.run();

        // Wait for TUI to exit
        await waitUntilExit();

        // TUI has exited, now wait for runner cleanup
        inCleanup = true;
        const result = await runPromise;
        inCleanup = false;

        // Clean up signal handlers
        signalCtrl.cleanup();
        process.off("SIGINT", forceExitHandler);
        process.off("SIGTERM", forceExitHandler);

        // If aborted, exit cleanly without error message
        if (runner.isAborted()) {
          console.log("\nCancelled. Cleanup complete.");
          process.exit(SIGINT_EXIT_CODE);
        }

        // Show error summary if failed
        if (!result.success && result.errors.length > 0) {
          console.log(
            `\nFound ${result.errors.length} error(s). Run with --verbose for details.`
          );
        }

        process.exit(result.success ? 0 : 1);
      }
    } catch (error) {
      // Clean up signal handlers on error
      signalCtrl.cleanup();
      process.off("SIGINT", forceExitHandler);
      process.off("SIGTERM", forceExitHandler);

      const message = formatError(error);
      console.error(`\n✗ Check failed: ${message}\n`);

      if (!verbose) {
        console.error("Run with --verbose for more details.");
      }

      process.exit(1);
    }
  },
});
