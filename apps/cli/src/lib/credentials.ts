/**
 * Credentials management for WorkOS authentication
 *
 * Stores access and refresh tokens in .detent/credentials.json
 * Follows the same security patterns as config.ts (0o600 permissions)
 */

import {
  existsSync,
  mkdirSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { getRepoDetentDir } from "./config.js";

export interface Credentials {
  access_token: string;
  refresh_token: string;
  expires_at: number;
}

const CREDENTIALS_FILE = "credentials.json";

const getCredentialsPath = (repoRoot: string): string => {
  return join(getRepoDetentDir(repoRoot), CREDENTIALS_FILE);
};

const isValidCredentials = (data: unknown): data is Credentials => {
  if (typeof data !== "object" || data === null) {
    return false;
  }
  const obj = data as Record<string, unknown>;
  return (
    typeof obj.access_token === "string" &&
    typeof obj.refresh_token === "string" &&
    typeof obj.expires_at === "number"
  );
};

export const loadCredentials = (repoRoot: string): Credentials | null => {
  const path = getCredentialsPath(repoRoot);

  if (!existsSync(path)) {
    return null;
  }

  try {
    const data = readFileSync(path, "utf-8");
    if (!data.trim()) {
      return null;
    }
    const parsed: unknown = JSON.parse(data);
    if (!isValidCredentials(parsed)) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
};

export const saveCredentials = (
  credentials: Credentials,
  repoRoot: string
): void => {
  const dir = getRepoDetentDir(repoRoot);

  if (!existsSync(dir)) {
    mkdirSync(dir, { mode: 0o700, recursive: true });
  }

  const path = getCredentialsPath(repoRoot);
  const data = `${JSON.stringify(credentials, null, 2)}\n`;

  writeFileSync(path, data, { mode: 0o600 });
};

export const clearCredentials = (repoRoot: string): boolean => {
  const path = getCredentialsPath(repoRoot);

  if (!existsSync(path)) {
    return false;
  }

  try {
    unlinkSync(path);
    return true;
  } catch {
    return false;
  }
};

export const isLoggedIn = (repoRoot: string): boolean => {
  const creds = loadCredentials(repoRoot);
  return creds !== null;
};

export const isTokenExpired = (credentials: Credentials): boolean => {
  const bufferMs = 5 * 60 * 1000;
  return credentials.expires_at < Date.now() + bufferMs;
};
