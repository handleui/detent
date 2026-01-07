/**
 * GitHub account unlink command
 *
 * Removes the GitHub account link from a user's Detent team membership.
 */

import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import { getTeams, type Team, unlinkGithub } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { findTeamByIdOrSlug, selectTeam } from "../../lib/ui.js";

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
    description: "Unlink your GitHub account from your Detent team",
  },
  args: {
    team: {
      type: "string",
      description: "Team ID or slug (optional - will prompt if not provided)",
      alias: "t",
    },
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

    let accessToken: string;
    try {
      accessToken = await getAccessToken(repoRoot);
    } catch {
      console.error("Not logged in. Run `detent auth login` first.");
      process.exit(1);
    }

    // Get user's teams
    console.log("Fetching your teams...");
    const teamsResponse = await getTeams(accessToken).catch((error) => {
      console.error(
        "Failed to fetch teams:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    });

    // Select team
    let selectedTeam: Team;

    if (args.team) {
      const found = findTeamByIdOrSlug(teamsResponse.teams, args.team);
      if (!found) {
        console.error(`Team not found: ${args.team}`);
        process.exit(1);
      }
      selectedTeam = found;
    } else {
      const selected = await selectTeam(teamsResponse.teams);
      if (!selected) {
        process.exit(1);
      }
      selectedTeam = selected;
    }

    // Check if linked
    if (!selectedTeam.github_linked) {
      console.log(
        `\nNo GitHub account linked to team: ${selectedTeam.team_name}`
      );
      return;
    }

    // Confirm before unlinking
    console.log(`\nTeam: ${selectedTeam.team_name}`);
    console.log(`GitHub Account: @${selectedTeam.github_username}`);

    if (!args.force) {
      const confirmed = await confirm("\nAre you sure you want to unlink?");
      if (!confirmed) {
        console.log("Cancelled.");
        return;
      }
    }

    // Unlink
    try {
      await unlinkGithub(accessToken, selectedTeam.team_id);
      console.log("\nSuccessfully unlinked GitHub account.");
    } catch (error) {
      console.error(
        "\nFailed to unlink:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    }
  },
});
