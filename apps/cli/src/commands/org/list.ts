/**
 * Organization list command
 *
 * Lists all organizations the user is a member of.
 */

import { defineCommand } from "citty";
import { getOrganizations } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";

export const listCommand = defineCommand({
  meta: {
    name: "list",
    description: "List organizations you are a member of",
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
      const response = await getOrganizations(accessToken);

      if (response.organizations.length === 0) {
        console.log("You are not a member of any organizations.\n");
        console.log("To create an organization, run: detent org create");
        return;
      }

      console.log("\nYour Organizations\n");
      console.log("-".repeat(60));

      for (const organization of response.organizations) {
        const githubStatus = organization.github_linked
          ? `@${organization.github_username}`
          : "not linked";

        console.log(`${organization.organization_name}`);
        console.log(`  Slug:       ${organization.organization_slug}`);
        console.log(`  GitHub Org: ${organization.github_org}`);
        console.log(`  Role:       ${organization.role}`);
        console.log(`  GitHub:     ${githubStatus}`);
        console.log("");
      }

      console.log("-".repeat(60));
      console.log(`Total: ${response.organizations.length} organization(s)`);
    } catch (error) {
      console.error(
        "Failed to fetch organizations:",
        error instanceof Error ? error.message : String(error)
      );
      process.exit(1);
    }
  },
});
