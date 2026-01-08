/**
 * Organization members command
 *
 * Lists all members of an organization.
 */

import { defineCommand } from "citty";
import {
  getOrganizations,
  listOrganizationMembers,
  type Organization,
  type OrganizationMember,
} from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import {
  findOrganizationByIdOrSlug,
  selectOrganization,
} from "../../lib/ui.js";

const formatRole = (role: OrganizationMember["role"]): string => {
  const roleColors: Record<OrganizationMember["role"], string> = {
    owner: "👑 owner",
    admin: "⚙️  admin",
    member: "   member",
  };
  return roleColors[role];
};

const displayMembers = (members: OrganizationMember[]): void => {
  if (members.length === 0) {
    console.log("No members found.");
    return;
  }

  // Sort by role priority: owner > admin > member
  const rolePriority = { owner: 0, admin: 1, member: 2 };
  const sorted = [...members].sort(
    (a, b) => rolePriority[a.role] - rolePriority[b.role]
  );

  console.log("\nMembers");
  console.log("-".repeat(60));

  for (const member of sorted) {
    const github = member.github_linked
      ? `@${member.github_username}`
      : "(not linked)";
    const joined = new Date(member.joined_at).toLocaleDateString();

    console.log(`${formatRole(member.role)}  ${github}  •  joined ${joined}`);
  }

  console.log("-".repeat(60));
  console.log(`Total: ${members.length} member(s)`);
};

export const membersCommand = defineCommand({
  meta: {
    name: "members",
    description: "List members of an organization",
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

    console.log(`\nOrganization: ${selectedOrganization.organization_name}`);

    const membersResponse = await listOrganizationMembers(
      accessToken,
      selectedOrganization.organization_id
    ).catch((error: unknown) => {
      console.error(
        "Failed to fetch members:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    });

    displayMembers(membersResponse.members);
  },
});
