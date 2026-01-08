import type { Env } from "../types/env";

// GitLab API service (stub)
// TODO: Implement GitLab integration
// Reference: https://docs.gitlab.com/ee/api/

const GITLAB_API = "https://gitlab.com/api/v4";

// Validation patterns for GitLab identifiers
// Group/project paths: alphanumeric, hyphen, underscore, period (not starting with period)
const GITLAB_PATH_PATTERN = /^[a-zA-Z0-9][-a-zA-Z0-9._]*$/;
// Full path with namespace: includes forward slashes
const GITLAB_FULL_PATH_PATTERN = /^[a-zA-Z0-9][-a-zA-Z0-9._/]*$/;

const isValidGitLabPath = (path: string): boolean => {
  return (
    path.length > 0 &&
    path.length <= 255 &&
    GITLAB_PATH_PATTERN.test(path) &&
    !path.includes("..")
  );
};

const isValidGitLabFullPath = (path: string): boolean => {
  return (
    path.length > 0 &&
    path.length <= 255 &&
    GITLAB_FULL_PATH_PATTERN.test(path) &&
    !path.includes("..") &&
    !path.startsWith("/") &&
    !path.endsWith("/")
  );
};

// API response types
interface GitLabGroup {
  id: number;
  name: string;
  path: string;
  full_path: string;
  avatar_url: string | null;
  web_url: string;
}

interface GitLabProject {
  id: number;
  name: string;
  path: string;
  path_with_namespace: string;
  default_branch: string;
  visibility: "private" | "internal" | "public";
  web_url: string;
}

interface GitLabPipeline {
  id: number;
  status: "pending" | "running" | "success" | "failed" | "canceled" | "skipped";
  ref: string;
  sha: string;
  web_url: string;
}

interface GitLabUser {
  id: number;
  username: string;
  name: string;
  avatar_url: string;
}

export interface GitLabServiceConfig {
  // GitLab instance URL (default: https://gitlab.com)
  baseUrl?: string;
}

const notImplemented = (method: string): never => {
  throw new Error(`GitLab service not yet implemented: ${method}`);
};

/**
 * Timing-safe string comparison to prevent timing attacks
 * Used for webhook signature verification
 */
const timingSafeEqual = (a: string, b: string): boolean => {
  // Pad to same length to prevent timing leaks from length comparison
  const maxLen = Math.max(a.length, b.length);
  const paddedA = a.padEnd(maxLen, "\0");
  const paddedB = b.padEnd(maxLen, "\0");

  // XOR comparison - biome-ignore: bitwise ops are intentional for crypto
  // biome-ignore lint/suspicious/noBitwiseOperators: timing-safe crypto requires XOR
  let mismatch = a.length ^ b.length;
  for (let i = 0; i < maxLen; i++) {
    // biome-ignore lint/suspicious/noBitwiseOperators: timing-safe crypto requires XOR
    mismatch |= paddedA.charCodeAt(i) ^ paddedB.charCodeAt(i);
  }
  return mismatch === 0;
};

export const createGitLabService = (
  _env: Env,
  config?: GitLabServiceConfig
) => {
  const baseUrl = config?.baseUrl ?? GITLAB_API;

  /**
   * Verify GitLab webhook signature
   * GitLab uses a simple shared secret token (X-Gitlab-Token header)
   * Uses timing-safe comparison to prevent timing attacks
   */
  const verifyWebhookSignature = (
    receivedToken: string,
    expectedSecret: string
  ): boolean => {
    return timingSafeEqual(receivedToken, expectedSecret);
  };

  /**
   * Fetch group information from GitLab API
   * TODO: Implement
   */
  const fetchGroupInfo = (
    _token: string,
    _groupIdOrPath: string
  ): Promise<GitLabGroup> => {
    // TODO: Implement GitLab API call
    // GET /groups/:id
    return Promise.reject(notImplemented("fetchGroupInfo"));
  };

  /**
   * Fetch project information from GitLab API
   * TODO: Implement
   */
  const fetchProjectInfo = (
    _token: string,
    _projectIdOrPath: string
  ): Promise<GitLabProject> => {
    // TODO: Implement GitLab API call
    // GET /projects/:id
    return Promise.reject(notImplemented("fetchProjectInfo"));
  };

  /**
   * Fetch pipeline job logs from GitLab
   * TODO: Implement
   */
  const fetchPipelineLogs = (
    _token: string,
    _projectPath: string,
    _pipelineId: number
  ): Promise<string> => {
    // TODO: Implement GitLab API call
    // GET /projects/:id/pipelines/:pipeline_id/jobs
    // Then: GET /projects/:id/jobs/:job_id/trace
    return Promise.reject(notImplemented("fetchPipelineLogs"));
  };

  /**
   * Post a comment on a merge request
   * TODO: Implement
   */
  const postMergeRequestComment = (
    _token: string,
    _projectPath: string,
    _mrIid: number,
    _body: string
  ): Promise<void> => {
    // TODO: Implement GitLab API call
    // POST /projects/:id/merge_requests/:merge_request_iid/notes
    return Promise.reject(notImplemented("postMergeRequestComment"));
  };

  /**
   * Push a commit to a GitLab repository
   * TODO: Implement
   */
  const pushCommit = (
    _token: string,
    _projectPath: string,
    _branch: string,
    _message: string,
    _files: Array<{ path: string; content: string }>
  ): Promise<string> => {
    // TODO: Implement GitLab API call
    // POST /projects/:id/repository/commits
    return Promise.reject(notImplemented("pushCommit"));
  };

  /**
   * Get merge request for a pipeline
   * TODO: Implement
   */
  const getMergeRequestForPipeline = (
    _token: string,
    _projectPath: string,
    _pipelineId: number
  ): Promise<number | null> => {
    // TODO: Implement GitLab API call
    // GET /projects/:id/pipelines/:pipeline_id
    // Check for merge_request_iid in response
    return Promise.reject(notImplemented("getMergeRequestForPipeline"));
  };

  /**
   * Get current user info from token
   * TODO: Implement
   */
  const getCurrentUser = (_token: string): Promise<GitLabUser> => {
    // TODO: Implement GitLab API call
    // GET /user
    return Promise.reject(notImplemented("getCurrentUser"));
  };

  /**
   * List projects accessible with the token
   * TODO: Implement
   */
  const listProjects = (
    _token: string,
    _groupId?: string
  ): Promise<GitLabProject[]> => {
    // TODO: Implement GitLab API call
    // GET /groups/:id/projects or GET /projects
    return Promise.reject(notImplemented("listProjects"));
  };

  return {
    // Validation helpers (exported for use by routes)
    isValidGitLabPath,
    isValidGitLabFullPath,

    // Webhook verification
    verifyWebhookSignature,

    // API methods
    fetchGroupInfo,
    fetchProjectInfo,
    fetchPipelineLogs,
    postMergeRequestComment,
    pushCommit,
    getMergeRequestForPipeline,
    getCurrentUser,
    listProjects,

    // Config
    baseUrl,
  };
};

export type GitLabService = ReturnType<typeof createGitLabService>;

// Re-export types for use by other modules
export type { GitLabGroup, GitLabProject, GitLabPipeline, GitLabUser };
