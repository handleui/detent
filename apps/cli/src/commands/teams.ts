/**
 * Teams command - lists all teams the user is a member of
 */

import { defineCommand } from "citty";
import { getTeams } from "../lib/api.js";
import { getAccessToken } from "../lib/auth.js";

export const teamsCommand = defineCommand({
  meta: {
    name: "teams",
    description: "List teams you are a member of",
  },
  run: async () => {
    let accessToken: string;
    try {
      accessToken = await getAccessToken();
    } catch {
      console.error("Not logged in. Run `detent auth login` first.");
      process.exit(1);
    }

    try {
      const response = await getTeams(accessToken);

      if (response.teams.length === 0) {
        console.log("You are not a member of any teams.");
        console.log("Ask a team admin to invite you.");
        return;
      }

      console.log("\nYour Teams\n");
      console.log("-".repeat(60));

      for (const team of response.teams) {
        const githubStatus = team.github_linked
          ? `@${team.github_username}`
          : "not linked";

        console.log(`${team.team_name}`);
        console.log(`  Slug:       ${team.team_slug}`);
        console.log(`  GitHub Org: ${team.github_org}`);
        console.log(`  Role:       ${team.role}`);
        console.log(`  GitHub:     ${githubStatus}`);
        console.log("");
      }

      console.log("-".repeat(60));
      console.log(`Total: ${response.teams.length} team(s)`);
    } catch (error) {
      console.error(
        "Failed to fetch teams:",
        error instanceof Error ? error.message : String(error)
      );
      process.exit(1);
    }
  },
});
