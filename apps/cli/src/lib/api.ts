/**
 * Detent API client for CLI
 *
 * Handles authenticated requests to the Detent API.
 */

const API_BASE_URL = process.env.DETENT_API_URL ?? "https://api.detent.dev";

interface ApiOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: unknown;
  accessToken: string;
}

interface ApiError {
  error: string;
}

export const apiRequest = async <T>(
  path: string,
  options: ApiOptions
): Promise<T> => {
  const { method = "GET", body, accessToken } = options;

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${accessToken}`,
      "Content-Type": "application/json",
    },
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    const errorData = (await response.json().catch(() => ({}))) as ApiError;
    throw new Error(
      errorData.error ?? `API request failed: ${response.status}`
    );
  }

  return response.json() as Promise<T>;
};

// GitHub linking types
export interface Team {
  team_id: string;
  team_name: string;
  team_slug: string;
  github_org: string;
  role: string;
  github_linked: boolean;
  github_username: string | null;
}

export interface TeamsResponse {
  teams: Team[];
}

export interface AuthorizeResponse {
  authorization_url: string;
  state: string;
}

export interface CallbackResponse {
  success: boolean;
  github_user_id: string;
  github_username: string;
}

export interface StatusResponse {
  team_id: string;
  team_name: string;
  team_slug: string;
  github_org: string;
  github_linked: boolean;
  github_user_id: string | null;
  github_username: string | null;
  github_linked_at: string | null;
}

// GitHub linking API methods
export const getTeams = (accessToken: string): Promise<TeamsResponse> =>
  apiRequest<TeamsResponse>("/v1/github/teams", { accessToken });

export const getAuthorizeUrl = (
  accessToken: string,
  teamId: string,
  redirectUri: string,
  codeChallenge: string
): Promise<AuthorizeResponse> =>
  apiRequest<AuthorizeResponse>(
    `/v1/github/authorize?team_id=${encodeURIComponent(teamId)}&redirect_uri=${encodeURIComponent(redirectUri)}&code_challenge=${encodeURIComponent(codeChallenge)}`,
    { accessToken }
  );

export const submitCallback = (
  accessToken: string,
  code: string,
  state: string,
  teamId: string,
  codeVerifier: string
): Promise<CallbackResponse> =>
  apiRequest<CallbackResponse>("/v1/github/callback", {
    accessToken,
    method: "POST",
    body: { code, state, team_id: teamId, code_verifier: codeVerifier },
  });

export const getLinkStatus = (
  accessToken: string,
  teamId: string
): Promise<StatusResponse> =>
  apiRequest<StatusResponse>(
    `/v1/github/status?team_id=${encodeURIComponent(teamId)}`,
    { accessToken }
  );

export interface UnlinkResponse {
  success: boolean;
}

export const unlinkGithub = (
  accessToken: string,
  teamId: string
): Promise<UnlinkResponse> =>
  apiRequest<UnlinkResponse>("/v1/github/unlink", {
    accessToken,
    method: "POST",
    body: { team_id: teamId },
  });
