import { createGitHubService } from "../services/github";
import type { Env } from "../types/env";

const GITHUB_API = "https://api.github.com";

interface GitHubMembershipResult {
  isMember: boolean;
  role: "admin" | "member" | null;
}

interface GitHubMembershipResponse {
  state: "active" | "pending";
  role: "admin" | "member";
  user: {
    login: string;
    id: number;
  };
}

// Check if a GitHub user is a member of the org using installation token
// Uses GET /orgs/{org}/memberships/{username} which can see private members
export const verifyGitHubMembership = async (
  githubUsername: string,
  githubOrgLogin: string,
  installationId: string,
  env: Env
): Promise<GitHubMembershipResult> => {
  const github = createGitHubService(env);

  // Get installation token for API access
  const token = await github.getInstallationToken(Number(installationId));

  // Call the membership API endpoint
  const response = await fetch(
    `${GITHUB_API}/orgs/${encodeURIComponent(githubOrgLogin)}/memberships/${encodeURIComponent(githubUsername)}`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
        Accept: "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
        "User-Agent": "Detent-App",
      },
    }
  );

  // 404 means user is not a member of the org
  if (response.status === 404) {
    console.log(
      `[github-membership] ${githubUsername} is not a member of ${githubOrgLogin}`
    );
    return { isMember: false, role: null };
  }

  if (!response.ok) {
    const error = await response.text();
    throw new Error(
      `Failed to check org membership: ${response.status} ${error}`
    );
  }

  const data = (await response.json()) as GitHubMembershipResponse;

  // Only count active members (not pending invitations)
  if (data.state !== "active") {
    console.log(
      `[github-membership] ${githubUsername} has pending membership in ${githubOrgLogin}`
    );
    return { isMember: false, role: null };
  }

  console.log(
    `[github-membership] ${githubUsername} is ${data.role} of ${githubOrgLogin}`
  );

  return {
    isMember: true,
    role: data.role,
  };
};
