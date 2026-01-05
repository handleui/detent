import { defineCommand } from "citty";
import { CheckRunner } from "../runner/index.js";
import type { RunConfig } from "../runner/types.js";
import { printHeader } from "../tui/components/index.js";
import { formatError } from "../utils/error.js";

export const checkCommand = defineCommand({
  meta: {
    name: "check",
    description: "Run GitHub Actions workflows locally and extract errors",
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
      console.error("Error: --job requires a workflow to be specified");
      process.exit(1);
    }

    // Build RunConfig
    const config: RunConfig = {
      workflow,
      job,
      repoRoot: process.cwd(),
      verbose,
    };

    // Show header
    if (verbose) {
      printHeader("check");
    }

    try {
      // Create and run CheckRunner
      const runner = new CheckRunner(config);

      if (verbose) {
        console.log("Running preflight checks...");
      }

      const result = await runner.run();

      // Display results
      if (verbose) {
        console.log();
        if (result.success) {
          console.log("✓ All checks passed");
        } else {
          console.log(`Found ${result.errors.length} error(s):`);
          for (const error of result.errors) {
            const location = error.filePath
              ? `${error.filePath}`
              : "unknown location";
            console.log(`  - ${error.errorId}: ${error.message} (${location})`);
          }
        }
        console.log();
        console.log(`Run ID: ${result.runID}`);
        console.log(`Duration: ${(result.duration / 1000).toFixed(1)}s`);
      }

      // Exit with appropriate code
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      console.error("Error running check:", formatError(error));
      process.exit(1);
    }
  },
});
