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
  run: ({ args }) => {
    const key = args.key;

    if (!isConfigKey(key)) {
      console.error(`Unknown key: ${key}`);
      console.error(`Valid keys: ${CONFIG_KEYS.join(", ")}`);
      process.exit(1);
    }

    try {
      const config = loadConfig();
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
