/**
 * Organization status command
 *
 * Shows detailed status of an organization including GitHub App installation.
 */

import { defineCommand } from "citty";
import {
  getOrganizations,
  getOrgStatus,
  type Organization,
  type OrgStatusResponse,
} from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import {
  findOrganizationByIdOrSlug,
  selectOrganization,
} from "../../lib/ui.js";

const printAvailableOrganizations = (organizations: Organization[]): void => {
  if (organizations.length === 0) {
    return;
  }
  console.error("\nAvailable organizations:");
  for (const organization of organizations) {
    console.error(
      `  - ${organization.organization_slug} (${organization.organization_name})`
    );
  }
};

const displayOrgStatus = (status: OrgStatusResponse): void => {
  console.log("\nOrganization Status\n");
  console.log("-".repeat(50));
  console.log(`Name:         ${status.organization_name}`);
  console.log(`Slug:         ${status.organization_slug}`);
  console.log(`Provider:     ${status.provider}`);
  console.log(`Account:      ${status.provider_account_login}`);
  console.log(`Account Type: ${status.provider_account_type}`);
  console.log("-".repeat(50));

  if (status.provider === "github") {
    if (status.app_installed) {
      console.log("GitHub App:   Installed ✓");
    } else {
      console.log("GitHub App:   Not installed");
      console.log("\nRun `detent org create` to install the GitHub App.");
    }
  }

  if (status.suspended_at) {
    console.log("\n⚠️  Warning: Organization is suspended");
    console.log(
      `Suspended:    ${new Date(status.suspended_at).toLocaleString()}`
    );
  }

  console.log("-".repeat(50));
  console.log(`Projects:     ${status.project_count}`);
  console.log(`Created:      ${new Date(status.created_at).toLocaleString()}`);
  console.log("");
};

export const statusCommand = defineCommand({
  meta: {
    name: "status",
    description: "Show organization status and details",
  },
  args: {
    organization: {
      type: "positional",
      description:
        "Organization ID or slug (optional - will prompt if not provided)",
      required: false,
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

    const organizationsResponse = await getOrganizations(accessToken).catch(
      (error: unknown) => {
        console.error(
          "Failed to fetch organizations:",
          error instanceof Error ? error.message : error
        );
        process.exit(1);
      }
    );

    let selectedOrganization: Organization;

    if (args.organization) {
      const found = findOrganizationByIdOrSlug(
        organizationsResponse.organizations,
        args.organization
      );
      if (!found) {
        console.error(`Organization not found: ${args.organization}`);
        printAvailableOrganizations(organizationsResponse.organizations);
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

    const status = await getOrgStatus(
      accessToken,
      selectedOrganization.organization_id
    ).catch((error: unknown) => {
      console.error(
        "Failed to get organization status:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    });

    displayOrgStatus(status);
  },
});
