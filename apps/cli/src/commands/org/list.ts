/**
 * Organization list command
 *
 * Lists all organizations the user is a member of, with their projects.
 * With --available flag, shows all GitHub orgs the user belongs to.
 */

import { defineCommand } from "citty";
import {
  type GitHubOrgWithStatus,
  getGitHubOrgs,
  getOrganizations,
  listProjects,
  type Organization,
  type Project,
} from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { handleGitHubOrgError } from "../../lib/errors.js";

const formatVisibility = (isPrivate: boolean): string =>
  isPrivate ? "private" : "public";

const displayOrganizationWithProjects = (
  organization: Organization,
  projects: Project[]
): void => {
  const githubStatus = organization.github_linked
    ? `@${organization.github_username}`
    : "not linked";

  console.log(`\n┌─ ${organization.organization_name}`);
  console.log(`│  Slug:       ${organization.organization_slug}`);
  console.log(`│  GitHub Org: ${organization.github_org}`);
  console.log(`│  Role:       ${organization.role}`);
  console.log(`│  GitHub:     ${githubStatus}`);

  if (projects.length === 0) {
    console.log("│");
    console.log("│  No projects yet");
  } else {
    console.log("│");
    console.log("│  Projects:");
    for (const project of projects) {
      const visibility = formatVisibility(project.is_private);
      const branch = project.provider_default_branch ?? "—";
      console.log(
        `│    • ${project.provider_repo_full_name}  (${visibility}, ${branch})`
      );
    }
  }
  console.log(`└${"─".repeat(50)}`);
};

const displayOrganizationSimple = (organization: Organization): void => {
  const githubStatus = organization.github_linked
    ? `@${organization.github_username}`
    : "not linked";

  console.log(`${organization.organization_name}`);
  console.log(`  Slug:       ${organization.organization_slug}`);
  console.log(`  GitHub Org: ${organization.github_org}`);
  console.log(`  Role:       ${organization.role}`);
  console.log(`  GitHub:     ${githubStatus}`);
  console.log("");
};

const getStatusText = (org: GitHubOrgWithStatus): string => {
  if (org.already_installed) {
    return "Installed";
  }
  return "Not installed";
};

const getActionText = (org: GitHubOrgWithStatus): string => {
  if (org.already_installed) {
    return "detent org list (default)";
  }
  if (org.can_install) {
    return "detent org install";
  }
  return "Ask org admin";
};

const displayAvailableOrgs = (orgs: GitHubOrgWithStatus[]): void => {
  console.log("\nAvailable GitHub Organizations:\n");

  // Calculate column widths
  const nameWidth = Math.max(
    "NAME".length,
    ...orgs.map((org) => org.login.length)
  );
  const statusWidth = Math.max(
    "STATUS".length,
    ...orgs.map((org) => getStatusText(org).length)
  );

  // Print header
  const header = `  ${"NAME".padEnd(nameWidth)}  ${"STATUS".padEnd(statusWidth)}  ACTION`;
  console.log(header);

  // Print rows
  for (const org of orgs) {
    const status = getStatusText(org);
    const action = getActionText(org);
    console.log(
      `  ${org.login.padEnd(nameWidth)}  ${status.padEnd(statusWidth)}  ${action}`
    );
  }

  console.log(
    "\nUse 'detent org install' to install Detent on an organization."
  );
};

const listAvailableOrgs = async (accessToken: string): Promise<void> => {
  try {
    const response = await getGitHubOrgs(accessToken);

    if (response.orgs.length === 0) {
      console.log("No GitHub organizations found.\n");
      console.log(
        "You need to be a member of a GitHub organization to install Detent."
      );
      return;
    }

    displayAvailableOrgs(response.orgs);
  } catch (error) {
    handleGitHubOrgError(error);
  }
};

const listMemberOrgs = async (
  accessToken: string,
  showProjects: boolean
): Promise<void> => {
  const response = await getOrganizations(accessToken);

  if (response.organizations.length === 0) {
    console.log("You are not a member of any organizations.\n");
    console.log(
      "To install Detent on a GitHub organization, run: detent org install"
    );
    return;
  }

  console.log("\nYour Organizations");
  console.log("=".repeat(55));

  if (showProjects) {
    // Fetch projects for all organizations in parallel
    const projectsByOrg = await Promise.all(
      response.organizations.map(async (org) => {
        try {
          const projectsResponse = await listProjects(
            accessToken,
            org.organization_id
          );
          return { org, projects: projectsResponse.projects };
        } catch {
          return { org, projects: [] };
        }
      })
    );

    let totalProjects = 0;
    for (const { org, projects } of projectsByOrg) {
      displayOrganizationWithProjects(org, projects);
      totalProjects += projects.length;
    }

    console.log("");
    console.log(
      `Total: ${response.organizations.length} organization(s), ${totalProjects} project(s)`
    );
  } else {
    console.log("");
    for (const organization of response.organizations) {
      displayOrganizationSimple(organization);
    }
    console.log("-".repeat(55));
    console.log(`Total: ${response.organizations.length} organization(s)`);
  }
};

export const listCommand = defineCommand({
  meta: {
    name: "list",
    description: "List organizations and their projects",
  },
  args: {
    projects: {
      type: "boolean",
      description: "Show projects under each organization (default: true)",
      default: true,
    },
    available: {
      type: "boolean",
      alias: "a",
      description: "Show all GitHub orgs you belong to (installed and not)",
      default: false,
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

    // Handle --available flag: show all GitHub orgs
    if (args.available) {
      await listAvailableOrgs(accessToken);
      return;
    }

    // Default behavior: show organizations the user is a member of
    try {
      await listMemberOrgs(accessToken, args.projects);
    } catch (error) {
      console.error(
        "Failed to fetch organizations:",
        error instanceof Error ? error.message : String(error)
      );
      process.exit(1);
    }
  },
});
