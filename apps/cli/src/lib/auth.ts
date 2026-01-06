/**
 * WorkOS Device Authorization Flow for CLI authentication
 *
 * Implements OAuth 2.0 Device Authorization Grant (RFC 8628)
 * for authenticating CLI users through their browser.
 */

import { decodeJwt } from "jose";
import type { Credentials } from "./credentials.js";
import {
  isTokenExpired,
  loadCredentials,
  saveCredentials,
} from "./credentials.js";

const WORKOS_CLIENT_ID = process.env.WORKOS_CLIENT_ID ?? "";
const WORKOS_API_BASE = "https://api.workos.com";

export interface DeviceAuthorizationResponse {
  device_code: string;
  user_code: string;
  verification_uri: string;
  verification_uri_complete: string;
  expires_in: number;
  interval: number;
}

export interface TokenResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
}

export interface TokenErrorResponse {
  error: string;
  error_description?: string;
}

export interface UserInfo {
  sub: string;
  email?: string;
  first_name?: string;
  last_name?: string;
  org_id?: string;
}

const sleep = (ms: number): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, ms));

export const requestDeviceAuthorization =
  async (): Promise<DeviceAuthorizationResponse> => {
    if (!WORKOS_CLIENT_ID) {
      throw new Error(
        "WORKOS_CLIENT_ID environment variable is not set. " +
          "Set it in your shell or .env file."
      );
    }

    const response = await fetch(
      `${WORKOS_API_BASE}/user_management/authorize/device`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ client_id: WORKOS_CLIENT_ID }),
      }
    );

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`Failed to request device authorization: ${error}`);
    }

    return response.json() as Promise<DeviceAuthorizationResponse>;
  };

const MAX_POLL_ATTEMPTS = 120;

export const pollForTokens = async (
  deviceCode: string,
  interval: number,
  onPoll?: () => void
): Promise<TokenResponse> => {
  let pollInterval = interval;
  let attempts = 0;

  while (attempts < MAX_POLL_ATTEMPTS) {
    attempts++;
    await sleep(pollInterval * 1000);
    onPoll?.();

    const response = await fetch(
      `${WORKOS_API_BASE}/user_management/authenticate`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          grant_type: "urn:ietf:params:oauth:grant-type:device_code",
          device_code: deviceCode,
          client_id: WORKOS_CLIENT_ID,
        }),
      }
    );

    const data = (await response.json()) as TokenResponse | TokenErrorResponse;

    if ("error" in data) {
      if (data.error === "authorization_pending") {
        continue;
      }
      if (data.error === "slow_down") {
        pollInterval += 5;
        continue;
      }
      if (data.error === "expired_token") {
        throw new Error("Device code expired. Please try logging in again.");
      }
      if (data.error === "access_denied") {
        throw new Error("Authorization was denied.");
      }
      throw new Error(data.error_description ?? data.error);
    }

    return data;
  }

  throw new Error(
    "Authentication timed out. Please try again with `detent auth login`."
  );
};

export const refreshAccessToken = async (
  refreshToken: string
): Promise<TokenResponse> => {
  const response = await fetch(
    `${WORKOS_API_BASE}/user_management/authenticate`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        grant_type: "refresh_token",
        refresh_token: refreshToken,
        client_id: WORKOS_CLIENT_ID,
      }),
    }
  );

  if (!response.ok) {
    const error = await response.text();
    throw new Error(`Failed to refresh token: ${error}`);
  }

  return response.json() as Promise<TokenResponse>;
};

export const getAccessToken = async (repoRoot: string): Promise<string> => {
  const credentials = loadCredentials(repoRoot);

  if (!credentials) {
    throw new Error("Not logged in. Run `detent auth login` first.");
  }

  if (!isTokenExpired(credentials)) {
    return credentials.access_token;
  }

  const tokens = await refreshAccessToken(credentials.refresh_token);
  const newCredentials: Credentials = {
    access_token: tokens.access_token,
    refresh_token: tokens.refresh_token,
    expires_at: Date.now() + tokens.expires_in * 1000,
  };

  saveCredentials(newCredentials, repoRoot);
  return newCredentials.access_token;
};

export const decodeUserInfo = (accessToken: string): UserInfo => {
  const payload = decodeJwt(accessToken);
  return {
    sub: payload.sub as string,
    email: payload.email as string | undefined,
    first_name: payload.first_name as string | undefined,
    last_name: payload.last_name as string | undefined,
    org_id: payload.org_id as string | undefined,
  };
};

export const getExpiresAt = (accessToken: string): Date | null => {
  const payload = decodeJwt(accessToken);
  if (typeof payload.exp === "number") {
    return new Date(payload.exp * 1000);
  }
  return null;
};
