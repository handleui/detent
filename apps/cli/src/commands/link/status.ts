/**
 * Link status command
 *
 * Shows the current link status for the repository,
 * including linked team info and GitHub App installation status.
 */

import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import type { Team } from "../../lib/api.js";
import { getTeams } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { getProjectConfig } from "../../lib/config.js";

export const statusCommand = defineCommand({
  meta: {
    name: "status",
    description: "Show link status for this repository",
  },
  run: async () => {
    const repoRoot = await findGitRoot(process.cwd());
    if (!repoRoot) {
      console.error("Not in a git repository.");
      process.exit(1);
    }

    // Check if repository is linked
    const projectConfig = getProjectConfig(repoRoot);
    if (!projectConfig) {
      console.log("\nThis repository is not linked to any team.");
      console.log("Run `detent link` to link it to a team.");
      return;
    }

    console.log("\nLink Status\n");
    console.log("-".repeat(40));
    console.log(`Team ID:     ${projectConfig.teamId}`);
    console.log(`Team Slug:   ${projectConfig.teamSlug}`);
    console.log("-".repeat(40));

    let accessToken: string;
    try {
      accessToken = await getAccessToken();
    } catch {
      console.log(
        "\nNote: Not logged in. Run `detent auth login` for more details."
      );
      return;
    }

    const teamsResponse = await getTeams(accessToken).catch(() => null);
    if (!teamsResponse) {
      console.log("\nNote: Could not fetch team details from API.");
      return;
    }

    const linkedTeam: Team | undefined = teamsResponse.teams.find(
      (t) => t.team_id === projectConfig.teamId
    );

    if (!linkedTeam) {
      console.log("\nWarning: You are not a member of the linked team.");
      console.log("Run `detent link --force` to link to a different team.");
      return;
    }

    console.log("\nTeam Details:\n");
    console.log(`  Name:        ${linkedTeam.team_name}`);
    console.log(`  GitHub Org:  ${linkedTeam.github_org}`);
    console.log(`  Your Role:   ${linkedTeam.role}`);

    if (linkedTeam.github_linked) {
      console.log(`\nGitHub Account: @${linkedTeam.github_username} (linked)`);
    } else {
      console.log("\nGitHub Account: Not linked");
      console.log(
        "GitHub identity is synced automatically when you log in via GitHub."
      );
    }

    console.log("");
  },
});
