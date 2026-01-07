/**
 * Link command - links a repository to a Detent team
 *
 * Similar to Vercel's project linking, this binds the current repo
 * to a team for Detent operations.
 */

import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import type { Team } from "../../lib/api.js";
import { getTeams } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { getProjectConfig, saveProjectConfig } from "../../lib/config.js";
import { findTeamByIdOrSlug, selectTeam } from "../../lib/ui.js";

export const linkCommand = defineCommand({
  meta: {
    name: "link",
    description: "Link this repository to a Detent team",
  },
  subCommands: {
    status: () => import("./status.js").then((m) => m.statusCommand),
    unlink: () => import("./unlink.js").then((m) => m.unlinkCommand),
  },
  args: {
    team: {
      type: "string",
      description: "Team ID or slug to link to",
      alias: "t",
    },
    force: {
      type: "boolean",
      description: "Overwrite existing link without prompting",
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
      accessToken = await getAccessToken();
    } catch {
      console.error("Not logged in. Run `detent auth login` first.");
      process.exit(1);
    }

    // Check if already linked
    const existingConfig = getProjectConfig(repoRoot);
    if (existingConfig && !args.force) {
      console.log(
        `\nThis repository is already linked to team: ${existingConfig.teamSlug}`
      );
      console.log("Run `detent link --force` to link to a different team.");
      console.log("Run `detent link status` to see details.");
      return;
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

    if (teamsResponse.teams.length === 0) {
      console.error("You are not a member of any teams.");
      console.error("Ask a team admin to invite you.");
      process.exit(1);
    }

    // Select team
    let selectedTeam: Team;

    if (args.team) {
      const found = findTeamByIdOrSlug(teamsResponse.teams, args.team);
      if (!found) {
        console.error(`Team not found: ${args.team}`);
        console.error("\nAvailable teams:");
        for (const team of teamsResponse.teams) {
          console.error(`  - ${team.team_slug} (${team.team_name})`);
        }
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

    // Save project config
    saveProjectConfig(repoRoot, {
      teamId: selectedTeam.team_id,
      teamSlug: selectedTeam.team_slug,
    });

    console.log(
      `\nLinked to team: ${selectedTeam.team_name} (${selectedTeam.team_slug})`
    );
    console.log("\nRun `detent link status` to see details.");
  },
});
