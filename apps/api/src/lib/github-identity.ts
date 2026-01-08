import { CACHE_TTL, getFromCache, setInCache } from "./cache";

interface WorkOSIdentity {
  idp_id: string;
  type: "OAuth";
  provider: string;
}

interface WorkOSIdentitiesResponse {
  data: WorkOSIdentity[];
}

interface GitHubUser {
  login: string;
}

export interface VerifiedGitHubIdentity {
  userId: string;
  username: string;
}

export const getVerifiedGitHubIdentity = async (
  workosUserId: string,
  workosApiKey: string
): Promise<VerifiedGitHubIdentity | null> => {
  const cacheKey = `github-identity:${workosUserId}`;

  // Check cache first
  const cached = getFromCache<VerifiedGitHubIdentity>(cacheKey);
  if (cached) {
    console.log(`[github-identity] Cache hit for ${workosUserId}`);
    return cached;
  }

  const identitiesResponse = await fetch(
    `https://api.workos.com/user_management/users/${workosUserId}/identities`,
    {
      headers: {
        Authorization: `Bearer ${workosApiKey}`,
      },
    }
  );

  if (!identitiesResponse.ok) {
    return null;
  }

  const identities =
    (await identitiesResponse.json()) as WorkOSIdentitiesResponse;

  const githubIdentity = identities.data?.find(
    (identity) => identity.provider === "GitHubOAuth"
  );

  if (!githubIdentity) {
    return null;
  }

  const githubUserId = githubIdentity.idp_id;

  const githubResponse = await fetch(
    `https://api.github.com/user/${githubUserId}`,
    {
      headers: {
        Accept: "application/vnd.github.v3+json",
        "User-Agent": "Detent-API",
      },
    }
  );

  if (!githubResponse.ok) {
    return null;
  }

  const githubUser = (await githubResponse.json()) as GitHubUser;

  const result: VerifiedGitHubIdentity = {
    userId: githubUserId,
    username: githubUser.login,
  };

  // Cache the result
  setInCache(cacheKey, result, CACHE_TTL.GITHUB_IDENTITY);
  console.log(`[github-identity] Cached identity for ${workosUserId}`);

  return result;
};
