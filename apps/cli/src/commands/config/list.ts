import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import {
  formatBudget,
  getRepoConfigPath,
  loadConfig,
  maskApiKey,
} from "../../lib/config.js";
import { printHeader } from "../../tui/components/index.js";

export const configListCommand = defineCommand({
  meta: {
    name: "list",
    description: "List all configuration values",
  },
  run: async () => {
    const repoRoot = await findGitRoot(process.cwd());
    if (!repoRoot) {
      console.error("Error: Not in a git repository.");
      process.exit(1);
    }
    const config = loadConfig(repoRoot);
    const configPath = getRepoConfigPath(repoRoot);
    const hasEnvApiKey = Boolean(process.env.ANTHROPIC_API_KEY);

    printHeader("config list");

    console.log(`Config file: ${configPath}`);
    console.log();
    console.log(
      `apiKey: ${maskApiKey(config.apiKey) || "(not set)"}${hasEnvApiKey ? " (from ANTHROPIC_API_KEY)" : ""}`
    );
    console.log(`model: ${config.model}`);
    console.log(`budgetPerRunUsd: ${formatBudget(config.budgetPerRunUsd)}`);
    console.log(`budgetMonthlyUsd: ${formatBudget(config.budgetMonthlyUsd)}`);
    console.log(`timeoutMins: ${config.timeoutMins}`);
    console.log();
  },
});
