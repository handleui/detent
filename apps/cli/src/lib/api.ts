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

export class ApiNetworkError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ApiNetworkError";
  }
}

export class ApiAuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ApiAuthError";
  }
}

export const apiRequest = async <T>(
  path: string,
  options: ApiOptions
): Promise<T> => {
  const { method = "GET", body, accessToken } = options;

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      method,
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: body ? JSON.stringify(body) : undefined,
    });
  } catch (error) {
    if (error instanceof TypeError && error.message.includes("fetch")) {
      throw new ApiNetworkError(
        "Network error: Unable to connect to the Detent API. Please check your internet connection."
      );
    }
    throw error;
  }

  if (!response.ok) {
    if (response.status === 401) {
      throw new ApiAuthError(
        "Authentication failed. Your session may have expired. Run `detent auth login` to re-authenticate."
      );
    }

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

// GitHub linking API methods
export const getTeams = (accessToken: string): Promise<TeamsResponse> =>
  apiRequest<TeamsResponse>("/v1/github/teams", { accessToken });

// GitHub App installation types
export interface AppStatusResponse {
  team_id: string;
  team_name: string;
  team_slug: string;
  github_org: string;
  app_installed: boolean;
  installation_id: string | null;
  installed_at: string | null;
  suspended_at: string | null;
}

export const getAppStatus = (
  accessToken: string,
  teamId: string
): Promise<AppStatusResponse> =>
  apiRequest<AppStatusResponse>(
    `/v1/github/app-status?team_id=${encodeURIComponent(teamId)}`,
    { accessToken }
  );

// Auth identity sync types
export interface SyncIdentityResponse {
  user_id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  github_synced: boolean;
  github_user_id?: string;
  github_username: string | null;
  teams_updated?: number;
}

export interface MeResponse {
  user_id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  github_linked: boolean;
  github_user_id: string | null;
  github_username: string | null;
}

// Auth API methods
export const syncIdentity = (
  accessToken: string
): Promise<SyncIdentityResponse> =>
  apiRequest<SyncIdentityResponse>("/v1/auth/sync-identity", {
    accessToken,
    method: "POST",
  });

export const getMe = (accessToken: string): Promise<MeResponse> =>
  apiRequest<MeResponse>("/v1/auth/me", { accessToken });
