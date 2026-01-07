/**
 * Organization install command
 *
 * OAuth-first GitHub App installation flow.
 * Lists user's GitHub organizations and opens browser to install on selected org.
 */

import { defineCommand } from "citty";
import { apiRequest } from "../../lib/api.js";
import { getAccessToken } from "../../lib/auth.js";

// Types for the github-orgs endpoint response
interface GitHubOrgWithStatus {
  id: number;
  login: string;
  avatar_url: string;
  can_install: boolean;
  already_installed: boolean;
  detent_org_id?: string;
}

interface GitHubOrgsResponse {
  orgs: GitHubOrgWithStatus[];
}

interface InstallUrlResponse {
  url: string;
}

// Fetch GitHub organizations from API
const getGitHubOrgs = (accessToken: string): Promise<GitHubOrgsResponse> =>
  apiRequest<GitHubOrgsResponse>("/v1/auth/github-orgs", { accessToken });

// Get installation URL for a specific organization
const getInstallUrl = (
  accessToken: string,
  targetId: number
): Promise<InstallUrlResponse> =>
  apiRequest<InstallUrlResponse>(
    `/v1/auth/install-url?target_id=${encodeURIComponent(String(targetId))}`,
    { accessToken }
  );

// Open browser cross-platform
const openBrowser = async (url: string): Promise<void> => {
  const { platform } = process;
  const { exec } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execAsync = promisify(exec);

  const commands: Record<string, string> = {
    darwin: `open "${url}"`,
    win32: `start "" "${url}"`,
    linux: `xdg-open "${url}"`,
  };

  const command = commands[platform];
  if (!command) {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  await execAsync(command);
};

// Format org display with status
const formatOrgLine = (
  org: GitHubOrgWithStatus,
  index: number
): { line: string; selectable: boolean } => {
  if (org.already_installed) {
    return {
      line: `  ${index + 1}. ${org.login} ✓ (already installed)`,
      selectable: false,
    };
  }

  if (org.can_install) {
    return {
      line: `  ${index + 1}. ${org.login} → Can install`,
      selectable: true,
    };
  }

  return {
    line: `  ${index + 1}. ${org.login} → Member only (ask admin)`,
    selectable: false,
  };
};

// Prompt user to select an organization
const selectGitHubOrg = async (
  orgs: GitHubOrgWithStatus[]
): Promise<GitHubOrgWithStatus | null> => {
  console.log("\nSelect a GitHub organization to install Detent:\n");

  const selectableIndices: number[] = [];
  for (const [i, org] of orgs.entries()) {
    const { line, selectable } = formatOrgLine(org, i);
    console.log(line);
    if (selectable) {
      selectableIndices.push(i);
    }
  }

  if (selectableIndices.length === 0) {
    console.log("\nNo organizations available for installation.");
    console.log("You need admin access to install the Detent GitHub App.");
    return null;
  }

  const readline = await import("node:readline");
  const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  const answer = await new Promise<string>((resolve) => {
    rl.question("\n> ", resolve);
  });
  rl.close();

  const index = Number.parseInt(answer, 10) - 1;
  if (Number.isNaN(index) || index < 0 || index >= orgs.length) {
    console.error("\nInvalid selection");
    return null;
  }

  const selectedOrg = orgs[index];
  if (!selectedOrg) {
    console.error("\nInvalid selection");
    return null;
  }

  if (selectedOrg.already_installed) {
    console.error(
      `\n${selectedOrg.login} is already installed. Run 'detent org list' to see your organizations.`
    );
    return null;
  }

  if (!selectedOrg.can_install) {
    console.error(
      `\nYou don't have admin access to ${selectedOrg.login}. Ask an organization admin to install Detent.`
    );
    return null;
  }

  return selectedOrg;
};

// Find organization by name (for --org flag)
const findOrgByName = (
  orgs: GitHubOrgWithStatus[],
  name: string
): GitHubOrgWithStatus | undefined =>
  orgs.find((org) => org.login.toLowerCase() === name.toLowerCase());

// Handle GitHub orgs API errors
const handleGitHubOrgsError = (error: unknown): never => {
  const message = error instanceof Error ? error.message : String(error);

  if (message.includes("GitHub account not connected")) {
    console.error("GitHub account not connected.");
    console.error(
      "Please authenticate with GitHub: run `detent auth login --force`"
    );
  } else if (message.includes("authorization expired")) {
    console.error("GitHub authorization expired.");
    console.error("Please re-authenticate: run `detent auth login --force`");
  } else {
    console.error("Failed to fetch GitHub organizations:", message);
  }
  process.exit(1);
};

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

// Open installation URL and print messages
const openInstallationUrl = async (
  installUrl: string,
  orgLogin: string
): Promise<void> => {
  console.log(`\nOpening browser to install Detent on ${orgLogin}...`);
  console.log(
    "After installing, run 'detent org list' to see your organizations.\n"
  );

  try {
    await openBrowser(installUrl);
  } catch (error) {
    console.error(
      "Failed to open browser:",
      error instanceof Error ? error.message : error
    );
    console.error(`\nPlease manually visit: ${installUrl}`);
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
        "GitHub organization name to install on (skips interactive selection)",
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

    // Fetch GitHub organizations
    const orgsResponse = await getGitHubOrgs(accessToken).catch(
      handleGitHubOrgsError
    );

    if (orgsResponse.orgs.length === 0) {
      console.log("No GitHub organizations found.");
      console.log(
        "\nYou need to be a member of a GitHub organization to install Detent."
      );
      console.log(
        "For personal accounts, install directly at: https://github.com/apps/detent"
      );
      process.exit(0);
    }

    // Select organization (interactive or via --org flag)
    const selectedOrg = args.org
      ? validateOrgFromFlag(orgsResponse.orgs, args.org)
      : await selectGitHubOrg(orgsResponse.orgs);

    if (!selectedOrg) {
      process.exit(1);
    }

    // Get installation URL
    let installUrl: string;
    try {
      const response = await getInstallUrl(accessToken, selectedOrg.id);
      installUrl = response.url;
    } catch (error) {
      console.error(
        "Failed to get installation URL:",
        error instanceof Error ? error.message : error
      );
      process.exit(1);
    }

    await openInstallationUrl(installUrl, selectedOrg.login);
  },
});
