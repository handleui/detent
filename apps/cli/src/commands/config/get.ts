import { findGitRoot } from "@detent/git";
import { loadConfig } from "@detent/persistence";
import { defineCommand } from "citty";
import { CONFIG_KEYS, isConfigKey } from "./constants.js";

export const configGetCommand = defineCommand({
  meta: {
    name: "get",
    description: "Get a configuration value (for AI/scripting)",
  },
  args: {
    key: {
      type: "positional",
      description: `Configuration key (${CONFIG_KEYS.join(", ")})`,
      required: true,
    },
  },
  run: async ({ args }) => {
    const key = args.key;

    if (!isConfigKey(key)) {
      console.error(`Unknown key: ${key}`);
      console.error(`Valid keys: ${CONFIG_KEYS.join(", ")}`);
      process.exit(1);
    }

    // Prevent exposing API key via CLI - use maskApiKey for safe display
    if (key === "apiKey") {
      console.error(
        "Error: API key cannot be retrieved via CLI for security reasons."
      );
      console.error("Use 'detent config list' to see a masked version.");
      process.exit(1);
    }

    try {
      const repoRoot = await findGitRoot(process.cwd());
      if (!repoRoot) {
        console.error("Error: Not in a git repository.");
        process.exit(1);
      }
      const config = loadConfig(repoRoot);
      const value = config[key];

      if (value === undefined || value === null) {
        console.log("");
      } else {
        console.log(value);
      }
    } catch (error) {
      console.error(
        `Error loading config: ${error instanceof Error ? error.message : "unknown error"}`
      );
      process.exit(1);
    }
  },
});
