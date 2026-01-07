/**
 * GitHub App status command
 *
 * Shows the GitHub App installation status for the user's team.
 */

import { defineCommand } from "citty";
import { getAppStatus, getTeams, type Team } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { findTeamByIdOrSlug, selectTeam } from "../../lib/ui.js";

export const statusCommand = defineCommand({
  meta: {
    name: "status",
    description: "Show GitHub App installation status for your team",
  },
  args: {
    team: {
      type: "string",
      description: "Team ID or slug (optional - will prompt if not provided)",
      alias: "t",
    },
  },
  run: async ({ args }) => {
    let accessToken: string;
    try {
      accessToken = await getAccessToken();
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

    // Select team
    let selectedTeam: Team;

    if (args.team) {
      const found = findTeamByIdOrSlug(teamsResponse.teams, args.team);
      if (!found) {
        console.error(`Team not found: ${args.team}`);
        if (teamsResponse.teams.length > 0) {
          console.error("\nAvailable teams:");
          for (const team of teamsResponse.teams) {
            console.error(`  - ${team.team_slug} (${team.team_name})`);
          }
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

    // Get app installation status from API
    const status = await getAppStatus(accessToken, selectedTeam.team_id).catch(
      (error) => {
        console.error(
          "Failed to get app status:",
          error instanceof Error ? error.message : error
        );
        process.exit(1);
      }
    );

    // Display status
    console.log("\nGitHub App Installation Status\n");
    console.log("-".repeat(40));
    console.log(`Team:        ${status.team_name}`);
    console.log(`Slug:        ${status.team_slug}`);
    console.log(`GitHub Org:  ${status.github_org}`);
    console.log("-".repeat(40));

    if (status.app_installed) {
      console.log("Status:      Installed");

      if (status.installation_id) {
        console.log(`Install ID:  ${status.installation_id}`);
      }

      if (status.installed_at) {
        console.log(
          `Installed:   ${new Date(status.installed_at).toLocaleString()}`
        );
      }

      if (status.suspended_at) {
        console.log("\nWarning: Installation is currently suspended.");
        console.log(
          `Suspended:   ${new Date(status.suspended_at).toLocaleString()}`
        );
      }
    } else {
      console.log("Status:      Not installed");
      console.log("\nRun `detent app install` to install the GitHub App.");
    }

    console.log("");
  },
});
