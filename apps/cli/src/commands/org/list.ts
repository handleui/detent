/**
 * Organization list command
 *
 * Lists all organizations the user is a member of, with their projects.
 */

import { defineCommand } from "citty";
import {
  getOrganizations,
  listProjects,
  type Organization,
  type Project,
} from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";

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
  },
  run: async ({ args }) => {
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

      console.log("\nYour Organizations");
      console.log("=".repeat(55));

      if (args.projects) {
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
    } catch (error) {
      console.error(
        "Failed to fetch organizations:",
        error instanceof Error ? error.message : String(error)
      );
      process.exit(1);
    }
  },
});
