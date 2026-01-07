/**
 * Link command - links a repository to a Detent organization
 *
 * Similar to Vercel's project linking, this binds the current repo
 * to an organization for Detent operations.
 */

import { findGitRoot } from "@detent/git";
import { defineCommand } from "citty";
import type { Organization } from "../../lib/api.js";
import { getOrganizations } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { getProjectConfig, saveProjectConfig } from "../../lib/config.js";
import {
  findOrganizationByIdOrSlug,
  selectOrganization,
} from "../../lib/ui.js";

export const linkCommand = defineCommand({
  meta: {
    name: "link",
    description: "Link this repository to a Detent organization",
  },
  subCommands: {
    status: () => import("./status.js").then((m) => m.statusCommand),
    unlink: () => import("./unlink.js").then((m) => m.unlinkCommand),
  },
  args: {
    organization: {
      type: "string",
      description: "Organization ID or slug to link to",
      alias: "o",
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
        `\nThis repository is already linked to organization: ${existingConfig.organizationSlug}`
      );
      console.log(
        "Run `detent link --force` to link to a different organization."
      );
      console.log("Run `detent link status` to see details.");
      return;
    }

    // Get user's organizations
    console.log("Fetching your organizations...");
    const organizationsResponse = await getOrganizations(accessToken).catch(
      (error) => {
        console.error(
          "Failed to fetch organizations:",
          error instanceof Error ? error.message : error
        );
        process.exit(1);
      }
    );

    if (organizationsResponse.organizations.length === 0) {
      console.error("You are not a member of any organizations.");
      console.error("Ask an organization admin to invite you.");
      process.exit(1);
    }

    // Select organization
    let selectedOrganization: Organization;

    if (args.organization) {
      const found = findOrganizationByIdOrSlug(
        organizationsResponse.organizations,
        args.organization
      );
      if (!found) {
        console.error(`Organization not found: ${args.organization}`);
        console.error("\nAvailable organizations:");
        for (const organization of organizationsResponse.organizations) {
          console.error(
            `  - ${organization.organization_slug} (${organization.organization_name})`
          );
        }
        process.exit(1);
      }
      selectedOrganization = found;
    } else {
      const selected = await selectOrganization(
        organizationsResponse.organizations
      );
      if (!selected) {
        process.exit(1);
      }
      selectedOrganization = selected;
    }

    // Save project config
    saveProjectConfig(repoRoot, {
      organizationId: selectedOrganization.organization_id,
      organizationSlug: selectedOrganization.organization_slug,
    });

    console.log(
      `\nLinked to organization: ${selectedOrganization.organization_name} (${selectedOrganization.organization_slug})`
    );
    console.log("\nRun `detent link status` to see details.");
  },
});
