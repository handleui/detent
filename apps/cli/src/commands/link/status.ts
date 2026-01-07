/**
 * Link status command
 *
 * Shows the current link status for the repository,
 * including linked organization info and GitHub App installation status.
 */

import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import type { Organization } from "../../lib/api.js";
import { getOrganizations } from "../../lib/api.js";
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
      console.log("\nThis repository is not linked to any organization.");
      console.log("Run `detent link` to link it to an organization.");
      return;
    }

    console.log("\nLink Status\n");
    console.log("-".repeat(40));
    console.log(`Organization ID:     ${projectConfig.organizationId}`);
    console.log(`Organization Slug:   ${projectConfig.organizationSlug}`);
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

    const organizationsResponse = await getOrganizations(accessToken).catch(
      () => null
    );
    if (!organizationsResponse) {
      console.log("\nNote: Could not fetch organization details from API.");
      return;
    }

    const linkedOrganization: Organization | undefined =
      organizationsResponse.organizations.find(
        (o) => o.organization_id === projectConfig.organizationId
      );

    if (!linkedOrganization) {
      console.log(
        "\nWarning: You are not a member of the linked organization."
      );
      console.log(
        "Run `detent link --force` to link to a different organization."
      );
      return;
    }

    console.log("\nOrganization Details:\n");
    console.log(`  Name:        ${linkedOrganization.organization_name}`);
    console.log(`  GitHub Org:  ${linkedOrganization.github_org}`);
    console.log(`  Your Role:   ${linkedOrganization.role}`);

    if (linkedOrganization.github_linked) {
      console.log(
        `\nGitHub Account: @${linkedOrganization.github_username} (linked)`
      );
    } else {
      console.log("\nGitHub Account: Not linked");
      console.log(
        "GitHub identity is synced automatically when you log in via GitHub."
      );
    }

    console.log("");
  },
});
