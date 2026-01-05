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
      process.exit(1);
    }

    const config = loadConfig();
    console.log(config[key]);
  },
});
