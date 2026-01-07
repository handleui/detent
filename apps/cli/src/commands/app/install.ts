/**
 * GitHub App installation command
 *
 * Opens browser to install the Detent GitHub App for the user's team.
 */

import { defineCommand } from "citty";
import { getTeams, type Team } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { findTeamByIdOrSlug, selectTeam } from "../../lib/ui.js";

const GITHUB_APP_NAME = process.env.GITHUB_APP_NAME ?? "detent-dev";

const openBrowser = async (url: string): Promise<void> => {
  const { exec } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execAsync = promisify(exec);

  const platform = process.platform;
  let command: string;
  if (platform === "darwin") {
    command = `open "${url}"`;
  } else if (platform === "win32") {
    command = `start "" "${url}"`;
  } else {
    command = `xdg-open "${url}"`;
  }

  await execAsync(command).catch(() => {
    // Ignore errors - user can manually open URL
  });
};

export const installCommand = defineCommand({
  meta: {
    name: "install",
    description: "Install the GitHub App for your team",
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

    // Get user's teams to determine org for pre-selection
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

    // Build GitHub App installation URL
    // Note: GitHub's target_id parameter requires a numeric org/user ID, not the org name.
    // Since we only have the org name, we let the user select the org in GitHub's UI.
    const installUrl = `https://github.com/apps/${GITHUB_APP_NAME}/installations/new`;

    console.log(`\nInstalling GitHub App for team: ${selectedTeam.team_name}`);
    console.log(`GitHub org: ${selectedTeam.github_org}\n`);

    console.log("Opening browser to install the GitHub App...");
    console.log(`If browser doesn't open, visit:\n  ${installUrl}\n`);

    await openBrowser(installUrl);

    console.log("After installation, run `detent app status` to verify.");
  },
});
