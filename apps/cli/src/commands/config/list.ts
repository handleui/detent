import { findGitRoot } from "@detent/git";
import {
  formatBudget,
  getRepoConfigPath,
  loadConfig,
  maskApiKey,
} from "@detent/persistence";
import { defineCommand } from "citty";
import { printHeader } from "../../tui/components/index.js";

export const configListCommand = defineCommand({
  meta: {
    name: "list",
    description: "List all configuration values",
  },
  run: async () => {
    const repoRoot = await findGitRoot(process.cwd());
    const config = loadConfig(repoRoot ?? undefined);
    const configPath = repoRoot
      ? getRepoConfigPath(repoRoot)
      : "(not in a git repository)";
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
