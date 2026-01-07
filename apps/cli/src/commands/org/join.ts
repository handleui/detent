/**
 * Organization join command
 *
 * Join an organization by slug.
 */

import { defineCommand } from "citty";
import { joinOrganization } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";

export const joinCommand = defineCommand({
  meta: {
    name: "join",
    description: "Join an organization",
  },
  args: {
    slug: {
      type: "positional",
      description: "Organization slug to join",
      required: true,
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

    if (!args.slug) {
      console.error("Organization slug is required.");
      console.error("\nUsage: detent org join <organization-slug>");
      process.exit(1);
    }

    try {
      const response = await joinOrganization(accessToken, args.slug);

      if (response.joined) {
        console.log(`\n✓ Successfully joined "${response.organization_name}"`);
        console.log(`  Slug: ${response.organization_slug}`);
        console.log(`  Role: ${response.role}`);
      } else {
        console.log(
          `\nYou are already a member of "${response.organization_name}"`
        );
        console.log(`  Role: ${response.role}`);
      }

      if (!response.github_linked) {
        console.log(
          "\n💡 Tip: Link your GitHub account for full functionality"
        );
        console.log("   Run: detent auth login");
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);

      if (message.includes("not found")) {
        console.error(`\nOrganization "${args.slug}" not found.`);
        console.error("Check the slug and try again.");
      } else if (message.includes("suspended")) {
        console.error(`\nOrganization "${args.slug}" is currently suspended.`);
      } else {
        console.error("Failed to join organization:", message);
      }
      process.exit(1);
    }
  },
});
