/**
 * Unlink command - removes the link between this repository and an organization
 */

import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import { getProjectConfig, removeProjectConfig } from "../../lib/config.js";

const confirm = async (message: string): Promise<boolean> => {
  const readline = await import("node:readline");
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  const answer = await new Promise<string>((resolve) => {
    rl.question(`${message} (y/N): `, resolve);
  });
  rl.close();

  return answer.toLowerCase() === "y";
};

export const unlinkCommand = defineCommand({
  meta: {
    name: "unlink",
    description: "Unlink this repository from its organization",
  },
  args: {
    force: {
      type: "boolean",
      description: "Skip confirmation prompt",
      alias: "f",
      default: false,
    },
  },
  run: async ({ args }) => {
    const repoRoot = await findGitRoot(process.cwd());
    if (!repoRoot) {
      console.error("Not in a git repository.");
      process.exit(1);
    }

    // Check if repository is linked
    const projectConfig = getProjectConfig(repoRoot);
    if (!projectConfig) {
      console.log("\nThis repository is not linked to any organization.");
      return;
    }

    console.log(`\nLinked to organization: ${projectConfig.organizationSlug}`);

    if (!args.force) {
      const confirmed = await confirm("\nAre you sure you want to unlink?");
      if (!confirmed) {
        console.log("Cancelled.");
        return;
      }
    }

    await removeProjectConfig(repoRoot);
    console.log("\nSuccessfully unlinked repository from organization.");
  },
});
