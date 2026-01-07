/**
 * GitHub link status command
 *
 * Shows the GitHub link status for the user's team membership.
 */

import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import { getLinkStatus, getTeams, type Team } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { findTeamByIdOrSlug, selectTeam } from "../../lib/ui.js";

const displayAllTeams = (teams: Team[]): void => {
  console.log("\nGitHub Link Status\n");
  console.log("─".repeat(60));

  for (const team of teams) {
    console.log(`\nTeam: ${team.team_name}`);
    console.log(`  Slug: ${team.team_slug}`);
    console.log(`  GitHub Org: ${team.github_org}`);
    console.log(`  Role: ${team.role}`);

    if (team.github_linked) {
      console.log(`  GitHub Account: @${team.github_username}`);
      console.log("  Status: ✅ Linked");
    } else {
      console.log("  Status: ❌ Not linked");
      console.log("  Run `detent link` to connect your GitHub account");
    }
  }

  console.log(`\n${"─".repeat(60)}`);
};

export const statusCommand = defineCommand({
  meta: {
    name: "status",
    description: "Show GitHub link status for your team",
  },
  args: {
    team: {
      type: "string",
      description: "Team ID or slug (optional - will prompt if not provided)",
      alias: "t",
    },
    all: {
      type: "boolean",
      description: "Show status for all teams",
      alias: "a",
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
    const teamsResponse = await getTeams(accessToken).catch((error) => {
      console.error(
        "Failed to fetch teams:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    });

    // Show all teams
    if (args.all) {
      displayAllTeams(teamsResponse.teams);
      return;
    }

    // Select specific team
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

    // Get detailed status from API
    const status = await getLinkStatus(accessToken, selectedTeam.team_id).catch(
      (error) => {
        console.error(
          "Failed to get status:",
          error instanceof Error ? error.message : error
        );
        process.exit(1);
      }
    );

    // Display status
    console.log("\nGitHub Link Status\n");
    console.log("─".repeat(40));
    console.log(`Team:        ${status.team_name}`);
    console.log(`Slug:        ${status.team_slug}`);
    console.log(`GitHub Org:  ${status.github_org}`);
    console.log("─".repeat(40));

    if (status.github_linked) {
      console.log(`GitHub:      @${status.github_username}`);
      console.log(`User ID:     ${status.github_user_id}`);
      console.log(
        `Linked:      ${status.github_linked_at ? new Date(status.github_linked_at).toLocaleString() : "Unknown"}`
      );
      console.log("\nStatus: ✅ Linked");
    } else {
      console.log("\nStatus: ❌ Not linked");
      console.log("\nRun `detent link` to connect your GitHub account.");
    }

    console.log("");
  },
});
