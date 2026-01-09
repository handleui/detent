/**
 * Organization install command
 *
 * OAuth-first GitHub App installation flow.
 * Shows user's GitHub organizations status and opens browser to GitHub App install page.
 */

import { defineCommand } from "citty";
import { type GitHubOrgWithStatus, getGitHubOrgs } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";
import { openBrowser } from "../../lib/browser.js";
import { handleGitHubOrgError } from "../../lib/errors.js";

const GITHUB_APP_INSTALL_URL =
  "https://github.com/apps/detent/installations/new";

// Display org status (info only, no selection)
const displayOrgStatus = (orgs: GitHubOrgWithStatus[]): void => {
  console.log("\nYour GitHub organizations:\n");

  for (const org of orgs) {
    if (org.already_installed) {
      console.log(`  ✓ ${org.login} (already installed)`);
    } else if (org.can_install) {
      console.log(`  ● ${org.login} (admin - can install)`);
    } else {
      console.log(`  ○ ${org.login} (member - ask admin to install)`);
    }
  }
};

// Find organization by name (for --org flag)
const findOrgByName = (
  orgs: GitHubOrgWithStatus[],
  name: string
): GitHubOrgWithStatus | undefined =>
  orgs.find((org) => org.login.toLowerCase() === name.toLowerCase());

// Validate organization from --org flag
const validateOrgFromFlag = (
  orgs: GitHubOrgWithStatus[],
  orgName: string
): GitHubOrgWithStatus => {
  const found = findOrgByName(orgs, orgName);

  if (!found) {
    console.error(`GitHub organization not found: ${orgName}`);
    console.error("\nAvailable organizations:");
    for (const org of orgs) {
      console.error(`  - ${org.login}`);
    }
    process.exit(1);
  }

  if (found.already_installed) {
    console.log(
      `${found.login} is already installed. Run 'detent org list' to see your organizations.`
    );
    process.exit(0);
  }

  if (!found.can_install) {
    console.error(
      `You don't have admin access to ${found.login}. Ask an organization admin to install Detent.`
    );
    process.exit(1);
  }

  return found;
};

// Open browser with appropriate messaging
const openInstallPage = async (
  url: string,
  orgLogin?: string
): Promise<void> => {
  if (orgLogin) {
    console.log(`\nOpening GitHub to install Detent on ${orgLogin}...`);
  } else {
    console.log("\nOpening GitHub to install Detent...");
  }
  console.log(
    "After installing, run 'detent org list' to see your organizations.\n"
  );

  try {
    await openBrowser(url);
  } catch (error) {
    console.error(
      "Failed to open browser:",
      error instanceof Error ? error.message : error
    );
    console.error(`\nPlease manually visit: ${url}`);
  }
};

export const installCommand = defineCommand({
  meta: {
    name: "install",
    description: "Install the Detent GitHub App on a GitHub organization",
  },
  args: {
    org: {
      type: "string",
      description:
        "GitHub organization name (pre-selects org in GitHub install page)",
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

    // Fetch GitHub organizations to show status
    const orgsResponse =
      await getGitHubOrgs(accessToken).catch(handleGitHubOrgError);

    // If --org flag provided, validate and open with target_id
    if (args.org) {
      if (orgsResponse.orgs.length === 0) {
        console.error(`GitHub organization not found: ${args.org}`);
        process.exit(1);
      }

      const selectedOrg = validateOrgFromFlag(orgsResponse.orgs, args.org);
      const installUrl = `${GITHUB_APP_INSTALL_URL}?target_id=${selectedOrg.id}`;
      await openInstallPage(installUrl, selectedOrg.login);
      return;
    }

    // Show org status (info only)
    if (orgsResponse.orgs.length > 0) {
      displayOrgStatus(orgsResponse.orgs);

      // Check if all orgs are already installed
      const allInstalled = orgsResponse.orgs.every(
        (org) => org.already_installed
      );
      if (allInstalled) {
        console.log("\nAll your organizations already have Detent installed.");
        console.log("Run 'detent org list' to see your organizations.");
        return;
      }

      // Check if user can install on any org
      const canInstallAny = orgsResponse.orgs.some((org) => org.can_install);
      if (!canInstallAny) {
        console.log(
          "\nYou need admin access to install the Detent GitHub App."
        );
        console.log(
          "Ask an organization admin to install, or use a personal account."
        );
      }
    } else {
      console.log("No GitHub organizations found.");
      console.log(
        "\nYou can still install Detent on your personal GitHub account."
      );
    }

    // Open GitHub install page (user picks org there)
    await openInstallPage(GITHUB_APP_INSTALL_URL);
  },
});
