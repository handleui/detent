import type { Env } from "../types/env";

// GitHub App API service
// Handles: JWT generation, installation tokens, API calls

const GITHUB_API = "https://api.github.com";

// Top-level regex constants for performance
const PEM_BEGIN_RSA = /-----BEGIN RSA PRIVATE KEY-----/;
const PEM_END_RSA = /-----END RSA PRIVATE KEY-----/;
const PEM_BEGIN_PKCS8 = /-----BEGIN PRIVATE KEY-----/;
const PEM_END_PKCS8 = /-----END PRIVATE KEY-----/;
const WHITESPACE = /\s/g;
const BASE64_TRAILING_EQUALS = /=+$/;

// Validation patterns for GitHub identifiers
// Owner/repo names: alphanumeric, hyphen, underscore, period (not starting with period)
const GITHUB_NAME_PATTERN = /^[a-zA-Z0-9][-a-zA-Z0-9._]*$/;
// Branch names: more permissive but no path traversal
const GITHUB_BRANCH_PATTERN = /^[a-zA-Z0-9][-a-zA-Z0-9._/]*$/;

const isValidGitHubName = (name: string): boolean => {
  return (
    name.length > 0 &&
    name.length <= 100 &&
    GITHUB_NAME_PATTERN.test(name) &&
    !name.includes("..")
  );
};

const isValidBranchName = (branch: string): boolean => {
  return (
    branch.length > 0 &&
    branch.length <= 255 &&
    GITHUB_BRANCH_PATTERN.test(branch) &&
    !branch.includes("..") &&
    !branch.startsWith("/") &&
    !branch.endsWith("/")
  );
};

interface GitHubServiceConfig {
  appId: string;
  privateKey: string;
}

// Convert PEM to ArrayBuffer for Web Crypto API
const pemToArrayBuffer = (pem: string): ArrayBuffer => {
  const base64 = pem
    .replace(PEM_BEGIN_RSA, "")
    .replace(PEM_END_RSA, "")
    .replace(PEM_BEGIN_PKCS8, "")
    .replace(PEM_END_PKCS8, "")
    .replace(WHITESPACE, "");
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
};

// Base64URL encode (JWT-safe)
const base64UrlEncode = (data: ArrayBuffer | string): string => {
  const bytes =
    typeof data === "string" ? new TextEncoder().encode(data) : data;
  const binary = Array.from(new Uint8Array(bytes))
    .map((b) => String.fromCharCode(b))
    .join("");
  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(BASE64_TRAILING_EQUALS, "");
};

// Generate JWT for GitHub App authentication (RS256)
const generateAppJwt = async (config: GitHubServiceConfig): Promise<string> => {
  const now = Math.floor(Date.now() / 1000);

  const header = { alg: "RS256", typ: "JWT" };
  const payload = {
    iat: now - 60, // 60 seconds in the past to account for clock drift
    exp: now + 600, // 10 minutes from now (max allowed)
    iss: config.appId,
  };

  const encodedHeader = base64UrlEncode(JSON.stringify(header));
  const encodedPayload = base64UrlEncode(JSON.stringify(payload));
  const signingInput = `${encodedHeader}.${encodedPayload}`;

  // Import the private key
  const keyData = pemToArrayBuffer(config.privateKey);

  // Try PKCS#8 first, fall back to PKCS#1
  let privateKey: CryptoKey;
  try {
    privateKey = await crypto.subtle.importKey(
      "pkcs8",
      keyData,
      { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
      false,
      ["sign"]
    );
  } catch {
    // GitHub generates PKCS#1 keys, need to convert or use different format
    throw new Error(
      "Failed to import private key. Ensure it's in PKCS#8 format. " +
        "Convert with: openssl pkcs8 -topk8 -inform PEM -outform PEM -nocrypt -in key.pem -out key-pkcs8.pem"
    );
  }

  // Sign the JWT
  const signature = await crypto.subtle.sign(
    "RSASSA-PKCS1-v1_5",
    privateKey,
    new TextEncoder().encode(signingInput)
  );

  return `${signingInput}.${base64UrlEncode(signature)}`;
};

// API response types
interface InstallationTokenResponse {
  token: string;
  expires_at: string;
}

interface WorkflowRunResponse {
  pull_requests: Array<{ number: number }>;
}

interface GitTreeItem {
  path: string;
  mode: "100644";
  type: "blob";
  content: string;
}

interface CreateTreeResponse {
  sha: string;
}

interface CreateCommitResponse {
  sha: string;
}

interface RefResponse {
  object: { sha: string };
}

interface InstallationInfo {
  id: number;
  account: {
    id: number;
    login: string;
    type: "Organization" | "User";
    avatar_url?: string;
  };
  suspended_at: string | null;
}

interface InstallationReposResponse {
  total_count: number;
  repositories: Array<{
    id: number;
    name: string;
    full_name: string;
    private: boolean;
    default_branch: string;
  }>;
}

export const createGitHubService = (env: Env) => {
  const config: GitHubServiceConfig = {
    appId: env.GITHUB_APP_ID,
    privateKey: env.GITHUB_APP_PRIVATE_KEY,
  };

  // Cache for installation tokens (they last 1 hour)
  const tokenCache = new Map<number, { token: string; expiresAt: number }>();

  const getInstallationToken = async (
    installationId: number
  ): Promise<string> => {
    // Check cache first
    const cached = tokenCache.get(installationId);
    if (cached && cached.expiresAt > Date.now() + 60_000) {
      return cached.token;
    }

    // Generate app JWT
    const jwt = await generateAppJwt(config);

    // Exchange JWT for installation token
    const response = await fetch(
      `${GITHUB_API}/app/installations/${installationId}/access_tokens`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${jwt}`,
          Accept: "application/vnd.github+json",
          "X-GitHub-Api-Version": "2022-11-28",
          "User-Agent": "Detent-App",
        },
      }
    );

    if (!response.ok) {
      const error = await response.text();
      throw new Error(
        `Failed to get installation token: ${response.status} ${error}`
      );
    }

    const data = (await response.json()) as InstallationTokenResponse;

    // Cache the token
    tokenCache.set(installationId, {
      token: data.token,
      expiresAt: new Date(data.expires_at).getTime(),
    });

    console.log(
      `[github] Got installation token for ${installationId} (expires: ${data.expires_at})`
    );

    return data.token;
  };

  const fetchWorkflowLogs = async (
    token: string,
    owner: string,
    repo: string,
    runId: number
  ): Promise<string> => {
    // Validate inputs to prevent URL manipulation
    if (!(isValidGitHubName(owner) && isValidGitHubName(repo))) {
      throw new Error("Invalid owner or repo name");
    }

    // GitHub returns a redirect to a zip file containing logs
    const response = await fetch(
      `${GITHUB_API}/repos/${owner}/${repo}/actions/runs/${runId}/logs`,
      {
        headers: {
          Authorization: `Bearer ${token}`,
          Accept: "application/vnd.github+json",
          "X-GitHub-Api-Version": "2022-11-28",
          "User-Agent": "Detent-App",
        },
        redirect: "follow",
      }
    );

    if (!response.ok) {
      throw new Error(`Failed to fetch logs: ${response.status}`);
    }

    // Response is a zip file - we need to extract it
    // For now, return the raw response to be processed by the caller
    // In production, we'd use a zip library to extract specific job logs
    const blob = await response.blob();
    console.log(
      `[github] Fetched logs for ${owner}/${repo} run ${runId} (${blob.size} bytes)`
    );

    // Future: Unzip and extract relevant job logs
    // Log extraction will require a zip library (e.g., pako or fflate)
    return `[Log archive: ${blob.size} bytes - extraction not yet implemented]`;
  };

  const postComment = async (
    token: string,
    owner: string,
    repo: string,
    issueNumber: number,
    body: string
  ): Promise<void> => {
    // Validate inputs to prevent URL manipulation
    if (!(isValidGitHubName(owner) && isValidGitHubName(repo))) {
      throw new Error("Invalid owner or repo name");
    }

    const response = await fetch(
      `${GITHUB_API}/repos/${owner}/${repo}/issues/${issueNumber}/comments`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          Accept: "application/vnd.github+json",
          "X-GitHub-Api-Version": "2022-11-28",
          "User-Agent": "Detent-App",
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ body }),
      }
    );

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Failed to post comment: ${response.status} ${error}`);
    }

    console.log(`[github] Posted comment to ${owner}/${repo}#${issueNumber}`);
  };

  const pushCommit = async (
    token: string,
    owner: string,
    repo: string,
    branch: string,
    message: string,
    files: Array<{ path: string; content: string }>
  ): Promise<string> => {
    // Validate inputs to prevent URL manipulation
    if (!(isValidGitHubName(owner) && isValidGitHubName(repo))) {
      throw new Error("Invalid owner or repo name");
    }
    if (!isValidBranchName(branch)) {
      throw new Error("Invalid branch name");
    }

    const headers = {
      Authorization: `Bearer ${token}`,
      Accept: "application/vnd.github+json",
      "X-GitHub-Api-Version": "2022-11-28",
      "User-Agent": "Detent-App",
      "Content-Type": "application/json",
    };

    // 1. Get current commit SHA for the branch
    const refResponse = await fetch(
      `${GITHUB_API}/repos/${owner}/${repo}/git/ref/heads/${branch}`,
      { headers }
    );

    if (!refResponse.ok) {
      throw new Error(`Failed to get branch ref: ${refResponse.status}`);
    }

    const refData = (await refResponse.json()) as RefResponse;
    const baseSha = refData.object.sha;

    // 2. Build tree items with content (GitHub creates blobs automatically)
    const treeItems: GitTreeItem[] = files.map((file) => ({
      path: file.path,
      mode: "100644" as const,
      type: "blob" as const,
      content: file.content,
    }));

    // 3. Create tree (GitHub will create blobs from content)
    const treeResponse = await fetch(
      `${GITHUB_API}/repos/${owner}/${repo}/git/trees`,
      {
        method: "POST",
        headers,
        body: JSON.stringify({
          base_tree: baseSha,
          tree: treeItems,
        }),
      }
    );

    if (!treeResponse.ok) {
      throw new Error(`Failed to create tree: ${treeResponse.status}`);
    }

    const treeData = (await treeResponse.json()) as CreateTreeResponse;

    // 4. Create commit
    const commitResponse = await fetch(
      `${GITHUB_API}/repos/${owner}/${repo}/git/commits`,
      {
        method: "POST",
        headers,
        body: JSON.stringify({
          message,
          tree: treeData.sha,
          parents: [baseSha],
        }),
      }
    );

    if (!commitResponse.ok) {
      throw new Error(`Failed to create commit: ${commitResponse.status}`);
    }

    const commitData = (await commitResponse.json()) as CreateCommitResponse;

    // 5. Update branch ref
    const updateRefResponse = await fetch(
      `${GITHUB_API}/repos/${owner}/${repo}/git/refs/heads/${branch}`,
      {
        method: "PATCH",
        headers,
        body: JSON.stringify({
          sha: commitData.sha,
        }),
      }
    );

    if (!updateRefResponse.ok) {
      throw new Error(`Failed to update ref: ${updateRefResponse.status}`);
    }

    console.log(
      `[github] Pushed commit ${commitData.sha.slice(0, 7)} to ${owner}/${repo}:${branch}`
    );

    return commitData.sha;
  };

  const getPullRequestForRun = async (
    token: string,
    owner: string,
    repo: string,
    runId: number
  ): Promise<number | null> => {
    // Validate inputs to prevent URL manipulation
    if (!(isValidGitHubName(owner) && isValidGitHubName(repo))) {
      throw new Error("Invalid owner or repo name");
    }

    const response = await fetch(
      `${GITHUB_API}/repos/${owner}/${repo}/actions/runs/${runId}`,
      {
        headers: {
          Authorization: `Bearer ${token}`,
          Accept: "application/vnd.github+json",
          "X-GitHub-Api-Version": "2022-11-28",
          "User-Agent": "Detent-App",
        },
      }
    );

    if (!response.ok) {
      throw new Error(`Failed to get workflow run: ${response.status}`);
    }

    const data = (await response.json()) as WorkflowRunResponse;

    const firstPR = data.pull_requests[0];
    return firstPR?.number ?? null;
  };

  const getInstallationInfo = async (
    installationId: number
  ): Promise<InstallationInfo | null> => {
    // Generate app JWT to call app-level endpoints
    const jwt = await generateAppJwt(config);

    const response = await fetch(
      `${GITHUB_API}/app/installations/${installationId}`,
      {
        headers: {
          Authorization: `Bearer ${jwt}`,
          Accept: "application/vnd.github+json",
          "X-GitHub-Api-Version": "2022-11-28",
          "User-Agent": "Detent-App",
        },
      }
    );

    if (response.status === 404) {
      // Installation not found (uninstalled)
      return null;
    }

    if (!response.ok) {
      const error = await response.text();
      throw new Error(
        `Failed to get installation info: ${response.status} ${error}`
      );
    }

    return (await response.json()) as InstallationInfo;
  };

  const getInstallationRepos = async (
    installationId: number
  ): Promise<InstallationReposResponse["repositories"]> => {
    const token = await getInstallationToken(installationId);
    const allRepos: InstallationReposResponse["repositories"] = [];
    let page = 1;
    const perPage = 100;

    // Paginate through all repos
    while (true) {
      const response = await fetch(
        `${GITHUB_API}/installation/repositories?per_page=${perPage}&page=${page}`,
        {
          headers: {
            Authorization: `Bearer ${token}`,
            Accept: "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "Detent-App",
          },
        }
      );

      if (!response.ok) {
        const error = await response.text();
        throw new Error(
          `Failed to get installation repos: ${response.status} ${error}`
        );
      }

      const data = (await response.json()) as InstallationReposResponse;
      allRepos.push(...data.repositories);

      // Check if we've fetched all repos
      if (allRepos.length >= data.total_count) {
        break;
      }
      page++;
    }

    return allRepos;
  };

  return {
    getInstallationToken,
    getInstallationInfo,
    getInstallationRepos,
    fetchWorkflowLogs,
    postComment,
    pushCommit,
    getPullRequestForRun,
  };
};

export type GitHubService = ReturnType<typeof createGitHubService>;
