import { defineCommand } from "citty";
import { render } from "ink";
import { TUIEventEmitter } from "../runner/event-emitter.js";
import { CheckRunner } from "../runner/index.js";
import type { RunConfig } from "../runner/types.js";
import { CheckTUI } from "../tui/check-tui.js";
import { printHeaderWithUpdateCheck } from "../tui/components/index.js";
import { formatError } from "../utils/error.js";

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

    try {
      if (verbose) {
        // Verbose mode: run without TUI, show detailed output
        const runner = new CheckRunner(config);

        console.log("Detent check (verbose mode)\n");

        const result = await runner.run();

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

        // Render TUI
        const { waitUntilExit } = render(
          <CheckTUI
            onCancel={() => {
              // TUI will handle exit
            }}
            onEvent={(callback) => eventEmitter.on(callback)}
          />
        );

        // Run workflow in background
        const runPromise = runner.run();

        // Wait for TUI to exit
        await waitUntilExit();

        // Wait for runner to finish
        const result = await runPromise;

        // Show error summary if failed
        if (!result.success && result.errors.length > 0) {
          console.log(
            `\nFound ${result.errors.length} error(s). Run with --verbose for details.`
          );
        }

        process.exit(result.success ? 0 : 1);
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
