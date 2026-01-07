/**
 * Shared UI utilities for the Detent CLI
 */

import type { Organization } from "./api.js";

/**
 * Find an organization by ID or slug
 *
 * Returns the matching organization or undefined if not found.
 */
export const findOrganizationByIdOrSlug = (
  organizations: Organization[],
  idOrSlug: string
): Organization | undefined =>
  organizations.find(
    (o) => o.organization_id === idOrSlug || o.organization_slug === idOrSlug
  );

/**
 * Prompt user to select an organization from a list
 *
 * Returns the selected organization or null if no valid selection was made.
 */
export const selectOrganization = async (
  organizations: Organization[]
): Promise<Organization | null> => {
  if (organizations.length === 0) {
    console.error(
      "You are not a member of any organizations. Ask an organization admin to invite you."
    );
    return null;
  }

  const firstOrganization = organizations[0];
  if (organizations.length === 1 && firstOrganization) {
    return firstOrganization;
  }

  // Multiple organizations - let user select
  console.log("\nYou are a member of multiple organizations:\n");
  for (const [i, organization] of organizations.entries()) {
    const linked = organization.github_linked
      ? `(linked: @${organization.github_username})`
      : "(not linked)";
    console.log(
      `  ${i + 1}. ${organization.organization_name} (${organization.github_org}) ${linked}`
    );
  }

  const readline = await import("node:readline");
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  const answer = await new Promise<string>((resolve) => {
    rl.question("\nSelect organization number: ", resolve);
  });
  rl.close();

  const index = Number.parseInt(answer, 10) - 1;
  if (Number.isNaN(index) || index < 0 || index >= organizations.length) {
    console.error("Invalid selection");
    return null;
  }

  const selectedOrganization = organizations[index];
  return selectedOrganization ?? null;
};
