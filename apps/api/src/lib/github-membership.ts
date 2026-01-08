import { createGitHubService } from "../services/github";
import type { Env } from "../types/env";
import { CACHE_TTL, getFromCache, setInCache } from "./cache";

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
  const cacheKey = `github-membership:${githubUsername}:${githubOrgLogin}`;

  // Check cache first
  const cached = getFromCache<GitHubMembershipResult>(cacheKey);
  if (cached) {
    console.log(
      `[github-membership] Cache hit for ${githubUsername}@${githubOrgLogin}`
    );
    return cached;
  }

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
    const result: GitHubMembershipResult = { isMember: false, role: null };
    // Cache non-membership for shorter time (they might get added)
    setInCache(cacheKey, result, CACHE_TTL.GITHUB_MEMBERSHIP);
    return result;
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
    const result: GitHubMembershipResult = { isMember: false, role: null };
    // Cache pending status for shorter time
    setInCache(cacheKey, result, CACHE_TTL.GITHUB_MEMBERSHIP);
    return result;
  }

  console.log(
    `[github-membership] ${githubUsername} is ${data.role} of ${githubOrgLogin}`
  );

  const result: GitHubMembershipResult = {
    isMember: true,
    role: data.role,
  };

  // Cache successful membership
  setInCache(cacheKey, result, CACHE_TTL.GITHUB_MEMBERSHIP);

  return result;
};
