import {
  type GlobalConfig,
  loadGlobalConfig,
  saveConfig,
} from "@detent/persistence";
import { defineCommand } from "citty";
import { CONFIG_KEYS, type ConfigKey, isConfigKey } from "./constants.js";

const parseValue = (key: ConfigKey, value: string): unknown => {
  if (
    key === "budgetPerRunUsd" ||
    key === "budgetMonthlyUsd" ||
    key === "timeoutMins"
  ) {
    const num = Number(value);
    if (Number.isNaN(num)) {
      throw new Error(`Invalid number: ${value}`);
    }
    return num;
  }
  return value;
};

export const configSetCommand = defineCommand({
  meta: {
    name: "set",
    description: "Set a configuration value (for AI/scripting)",
  },
  args: {
    key: {
      type: "positional",
      description: `Configuration key (${CONFIG_KEYS.join(", ")})`,
      required: true,
    },
    value: {
      type: "positional",
      description: "Value to set",
      required: true,
    },
  },
  run: ({ args }) => {
    const key = args.key;
    const rawValue = args.value;

    if (!isConfigKey(key)) {
      console.error(`Unknown key: ${key}`);
      process.exit(1);
    }

    try {
      const parsed = parseValue(key, rawValue);
      const config = loadGlobalConfig();
      const updated: GlobalConfig = { ...config, [key]: parsed };
      saveConfig(updated);
      console.log("ok");
    } catch (error) {
      console.error(error instanceof Error ? error.message : "unknown error");
      process.exit(1);
    }
  },
});
